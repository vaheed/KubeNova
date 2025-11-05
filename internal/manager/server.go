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
	"github.com/golang-jwt/jwt/v5"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/vaheed/kubenova/internal/cluster"
	"github.com/vaheed/kubenova/internal/logging"
	"github.com/vaheed/kubenova/internal/metrics"
	"github.com/vaheed/kubenova/internal/store"
	"github.com/vaheed/kubenova/internal/telemetry"
	"github.com/vaheed/kubenova/pkg/types"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
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

	mux.Route("/api/v1", func(r chi.Router) {
		if srv.requireAuth {
			r.Use(srv.jwtMiddleware)
		}

		// System endpoints (version/features under /api/v1 for consistency)
		r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200); _, _ = w.Write([]byte("ok")) })
		r.Get("/readyz", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200); _, _ = w.Write([]byte("ok")) })
		r.Get("/version", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"version":"1.0.0"}`))
		})
		r.Get("/features", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"tenancy":true,"vela":true,"proxy":true}`))
		})

		// Access & Tokens
		r.Post("/tokens", srv.issueToken)
		r.Get("/me", srv.getMe)

		r.Get("/tenants", srv.listTenants)
		r.Post("/tenants", srv.createTenant)
		r.Route("/tenants/{name}", func(r chi.Router) {
			r.Get("/", srv.getTenant)
			r.Put("/", srv.updateTenant)
			r.Delete("/", srv.deleteTenant)
			r.Get("/projects", srv.listProjects)
		})

		r.Post("/projects", srv.createProject)
		r.Route("/projects/{tenant}/{name}", func(r chi.Router) {
			r.Get("/", srv.getProject)
			r.Delete("/", srv.deleteProject)
			r.Get("/apps", srv.listApps)
		})

		r.Post("/apps", srv.createApp)
		r.Route("/apps/{tenant}/{project}/{name}", func(r chi.Router) {
			r.Get("/", srv.getApp)
			r.Delete("/", srv.deleteApp)
		})

		r.Post("/kubeconfig-grants", srv.issueKubeconfig)
		// Also expose new scoped kubeconfig path (tenant-focused)
		r.Post("/tenants/{name}/kubeconfig", srv.issueKubeconfigTenantScoped)

		// clusters
		r.Post("/clusters", srv.createCluster)
		r.Get("/clusters/{id}", srv.getCluster)
		r.Get("/clusters/{id}/events", srv.getClusterEvents)
	})
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

// --- Auth ---
type Claims struct {
	Role   string `json:"role"`
	Tenant string `json:"tenant"`
	jwt.RegisteredClaims
}

type ctxKey int

const claimsKey ctxKey = 1

func (s *Server) jwtMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hdr := r.Header.Get("Authorization")
		if hdr == "" || !strings.HasPrefix(strings.ToLower(hdr), "bearer ") {
			http.Error(w, "missing bearer", http.StatusUnauthorized)
			return
		}
		tok := strings.TrimSpace(strings.TrimPrefix(hdr, "Bearer"))
		var c Claims
		_, err := jwt.ParseWithClaims(tok, &c, func(token *jwt.Token) (interface{}, error) { return s.jwtKey, nil })
		if err != nil {
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}
		ctx := context.WithValue(r.Context(), claimsKey, &c)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) caller(r *http.Request) *Claims {
	if !s.requireAuth {
		return &Claims{Role: "tenant-admin", Tenant: "*"}
	}
	if v := r.Context().Value(claimsKey); v != nil {
		return v.(*Claims)
	}
	return &Claims{}
}

