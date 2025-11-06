package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/vaheed/kubenova/internal/store"
)

func TestCatalogEndpoints(t *testing.T) {
	st := store.NewMemory()
	api := NewAPIServer(st)
	r := chi.NewRouter()
	_ = HandlerWithOptions(api, ChiServerOptions{BaseRouter: r})
	ts := httptest.NewServer(r)
	defer ts.Close()

	cases := []struct {
		path   string
		expect string
	}{
		{"/api/v1/catalog/components", "web"},
		{"/api/v1/catalog/traits", "scaler"},
		{"/api/v1/catalog/workflows", "rollout"},
	}

	for _, c := range cases {
		resp, err := http.Get(ts.URL + c.path)
		if err != nil {
			t.Fatalf("GET %s: %v", c.path, err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status for %s: %s", c.path, resp.Status)
		}
		var arr []map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&arr)
		resp.Body.Close()
		if len(arr) == 0 {
			t.Fatalf("empty catalog for %s", c.path)
		}
		if arr[0]["name"].(string) != c.expect {
			t.Fatalf("unexpected first item name for %s: %v", c.path, arr[0])
		}
	}
}
