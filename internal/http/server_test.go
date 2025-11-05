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
    reg := ClusterRegistration{Name: "kind", Kubeconfig: kcfg}
    b, _ := json.Marshal(reg)
    req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/clusters", bytes.NewReader(b))
    req.Header.Set("Content-Type", "application/json")
    resp, err := http.DefaultClient.Do(req)
    if err != nil { t.Fatal(err) }
    if resp.StatusCode != http.StatusOK { t.Fatalf("cluster create: %s", resp.Status) }
    var c Cluster
    _ = json.NewDecoder(resp.Body).Decode(&c)
    resp.Body.Close()
    if c.Name != "kind" { t.Fatalf("unexpected cluster name: %s", c.Name) }

    // Capabilities
    resp, err = http.Get(ts.URL+"/api/v1/clusters/"+c.Name+"/capabilities")
    if err != nil { t.Fatal(err) }
    if resp.StatusCode != http.StatusOK { t.Fatalf("capabilities: %s", resp.Status) }
    resp.Body.Close()

    // Cluster status (use E2E fake to ensure ready)
    t.Setenv("KUBENOVA_E2E_FAKE", "1")
    resp, err = http.Get(ts.URL+"/api/v1/clusters/"+c.Name)
    if err != nil { t.Fatal(err) }
    if resp.StatusCode != http.StatusOK { t.Fatalf("cluster get: %s", resp.Status) }
    var cobj Cluster
    _ = json.NewDecoder(resp.Body).Decode(&cobj)
    resp.Body.Close()
    if cobj.Conditions == nil || len(*cobj.Conditions) == 0 { t.Fatalf("expected conditions, got %#v", cobj) }

    // Tenants create
    tnt := Tenant{Name: "acme"}
    tb, _ := json.Marshal(tnt)
    req, _ = http.NewRequest(http.MethodPost, ts.URL+"/api/v1/clusters/cluster-a/tenants", bytes.NewReader(tb))
    req.Header.Set("Content-Type", "application/json")
    resp, err = http.DefaultClient.Do(req)
    if err != nil { t.Fatal(err) }
    if resp.StatusCode != http.StatusOK { t.Fatalf("tenant create: %s", resp.Status) }
    resp.Body.Close()

    // Tenants list
    resp, err = http.Get(ts.URL+"/api/v1/clusters/cluster-a/tenants")
    if err != nil { t.Fatal(err) }
    if resp.StatusCode != http.StatusOK { t.Fatalf("tenant list: %s", resp.Status) }
    var list []Tenant
    _ = json.NewDecoder(resp.Body).Decode(&list)
    resp.Body.Close()
    if len(list) != 1 || list[0].Name != "acme" { t.Fatalf("unexpected list: %+v", list) }

    // Tenants get
    resp, err = http.Get(ts.URL+"/api/v1/clusters/cluster-a/tenants/acme")
    if err != nil { t.Fatal(err) }
    if resp.StatusCode != http.StatusOK { t.Fatalf("tenant get: %s", resp.Status) }
    resp.Body.Close()
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
    body := []byte(`{"name":"kind","kubeconfig":"`+enc+`"}`)
    req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/clusters", bytes.NewReader(body))
    req.Header.Set("Content-Type", "application/json")
    resp, err := http.DefaultClient.Do(req)
    if err != nil { t.Fatal(err) }
    if resp.StatusCode != http.StatusOK { t.Fatalf("status: %s", resp.Status) }
    resp.Body.Close()
}
