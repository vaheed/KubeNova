package reconcile

import (
	"context"
	"os"
	"time"

	"github.com/vaheed/kubenova/internal/logging"
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
					}
				}
			}
		}
		if shouldRecreate {
			logging.FromContext(ctx).Info("bootstrap.job.deleting")
			_ = client.BatchV1().Jobs(ns).Delete(ctx, name, metav1.DeleteOptions{})
			// Wait briefly for deletion so we can recreate with the same name.
			deadline := time.Now().Add(30 * time.Second)
			for time.Now().Before(deadline) {
				if _, err := client.BatchV1().Jobs(ns).Get(ctx, name, metav1.GetOptions{}); errors.IsNotFound(err) {
					logging.FromContext(ctx).Info("bootstrap.job.deleted")
					break
				}
				time.Sleep(1 * time.Second)
			}
		} else {
			logging.FromContext(ctx).Info("bootstrap.job.exists_ok")
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
		Image:   "alpine/helm:3.14.4",
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
# prepare tools (install kubectl non-root)
export PATH="/tmp/bin:$PATH"
mkdir -p /tmp/bin /tmp/helm/cache /tmp/helm/config /tmp/helm/data
KVER=$(curl -sSL https://storage.googleapis.com/kubernetes-release/release/stable.txt || echo v1.29.0)
curl -sSL -o /tmp/bin/kubectl https://storage.googleapis.com/kubernetes-release/release/${KVER}/bin/linux/amd64/kubectl || true
chmod +x /tmp/bin/kubectl || true

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

# Install KubeVela core and wait
echo "[bootstrap] installing vela-core (multicluster disabled)"
VELA_VER=""; if [ -n "$VELA_CORE_VERSION" ]; then VELA_VER="--version $VELA_CORE_VERSION"; fi
helm upgrade --install vela-core kubevela/vela-core \
  -n vela-system --create-namespace \
  --set admissionWebhooks.enabled=true \
  --set multicluster.enabled=false \
  $VELA_VER
echo "[bootstrap] waiting for vela-core rollout"
kubectl -n vela-system rollout status deploy/vela-core --timeout=10m || true
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
		return err
	}
	logging.FromContext(ctx).Info("bootstrap.job.created")
	return nil
}

func int32ptr(i int32) *int32 { return &i }
func boolPtr(b bool) *bool    { return &b }
func int64ptr(i int64) *int64 { return &i }
