package client

import (
    "net/http/httptest"
    "testing"
    "github.com/vaheed/kubenova/internal/api"
    "github.com/vaheed/kubenova/internal/store"
    "github.com/vaheed/kubenova/pkg/types"
    "context"
)

func TestClientTenantProjectApp(t *testing.T) {
    srv := api.NewServer(store.NewMemory())
    ts := httptest.NewServer(srv.Router())
    defer ts.Close()
    c := New(ts.URL, "")
    ctx := context.Background()
    if _, err := c.CreateTenant(ctx, types.Tenant{Name: "alice"}); err != nil { t.Fatal(err) }
    if _, err := c.CreateProject(ctx, types.Project{Tenant: "alice", Name: "demo"}); err != nil { t.Fatal(err) }
    if _, err := c.CreateApp(ctx, types.App{Tenant: "alice", Project: "demo", Name: "app"}); err != nil { t.Fatal(err) }
    ps, err := c.ListProjects(ctx, "alice"); if err != nil || len(ps) != 1 { t.Fatalf("projects list: %v %d", err, len(ps)) }
}

