package httpapi

import (
  "encoding/json"
  "net/http"
  "net/http/httptest"
  "testing"

  "github.com/go-chi/chi/v5"
  kn "github.com/vaheed/kubenova/pkg/types"
  "github.com/vaheed/kubenova/internal/store"
)

func TestTenantsListFilters(t *testing.T) {
  st := store.NewMemory()
  _ = st.CreateTenant(httptest.NewRequest("GET", "/", nil).Context(), kn.Tenant{Name: "acme", Labels: map[string]string{"env":"prod","tier":"gold"}, Owners: []string{"alice@example.com"}})
  _ = st.CreateTenant(httptest.NewRequest("GET", "/", nil).Context(), kn.Tenant{Name: "beta", Labels: map[string]string{"env":"dev"}, Owners: []string{"bob@example.com"}})

  api := NewAPIServer(st)
  r := chi.NewRouter()
  _ = HandlerWithOptions(api, ChiServerOptions{BaseRouter: r})
  ts := httptest.NewServer(r)
  defer ts.Close()

  // labelSelector match
  resp, err := http.Get(ts.URL + "/api/v1/clusters/c/tenants?labelSelector=env%3Dprod,tier%3Dgold")
  if err != nil { t.Fatal(err) }
  if resp.StatusCode != http.StatusOK { t.Fatalf("status: %s", resp.Status) }
  var arr []map[string]any
  _ = json.NewDecoder(resp.Body).Decode(&arr)
  resp.Body.Close()
  if len(arr) != 1 || arr[0]["name"].(string) != "acme" {
    t.Fatalf("unexpected tenants: %+v", arr)
  }

  // owner filter
  resp, err = http.Get(ts.URL + "/api/v1/clusters/c/tenants?owner=alice@example.com")
  if err != nil { t.Fatal(err) }
  if resp.StatusCode != http.StatusOK { t.Fatalf("status: %s", resp.Status) }
  arr = nil
  _ = json.NewDecoder(resp.Body).Decode(&arr)
  resp.Body.Close()
  if len(arr) != 1 || arr[0]["name"].(string) != "acme" {
    t.Fatalf("unexpected tenants by owner: %+v", arr)
  }
}

