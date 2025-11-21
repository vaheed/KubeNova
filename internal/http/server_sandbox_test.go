package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	clusterpkg "github.com/vaheed/kubenova/internal/cluster"
	"github.com/vaheed/kubenova/internal/store"
)

func TestSandboxCreation(t *testing.T) {
	st := store.NewMemory()
	api := NewAPIServer(st)
	r := chi.NewRouter()
	_ = HandlerWithOptions(api, ChiServerOptions{BaseRouter: r})
	ts := httptest.NewServer(r)
	defer ts.Close()

	cl := mustCreateCluster(t, ts)
	tenant := mustCreateTenant(t, ts, cl)
	t.Setenv("KUBENOVA_E2E_FAKE", "1")

	payload := map[string]any{"name": "playground", "ttlSeconds": 60}
	reqBody, _ := json.Marshal(payload)
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/tenants/"+idStr(tenant.Id)+"/sandbox", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("sandbox create: %s", resp.Status)
	}
	var out struct {
		Id         string `json:"id"`
		Tenant     string `json:"tenant"`
		Name       string `json:"name"`
		Namespace  string `json:"namespace"`
		Kubeconfig string `json:"kubeconfig"`
		ExpiresAt  string `json:"expiresAt"`
		CreatedAt  string `json:"createdAt"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.Tenant != tenant.Name {
		t.Fatalf("unexpected tenant: %s", out.Tenant)
	}
	if out.Name != "playground" {
		t.Fatalf("unexpected name: %s", out.Name)
	}
	expectedNS := clusterpkg.SandboxNamespaceName(tenant.Name, "playground")
	if out.Namespace != expectedNS {
		t.Fatalf("unexpected namespace: %s", out.Namespace)
	}
	if out.Kubeconfig == "" {
		t.Fatalf("kubeconfig missing")
	}
	if _, err := time.Parse(time.RFC3339, out.ExpiresAt); err != nil {
		t.Fatalf("invalid expiresAt: %v", err)
	}
	if _, err := st.GetSandbox(t.Context(), tenant.Name, "playground"); err != nil {
		t.Fatalf("sandbox missing in store: %v", err)
	}
}

func TestSandboxDuplicate(t *testing.T) {
	st := store.NewMemory()
	api := NewAPIServer(st)
	r := chi.NewRouter()
	_ = HandlerWithOptions(api, ChiServerOptions{BaseRouter: r})
	ts := httptest.NewServer(r)
	defer ts.Close()

	cl := mustCreateCluster(t, ts)
	tenant := mustCreateTenant(t, ts, cl)
	t.Setenv("KUBENOVA_E2E_FAKE", "1")

	for i := 0; i < 2; i++ {
		payload := map[string]any{"name": "playground"}
		reqBody, _ := json.Marshal(payload)
		req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/tenants/"+idStr(tenant.Id)+"/sandbox", bytes.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 && resp.StatusCode != http.StatusOK {
			t.Fatalf("sandbox create: %s", resp.Status)
		}
		if i == 1 && resp.StatusCode != http.StatusConflict {
			t.Fatalf("expected conflict on duplicate sandbox, got %s", resp.Status)
		}
		resp.Body.Close()
	}
}

func mustCreateCluster(t *testing.T, ts *httptest.Server) Cluster {
	t.Helper()
	kcfg := []byte("apiVersion: v1\nclusters: []\ncontexts: []\n")
	reg := ClusterRegistration{Name: "kind", Kubeconfig: kcfg, CapsuleProxyUrl: "https://capsule-proxy.example.com:9001"}
	body, _ := json.Marshal(reg)
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/clusters", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("cluster create: %s", resp.Status)
	}
	var c Cluster
	if err := json.NewDecoder(resp.Body).Decode(&c); err != nil {
		t.Fatal(err)
	}
	return c
}

func mustCreateTenant(t *testing.T, ts *httptest.Server, cl Cluster) Tenant {
	t.Helper()
	tnt := Tenant{Name: "acme"}
	body, _ := json.Marshal(tnt)
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/clusters/"+idStr(cl.Id)+"/tenants", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("tenant create: %s", resp.Status)
	}
	var out Tenant
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out
}
