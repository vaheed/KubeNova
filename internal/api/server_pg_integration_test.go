//go:build integration && !darwin
// +build integration,!darwin

package api

import (
    "bytes"
    "context"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "os"
    "testing"
    "time"
    tc "github.com/testcontainers/testcontainers-go"
    "github.com/testcontainers/testcontainers-go/wait"
    "github.com/vaheed/kubenova/internal/store"
    "github.com/vaheed/kubenova/pkg/types"
    "fmt"
)

func startPG(t *testing.T) (string, func()) {
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
    port, _ := c.MappedPort(ctx, "5432/tcp")
    dsn := "postgres://kubenova:pw@" + host + ":" + port.Port() + "/kubenova?sslmode=disable"
    return dsn, func(){ _ = c.Terminate(ctx) }
}

func TestManagerEventIngestion_Postgres(t *testing.T) {
    if os.Getenv("RUN_PG_INTEGRATION") == "" { t.Skip("set RUN_PG_INTEGRATION=1 to run") }
    dsn, stop := startPG(t); defer stop()
    p, err := store.NewPostgres(context.Background(), dsn)
    if err != nil { t.Fatal(err) }
    defer p.Close(context.Background())
    // create a cluster to attach events
    id, err := p.CreateCluster(context.Background(), types.Cluster{Name:"c1"}, "ZW5j")
    if err != nil { t.Fatal(err) }
    InstallAgentFunc = func(ctx context.Context, kubeconfig []byte, image, managerURL string) error { return nil }
    srv := NewServer(p)
    ts := httptest.NewServer(srv.Router()); defer ts.Close()
    evs := []types.Event{{Type:"Info", Resource:"agent", Payload: map[string]any{"m":"ok"}, TS: time.Now()}}
    b, _ := json.Marshal(evs)
    req, _ := http.NewRequest(http.MethodPost, ts.URL+"/sync/events?cluster_id="+fmt.Sprint(id), bytes.NewReader(b))
    resp, err := http.DefaultClient.Do(req)
    if err != nil { t.Fatal(err) }
    resp.Body.Close()
    if resp.StatusCode != 204 { t.Fatalf("status %s", resp.Status) }
}
