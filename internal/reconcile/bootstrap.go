package reconcile

import (
	"context"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
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
	job := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: "kubenova-bootstrap", Namespace: ns}}
	_, err = client.BatchV1().Jobs(ns).Get(ctx, job.Name, metav1.GetOptions{})
	if err == nil {
		return nil
	}
	// Use a dedicated bootstrap SA with elevated permissions (cluster-admin) for Helm installs
	job.Spec.Template.Spec.ServiceAccountName = "agent-bootstrap"
	job.Spec.BackoffLimit = int32ptr(1)
	job.Spec.Template.Spec.Containers = []corev1.Container{{
		Name:    "helm",
		Image:   "alpine/helm:3.14.4",
		Command: []string{"/bin/sh", "-c"},
		Args: []string{`set -e
# Add repos (cert-manager required by Capsule and capsule-proxy)
helm repo add jetstack https://charts.jetstack.io
helm repo add clastix https://clastix.github.io/charts
helm repo add kubevela https://kubevela.github.io/charts
helm repo update

# Install cert-manager first and wait for readiness (installs CRDs)
helm upgrade --install cert-manager jetstack/cert-manager \
  -n cert-manager --create-namespace --set crds.enabled=true --wait --timeout 10m

# Install Capsule and capsule-proxy (depends on cert-manager certs) and wait
helm upgrade --install capsule clastix/capsule \
  -n capsule-system --create-namespace --set manager.leaderElection=true --wait --timeout 10m
helm upgrade --install capsule-proxy clastix/capsule-proxy \
  -n capsule-system --set service.enabled=true \
  --set options.allowedUserGroups='{tenant-admins,tenant-maintainers}' --wait --timeout 10m

# Install KubeVela core and wait
helm upgrade --install vela-core kubevela/vela-core \
  -n vela-system --create-namespace --set admissionWebhooks.enabled=true --wait --timeout 10m
`},
	}}
	job.Spec.Template.Spec.RestartPolicy = corev1.RestartPolicyNever
	_, err = client.BatchV1().Jobs(ns).Create(ctx, job, metav1.CreateOptions{})
	return err
}

func int32ptr(i int32) *int32 { return &i }
