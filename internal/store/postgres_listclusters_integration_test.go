//go:build integration && !darwin
// +build integration,!darwin

package store

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	tc "github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"github.com/vaheed/kubenova/pkg/types"
)

func startPG_store(t *testing.T) (string, func()) {
	t.Helper()
	ctx := context.Background()
	req := tc.ContainerRequest{Image: "postgres:16", ExposedPorts: []string{"5432/tcp"}, Env: map[string]string{"POSTGRES_PASSWORD": "pw", "POSTGRES_DB": "kubenova", "POSTGRES_USER": "kubenova"}, WaitingFor: wait.ForListeningPort("5432/tcp").WithStartupTimeout(90 * time.Second)}
	c, err := tc.GenericContainer(ctx, tc.GenericContainerRequest{ContainerRequest: req, Started: true})
	if err != nil {
		t.Fatalf("container: %v", err)
	}
	port, _ := c.MappedPort(ctx, "5432/tcp")
	host := "127.0.0.1"
	dsn := "postgres://kubenova:pw@" + host + ":" + port.Port() + "/kubenova?sslmode=disable"
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

func TestPostgres_ListClustersWithLabelsAndCursor(t *testing.T) {
	dsn, stop := startPG_store(t)
	defer stop()
	p, err := NewPostgres(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close(context.Background())

	// seed clusters
	_, _ = p.CreateCluster(context.Background(), types.Cluster{Name: "c1", Labels: map[string]string{"env": "dev"}}, "ZW5j")
	_, _ = p.CreateCluster(context.Background(), types.Cluster{Name: "c2", Labels: map[string]string{"env": "prod"}}, "ZW5j")
	_, _ = p.CreateCluster(context.Background(), types.Cluster{Name: "c3", Labels: map[string]string{"env": "prod"}}, "ZW5j")

	items, next, err := p.ListClusters(context.Background(), 2, "", "")
	if err != nil || len(items) != 2 || next == "" {
		t.Fatalf("page1 err=%v len=%d next=%s", err, len(items), next)
	}
	items2, next2, err := p.ListClusters(context.Background(), 2, next, "")
	if err != nil || len(items2) < 1 {
		t.Fatalf("page2 err=%v len=%d", err, len(items2))
	}
	if next2 != "" && next2 == next {
		t.Fatalf("cursor not advancing")
	}

	prods, _, err := p.ListClusters(context.Background(), 10, "", "env=prod")
	if err != nil || len(prods) != 2 {
		t.Fatalf("label filter err=%v len=%d", err, len(prods))
	}
}
