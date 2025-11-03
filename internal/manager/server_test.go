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
	// create
	body, _ := json.Marshal(types.Tenant{Name: "alice", CreatedAt: time.Now()})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tenants", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.Router().ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("create tenant failed: %d", w.Code)
	}
	// list
	req = httptest.NewRequest(http.MethodGet, "/api/v1/tenants", nil)
	w = httptest.NewRecorder()
	s.Router().ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("list tenant failed: %d", w.Code)
	}
}

func TestRBACFiltersTenants(t *testing.T) {
	st := store.NewMemory()
	_ = st.CreateTenant(context.Background(), types.Tenant{Name: "alice"})
	_ = st.CreateTenant(context.Background(), types.Tenant{Name: "bob"})
	srv := NewServer(st)
	srv.requireAuth = true // force rbac path

	// Build a request with a fake bearer token that we bypass by injecting context
	req := httptest.NewRequest(http.MethodGet, "/api/v1/tenants", nil)
	// Inject claims directly
	req = req.WithContext(context.WithValue(req.Context(), claimsKey, &Claims{Role: "read-only", Tenant: "alice"}))
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("rbac list failed: %d", w.Code)
	}
	if !bytes.Contains(w.Body.Bytes(), []byte("alice")) || bytes.Contains(w.Body.Bytes(), []byte("bob")) {
		t.Fatalf("rbac filtering failed: %s", w.Body.String())
	}
}

func TestRBACGrantForbidden(t *testing.T) {
	st := store.NewMemory()
	_ = st.CreateTenant(context.Background(), types.Tenant{Name: "alice"})
	srv := NewServer(st)
	srv.requireAuth = true
	g := types.KubeconfigGrant{Tenant: "alice", Role: "read-only", Expires: time.Now().Add(time.Hour)}
	b, _ := json.Marshal(g)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/kubeconfig-grants", bytes.NewReader(b))
	req = req.WithContext(context.WithValue(req.Context(), claimsKey, &Claims{Role: "read-only", Tenant: "alice"}))
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
}

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
