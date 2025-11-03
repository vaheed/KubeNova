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
	job.Spec.Template.Spec.ServiceAccountName = "kubenova-agent"
	job.Spec.BackoffLimit = int32ptr(1)
	job.Spec.Template.Spec.Containers = []corev1.Container{{
		Name:    "helm",
		Image:   "alpine/helm:3.14.4",
		Command: []string{"/bin/sh", "-c"},
		Args: []string{`set -e
helm repo add clastix https://clastix.github.io/charts
helm repo add kubevela https://kubevela.github.io/charts
helm repo update
helm upgrade --install capsule clastix/capsule -n capsule-system --create-namespace --set manager.leaderElection=true
helm upgrade --install capsule-proxy clastix/capsule-proxy -n capsule-system --set service.enabled=true --set options.allowedUserGroups='{tenant-admins,tenant-maintainers}'
helm upgrade --install vela-core kubevela/vela-core -n vela-system --create-namespace --set admissionWebhooks.enabled=true
`},
	}}
	job.Spec.Template.Spec.RestartPolicy = corev1.RestartPolicyNever
	_, err = client.BatchV1().Jobs(ns).Create(ctx, job, metav1.CreateOptions{})
	return err
}

func int32ptr(i int32) *int32 { return &i }
