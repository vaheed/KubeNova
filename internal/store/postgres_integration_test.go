//go:build integration && !darwin
// +build integration,!darwin

package store

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	tc "github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"github.com/vaheed/kubenova/pkg/types"
	"os"
	"testing"
	"time"
)

func startPostgres(t *testing.T) (dsn string, terminate func()) {
	t.Helper()
	ctx := context.Background()
	req := tc.ContainerRequest{
		Image:        "postgres:16",
		ExposedPorts: []string{"5432/tcp"},
		Env:          map[string]string{"POSTGRES_PASSWORD": "pw", "POSTGRES_DB": "kubenova", "POSTGRES_USER": "kubenova"},
		WaitingFor:   wait.ForListeningPort("5432/tcp").WithStartupTimeout(90 * time.Second),
	}
	c, err := tc.GenericContainer(ctx, tc.GenericContainerRequest{ContainerRequest: req, Started: true})
	if err != nil {
		t.Fatalf("container: %v", err)
	}
	port, _ := c.MappedPort(ctx, "5432/tcp")
	// Prefer IPv4 to avoid ::1 issues on CI
	dsn = fmt.Sprintf("postgres://kubenova:pw@127.0.0.1:%s/kubenova?sslmode=disable", port.Port())
	// Proactively wait until DB accepts connections (listening isn't enough)
	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		cfg, err := pgxpool.ParseConfig(dsn)
		if err == nil {
			if db, err := pgxpool.NewWithConfig(ctx, cfg); err == nil {
				if err = db.Ping(ctx); err == nil {
					db.Close()
					break
				}
				db.Close()
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	return dsn, func() { _ = c.Terminate(ctx) }
}

func TestPostgresStoreIntegration(t *testing.T) {
	if os.Getenv("RUN_PG_INTEGRATION") == "" {
		t.Skip("set RUN_PG_INTEGRATION=1 to run")
	}
	dsn, stop := startPostgres(t)
	defer stop()
	ctx := context.Background()
	p, err := NewPostgres(ctx, dsn)
	if err != nil {
		t.Fatalf("pg connect: %v", err)
	}
	defer p.Close(ctx)

	// basic CRUD
	if err := p.CreateTenant(ctx, types.Tenant{Name: "alice", CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := p.CreateProject(ctx, types.Project{Tenant: "alice", Name: "demo", CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := p.CreateApp(ctx, types.App{Tenant: "alice", Project: "demo", Name: "app", CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	id, err := p.CreateCluster(ctx, types.Cluster{Name: "kind"}, "ZW5j")
	if err != nil {
		t.Fatal(err)
	}
	// events
	e := []types.Event{{Type: "Info", Resource: "agent", Payload: map[string]any{"m": "ok"}, TS: time.Now()}}
	if err := p.AddEvents(ctx, &id, e); err != nil {
		t.Fatal(err)
	}
	got, err := p.ListClusterEvents(ctx, id, 10)
	if err != nil || len(got) != 1 {
		t.Fatalf("events %#v %v", got, err)
	}
	// history
	conds := []types.Condition{{Type: "AgentReady", Status: "True", LastTransitionTime: time.Now()}}
	if err := p.AddConditionHistory(ctx, id, conds); err != nil {
		t.Fatal(err)
	}
}
