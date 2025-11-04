package reconcile

import (
	"context"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"testing"
)

func TestProjectReconcilerCreatesNamespace(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	r := &ProjectReconciler{Client: c}
	_, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "proj-demo"}})
	if err != nil {
		t.Fatal(err)
	}
	ns := &corev1.Namespace{}
	if err := c.Get(context.Background(), types.NamespacedName{Name: "proj-demo"}, ns); err != nil {
		t.Fatalf("namespace not created: %v", err)
	}
	if ns.Labels["kubenova.project"] != "proj-demo" {
		t.Fatalf("label missing: %v", ns.Labels)
	}
}
