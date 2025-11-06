package manager

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"encoding/base64"
	"github.com/vaheed/kubenova/internal/store"
	"github.com/vaheed/kubenova/pkg/types"
)

func TestTenantsCRUD(t *testing.T) {
	s := NewServer(store.NewMemory())
	// create (new API surface)
	body := []byte(`{"name":"alice"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/clusters/c/tenants", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.Router().ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("create tenant failed: %d", w.Code)
	}
	// list
	req = httptest.NewRequest(http.MethodGet, "/api/v1/clusters/c/tenants", nil)
	w = httptest.NewRecorder()
	s.Router().ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("list tenant failed: %d", w.Code)
	}
}

// RBAC-related legacy tests removed; new RBAC is enforced in the OpenAPI server tests.

func TestCreateClusterEndpoint(t *testing.T) {
	st := store.NewMemory()
	srv := NewServer(st)
	// patch installer to no-op
	old := InstallAgentFunc
	InstallAgentFunc = func(ctx context.Context, kubeconfig []byte, image, managerURL string) error { return nil }
	defer func() { InstallAgentFunc = old }()

	// bogus kubeconfig base64 to pass validation
	kcfg := base64.StdEncoding.EncodeToString([]byte("apiVersion: v1\nclusters: []\ncontexts: []\n"))
	body := map[string]any{"name": "c1", "kubeconfig": kcfg}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/clusters", bytes.NewReader(b))
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("cluster create failed: %d %s", w.Code, w.Body.String())
	}
}

func TestIngestEventsAndList(t *testing.T) {
	st := store.NewMemory()
	srv := NewServer(st)
	evs := []types.Event{{Type: "Info", Resource: "agent", Payload: map[string]any{"m": "started"}, TS: time.Now()}}
	b, _ := json.Marshal(evs)
	req := httptest.NewRequest(http.MethodPost, "/sync/events?cluster_id=1", bytes.NewReader(b))
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("ingest failed: %d", w.Code)
	}
	items, err := st.ListClusterEvents(context.Background(), 1, 10)
	if err != nil || len(items) != 1 {
		t.Fatalf("list events: %v %d", err, len(items))
	}
}
