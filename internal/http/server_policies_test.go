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
    capib "github.com/vaheed/kubenova/internal/backends/capsule"
    "github.com/vaheed/kubenova/internal/store"
)

type fakeCaps struct{
    quotasCalled bool
    limitsCalled bool
    netpolCalled bool
}
func (f *fakeCaps) EnsureTenant(_ context.Context, _ string, _ []string, _ map[string]string) error { return nil }
func (f *fakeCaps) DeleteTenant(_ context.Context, _ string) error { return nil }
func (f *fakeCaps) ListTenants(_ context.Context, _ string, _ int, _ string) ([]capib.Tenant, string, error) { return nil, "", nil }
func (f *fakeCaps) GetTenant(_ context.Context, _ string) (capib.Tenant, error) { return capib.Tenant{}, nil }
func (f *fakeCaps) SetTenantQuotas(_ context.Context, _ string, _ map[string]string) error { f.quotasCalled = true; return nil }
func (f *fakeCaps) SetTenantLimits(_ context.Context, _ string, _ map[string]string) error { f.limitsCalled = true; return nil }
func (f *fakeCaps) SetTenantNetworkPolicies(_ context.Context, _ string, _ map[string]any) error { f.netpolCalled = true; return nil }

func TestPoliciesHandlersInvokeBackend(t *testing.T) {
    st := store.NewMemory()
    api := NewAPIServer(st)
    fake := &fakeCaps{}
    api.newCapsule = func([]byte) capib.Client { return fake }

    r := chi.NewRouter()
    _ = HandlerWithOptions(api, ChiServerOptions{BaseRouter: r})
    ts := httptest.NewServer(r)
    defer ts.Close()

    // Register a cluster so handlers can resolve kubeconfig by name
    kcfg := base64.StdEncoding.EncodeToString([]byte("apiVersion: v1\nclusters: []\ncontexts: []\n"))
    reqBody := []byte(`{"name":"cluster-a","kubeconfig":"`+kcfg+`"}`)
    resp, err := http.Post(ts.URL+"/api/v1/clusters", "application/json", bytes.NewReader(reqBody))
    if err != nil { t.Fatal(err) }
    resp.Body.Close()

    body, _ := json.Marshal(map[string]string{"cpu":"2"})
    req, _ := http.NewRequest(http.MethodPut, ts.URL+"/api/v1/clusters/cluster-a/tenants/acme/quotas", bytes.NewReader(body))
    req.Header.Set("Content-Type", "application/json")
    resp, err = http.DefaultClient.Do(req)
    if err != nil { t.Fatal(err) }
    if resp.StatusCode != http.StatusOK { t.Fatalf("quotas: %s", resp.Status) }
    resp.Body.Close()

    req, _ = http.NewRequest(http.MethodPut, ts.URL+"/api/v1/clusters/cluster-a/tenants/acme/limits", bytes.NewReader(body))
    req.Header.Set("Content-Type", "application/json")
    resp, err = http.DefaultClient.Do(req)
    if err != nil { t.Fatal(err) }
    if resp.StatusCode != http.StatusOK { t.Fatalf("limits: %s", resp.Status) }
    resp.Body.Close()

    nb, _ := json.Marshal(map[string]any{"defaultDeny":true})
    req, _ = http.NewRequest(http.MethodPut, ts.URL+"/api/v1/clusters/cluster-a/tenants/acme/network-policies", bytes.NewReader(nb))
    req.Header.Set("Content-Type", "application/json")
    resp, err = http.DefaultClient.Do(req)
    if err != nil { t.Fatal(err) }
    if resp.StatusCode != http.StatusOK { t.Fatalf("netpol: %s", resp.Status) }
    resp.Body.Close()

    if !fake.quotasCalled || !fake.limitsCalled || !fake.netpolCalled {
        t.Fatalf("backend not invoked: %+v", fake)
    }
}
