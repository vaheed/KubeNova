//go:build integration
// +build integration

package store

import (
    "context"
    "fmt"
    "os"
    "testing"
    "time"
    tc "github.com/testcontainers/testcontainers-go"
    "github.com/testcontainers/testcontainers-go/wait"
    "github.com/vaheed/kubenova/pkg/types"
)

func startPostgres(t *testing.T) (dsn string, terminate func()) {
    t.Helper()
    ctx := context.Background()
    req := tc.ContainerRequest{
        Image:        "postgres:16",
        ExposedPorts: []string{"5432/tcp"},
        Env: map[string]string{ "POSTGRES_PASSWORD":"pw", "POSTGRES_DB":"kubenova", "POSTGRES_USER":"kubenova" },
        WaitingFor:   wait.ForLog("database system is ready to accept connections").WithStartupTimeout(60*time.Second),
    }
    c, err := tc.GenericContainer(ctx, tc.GenericContainerRequest{ContainerRequest: req, Started: true})
    if err != nil { t.Fatalf("container: %v", err) }
    host, _ := c.Host(ctx)
    port, _ := c.MappedPort(ctx, "5432")
    dsn = fmt.Sprintf("postgres://kubenova:pw@%s:%s/kubenova?sslmode=disable", host, port.Port())
    return dsn, func(){ _ = c.Terminate(ctx) }
}

func TestPostgresStoreIntegration(t *testing.T) {
    if os.Getenv("RUN_PG_INTEGRATION") == "" { t.Skip("set RUN_PG_INTEGRATION=1 to run") }
    dsn, stop := startPostgres(t); defer stop()
    ctx := context.Background()
    p, err := NewPostgres(ctx, dsn)
    if err != nil { t.Fatalf("pg connect: %v", err) }
    defer p.Close(ctx)

    // basic CRUD
    if err := p.CreateTenant(ctx, types.Tenant{Name:"alice", CreatedAt: time.Now()}); err != nil { t.Fatal(err) }
    if err := p.CreateProject(ctx, types.Project{Tenant:"alice", Name:"demo", CreatedAt: time.Now()}); err != nil { t.Fatal(err) }
    if err := p.CreateApp(ctx, types.App{Tenant:"alice", Project:"demo", Name:"app", CreatedAt: time.Now()}); err != nil { t.Fatal(err) }
    id, err := p.CreateCluster(ctx, types.Cluster{Name:"kind"}, "ZW5j")
    if err != nil { t.Fatal(err) }
    // events
    e := []types.Event{{Type:"Info", Resource:"agent", Payload: map[string]any{"m":"ok"}, TS: time.Now()}}
    if err := p.AddEvents(ctx, &id, e); err != nil { t.Fatal(err) }
    got, err := p.ListClusterEvents(ctx, id, 10)
    if err != nil || len(got) != 1 { t.Fatalf("events %#v %v", got, err) }
    // history
    conds := []types.Condition{{Type:"AgentReady", Status:"True", LastTransitionTime: time.Now()}}
    if err := p.AddConditionHistory(ctx, id, conds); err != nil { t.Fatal(err) }
}

