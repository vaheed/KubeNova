package telemetry

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	miniredis "github.com/alicebob/miniredis/v2"
)

func TestRedisBuffer(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	defer mr.Close()
	os.Setenv("REDIS_ADDR", mr.Addr())
	defer os.Unsetenv("REDIS_ADDR")

	mux := http.NewServeMux()
	got := 0
	mux.HandleFunc("/sync/metrics", func(w http.ResponseWriter, r *http.Request) { got++; io.ReadAll(r.Body); w.WriteHeader(204) })
	srv := httptest.NewServer(mux)
	defer srv.Close()
	os.Setenv("MANAGER_URL", srv.URL)
	defer os.Unsetenv("MANAGER_URL")

	b := NewRedisBuffer()
	b.Enqueue("metrics", map[string]int{"a": 1})
	b.Enqueue("metrics", map[string]int{"b": 2})
	b.tick = 100 * time.Millisecond
	b.max = 10
	b.Run()
	defer b.Stop()
	time.Sleep(300 * time.Millisecond)
	if got < 1 {
		t.Fatalf("expected flush to happen, got=%d", got)
	}
}
