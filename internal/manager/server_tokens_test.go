package manager

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/vaheed/kubenova/internal/store"
)

func TestIssueTokenAndMe(t *testing.T) {
    s := NewServer(store.NewMemory())
    // issue token
    body := []byte(`{"subject":"tester","roles":["admin"],"ttlSeconds":3600}`)
    req := httptest.NewRequest(http.MethodPost, "/api/v1/tokens", bytes.NewReader(body))
    w := httptest.NewRecorder()
    s.Router().ServeHTTP(w, req)
    if w.Code != 200 {
        t.Fatalf("issue token failed: %d %s", w.Code, w.Body.String())
    }
	var tok map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &tok)
	if tok["token"] == "" || tok["token"] == nil {
		t.Fatalf("expected token, got %v", tok)
	}

	// me
    req = httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
    w = httptest.NewRecorder()
    s.Router().ServeHTTP(w, req)
    if w.Code != 200 {
        t.Fatalf("me failed: %d", w.Code)
    }
}
