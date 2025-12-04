package reconcile

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	ctrl "sigs.k8s.io/controller-runtime/pkg/reconcile"

	capsulebackend "github.com/vaheed/kubenova/internal/backends/capsule"
	velabackend "github.com/vaheed/kubenova/internal/backends/vela"
	v1alpha1 "github.com/vaheed/kubenova/pkg/api/v1alpha1"
)

type mockCapsule struct{ called bool }

func (m *mockCapsule) EnsureTenant(ctx context.Context, spec map[string]any) error {
	m.called = true
	return nil
}

type mockProxy struct{ called bool }

func (m *mockProxy) Publish(ctx context.Context, tenant, endpoint string) error {
	m.called = true
	return nil
}

type mockVela struct{ called bool }

func (m *mockVela) ApplyApp(ctx context.Context, spec map[string]any) error {
	m.called = true
	return nil
}

func (m *mockVela) ApplyProject(ctx context.Context, spec map[string]any) error {
	m.called = true
	return nil
}

func TestTenantReconcilerCreatesNamespacesAndPublishes(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	capsulebackend.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)
	_ = rbacv1.AddToScheme(scheme)
	tenant := &v1alpha1.NovaTenant{
		ObjectMeta: metav1.ObjectMeta{Name: "acme"},
		Spec: v1alpha1.NovaTenantSpec{
			OwnerNamespace: "acme-owner",
			AppsNamespace:  "acme-apps",
			ProxyEndpoint:  "https://proxy.example",
		},
	}
	caps := &mockCapsule{}
	pro := &mockProxy{}
	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(tenant).Build()
	r := &TenantReconciler{
		Client:  client,
		Capsule: caps,
		Proxy:   pro,
	}
	_, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "acme"}})
	if err != nil {
		t.Fatalf("reconcile error: %v", err)
	}
	if !caps.called || !pro.called {
		t.Fatalf("expected capsule and proxy calls")
	}
	var ns corev1.Namespace
	if err := client.Get(context.Background(), types.NamespacedName{Name: "acme-owner"}, &ns); err != nil {
		t.Fatalf("owner namespace missing: %v", err)
	}
}

func TestAppReconcilerCallsVelaBackend(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	velabackend.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)
	app := &v1alpha1.NovaApp{
		ObjectMeta: metav1.ObjectMeta{Name: "web"},
		Spec: v1alpha1.NovaAppSpec{
			Tenant:   "tenant1",
			Project:  "proj1",
			Template: map[string]any{"component": "web"},
		},
	}
	tenant := &v1alpha1.NovaTenant{
		ObjectMeta: metav1.ObjectMeta{Name: "tenant1"},
		Spec:       v1alpha1.NovaTenantSpec{AppsNamespace: "tenant1-apps"},
	}
	mock := &mockVela{}
	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(app, tenant).Build()
	r := &AppReconciler{
		Client:  client,
		Backend: mock,
	}
	_, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "web"}})
	if err != nil {
		t.Fatalf("reconcile error: %v", err)
	}
	if !mock.called {
		t.Fatalf("expected vela backend call")
	}
}

func TestProjectReconcilerEnsuresNamespaces(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)
	project := &v1alpha1.NovaProject{
		ObjectMeta: metav1.ObjectMeta{Name: "proj"},
		Spec:       v1alpha1.NovaProjectSpec{Tenant: "tenantx"},
	}
	tenant := &v1alpha1.NovaTenant{
		ObjectMeta: metav1.ObjectMeta{Name: "tenantx"},
		Spec: v1alpha1.NovaTenantSpec{
			AppsNamespace: "tenantx-apps",
		},
	}
	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(project, tenant).Build()
	mock := &mockVela{}
	r := &ProjectReconciler{Client: client, Scheme: scheme, Backend: mock}
	_, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "proj"}})
	if err != nil {
		t.Fatalf("reconcile error: %v", err)
	}
	if !mock.called {
		t.Fatalf("expected vela project call")
	}
	var ns corev1.Namespace
	if err := client.Get(context.Background(), types.NamespacedName{Name: "tenantx-apps"}, &ns); err != nil {
		t.Fatalf("apps namespace missing: %v", err)
	}
}
