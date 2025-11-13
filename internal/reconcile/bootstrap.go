package reconcile

import (
	"context"
	"os"
	"time"

	"github.com/vaheed/kubenova/internal/logging"
	"github.com/vaheed/kubenova/internal/telemetry"
	"go.uber.org/zap"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
)

// Bootstrap addons by creating a one-shot Helm job that installs Capsule, capsule-proxy, and KubeVela if missing.
func BootstrapHelmJob(ctx context.Context) error {
	c := ctrl.GetConfigOrDie()
	client, err := kubernetes.NewForConfig(c)
	if err != nil {
		return err
	}
	ns := "kubenova-system"
	name := "kubenova-bootstrap"
	logging.FromContext(ctx).Info("bootstrap.job.check", zap.String("namespace", ns), zap.String("name", name))
	telemetry.PublishStage("bootstrap", "check", "start", "checking existing job state")
	// If a previous job exists and failed, delete and recreate to self-heal.
	if existing, err := client.BatchV1().Jobs(ns).Get(ctx, name, metav1.GetOptions{}); err == nil {
		// If it failed previously, clean it up and recreate; otherwise consider pod-level stuck states.
		shouldRecreate := existing.Status.Failed > 0 && existing.Status.Succeeded == 0
		if !shouldRecreate {
			// Detect pods stuck in CreateContainerConfigError / ImagePullBackOff / ErrImagePull, etc.
			podList, _ := client.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{LabelSelector: "job-name=" + name})
			for _, p := range podList.Items {
				if len(p.Status.ContainerStatuses) == 0 {
					continue
				}
				st := p.Status.ContainerStatuses[0].State
				if st.Waiting != nil {
					switch st.Waiting.Reason {
					case "CreateContainerConfigError", "ErrImagePull", "ImagePullBackOff", "CrashLoopBackOff":
						shouldRecreate = true
						logging.FromContext(ctx).Info("bootstrap.job.recreate_due_to_pod_state", zap.String("pod", p.Name), zap.String("reason", st.Waiting.Reason))
						telemetry.PublishStage("bootstrap", "recreate", st.Waiting.Reason, p.Name)
					}
				}
			}
		}
		if shouldRecreate {
			logging.FromContext(ctx).Info("bootstrap.job.deleting")
			telemetry.PublishStage("bootstrap", "delete", "start", "deleting failed/stuck job")
			_ = client.BatchV1().Jobs(ns).Delete(ctx, name, metav1.DeleteOptions{})
			// Wait briefly for deletion so we can recreate with the same name.
			deadline := time.Now().Add(30 * time.Second)
			for time.Now().Before(deadline) {
				if _, err := client.BatchV1().Jobs(ns).Get(ctx, name, metav1.GetOptions{}); errors.IsNotFound(err) {
					logging.FromContext(ctx).Info("bootstrap.job.deleted")
					telemetry.PublishStage("bootstrap", "delete", "ok", "job deleted")
					break
				}
				time.Sleep(1 * time.Second)
			}
		} else {
			logging.FromContext(ctx).Info("bootstrap.job.exists_ok")
			telemetry.PublishStage("bootstrap", "exists", "ok", "job already present")
			return nil
		}
	}
	job := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns}}
	// Use a dedicated bootstrap SA with elevated permissions (cluster-admin) for Helm installs
	job.Spec.Template.Spec.ServiceAccountName = "agent-bootstrap"
	job.Spec.BackoffLimit = int32ptr(1)
	// Auto-clean finished jobs and pods after a few minutes to reduce clutter
	job.Spec.TTLSecondsAfterFinished = int32ptr(300)
	// Pass through optional chart version pins from the Agent env
	helmEnv := []corev1.EnvVar{
		{Name: "CAPSULE_VERSION", Value: os.Getenv("CAPSULE_VERSION")},
		{Name: "CAPSULE_PROXY_VERSION", Value: os.Getenv("CAPSULE_PROXY_VERSION")},
		{Name: "VELA_CORE_VERSION", Value: os.Getenv("VELA_CORE_VERSION")},
		// Ensure non-root user can write helm cache/config/data
		{Name: "HOME", Value: "/tmp"},
		{Name: "HELM_CACHE_HOME", Value: "/tmp/helm/cache"},
		{Name: "HELM_CONFIG_HOME", Value: "/tmp/helm/config"},
		{Name: "HELM_DATA_HOME", Value: "/tmp/helm/data"},
	}
	job.Spec.Template.Spec.Containers = []corev1.Container{{
		Name:    "helm",
		Image:   "dtzar/helm-kubectl:3.14.4",
		Command: []string{"/bin/sh", "-c"},
		Env:     helmEnv,
		SecurityContext: &corev1.SecurityContext{
			AllowPrivilegeEscalation: boolPtr(false),
			RunAsNonRoot:             boolPtr(true),
			RunAsUser:                int64ptr(10001),
			RunAsGroup:               int64ptr(10001),
			Capabilities:             &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
			SeccompProfile:           &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
		},
		Args: []string{`set -euo pipefail
set -x
echo "[bootstrap] starting helm bootstrap"
# prepare non-root helm dirs
mkdir -p /tmp/helm/cache /tmp/helm/config /tmp/helm/data

# Add repos (cert-manager required by Capsule and capsule-proxy)
echo "[bootstrap] helm repo add"
helm repo add jetstack https://charts.jetstack.io
helm repo add clastix https://clastix.github.io/charts
helm repo add kubevela https://kubevela.github.io/charts
helm repo update

# Install cert-manager first and wait for readiness (installs CRDs)
echo "[bootstrap] installing cert-manager"
helm upgrade --install cert-manager jetstack/cert-manager \
  -n cert-manager --create-namespace --set crds.enabled=true --wait --timeout 10m
# Extra readiness checks for cert-manager deployments
for d in cert-manager cert-manager-cainjector cert-manager-webhook; do
  kubectl -n cert-manager rollout status deploy/$d --timeout=10m || {
    echo "[bootstrap][error] cert-manager deployment $d not ready";
    echo "[diag] cert-manager resources:";
    kubectl -n cert-manager get deploy,po -o wide || true;
    kubectl -n cert-manager describe deploy/$d || true;
    kubectl get events -n cert-manager --sort-by=.lastTimestamp | tail -n 50 || true;
    exit 1;
  }
done

# Install Capsule and capsule-proxy (depends on cert-manager certs) and wait
echo "[bootstrap] installing capsule"
CAPS_VER=""; if [ -n "$CAPSULE_VERSION" ]; then CAPS_VER="--version $CAPSULE_VERSION"; fi
helm upgrade --install capsule clastix/capsule \
  -n capsule-system --create-namespace --set manager.leaderElection=true $CAPS_VER --wait --timeout 10m
echo "[bootstrap] installing capsule-proxy"
PROXY_VER=""; if [ -n "$CAPSULE_PROXY_VERSION" ]; then PROXY_VER="--version $CAPSULE_PROXY_VERSION"; fi
helm upgrade --install capsule-proxy clastix/capsule-proxy \
  -n capsule-system --set service.enabled=true \
  --set options.allowedUserGroups='{tenant-admins,tenant-maintainers}' $PROXY_VER --wait --timeout 10m
# Extra readiness checks for capsule and proxy
kubectl -n capsule-system rollout status deploy/capsule-controller-manager --timeout=10m || {
  echo "[bootstrap][error] capsule-controller-manager not ready";
  kubectl -n capsule-system get deploy,po -o wide || true;
  kubectl -n capsule-system describe deploy/capsule-controller-manager || true;
  kubectl get events -n capsule-system --sort-by=.lastTimestamp | tail -n 50 || true;
  exit 1;
}
kubectl -n capsule-system rollout status deploy/capsule-proxy --timeout=10m || {
  echo "[bootstrap][error] capsule-proxy not ready";
  kubectl -n capsule-system get deploy,po -o wide || true;
  kubectl -n capsule-system describe deploy/capsule-proxy || true;
  kubectl get events -n capsule-system --sort-by=.lastTimestamp | tail -n 50 || true;
  exit 1;
}

# Finalizer: assert Capsule CRDs exist
echo "[bootstrap] asserting Capsule CRDs"
kubectl get crd tenants.capsule.clastix.io >/dev/null 2>&1 || {
  echo "[bootstrap][error] missing CRD tenants.capsule.clastix.io";
  kubectl get crd | grep -E 'capsule|clastix' || true;
  exit 1;
}

# Install KubeVela core and wait
echo "[bootstrap] installing vela-core (multicluster disabled)"
VELA_VER=""; if [ -n "$VELA_CORE_VERSION" ]; then VELA_VER="--version $VELA_CORE_VERSION"; fi
helm upgrade --install vela-core kubevela/vela-core \
  -n vela-system --create-namespace \
  --set admissionWebhooks.enabled=true \
  --set multicluster.enabled=false \
  $VELA_VER
echo "[bootstrap] waiting for vela-core rollout"
kubectl -n vela-system rollout status deploy/vela-core --timeout=10m || {
  echo "[bootstrap][warn] vela-core not ready after timeout";
  kubectl -n vela-system get deploy,po -o wide || true;
  kubectl -n vela-system describe deploy/vela-core || true;
  kubectl get apiservice | grep -i oam || true;
}
echo "[bootstrap] complete"
`},
	}}
	job.Spec.Template.Spec.RestartPolicy = corev1.RestartPolicyNever
	// Pod-level security context (PodSecurity restricted compatibility)
	job.Spec.Template.Spec.SecurityContext = &corev1.PodSecurityContext{
		SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
		RunAsNonRoot:   boolPtr(true),
	}
	_, err = client.BatchV1().Jobs(ns).Create(ctx, job, metav1.CreateOptions{})
	if err != nil {
		logging.FromContext(ctx).Error("bootstrap.job.create_failed", zap.Error(err))
		telemetry.PublishStage("bootstrap", "create", "error", err.Error())
		return err
	}
	logging.FromContext(ctx).Info("bootstrap.job.created")
	telemetry.PublishStage("bootstrap", "create", "ok", "job created")
	go monitorBootstrapJob(ctx, client, ns, name)
	return nil
}

