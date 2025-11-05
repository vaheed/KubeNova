package manager

import (
    "context"
    "encoding/base64"
    "encoding/json"
    "errors"
    "net/http"
    "os"
    "strings"
    "time"

    "github.com/go-chi/chi/v5"
    "github.com/go-chi/chi/v5/middleware"
    "github.com/prometheus/client_golang/prometheus/promhttp"
    "github.com/vaheed/kubenova/internal/cluster"
    httpapi "github.com/vaheed/kubenova/internal/http"
    "github.com/vaheed/kubenova/internal/logging"
    "github.com/vaheed/kubenova/internal/metrics"
    "github.com/vaheed/kubenova/internal/store"
    "github.com/vaheed/kubenova/internal/telemetry"
    "github.com/vaheed/kubenova/pkg/types"
	"go.uber.org/zap"
    // removed local compute code; health checks moved to internal/cluster
)

// indirection for testing
var InstallAgentFunc = cluster.InstallAgent

type Server struct {
    r           *chi.Mux
    store       store.Store
    jwtKey      []byte
    requireAuth bool
}

func NewServer(s store.Store) *Server {
	mux := chi.NewRouter()
	mux.Use(middleware.RequestID, middleware.RealIP, middleware.Recoverer)
	mux.Use(func(next http.Handler) http.Handler { // zap logging with request_id, correlation_id and traces
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			reqID := middleware.GetReqID(r.Context())
			ctx := logging.WithRequestID(r.Context(), reqID)
			corr := r.Header.Get("X-Correlation-ID")
			if corr == "" {
				corr = reqID
			}
			ctx = logging.WithCorrelationID(ctx, corr)
			lg := logging.WithTrace(ctx, logging.FromContext(ctx))
			lg.Info("http", zap.String("method", r.Method), zap.String("path", r.URL.Path))
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	})
	srv := &Server{r: mux, store: s, jwtKey: []byte(os.Getenv("JWT_SIGNING_KEY")), requireAuth: parseBool(os.Getenv("KUBENOVA_REQUIRE_AUTH"))}

	mux.Get("/healthz", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200); _, _ = w.Write([]byte("ok")) })
	mux.Get("/readyz", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200); _, _ = w.Write([]byte("ok")) })
    mux.Method(http.MethodGet, "/metrics", promhttp.Handler())
    mux.Get("/openapi.yaml", func(w http.ResponseWriter, r *http.Request) { http.ServeFile(w, r, "docs/openapi/openapi.yaml") })
	// Wait endpoint: blocks until store is usable (DB ready) or timeout
	mux.Get("/wait", srv.waitReady)

    mux.Route("/sync", func(r chi.Router) {
		r.Post("/events", srv.ingestEvents)
		r.Post("/metrics", srv.heartbeat)
		r.Post("/logs", srv.accept204)
    })

    // Single OpenAPI-first HTTP server mounted at /api/v1
    opts := httpapi.ChiServerOptions{BaseRouter: mux, BaseURL: ""}
    _ = httpapi.HandlerWithOptions(httpapi.NewAPIServer(s), opts)
    telemetry.InitOTelProvider() // best-effort noop if not configured
    return srv
}

func (s *Server) Router() http.Handler { return s.r }

func (s *Server) accept204(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}

// waitReady blocks until the underlying store is usable (e.g., DB connected).
// Query param `timeout` (seconds) can override the default (60s).
func (s *Server) waitReady(w http.ResponseWriter, r *http.Request) {
	to := atoi(r.URL.Query().Get("timeout"))
	if to <= 0 {
		to = 60
	}
	ctx, cancel := context.WithTimeout(r.Context(), time.Duration(to)*time.Second)
	defer cancel()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			http.Error(w, "timeout", http.StatusServiceUnavailable)
			return
		case <-ticker.C:
			if _, err := s.store.ListTenants(r.Context()); err == nil {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("ok"))
				return
			}
		}
	}
}

func (s *Server) heartbeat(w http.ResponseWriter, r *http.Request) {
	// increment prometheus heartbeat counter for smoke assertions
	metrics.HeartbeatsTotal.Inc()
	w.WriteHeader(http.StatusNoContent)
}

// helpers
func getenv(k, d string) string {
    if v := os.Getenv(k); v != "" {
        return v
    }
	return d
}
func atoi(s string) int {
	n := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + int(c-'0')
	}
	return n
}
func encodeB64(b []byte) string          { return base64.StdEncoding.EncodeToString(b) }
func decodeB64(s string) ([]byte, error) { return base64.StdEncoding.DecodeString(s) }

// legacy tenant/project/app handlers removed; API is provided by OpenAPI server

// --- Projects ---
// legacy project handlers removed

// --- Apps ---
// legacy app handlers removed

// legacy kubeconfig grant and cluster handlers removed; provided by OpenAPI server

func (s *Server) ingestEvents(w http.ResponseWriter, r *http.Request) {
	var list []types.Event
	if err := json.NewDecoder(r.Body).Decode(&list); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	// cluster optional via query id
	var cid *int
	if v := chi.URLParam(r, "id"); v != "" {
	}
	if q := r.URL.Query().Get("cluster_id"); q != "" {
		id := atoi(q)
		cid = &id
	}
	if cid != nil {
		logging.WithTrace(r.Context(), logging.FromContext(r.Context())).Info("ingest_events", zap.Int("cluster_id", *cid), zap.Int("count", len(list)))
	}
	if err := s.store.AddEvents(r.Context(), cid, list); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) getClusterEvents(w http.ResponseWriter, r *http.Request) {
	id := atoi(chi.URLParam(r, "id"))
	evts, err := s.store.ListClusterEvents(r.Context(), id, 100)
	respond(w, evts, err)
}

// query target cluster for readiness
// computeClusterConditions moved to internal/cluster.ComputeClusterConditions

func parseBool(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "t", "true", "y", "yes", "on":
		return true
	default:
		return false
	}
}

func respond(w http.ResponseWriter, v any, err error) {
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			http.Error(w, "not found", 404)
			return
		}
		http.Error(w, err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		http.Error(w, err.Error(), 500)
	}
}

// StartHTTP starts the server on the provided addr with graceful shutdown.
func StartHTTP(ctx context.Context, srv *http.Server) error {
	go func() { _ = srv.ListenAndServe() }()
	<-ctx.Done()
	c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return srv.Shutdown(c)
}

// legacy access endpoints removed; implemented by OpenAPI server

// Tenant-scoped kubeconfig issuance mapped to existing generator; proxy binding handled server-side.
// legacy kubeconfig issuance removed; provided by OpenAPI server