func canReadTenant(c *Claims, tenant string) bool {
	if c.Tenant == "*" {
		return true
	}
	return c.Tenant == tenant && (c.Role == "tenant-admin" || c.Role == "tenant-dev" || c.Role == "read-only")
}
func canWriteTenant(c *Claims, tenant string) bool {
	if c.Tenant == "*" {
		return true
	}
	return c.Tenant == tenant && (c.Role == "tenant-admin")
}
func canDevTenant(c *Claims, tenant string) bool {
	if c.Tenant == "*" {
		return true
	}
	return c.Tenant == tenant && (c.Role == "tenant-admin" || c.Role == "tenant-dev")
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

// --- Tenants ---
func (s *Server) listTenants(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.ListTenants(r.Context())
	// RBAC filter: tenant roles only see their own
	c := s.caller(r)
	if c.Tenant != "*" {
		filtered := make([]types.Tenant, 0)
		for _, t := range items {
			if t.Name == c.Tenant {
				filtered = append(filtered, t)
			}
		}
		items = filtered
	}
	respond(w, items, err)
}
func (s *Server) createTenant(w http.ResponseWriter, r *http.Request) {
	var t types.Tenant
	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	t.CreatedAt = time.Now().UTC()
	if !canWriteTenant(s.caller(r), t.Name) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	logging.WithTrace(r.Context(), logging.FromContext(r.Context())).Info("create_tenant", zap.String("tenant", t.Name))
	err := s.store.CreateTenant(r.Context(), t)
	respond(w, t, err)
}
func (s *Server) getTenant(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if !canReadTenant(s.caller(r), name) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	t, err := s.store.GetTenant(r.Context(), name)
	respond(w, t, err)
}
func (s *Server) updateTenant(w http.ResponseWriter, r *http.Request) {
	var t types.Tenant
	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if !canWriteTenant(s.caller(r), t.Name) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	err := s.store.UpdateTenant(r.Context(), t)
	respond(w, t, err)
}
func (s *Server) deleteTenant(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if !canWriteTenant(s.caller(r), name) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	err := s.store.DeleteTenant(r.Context(), name)
	respond(w, map[string]string{"status": "ok"}, err)
}

// --- Projects ---
func (s *Server) listProjects(w http.ResponseWriter, r *http.Request) {
	tenant := chi.URLParam(r, "name")
	if !canReadTenant(s.caller(r), tenant) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	items, err := s.store.ListProjects(r.Context(), tenant)
	respond(w, items, err)
}
func (s *Server) createProject(w http.ResponseWriter, r *http.Request) {
	var p types.Project
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	p.CreatedAt = time.Now().UTC()
	if !canDevTenant(s.caller(r), p.Tenant) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	logging.WithTrace(r.Context(), logging.FromContext(r.Context())).Info("create_project", zap.String("tenant", p.Tenant), zap.String("project", p.Name))
	err := s.store.CreateProject(r.Context(), p)
	respond(w, p, err)
}
func (s *Server) getProject(w http.ResponseWriter, r *http.Request) {
	tenant, name := chi.URLParam(r, "tenant"), chi.URLParam(r, "name")
	if !canReadTenant(s.caller(r), tenant) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	p, err := s.store.GetProject(r.Context(), tenant, name)
	respond(w, p, err)
}
func (s *Server) deleteProject(w http.ResponseWriter, r *http.Request) {
	tenant, name := chi.URLParam(r, "tenant"), chi.URLParam(r, "name")
	if !canDevTenant(s.caller(r), tenant) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	err := s.store.DeleteProject(r.Context(), tenant, name)
	respond(w, map[string]string{"status": "ok"}, err)
}

// --- Apps ---
func (s *Server) listApps(w http.ResponseWriter, r *http.Request) {
	tenant, project := chi.URLParam(r, "tenant"), chi.URLParam(r, "name")
	if !canReadTenant(s.caller(r), tenant) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	items, err := s.store.ListApps(r.Context(), tenant, project)
	respond(w, items, err)
}
func (s *Server) createApp(w http.ResponseWriter, r *http.Request) {
	var a types.App
	if err := json.NewDecoder(r.Body).Decode(&a); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	a.CreatedAt = time.Now().UTC()
	if !canDevTenant(s.caller(r), a.Tenant) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	logging.WithTrace(r.Context(), logging.FromContext(r.Context())).Info("create_app", zap.String("tenant", a.Tenant), zap.String("project", a.Project), zap.String("app", a.Name))
	err := s.store.CreateApp(r.Context(), a)
	respond(w, a, err)
}
func (s *Server) getApp(w http.ResponseWriter, r *http.Request) {
	tenant, project, name := chi.URLParam(r, "tenant"), chi.URLParam(r, "project"), chi.URLParam(r, "name")
	if !canReadTenant(s.caller(r), tenant) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	a, err := s.store.GetApp(r.Context(), tenant, project, name)
	respond(w, a, err)
}
func (s *Server) deleteApp(w http.ResponseWriter, r *http.Request) {
	tenant, project, name := chi.URLParam(r, "tenant"), chi.URLParam(r, "project"), chi.URLParam(r, "name")
	if !canDevTenant(s.caller(r), tenant) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	err := s.store.DeleteApp(r.Context(), tenant, project, name)
	respond(w, map[string]string{"status": "ok"}, err)
}

// --- Kubeconfig Grant ---
func (s *Server) issueKubeconfig(w http.ResponseWriter, r *http.Request) {
	var g types.KubeconfigGrant
	if err := json.NewDecoder(r.Body).Decode(&g); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if g.Expires.IsZero() {
		g.Expires = time.Now().UTC().Add(1 * time.Hour)
	}
	if !canDevTenant(s.caller(r), g.Tenant) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	kubeconfig, err := GenerateKubeconfig(g, os.Getenv("CAPSULE_PROXY_URL"))
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	g.Kubeconfig = kubeconfig
	respond(w, g, nil)
}

// --- Clusters ---
func (s *Server) createCluster(w http.ResponseWriter, r *http.Request) {
	var c types.Cluster
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	// decode kubeconfig
	kb, err := decodeB64(c.KubeconfigB64)
	if err != nil {
		http.Error(w, "invalid kubeconfig", 400)
		return
	}
	// encrypt for storage
	enc := encodeB64(kb) // in real world, envelope encrypt; keep as base64 here for simplicity
	id, err := s.store.CreateCluster(r.Context(), c, enc)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	// install agent (skip in E2E fake mode). Do this asynchronously so the
	// registration call returns immediately and callers can poll readiness.
	if !parseBool(os.Getenv("KUBENOVA_E2E_FAKE")) {
		image := getenv("AGENT_IMAGE", "ghcr.io/vaheed/kubenova/agent:dev")
		mgr := getenv("MANAGER_URL_PUBLIC", "http://kubenova-manager.kubenova-system.svc.cluster.local:8080")
		logging.WithTrace(r.Context(), logging.FromContext(r.Context())).Info("install_agent_start", zap.String("cluster", c.Name), zap.String("image", image))
		kbCopy := append([]byte(nil), kb...)
		go func(clusterID int, clusterName string) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()
			if err := InstallAgentFunc(ctx, kbCopy, image, mgr); err != nil {
				logging.L.Error("install_agent_error", zap.String("cluster", clusterName), zap.Error(err))
			} else {
				logging.L.Info("install_agent_done", zap.String("cluster", clusterName))
			}
		}(id, c.Name)
	}
	c.ID = id
	c.KubeconfigB64 = "" // don’t echo
	respond(w, c, nil)
}

