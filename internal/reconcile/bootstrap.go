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
	// Use a dedicated SA with elevated rights for bootstrap; runtime agent stays least-privileged
	job.Spec.Template.Spec.ServiceAccountName = "agent-bootstrap"
	job.Spec.BackoffLimit = int32ptr(1)
	job.Spec.Template.Spec.Containers = []corev1.Container{{
		Name:    "helm",
		Image:   "dtzar/helm-kubectl:3.14.4",
		Command: []string{"/bin/sh", "-c"},
		Args: []string{`set -Eeuo pipefail
# helper funcs
rollout() { ns=$1; name=$2; kubectl -n "$ns" rollout status deploy/"$name" --timeout=10m; }
wait_crd() { crd=$1; for i in $(seq 1 60); do kubectl get crd "$crd" >/dev/null 2>&1 && return 0; sleep 5; done; echo "timeout waiting for CRD $crd" >&2; exit 1; }
# Add repos (cert-manager required by Capsule and capsule-proxy)
helm repo add jetstack https://charts.jetstack.io >/dev/null 2>&1 || true
helm repo add clastix https://clastix.github.io/charts >/dev/null 2>&1 || true
helm repo add projectcapsule https://projectcapsule.github.io/charts >/dev/null 2>&1 || true
helm repo add kubevela https://kubevela.github.io/charts >/dev/null 2>&1 || true
helm repo update

# 1) Install cert-manager first and wait for readiness (installs CRDs)
helm upgrade --install cert-manager jetstack/cert-manager \
  -n cert-manager --create-namespace --set crds.enabled=true --wait --timeout 10m
rollout cert-manager cert-manager
rollout cert-manager cert-manager-webhook
rollout cert-manager cert-manager-cainjector

# 2) Install Capsule (depends on cert-manager certs) and wait
# Prefer the projectcapsule Helm repo with a pinned version; fall back to OCI in projectcapsule,
# then to legacy clastix paths as a last resort.
CAPSULE_VER="${CAPSULE_VERSION:-0.10.6}"
helm upgrade --install capsule projectcapsule/capsule \
  --version "$CAPSULE_VER" \
  -n capsule-system --create-namespace --set manager.leaderElection=true --wait --timeout 10m \
  || helm upgrade --install capsule oci://ghcr.io/projectcapsule/charts/capsule \
  --version "$CAPSULE_VER" \
  -n capsule-system --create-namespace --set manager.leaderElection=true --wait --timeout 10m \
  || helm upgrade --install capsule clastix/capsule \
  -n capsule-system --create-namespace --set manager.leaderElection=true --wait --timeout 10m \
  || helm upgrade --install capsule oci://ghcr.io/clastix/charts/capsule \
  -n capsule-system --create-namespace --set manager.leaderElection=true --wait --timeout 10m
rollout capsule-system capsule-controller-manager
wait_crd tenants.capsule.clastix.io

# 3) Install capsule-proxy and wait
helm upgrade --install capsule-proxy clastix/capsule-proxy \
  -n capsule-system --set service.enabled=true \
  --set options.allowedUserGroups='{tenant-admins,tenant-maintainers}' --wait --timeout 10m \
  || helm upgrade --install capsule-proxy oci://ghcr.io/clastix/charts/capsule-proxy \
  -n capsule-system --set service.enabled=true \
  --set options.allowedUserGroups='{tenant-admins,tenant-maintainers}' --wait --timeout 10m \
  || helm upgrade --install capsule-proxy oci://ghcr.io/projectcapsule/charts/capsule-proxy \
  -n capsule-system --set service.enabled=true \
  --set options.allowedUserGroups='{tenant-admins,tenant-maintainers}' --wait --timeout 10m
rollout capsule-system capsule-proxy

# 4) Install KubeVela core and wait (admission webhooks enabled per docs)
helm upgrade --install vela-core kubevela/vela-core \
  -n vela-system --create-namespace --set admissionWebhooks.enabled=true --wait --timeout 10m
rollout vela-system vela-core
wait_crd applications.core.oam.dev
`},
	}}
	job.Spec.Template.Spec.RestartPolicy = corev1.RestartPolicyNever
	_, err = client.BatchV1().Jobs(ns).Create(ctx, job, metav1.CreateOptions{})
	return err
}

func int32ptr(i int32) *int32 { return &i }
