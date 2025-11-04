package manager

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/vaheed/kubenova/internal/store"
	"github.com/vaheed/kubenova/pkg/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"strings"
)

func TestCapsuleTenantsList(t *testing.T) {
	m := store.NewMemory()
	// seed a dummy cluster; kubeconfig value is ignored by overridden dynFactory
	_, _ = m.CreateCluster(context.Background(), types.Cluster{Name: "kind"}, "ZmFrZQ==")

	srv := NewServer(m)
	// override dynFactory with a dynamic fake containing one Tenant
	scheme := runtime.NewScheme()
	listKinds := map[schema.GroupVersionResource]string{
		gvrTenants: "TenantList",
	}
	dyn := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds)
	srv.dynFactory = func(ctx context.Context, kc []byte) (dynamic.Interface, error) { return dyn, nil }

	// Seed a Tenant via the dynamic client
	_, _ = dyn.Resource(gvrTenants).Create(context.Background(), &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "capsule.clastix.io/v1beta2",
		"kind":       "Tenant",
		"metadata":   map[string]interface{}{"name": "acme"},
		"spec":       map[string]interface{}{"owners": []interface{}{}},
	}}, metav1.CreateOptions{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tenants?cluster_id=1", nil)
	rr := httptest.NewRecorder()
	srv.Router().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if got := rr.Body.String(); !strings.Contains(got, "acme") {
		t.Fatalf("expected body to contain tenant name, got: %s", got)
	}
}