func (s *Server) getCluster(w http.ResponseWriter, r *http.Request) {
	id := atoi(chi.URLParam(r, "id"))
	c, enc, err := s.store.GetCluster(r.Context(), id)
	if err != nil {
		respond(w, nil, err)
		return
	}
	// Compute conditions live from cluster status
	kb, _ := decodeB64(enc)
	conds := computeClusterConditions(r.Context(), kb)
	c.Conditions = conds
	// persist history (best-effort)
	_ = s.store.AddConditionHistory(r.Context(), id, conds)
	respond(w, c, nil)
}

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
func computeClusterConditions(ctx context.Context, kubeconfig []byte) []types.Condition {
	conds := []types.Condition{}
	if parseBool(os.Getenv("KUBENOVA_E2E_FAKE")) {
		now := time.Now().UTC()
		return []types.Condition{
			{Type: "AgentReady", Status: "True", LastTransitionTime: now},
			{Type: "AddonsReady", Status: "True", LastTransitionTime: now},
		}
	}
	cfg, err := clientcmd.RESTConfigFromKubeConfig(kubeconfig)
	if err != nil {
		return failConds(err)
	}
	cset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return failConds(err)
	}
	now := time.Now().UTC()
	// AgentReady
	agentReady := false
	if dep, err := cset.AppsV1().Deployments("kubenova-system").Get(ctx, "kubenova-agent", metav1.GetOptions{}); err == nil {
		if dep.Status.ReadyReplicas >= 2 {
			if _, err := cset.AutoscalingV2().HorizontalPodAutoscalers("kubenova-system").Get(ctx, "kubenova-agent", metav1.GetOptions{}); err == nil {
				agentReady = true
			}
		}
	}
	conds = append(conds, types.Condition{Type: "AgentReady", Status: boolstr(agentReady), LastTransitionTime: now})
	// AddonsReady
	addonReady := false
	if _, err := cset.AppsV1().Deployments("capsule-system").Get(ctx, "capsule-controller-manager", metav1.GetOptions{}); err == nil {
		if _, err := cset.AppsV1().Deployments("capsule-system").Get(ctx, "capsule-proxy", metav1.GetOptions{}); err == nil {
			if _, err := cset.AppsV1().Deployments("vela-system").Get(ctx, "vela-core", metav1.GetOptions{}); err == nil {
				// CRDs present check
				discs, _ := cset.Discovery().ServerPreferredNamespacedResources()
				crdCapsule := false
				crdVela := false
				for _, rl := range discs {
					if rl.GroupVersion == "capsule.clastix.io/v1beta2" {
						crdCapsule = true
					}
					if rl.GroupVersion == "core.oam.dev/v1beta1" {
						crdVela = true
					}
				}
				if crdCapsule && crdVela {
					addonReady = true
				}
			}
		}
	}
	conds = append(conds, types.Condition{Type: "AddonsReady", Status: boolstr(addonReady), LastTransitionTime: now})
	return conds
}

