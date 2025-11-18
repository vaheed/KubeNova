package manager

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/httprate"
	"github.com/golang-jwt/jwt/v5"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/vaheed/kubenova/internal/cluster"
	httpapi "github.com/vaheed/kubenova/internal/http"
	"github.com/vaheed/kubenova/internal/lib/httperr"
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
	// Basic rate limiting to protect the manager API: 100 req/min/IP
	mux.Use(httprate.LimitByIP(100, time.Minute))
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

	// Custom endpoints that are operational (not yet in generated router)
	mux.Post("/api/v1/clusters/{c}/cleanup", srv.cleanupCluster)
	mux.Post("/api/v1/clusters/{c}/bootstrap/paas", srv.paasBootstrap)

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

// cleanupCluster triggers a best-effort cleanup on the target cluster to prepare for bootstrap.
// Requires roles: admin or ops. Returns 202 Accepted and runs asynchronously.
func (s *Server) cleanupCluster(w http.ResponseWriter, r *http.Request) {
	if s.requireAuth {
		roles := rolesFromReq(r, s.jwtKey)
		if !hasAnyRole(roles, "admin", "ops") {
			httperr.Write(w, http.StatusForbidden, "KN-403", "forbidden")
			return
		}
	}
	cid := chi.URLParam(r, "c")
	if cid == "" {
		httperr.Write(w, http.StatusUnprocessableEntity, "KN-422", "cluster id required")
		return
	}
	// Obtain kubeconfig
	_, enc, err := s.store.GetClusterByUID(r.Context(), cid)
	if err != nil || enc == "" {
		httperr.Write(w, http.StatusNotFound, "KN-404", "cluster not found")
		return
	}
	kb, _ := base64.StdEncoding.DecodeString(enc)
	// Async execution
	go func() {
		_ = cluster.CleanPlatform(context.Background(), kb)
	}()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_, _ = w.Write([]byte(`{"accepted":true}`))
}

