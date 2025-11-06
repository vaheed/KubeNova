package manager

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/vaheed/kubenova/internal/store"
)

func TestSystemEndpoints(t *testing.T) {
	s := NewServer(store.NewMemory())
	for _, p := range []string{"/api/v1/version", "/api/v1/features"} {
		req := httptest.NewRequest(http.MethodGet, p, nil)
		w := httptest.NewRecorder()
		s.Router().ServeHTTP(w, req)
		if w.Code != 200 {
			t.Fatalf("%s failed: %d", p, w.Code)
		}
	}
}