func boolstr(b bool) string {
	if b {
		return "True"
	}
	return "False"
}
func failConds(err error) []types.Condition {
	return []types.Condition{{Type: "AgentReady", Status: "False", Reason: "Error"}, {Type: "AddonsReady", Status: "False", Reason: "Error"}}
}

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

// Build a minimal kubeconfig string bound to the capsule-proxy URL and JWT token
func GenerateKubeconfig(g types.KubeconfigGrant, server string) ([]byte, error) {
	if server == "" {
		server = "https://capsule-proxy.kubenova.svc"
	}
	// token is NOT issued here for real clusters; we return a placeholder to keep the example
	token := "placeholder"
	b := strings.Builder{}
	b.WriteString("apiVersion: v1\nkind: Config\n")
	b.WriteString("clusters:\n- name: capsule\n  cluster:\n    insecure-skip-tls-verify: true\n    server: ")
	b.WriteString(server)
	b.WriteString("\ncontexts:\n- name: tenant\n  context:\n    cluster: capsule\n    user: tenant-user\ncurrent-context: tenant\nusers:\n- name: tenant-user\n  user:\n    token: ")
	b.WriteString(token)
	b.WriteString("\n")
	return []byte(b.String()), nil
}

// StartHTTP starts the server on the provided addr with graceful shutdown.
func StartHTTP(ctx context.Context, srv *http.Server) error {
	go func() { _ = srv.ListenAndServe() }()
	<-ctx.Done()
	c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return srv.Shutdown(c)
}

// --- Access & Tokens ---
type tokenReq struct {
	Subject    string   `json:"subject"`
	Roles      []string `json:"roles"`
	TTLSeconds int      `json:"ttlSeconds"`
}

func (s *Server) issueToken(w http.ResponseWriter, r *http.Request) {
	var req tokenReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "KN-422: invalid payload", http.StatusUnprocessableEntity)
		return
	}
	if req.Subject == "" {
		http.Error(w, "KN-422: subject required", http.StatusUnprocessableEntity)
		return
	}
	if len(req.Roles) == 0 {
		req.Roles = []string{"read-only"}
	}
	ttl := req.TTLSeconds
	if ttl <= 0 || ttl > 2592000 {
		ttl = 3600
	}
	// Compose claims compatible with existing Claims struct
	role := req.Roles[0]
	c := jwt.MapClaims{
		"sub":   req.Subject,
		"role":  role,
		"roles": req.Roles,
		"exp":   time.Now().Add(time.Duration(ttl) * time.Second).Unix(),
	}
	key := s.jwtKey
	if len(key) == 0 {
		key = []byte("dev")
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, c)
	ss, err := tok.SignedString(key)
	if err != nil {
		http.Error(w, "KN-500: sign failure", 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"token": ss, "expiresAt": time.Now().Add(time.Duration(ttl) * time.Second).UTC()})
}

func (s *Server) getMe(w http.ResponseWriter, r *http.Request) {
	c := s.caller(r)
	roles := []string{c.Role}
	if roles[0] == "" {
		roles = []string{"read-only"}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"subject": "", "roles": roles})
}

// Tenant-scoped kubeconfig issuance mapped to existing generator; proxy binding handled server-side.
func (s *Server) issueKubeconfigTenantScoped(w http.ResponseWriter, r *http.Request) {
	tenant := chi.URLParam(r, "name")
	if tenant == "" {
		http.Error(w, "KN-422: tenant required", 422)
		return
	}
	var in struct {
		Project    string `json:"project"`
		Role       string `json:"role"`
		TTLSeconds int    `json:"ttlSeconds"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	if in.Role == "" {
		in.Role = "read-only"
	}
	if !canReadTenant(s.caller(r), tenant) {
		http.Error(w, "KN-403: forbidden", http.StatusForbidden)
		return
	}
	g := types.KubeconfigGrant{Tenant: tenant, Project: in.Project, Role: in.Role, Expires: time.Now().Add(1 * time.Hour)}
	cfg, err := GenerateKubeconfig(g, "")
	if err != nil {
		http.Error(w, "KN-500: kubeconfig", 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"kubeconfig": encodeB64(cfg), "expiresAt": g.Expires})
}
