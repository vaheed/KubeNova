package httpapi

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/vaheed/kubenova/internal/store"
)

// contract tests (subset) for Clusters + Tenants
func TestContract_ClustersAndTenants(t *testing.T) {
	st := store.NewMemory()
	api := NewAPIServer(st)
	r := chi.NewRouter()
	// mount without base prefix to avoid conflicts
	_ = HandlerWithOptions(api, ChiServerOptions{BaseRouter: r})
	ts := httptest.NewServer(r)
	defer ts.Close()

	// Create Cluster
	kcfg := []byte("apiVersion: v1\nclusters: []\ncontexts: []\n")
	reg := ClusterRegistration{Name: "kind", Kubeconfig: kcfg, CapsuleProxyUrl: "https://capsule-proxy.example.com:9001"}
	b, _ := json.Marshal(reg)
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/clusters", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("cluster create: %s", resp.Status)
	}
	var c Cluster
	_ = json.NewDecoder(resp.Body).Decode(&c)
	resp.Body.Close()
	if c.Name != "kind" {
		t.Fatalf("unexpected cluster name: %s", c.Name)
	}

	// Capabilities
	if c.Id == nil {
		t.Fatalf("cluster uid missing")
	}
	resp, err = http.Get(ts.URL + "/api/v1/clusters/" + idStr(c.Id) + "/capabilities")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("capabilities: %s", resp.Status)
	}
	resp.Body.Close()

	// Cluster status (use E2E fake to ensure ready)
	t.Setenv("KUBENOVA_E2E_FAKE", "1")
	resp, err = http.Get(ts.URL + "/api/v1/clusters/" + idStr(c.Id))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("cluster get: %s", resp.Status)
	}
	var cobj Cluster
	_ = json.NewDecoder(resp.Body).Decode(&cobj)
	resp.Body.Close()
	if cobj.Conditions == nil || len(*cobj.Conditions) == 0 {
		t.Fatalf("expected conditions, got %#v", cobj)
	}

	// Tenants create
	tnt := Tenant{Name: "acme"}
	tb, _ := json.Marshal(tnt)
	req, _ = http.NewRequest(http.MethodPost, ts.URL+"/api/v1/clusters/"+idStr(c.Id)+"/tenants", bytes.NewReader(tb))
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("tenant create: %s", resp.Status)
	}
	resp.Body.Close()

	// Tenants list
	resp, err = http.Get(ts.URL + "/api/v1/clusters/" + idStr(c.Id) + "/tenants")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("tenant list: %s", resp.Status)
	}
	var list []Tenant
	_ = json.NewDecoder(resp.Body).Decode(&list)
	resp.Body.Close()
	if len(list) != 1 || list[0].Name != "acme" {
		t.Fatalf("unexpected list: %+v", list)
	}

	// Tenants get
	if list[0].Id == nil {
		t.Fatalf("tenant uid missing")
	}
	resp, err = http.Get(ts.URL + "/api/v1/clusters/" + idStr(c.Id) + "/tenants/" + idStr(list[0].Id))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("tenant get: %s", resp.Status)
	}
	resp.Body.Close()
}

func TestContract_ClustersListWithCursorAndLabels(t *testing.T) {
	st := store.NewMemory()
	api := NewAPIServer(st)
	r := chi.NewRouter()
	_ = HandlerWithOptions(api, ChiServerOptions{BaseRouter: r})
	ts := httptest.NewServer(r)
	defer ts.Close()

	// seed clusters with labels
	seed := func(name, env string) {
		kcfg := []byte("apiVersion: v1\nclusters: []\ncontexts: []\n")
		reg := ClusterRegistration{Name: name, Labels: &map[string]string{"env": env}, Kubeconfig: kcfg, CapsuleProxyUrl: "https://capsule-proxy.example.com:9001"}
		b, _ := json.Marshal(reg)
		req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/clusters", bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		_, _ = http.DefaultClient.Do(req)
	}
	seed("c1", "dev")
	seed("c2", "prod")
	seed("c3", "prod")

	// page 1
	resp, err := http.Get(ts.URL + "/api/v1/clusters?limit=1")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list: %s", resp.Status)
	}
	next := resp.Header.Get("X-Next-Cursor")
	var page1 []Cluster
	_ = json.NewDecoder(resp.Body).Decode(&page1)
	resp.Body.Close()
	if len(page1) != 1 || next == "" {
		t.Fatalf("pagination not working: next=%s items=%d", next, len(page1))
	}

	// page 2
	resp, err = http.Get(ts.URL + "/api/v1/clusters?limit=2&cursor=" + next)
	if err != nil {
		t.Fatal(err)
	}
	var page2 []Cluster
	_ = json.NewDecoder(resp.Body).Decode(&page2)
	resp.Body.Close()
	if len(page2) == 0 {
		t.Fatalf("expected more items")
	}

	// label filter
	resp, err = http.Get(ts.URL + "/api/v1/clusters?labelSelector=env=prod")
	if err != nil {
		t.Fatal(err)
	}
	var filtered []Cluster
	_ = json.NewDecoder(resp.Body).Decode(&filtered)
	resp.Body.Close()
	if len(filtered) != 2 {
		t.Fatalf("expected 2 prod clusters, got %d", len(filtered))
	}
}

// Ensure byte format honors base64
func TestContract_ClusterRegistrationBase64(t *testing.T) {
	st := store.NewMemory()
	api := NewAPIServer(st)
	r := chi.NewRouter()
	_ = HandlerWithOptions(api, ChiServerOptions{BaseRouter: r})
	ts := httptest.NewServer(r)
	defer ts.Close()

	raw := []byte("raw-kubeconfig")
	enc := base64.StdEncoding.EncodeToString(raw)
	body := []byte(`{"name":"kind","kubeconfig":"` + enc + `","capsuleProxyUrl":"https://capsule-proxy.example.com:9001"}`)
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/clusters", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: %s", resp.Status)
	}
	resp.Body.Close()
}