func int32ptr(i int32) *int32 { return &i }
func boolPtr(b bool) *bool    { return &b }
func int64ptr(i int64) *int64 { return &i }

// monitorBootstrapJob emits progress heartbeats and final status for the bootstrap job.
func monitorBootstrapJob(ctx context.Context, client *kubernetes.Clientset, ns, name string) {
	l := logging.FromContext(ctx)
	l.Info("bootstrap.job.monitor.start", zap.String("namespace", ns), zap.String("name", name))
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	lastPhase := ""
	start := time.Now()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			j, err := client.BatchV1().Jobs(ns).Get(ctx, name, metav1.GetOptions{})
			if err != nil {
				if errors.IsNotFound(err) {
					return
				}
				continue
			}
			if j.Status.Succeeded > 0 {
				telemetry.PublishStage("bootstrap", "job", "succeeded", "bootstrap completed")
				l.Info("bootstrap.job.succeeded", zap.Duration("duration", time.Since(start)))
				return
			}
			if j.Status.Failed > 0 {
				// collect pod reasons
				pods, _ := client.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{LabelSelector: "job-name=" + name})
				reason := "failed"
				for _, p := range pods.Items {
					if len(p.Status.ContainerStatuses) == 0 {
						continue
					}
					st := p.Status.ContainerStatuses[0].State
					if st.Waiting != nil && st.Waiting.Reason != "" {
						reason = st.Waiting.Reason
						break
					}
					if st.Terminated != nil && st.Terminated.Reason != "" {
						reason = st.Terminated.Reason
						break
					}
				}
				telemetry.PublishStage("bootstrap", "job", reason, "bootstrap job failed")
				l.Info("bootstrap.job.failed", zap.String("reason", reason))
				// Attempt self-heal: delete job and re-run bootstrap after backoff
				_ = client.BatchV1().Jobs(ns).Delete(ctx, name, metav1.DeleteOptions{})
				deadline := time.Now().Add(30 * time.Second)
				for time.Now().Before(deadline) {
					if _, err := client.BatchV1().Jobs(ns).Get(ctx, name, metav1.GetOptions{}); errors.IsNotFound(err) {
						break
					}
					time.Sleep(1 * time.Second)
				}
				// backoff before re-create
				time.Sleep(15 * time.Second)
				_ = BootstrapHelmJob(ctx)
				return
			}
			if j.Status.Active > 0 && lastPhase != "active" {
				telemetry.PublishStage("bootstrap", "job", "active", "bootstrap job running")
				lastPhase = "active"
			}
			// heartbeat with counts
			telemetry.PublishEvent(map[string]any{
				"component": "bootstrap",
				"stage":     "job_heartbeat",
				"active":    j.Status.Active,
				"failed":    j.Status.Failed,
				"succeeded": j.Status.Succeeded,
				"ts":        time.Now().UTC().Format(time.RFC3339Nano),
			})
		}
	}
}