// paasBootstrap creates a default tenant and project on a given cluster and
// returns a project-scoped kubeconfig suitable for app deployment.
// Requires roles: admin or ops.
func (s *Server) paasBootstrap(w http.ResponseWriter, r *http.Request) {
	if s.requireAuth {
		roles := rolesFromReq(r, s.jwtKey)
		if !hasAnyRole(roles, "admin", "ops") {
			httperr.Write(w, http.StatusForbidden, "KN-403", "forbidden")
			return
		}
	}
	cid := chi.URLParam(r, "c")
	if cid == "" {
		httperr.Write(w, http.StatusUnprocessableEntity, "KN-422", "cluster id required")
		return
	}
	ctx := r.Context()
	// Ensure cluster exists and obtain kubeconfig for namespace operations.
	cl, enc, err := s.store.GetClusterByUID(ctx, cid)
	if err != nil || enc == "" {
		httperr.Write(w, http.StatusNotFound, "KN-404", "cluster not found")
		return
	}
	kb, _ := base64.StdEncoding.DecodeString(enc)

	tenantName := strings.TrimSpace(os.Getenv("KUBENOVA_BOOTSTRAP_TENANT_NAME"))
	if tenantName == "" {
		tenantName = "acme"
	}
	if tenantName == "" {
		httperr.Write(w, http.StatusUnprocessableEntity, "KN-422", "invalid tenant name")
		return
	}
	projectName := strings.TrimSpace(os.Getenv("KUBENOVA_BOOTSTRAP_PROJECT_NAME"))
	if projectName == "" {
		projectName = "web"
	}
	if projectName == "" {
		httperr.Write(w, http.StatusUnprocessableEntity, "KN-422", "invalid project name")
		return
	}

	now := time.Now().UTC()
	// Create or upsert tenant.
	ten := types.Tenant{
		Name:      tenantName,
		Labels:    map[string]string{"kubenova.cluster": cid},
		Owners:    []string{},
		CreatedAt: now,
	}
	if err := s.store.CreateTenant(ctx, ten); err != nil {
		httperr.Write(w, http.StatusInternalServerError, "KN-500", err.Error())
		return
	}
	tenStored, err := s.store.GetTenant(ctx, tenantName)
	if err != nil {
		httperr.Write(w, http.StatusInternalServerError, "KN-500", err.Error())
		return
	}

	// Best-effort: apply bootstrap plan if configured.
	if plan := strings.TrimSpace(os.Getenv("KUBENOVA_BOOTSTRAP_TENANT_PLAN")); plan != "" {
		api := httpapi.NewAPIServer(s.store)
		if _, err := api.ApplyPlanToTenant(ctx, tenStored.UID, plan); err != nil {
			// Log only; do not fail bootstrap if plan application fails.
			logging.WithTrace(ctx, logging.FromContext(ctx)).Error("paas.bootstrap.plan_failed", zap.String("plan", plan), zap.Error(err))
		}
	}

	// Create or upsert project.
	pr := types.Project{
		Tenant:    tenStored.Name,
		Name:      projectName,
		CreatedAt: now,
	}
	if err := s.store.CreateProject(ctx, pr); err != nil {
		httperr.Write(w, http.StatusInternalServerError, "KN-500", err.Error())
		return
	}
	// Ensure the project namespace exists and is labeled.
	_ = cluster.EnsureProjectNamespace(ctx, kb, tenStored.Name, projectName)

	prStored, err := s.store.GetProject(ctx, tenStored.Name, projectName)
	if err != nil {
		httperr.Write(w, http.StatusInternalServerError, "KN-500", err.Error())
		return
	}

	// Issue a project-scoped kubeconfig via Capsule proxy using a bound
	// ServiceAccount token instead of a Manager-signed JWT.
	role := "projectDev"
	ttl := atoi(os.Getenv("KUBENOVA_BOOTSTRAP_KUBECONFIG_TTL"))
	if ttl <= 0 {
		ttl = 3600
	}
	// Derive proxy configuration from cluster labels; PaaS bootstrap requires
	// a per-cluster Capsule proxy URL.
	proxyURL := ""
	proxyCA := ""
	if len(cl.Labels) > 0 {
		if v, ok := cl.Labels["kubenova.capsuleProxyUrl"]; ok {
			proxyURL = v
		}
		if v, ok := cl.Labels["kubenova.capsuleProxyCa"]; ok {
			proxyCA = v
		}
	}
	kcfg, expTime, err := cluster.IssueProjectKubeconfig(ctx, kb, proxyURL, proxyCA, tenStored.Name, prStored.Name, role, ttl)
	if err != nil {
		httperr.Write(w, http.StatusInternalServerError, "KN-500", err.Error())
		return
	}

	resp := map[string]any{
		"cluster":    cl.UID,
		"tenant":     tenStored.Name,
		"tenantId":   tenStored.UID,
		"project":    prStored.Name,
		"projectId":  prStored.UID,
		"kubeconfig": kcfg,
		"expiresAt":  expTime.UTC(),
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// rolesFromReq parses roles from Authorization JWT or X-KN-Roles.
func rolesFromReq(r *http.Request, jwtKey []byte) []string {
	hdr := r.Header.Get("Authorization")
	if hdr != "" && strings.HasPrefix(strings.ToLower(hdr), "bearer ") {
		tok := strings.TrimSpace(strings.TrimPrefix(hdr, "Bearer"))
		var claims jwt.MapClaims
		if _, err := jwt.ParseWithClaims(tok, &claims, func(token *jwt.Token) (interface{}, error) { return jwtKey, nil }); err == nil {
			if arr, ok := claims["roles"].([]any); ok {
				out := make([]string, 0, len(arr))
				for _, v := range arr {
					if s, ok := v.(string); ok {
						out = append(out, s)
					}
				}
				if len(out) > 0 {
					return out
				}
			}
			if v, ok := claims["role"].(string); ok && v != "" {
				return []string{v}
			}
		}
	}
	if v := r.Header.Get("X-KN-Roles"); v != "" {
		return strings.Split(v, ",")
	}
	return nil
}

func hasAnyRole(roles []string, allowed ...string) bool {
	if len(allowed) == 0 {
		return true
	}
	if len(roles) == 0 {
		roles = []string{"readOnly"}
	}
	have := map[string]struct{}{}
	for _, r := range roles {
		have[strings.TrimSpace(r)] = struct{}{}
	}
	for _, want := range allowed {
		if _, ok := have[want]; ok {
			return true
		}
	}
	return false
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
	// cluster optional via query id (UUID)
	var cid *types.ID
	if v := chi.URLParam(r, "id"); v != "" {
	}
	if q := r.URL.Query().Get("cluster_id"); q != "" {
		if id, err := types.ParseID(q); err == nil {
			cid = &id
		}
	}
	if cid != nil {
		logging.WithTrace(r.Context(), logging.FromContext(r.Context())).Info("ingest_events", zap.String("cluster_id", cid.String()), zap.Int("count", len(list)))
	}
	if err := s.store.AddEvents(r.Context(), cid, list); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// legacy cluster events endpoint removed

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

// respond helper removed with legacy handlers

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
