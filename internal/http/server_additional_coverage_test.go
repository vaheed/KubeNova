package httpapi

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/vaheed/kubenova/internal/store"
	kn "github.com/vaheed/kubenova/pkg/types"
)

// This test exercises endpoints previously marked Missing/Partial in the report:
// - POST /api/v1/clusters/{c}/bootstrap/{component}
// - GET  /api/v1/clusters/{c}/policysets/catalog
// - PUT  /api/v1/clusters/{c}/tenants/{t}/owners
// - GET  /api/v1/clusters/{c}/tenants/{t}/summary
// - PUT  /api/v1/clusters/{c}/tenants/{t}/projects/{p}/access
// - POST /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}:delete
// - GET  /api/v1/projects/{p}/usage
// - GET  /api/v1/tenants/{t}/usage
func TestAdditionalEndpointsCoverage(t *testing.T) {
	st := store.NewMemory()
	api := NewAPIServer(st)

	r := chi.NewRouter()
	_ = HandlerWithOptions(api, ChiServerOptions{BaseRouter: r})
	ts := httptest.NewServer(r)
	defer ts.Close()

	// Seed a cluster via API to back bootstrap and kubeconfig-backed flows.
	kcfg := base64.StdEncoding.EncodeToString([]byte("apiVersion: v1\nclusters: []\ncontexts: []\n"))
	reqBody := []byte(`{"name":"kind","kubeconfig":"` + kcfg + `"}`)
	resp, err := http.Post(ts.URL+"/api/v1/clusters", "application/json", bytes.NewReader(reqBody))
	if err != nil {
		t.Fatalf("register cluster: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("register cluster status: %s", resp.Status)
	}
	resp.Body.Close()

	// Seed tenant and project directly in the store (UIDs default to names)
	_ = st.CreateTenant(context.Background(), kn.Tenant{Name: "acme"})
	_ = st.CreateProject(context.Background(), kn.Project{Tenant: "acme", Name: "proj1"})

	// 1) Bootstrap component
	resp, err = http.Post(ts.URL+"/api/v1/clusters/kind/bootstrap/tenancy", "application/json", nil)
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("bootstrap status: %s", resp.Status)
	}
	resp.Body.Close()

	// 2) PolicySet catalog
	resp, err = http.Get(ts.URL + "/api/v1/clusters/kind/policysets/catalog")
	if err != nil {
		t.Fatalf("catalog: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("catalog status: %s", resp.Status)
	}
	var cat []map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&cat)
	resp.Body.Close()
	if len(cat) == 0 {
		t.Fatalf("empty policyset catalog")
	}

	// 3) Replace tenant owners
	owners := []byte(`{"owners":["owner@example.com"]}`)
	req, _ := http.NewRequest(http.MethodPut, ts.URL+"/api/v1/clusters/kind/tenants/acme/owners", bytes.NewReader(owners))
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("owners: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("owners status: %s", resp.Status)
	}
	resp.Body.Close()

	// 4) Tenant summary
	resp, err = http.Get(ts.URL + "/api/v1/clusters/kind/tenants/acme/summary")
	if err != nil {
		t.Fatalf("tenant summary: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("tenant summary status: %s", resp.Status)
	}
	var sum map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&sum)
	resp.Body.Close()

	// 5) Project access update
	req, _ = http.NewRequest(http.MethodPut, ts.URL+"/api/v1/clusters/kind/tenants/acme/projects/proj1/access", bytes.NewReader([]byte(`{"members":[]}`)))
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("project access: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("project access status: %s", resp.Status)
	}
	resp.Body.Close()

	// 6) App delete action (idempotent)
	resp, err = http.Post(ts.URL+"/api/v1/clusters/kind/tenants/acme/projects/proj1/apps/appA:delete", "application/json", nil)
	if err != nil {
		t.Fatalf("app delete: %v", err)
	}
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("app delete status: %s", resp.Status)
	}
	resp.Body.Close()

	// 7) Project usage
	resp, err = http.Get(ts.URL + "/api/v1/projects/proj1/usage")
	if err != nil {
		t.Fatalf("project usage: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("project usage status: %s", resp.Status)
	}
	var usage map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&usage)
	resp.Body.Close()
	if _, ok := usage["window"]; !ok {
		t.Fatalf("project usage missing window: %v", usage)
	}

	// 8) Tenant usage
	resp, err = http.Get(ts.URL + "/api/v1/tenants/acme/usage")
	if err != nil {
		t.Fatalf("tenant usage: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("tenant usage status: %s", resp.Status)
	}
	usage = map[string]any{}
	_ = json.NewDecoder(resp.Body).Decode(&usage)
	resp.Body.Close()
	if _, ok := usage["window"]; !ok {
		t.Fatalf("tenant usage missing window: %v", usage)
	}
}
