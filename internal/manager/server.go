package manager

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/golang-jwt/jwt/v5"
	"github.com/vaheed/kubenova/internal/cluster"
	"github.com/vaheed/kubenova/internal/logging"
	"github.com/vaheed/kubenova/internal/reconcile"
	"github.com/vaheed/kubenova/internal/store"
	v1alpha1 "github.com/vaheed/kubenova/pkg/api/v1alpha1"
	"github.com/vaheed/kubenova/pkg/types"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	version                   = "v0.1.2"
	authContextKey            = contextKey("auth")
	defaultTokenTTL           = 60 * time.Minute
	maxBodyBytes        int64 = 1 << 20 // 1MB
	otelServiceName           = "kubenova-manager"
	defaultCapsuleProxy       = "https://proxy.kubenova.local"
)

type contextKey string

type kubeClientFactory func(context.Context, string) (ctrlclient.Client, error)

// Server exposes the HTTP handlers for the Manager.
type Server struct {
	store       store.Store
	requireAuth bool
	signingKey  []byte
	kubeFactory kubeClientFactory
}

// NewServer builds a Server using the provided persistence store.
func NewServer(st store.Store) *Server {
	return &Server{
		store:       st,
		requireAuth: parseBool(os.Getenv("KUBENOVA_REQUIRE_AUTH")),
		signingKey:  []byte(os.Getenv("JWT_SIGNING_KEY")),
		kubeFactory: defaultKubeClientFactory(),
	}
}

// Router returns the configured HTTP handler.
func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(otelhttp.NewMiddleware(otelServiceName))
	r.Use(s.logMiddleware)

	r.Route("/api/v1", func(api chi.Router) {
		api.Get("/healthz", s.healthz)
		api.Get("/readyz", s.readyz)
		api.Get("/version", s.version)
		api.Get("/features", s.features)
		api.Post("/telemetry/events", s.telemetryEvent)

		api.Post("/tokens", s.issueToken)
		api.With(s.authMiddleware).Get("/me", s.me)

		api.Route("/clusters", func(r chi.Router) {
			r.Use(s.authMiddleware)
			r.Post("/", s.createCluster)
			r.Get("/", s.listClusters)
			r.Route("/{clusterID}", func(r chi.Router) {
				r.Get("/", s.getCluster)
				r.Delete("/", s.deleteCluster)
				r.Get("/capabilities", s.getCapabilities)
				r.Post("/bootstrap/{component}", s.bootstrapComponent)
				r.Post("/refresh", s.refreshCluster)

				r.Route("/tenants", func(r chi.Router) {
					r.Get("/", s.listTenants)
					r.Post("/", s.createTenant)
					r.Route("/{tenantID}", func(r chi.Router) {
						r.Get("/", s.getTenant)
						r.Delete("/", s.deleteTenant)
						r.Put("/owners", s.updateTenantOwners)
						r.Put("/quotas", s.updateTenantQuotas)
						r.Put("/limits", s.updateTenantLimits)
						r.Put("/network-policies", s.updateTenantNetworkPolicies)
						r.Get("/summary", s.tenantSummary)

						r.Route("/projects", func(r chi.Router) {
							r.Get("/", s.listProjects)
							r.Post("/", s.createProject)
							r.Route("/{projectID}", func(r chi.Router) {
								r.Get("/", s.getProject)
								r.Put("/", s.updateProject)
								r.Delete("/", s.deleteProject)
								r.Put("/access", s.updateProjectAccess)
								r.Get("/kubeconfig", s.projectKubeconfig)

								r.Route("/apps", func(r chi.Router) {
									r.Get("/", s.listApps)
									r.Post("/", s.createApp)
									r.Route("/{appID}", func(r chi.Router) {
										r.Get("/", s.getApp)
										r.Put("/", s.updateApp)
										r.Get("/status", s.appStatus)
										r.Get("/revisions", s.appRevisions)
										r.Get("/diff/{revA}/{revB}", s.appDiff)
										r.Get("/logs/{component}", s.appLogs)
										r.Put("/traits", s.updateAppTraits)
										r.Put("/policies", s.updateAppPolicies)
										r.Post("/workflow/run", s.runWorkflow)
										r.Get("/workflow/runs", s.listWorkflowRuns)
									})
									r.Post("/{appID}:deploy", s.deployApp)
									r.Post("/{appID}:suspend", s.suspendApp)
									r.Post("/{appID}:resume", s.resumeApp)
									r.Post("/{appID}:rollback", s.rollbackApp)
									r.Post("/{appID}:delete", s.deleteAppAction)
								})
							})
						})
					})
				})
			})
		})

		api.With(s.authMiddleware).Route("/tenants", func(r chi.Router) {
			r.Route("/{tenantID}", func(r chi.Router) {
				r.Post("/kubeconfig", s.tenantKubeconfig)
				r.Get("/usage", s.tenantUsage)
			})
		})

		api.With(s.authMiddleware).Route("/projects", func(r chi.Router) {
			r.Route("/{projectID}", func(r chi.Router) {
				r.Get("/usage", s.projectUsage)
			})
		})

		api.With(s.authMiddleware).Route("/apps", func(r chi.Router) {
			r.Route("/runs/{runID}", func(r chi.Router) {
				r.Get("/", s.getWorkflowRun)
			})
		})

		api.Post("/telemetry/events", s.telemetryEvent)
	})

	return r
}

// StartHTTP listens and serves until the context is canceled.
func StartHTTP(ctx context.Context, srv *http.Server) error {
	errCh := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		_ = srv.Shutdown(context.Background())
		return ctx.Err()
	case err := <-errCh:
		return err
	}
}

func (s *Server) logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)
		fields := []zap.Field{
			zap.String("method", r.Method),
			zap.String("path", r.URL.Path),
			zap.Int("status", ww.Status()),
			zap.Duration("duration", time.Since(start)),
			zap.String("request_id", middleware.GetReqID(r.Context())),
		}
		spanCtx := trace.SpanContextFromContext(r.Context())
		if spanCtx.IsValid() {
			fields = append(fields, zap.String("trace_id", spanCtx.TraceID().String()))
		}
		logging.L.Info("http_request", fields...)
	})
}

func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.requireAuth {
			rolesHeader := r.Header.Get("X-KN-Roles")
			roles := []string{}
			if rolesHeader != "" {
				for _, part := range strings.Split(rolesHeader, ",") {
					if trimmed := strings.TrimSpace(part); trimmed != "" {
						roles = append(roles, trimmed)
					}
				}
			}
			ctx := context.WithValue(r.Context(), authContextKey, &AuthContext{
				Subject: "anonymous",
				Roles:   roles,
			})
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}
		authz := r.Header.Get("Authorization")
		if !strings.HasPrefix(authz, "Bearer ") {
			writeError(w, http.StatusUnauthorized, "KN-401", "missing bearer token")
			return
		}
		tokenStr := strings.TrimPrefix(authz, "Bearer ")
		token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
			if t.Method.Alg() != jwt.SigningMethodHS256.Alg() {
				return nil, fmt.Errorf("unexpected signing method %s", t.Method.Alg())
			}
			return s.signingKey, nil
		})
		if err != nil || !token.Valid {
			writeError(w, http.StatusUnauthorized, "KN-401", "invalid token")
			return
		}
		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			writeError(w, http.StatusUnauthorized, "KN-401", "invalid token claims")
			return
		}
		roles := []string{}
		if raw, ok := claims["roles"].([]any); ok {
			for _, r := range raw {
				if str, ok := r.(string); ok {
					roles = append(roles, str)
				}
			}
		}
		subject, _ := claims["sub"].(string)
		ctx := context.WithValue(r.Context(), authContextKey, &AuthContext{
			Subject: subject,
			Roles:   roles,
		})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

type AuthContext struct {
	Subject string
	Roles   []string
}

func (s *Server) healthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) readyz(w http.ResponseWriter, r *http.Request) {
	if err := s.store.Health(r.Context()); err != nil {
		writeError(w, http.StatusServiceUnavailable, "KN-500", "store not ready")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (s *Server) version(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"version": version})
}

func (s *Server) features(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"auth":       s.requireAuth,
		"components": []string{"capsule", "capsule-proxy", "kubevela"},
	})
}

func (s *Server) issueToken(w http.ResponseWriter, r *http.Request) {
	if !s.requireRole(w, r, "admin", "ops") && s.requireAuth {
		return
	}
	if len(s.signingKey) == 0 {
		writeError(w, http.StatusInternalServerError, "KN-500", "signing key not configured")
		return
	}
	var req TokenRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "KN-400", err.Error())
		return
	}
	if req.Subject == "" {
		writeError(w, http.StatusBadRequest, "KN-400", "subject is required")
		return
	}
	ttl := defaultTokenTTL
	if req.TTLMinutes > 0 {
		ttl = time.Duration(req.TTLMinutes) * time.Minute
	}
	exp := time.Now().Add(ttl)
	claims := jwt.MapClaims{
		"sub":    req.Subject,
		"roles":  req.Roles,
		"exp":    exp.Unix(),
		"issued": time.Now().Unix(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString(s.signingKey)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "KN-500", "could not sign token")
		return
	}
	writeJSON(w, http.StatusCreated, TokenResponse{
		Token:     signed,
		ExpiresAt: exp,
	})
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	auth := s.authContext(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{
		"subject": auth.Subject,
		"roles":   auth.Roles,
	})
}

func (s *Server) createCluster(w http.ResponseWriter, r *http.Request) {
	if !s.requireRole(w, r, "admin", "ops") {
		return
	}
	var req ClusterRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "KN-400", err.Error())
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		writeError(w, http.StatusBadRequest, "KN-400", "name is required")
		return
	}
	if strings.TrimSpace(req.Kubeconfig) == "" {
		writeError(w, http.StatusBadRequest, "KN-400", "kubeconfig is required")
		return
	}
	kubeconfig := normalizeKubeconfig(req.Kubeconfig)
	cluster := &types.Cluster{
		Name:                 strings.TrimSpace(req.Name),
		Datacenter:           req.Datacenter,
		Labels:               req.Labels,
		Kubeconfig:           kubeconfig,
		CapsuleProxyEndpoint: strings.TrimSpace(req.CapsuleProxyEndpoint),
		Status:               "pending_bootstrap",
		Capabilities:         types.Capabilities{Capsule: true, CapsuleProxy: true, KubeVela: true},
	}
	if err := s.store.CreateCluster(r.Context(), cluster); err != nil {
		if errors.Is(err, store.ErrConflict) {
			writeError(w, http.StatusConflict, "KN-409", "cluster already exists")
			return
		}
		writeError(w, http.StatusInternalServerError, "KN-500", err.Error())
		return
	}
	cluster.Status = "bootstrapping"
	_ = s.store.UpdateCluster(r.Context(), cluster)
	go func(c *types.Cluster) {
		if err := s.installOperator(context.Background(), c); err != nil {
			logging.L.Error("operator_install_failed",
				zap.String("cluster_id", c.ID),
				zap.Error(err),
			)
			c.Status = "error"
		} else {
			c.Status = "connected"
		}
		c.UpdatedAt = time.Now().UTC()
		_ = s.store.UpdateCluster(context.Background(), c)
	}(cluster)
	writeJSON(w, http.StatusCreated, sanitizeCluster(cluster))
}

func (s *Server) listClusters(w http.ResponseWriter, r *http.Request) {
	if !s.requireRole(w, r, "admin", "ops", "readOnly") && s.requireAuth {
		return
	}
	clusters, err := s.store.ListClusters(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "KN-500", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, sanitizeClusters(clusters))
}

func (s *Server) getCluster(w http.ResponseWriter, r *http.Request) {
	if !s.requireRole(w, r, "admin", "ops", "readOnly") && s.requireAuth {
		return
	}
	id := chi.URLParam(r, "clusterID")
	c, err := s.store.GetCluster(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "KN-404", "cluster not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "KN-500", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, sanitizeCluster(c))
}

func (s *Server) deleteCluster(w http.ResponseWriter, r *http.Request) {
	if !s.requireRole(w, r, "admin", "ops") {
		return
	}
	id := chi.URLParam(r, "clusterID")
	if err := s.store.DeleteCluster(r.Context(), id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "KN-404", "cluster not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "KN-500", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) getCapabilities(w http.ResponseWriter, r *http.Request) {
	if !s.requireRole(w, r, "admin", "ops", "readOnly") && s.requireAuth {
		return
	}
	id := chi.URLParam(r, "clusterID")
	c, err := s.store.GetCluster(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "KN-404", "cluster not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "KN-500", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, c.Capabilities)
}

func (s *Server) bootstrapComponent(w http.ResponseWriter, r *http.Request) {
	if !s.requireRole(w, r, "admin", "ops") {
		return
	}
	id := chi.URLParam(r, "clusterID")
	component := chi.URLParam(r, "component")
	c, err := s.store.GetCluster(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "KN-404", "cluster not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "KN-500", err.Error())
		return
	}
	c.Status = "bootstrapping"
	c.UpdatedAt = time.Now().UTC()
	_ = s.store.UpdateCluster(r.Context(), c)

	if component == "operator" {
		if err := s.installOperator(r.Context(), c); err != nil {
			c.Status = "error"
			_ = s.store.UpdateCluster(r.Context(), c)
			writeError(w, http.StatusInternalServerError, "KN-500", fmt.Sprintf("operator install: %v", err))
			return
		}
		c.Status = "connected"
		c.UpdatedAt = time.Now().UTC()
		_ = s.store.UpdateCluster(r.Context(), c)
	}

	writeJSON(w, http.StatusAccepted, map[string]any{
		"clusterId": c.ID,
		"component": component,
		"status":    c.Status,
	})
}

func (s *Server) refreshCluster(w http.ResponseWriter, r *http.Request) {
	if !s.requireRole(w, r, "admin", "ops") {
		return
	}
	id := chi.URLParam(r, "clusterID")
	c, err := s.store.GetCluster(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "KN-404", "cluster not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "KN-500", err.Error())
		return
	}

	if c.Kubeconfig == "" {
		writeError(w, http.StatusBadRequest, "KN-400", "kubeconfig missing")
		return
	}

	c.Status = "reinstalling"
	c.UpdatedAt = time.Now().UTC()
	_ = s.store.UpdateCluster(r.Context(), c)

	cfg, err := clientcmd.RESTConfigFromKubeConfig([]byte(c.Kubeconfig))
	if err != nil {
		c.Status = "error"
		_ = s.store.UpdateCluster(r.Context(), c)
		writeError(w, http.StatusInternalServerError, "KN-500", fmt.Sprintf("parse kubeconfig: %v", err))
		return
	}
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	cli, err := ctrlclient.New(cfg, ctrlclient.Options{Scheme: scheme})
	if err != nil {
		c.Status = "error"
		_ = s.store.UpdateCluster(r.Context(), c)
		writeError(w, http.StatusInternalServerError, "KN-500", fmt.Sprintf("build client: %v", err))
		return
	}

	if err := reconcile.BootstrapHelmJob(r.Context(), cli, cli, scheme); err != nil {
		c.Status = "error"
		_ = s.store.UpdateCluster(r.Context(), c)
		writeError(w, http.StatusInternalServerError, "KN-500", fmt.Sprintf("reinstall components: %v", err))
		return
	}

	c.Status = "connected"
	c.UpdatedAt = time.Now().UTC()
	_ = s.store.UpdateCluster(r.Context(), c)

	writeJSON(w, http.StatusAccepted, map[string]any{
		"clusterId": c.ID,
		"status":    c.Status,
	})
}

func (s *Server) createTenant(w http.ResponseWriter, r *http.Request) {
	if !s.requireRole(w, r, "admin", "ops") {
		return
	}
	clusterID := chi.URLParam(r, "clusterID")
	cluster, err := s.store.GetCluster(r.Context(), clusterID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "KN-404", "cluster not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "KN-500", err.Error())
		return
	}
	var req TenantRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "KN-400", err.Error())
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		writeError(w, http.StatusBadRequest, "KN-400", "name is required")
		return
	}
	t := &types.Tenant{
		ClusterID:       clusterID,
		Name:            strings.TrimSpace(req.Name),
		Owners:          req.Owners,
		Plan:            req.Plan,
		Labels:          req.Labels,
		Quotas:          req.Quotas,
		Limits:          req.Limits,
		NetworkPolicies: req.NetworkPolicies,
	}
	if err := s.store.CreateTenant(r.Context(), t); err != nil {
		switch {
		case errors.Is(err, store.ErrConflict):
			writeError(w, http.StatusConflict, "KN-409", "tenant already exists")
		case errors.Is(err, store.ErrNotFound):
			writeError(w, http.StatusNotFound, "KN-404", "cluster not found")
		default:
			writeError(w, http.StatusInternalServerError, "KN-500", err.Error())
		}
		return
	}
	if err := s.syncTenantWithCluster(r.Context(), cluster, t); err != nil {
		_ = s.store.DeleteTenant(r.Context(), clusterID, t.ID)
		writeError(w, http.StatusInternalServerError, "KN-500", fmt.Sprintf("sync tenant: %v", err))
		return
	}
	writeJSON(w, http.StatusCreated, t)
}

func (s *Server) listTenants(w http.ResponseWriter, r *http.Request) {
	if !s.requireRole(w, r, "admin", "ops", "readOnly") && s.requireAuth {
		return
	}
	clusterID := chi.URLParam(r, "clusterID")
	tenants, err := s.store.ListTenants(r.Context(), clusterID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "KN-500", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, tenants)
}

func (s *Server) getTenant(w http.ResponseWriter, r *http.Request) {
	if !s.requireRole(w, r, "admin", "ops", "readOnly") && s.requireAuth {
		return
	}
	clusterID := chi.URLParam(r, "clusterID")
	tenantID := chi.URLParam(r, "tenantID")
	t, err := s.store.GetTenant(r.Context(), clusterID, tenantID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "KN-404", "tenant not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "KN-500", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (s *Server) deleteTenant(w http.ResponseWriter, r *http.Request) {
	if !s.requireRole(w, r, "admin", "ops") {
		return
	}
	clusterID := chi.URLParam(r, "clusterID")
	tenantID := chi.URLParam(r, "tenantID")
	tenant, err := s.store.GetTenant(r.Context(), clusterID, tenantID)
	if err != nil {
		writeError(w, http.StatusNotFound, "KN-404", "tenant not found")
		return
	}
	if err := s.deleteTenantResource(r.Context(), tenant); err != nil {
		writeError(w, http.StatusInternalServerError, "KN-500", fmt.Sprintf("delete tenant from cluster: %v", err))
		return
	}
	if err := s.store.DeleteTenant(r.Context(), clusterID, tenantID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "KN-404", "tenant not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "KN-500", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) updateTenantOwners(w http.ResponseWriter, r *http.Request) {
	clusterID := chi.URLParam(r, "clusterID")
	tenantID := chi.URLParam(r, "tenantID")
	var req OwnersRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "KN-400", err.Error())
		return
	}
	t, err := s.store.GetTenant(r.Context(), clusterID, tenantID)
	if err != nil {
		writeError(w, http.StatusNotFound, "KN-404", "tenant not found")
		return
	}
	t.Owners = req.Owners
	t.UpdatedAt = time.Now().UTC()
	if err := s.store.UpdateTenant(r.Context(), t); err != nil {
		writeError(w, http.StatusInternalServerError, "KN-500", err.Error())
		return
	}
	if err := s.syncTenant(r.Context(), t); err != nil {
		writeError(w, http.StatusInternalServerError, "KN-500", fmt.Sprintf("sync tenant: %v", err))
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (s *Server) updateTenantQuotas(w http.ResponseWriter, r *http.Request) {
	s.updateTenantMapField(w, r, func(t *types.Tenant, data map[string]string) { t.Quotas = data })
}

func (s *Server) updateTenantLimits(w http.ResponseWriter, r *http.Request) {
	s.updateTenantMapField(w, r, func(t *types.Tenant, data map[string]string) { t.Limits = data })
}

func (s *Server) updateTenantNetworkPolicies(w http.ResponseWriter, r *http.Request) {
	clusterID := chi.URLParam(r, "clusterID")
	tenantID := chi.URLParam(r, "tenantID")
	var req NetworkPolicyRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "KN-400", err.Error())
		return
	}
	t, err := s.store.GetTenant(r.Context(), clusterID, tenantID)
	if err != nil {
		writeError(w, http.StatusNotFound, "KN-404", "tenant not found")
		return
	}
	t.NetworkPolicies = req.Policies
	t.UpdatedAt = time.Now().UTC()
	if err := s.store.UpdateTenant(r.Context(), t); err != nil {
		writeError(w, http.StatusInternalServerError, "KN-500", err.Error())
		return
	}
	if err := s.syncTenant(r.Context(), t); err != nil {
		writeError(w, http.StatusInternalServerError, "KN-500", fmt.Sprintf("sync tenant: %v", err))
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (s *Server) updateTenantMapField(w http.ResponseWriter, r *http.Request, apply func(*types.Tenant, map[string]string)) {
	clusterID := chi.URLParam(r, "clusterID")
	tenantID := chi.URLParam(r, "tenantID")
	var req map[string]string
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "KN-400", err.Error())
		return
	}
	t, err := s.store.GetTenant(r.Context(), clusterID, tenantID)
	if err != nil {
		writeError(w, http.StatusNotFound, "KN-404", "tenant not found")
		return
	}
	apply(t, req)
	t.UpdatedAt = time.Now().UTC()
	if err := s.store.UpdateTenant(r.Context(), t); err != nil {
		writeError(w, http.StatusInternalServerError, "KN-500", err.Error())
		return
	}
	if err := s.syncTenant(r.Context(), t); err != nil {
		writeError(w, http.StatusInternalServerError, "KN-500", fmt.Sprintf("sync tenant: %v", err))
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (s *Server) tenantSummary(w http.ResponseWriter, r *http.Request) {
	if !s.requireRole(w, r, "admin", "ops", "readOnly") && s.requireAuth {
		return
	}
	clusterID := chi.URLParam(r, "clusterID")
	tenantID := chi.URLParam(r, "tenantID")
	projects, _ := s.store.ListProjects(r.Context(), clusterID, tenantID)
	apps, _ := s.store.ListApps(r.Context(), clusterID, tenantID, "")
	summary := types.TenantSummary{
		TenantID:        tenantID,
		ClusterID:       clusterID,
		Projects:        len(projects),
		Apps:            len(apps),
		Namespaces:      2,
		LoadBalancers:   0,
		QuotaViolations: 0,
	}
	writeJSON(w, http.StatusOK, summary)
}

func (s *Server) tenantKubeconfig(w http.ResponseWriter, r *http.Request) {
	if !s.requireRole(w, r, "admin", "ops", "tenantOwner") && s.requireAuth {
		return
	}
	tenantID := chi.URLParam(r, "tenantID")
	tenant := s.findTenant(r.Context(), tenantID)
	if tenant == nil {
		writeError(w, http.StatusNotFound, "KN-404", "tenant not found")
		return
	}
	base := s.clusterProxyBase(r.Context(), tenant.ClusterID)
	writeJSON(w, http.StatusOK, map[string]string{
		"owner":    fmt.Sprintf("%s/%s/owner", base, tenant.Name),
		"readonly": fmt.Sprintf("%s/%s/readonly", base, tenant.Name),
	})
}

func (s *Server) tenantUsage(w http.ResponseWriter, r *http.Request) {
	if !s.requireRole(w, r, "admin", "ops", "readOnly") && s.requireAuth {
		return
	}
	tenantID := chi.URLParam(r, "tenantID")
	tenant := s.findTenant(r.Context(), tenantID)
	if tenant == nil {
		writeError(w, http.StatusNotFound, "KN-404", "tenant not found")
		return
	}
	apps, _ := s.store.ListApps(r.Context(), tenant.ClusterID, tenant.ID, "")
	usage := types.UsageRecord{
		CPURequests:     "0",
		MemoryRequests:  "0",
		PVCStorage:      "0Gi",
		LoadBalancers:   0,
		Pods:            0,
		Namespaces:      2,
		Apps:            len(apps),
		QuotaViolations: 0,
		LastReportedAt:  time.Now().UTC(),
	}
	writeJSON(w, http.StatusOK, usage)
}

func (s *Server) createProject(w http.ResponseWriter, r *http.Request) {
	if !s.requireRole(w, r, "admin", "ops") {
		return
	}
	clusterID := chi.URLParam(r, "clusterID")
	tenantID := chi.URLParam(r, "tenantID")
	cluster, err := s.store.GetCluster(r.Context(), clusterID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "KN-404", "cluster not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "KN-500", err.Error())
		return
	}
	tenant, err := s.store.GetTenant(r.Context(), clusterID, tenantID)
	if err != nil {
		writeError(w, http.StatusNotFound, "KN-404", "tenant not found")
		return
	}
	var req ProjectRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "KN-400", err.Error())
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		writeError(w, http.StatusBadRequest, "KN-400", "name is required")
		return
	}
	p := &types.Project{
		ClusterID:   clusterID,
		TenantID:    tenantID,
		Name:        strings.TrimSpace(req.Name),
		Description: req.Description,
		Labels:      req.Labels,
		Access:      req.Access,
	}
	if err := s.store.CreateProject(r.Context(), p); err != nil {
		switch {
		case errors.Is(err, store.ErrConflict):
			writeError(w, http.StatusConflict, "KN-409", "project already exists")
		case errors.Is(err, store.ErrNotFound):
			writeError(w, http.StatusNotFound, "KN-404", "tenant not found")
		default:
			writeError(w, http.StatusInternalServerError, "KN-500", err.Error())
		}
		return
	}
	if err := s.syncProjectWithCluster(r.Context(), cluster, tenant, p); err != nil {
		_ = s.store.DeleteProject(r.Context(), clusterID, tenantID, p.ID)
		writeError(w, http.StatusInternalServerError, "KN-500", fmt.Sprintf("sync project: %v", err))
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

func (s *Server) listProjects(w http.ResponseWriter, r *http.Request) {
	clusterID := chi.URLParam(r, "clusterID")
	tenantID := chi.URLParam(r, "tenantID")
	projects, err := s.store.ListProjects(r.Context(), clusterID, tenantID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "KN-500", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, projects)
}

func (s *Server) getProject(w http.ResponseWriter, r *http.Request) {
	clusterID := chi.URLParam(r, "clusterID")
	tenantID := chi.URLParam(r, "tenantID")
	projectID := chi.URLParam(r, "projectID")
	project, err := s.store.GetProject(r.Context(), clusterID, tenantID, projectID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "KN-404", "project not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "KN-500", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, project)
}

func (s *Server) updateProject(w http.ResponseWriter, r *http.Request) {
	clusterID := chi.URLParam(r, "clusterID")
	tenantID := chi.URLParam(r, "tenantID")
	projectID := chi.URLParam(r, "projectID")
	project, err := s.store.GetProject(r.Context(), clusterID, tenantID, projectID)
	if err != nil {
		writeError(w, http.StatusNotFound, "KN-404", "project not found")
		return
	}
	var req ProjectRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "KN-400", err.Error())
		return
	}
	if req.Name != "" {
		project.Name = strings.TrimSpace(req.Name)
	}
	if req.Description != "" {
		project.Description = req.Description
	}
	if req.Labels != nil {
		project.Labels = req.Labels
	}
	if req.Access != nil {
		project.Access = req.Access
	}
	project.UpdatedAt = time.Now().UTC()
	if err := s.store.UpdateProject(r.Context(), project); err != nil {
		writeError(w, http.StatusInternalServerError, "KN-500", err.Error())
		return
	}
	if err := s.syncProject(r.Context(), project, nil); err != nil {
		writeError(w, http.StatusInternalServerError, "KN-500", fmt.Sprintf("sync project: %v", err))
		return
	}
	writeJSON(w, http.StatusOK, project)
}

func (s *Server) deleteProject(w http.ResponseWriter, r *http.Request) {
	if !s.requireRole(w, r, "admin", "ops") {
		return
	}
	clusterID := chi.URLParam(r, "clusterID")
	tenantID := chi.URLParam(r, "tenantID")
	projectID := chi.URLParam(r, "projectID")
	project, err := s.store.GetProject(r.Context(), clusterID, tenantID, projectID)
	if err != nil {
		writeError(w, http.StatusNotFound, "KN-404", "project not found")
		return
	}
	if err := s.deleteProjectResource(r.Context(), project); err != nil {
		writeError(w, http.StatusInternalServerError, "KN-500", fmt.Sprintf("delete project from cluster: %v", err))
		return
	}
	if err := s.store.DeleteProject(r.Context(), clusterID, tenantID, projectID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "KN-404", "project not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "KN-500", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) updateProjectAccess(w http.ResponseWriter, r *http.Request) {
	clusterID := chi.URLParam(r, "clusterID")
	tenantID := chi.URLParam(r, "tenantID")
	projectID := chi.URLParam(r, "projectID")
	project, err := s.store.GetProject(r.Context(), clusterID, tenantID, projectID)
	if err != nil {
		writeError(w, http.StatusNotFound, "KN-404", "project not found")
		return
	}
	var req AccessRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "KN-400", err.Error())
		return
	}
	project.Access = req.Access
	project.UpdatedAt = time.Now().UTC()
	if err := s.store.UpdateProject(r.Context(), project); err != nil {
		writeError(w, http.StatusInternalServerError, "KN-500", err.Error())
		return
	}
	if err := s.syncProject(r.Context(), project, nil); err != nil {
		writeError(w, http.StatusInternalServerError, "KN-500", fmt.Sprintf("sync project: %v", err))
		return
	}
	writeJSON(w, http.StatusOK, project)
}

func (s *Server) projectKubeconfig(w http.ResponseWriter, r *http.Request) {
	if !s.requireRole(w, r, "admin", "ops", "tenantOwner", "projectDev") && s.requireAuth {
		return
	}
	tenantID := chi.URLParam(r, "tenantID")
	projectID := chi.URLParam(r, "projectID")
	tenant := s.findTenant(r.Context(), tenantID)
	if tenant == nil {
		writeError(w, http.StatusNotFound, "KN-404", "tenant not found")
		return
	}
	base := s.clusterProxyBase(r.Context(), tenant.ClusterID)
	writeJSON(w, http.StatusOK, map[string]string{
		"projectId":  projectID,
		"kubeconfig": fmt.Sprintf("%s/%s/%s", base, tenant.Name, projectID),
	})
}

func (s *Server) projectUsage(w http.ResponseWriter, r *http.Request) {
	if !s.requireRole(w, r, "admin", "ops", "readOnly") && s.requireAuth {
		return
	}
	projectID := chi.URLParam(r, "projectID")
	project := s.findProject(r.Context(), projectID)
	if project == nil {
		writeError(w, http.StatusNotFound, "KN-404", "project not found")
		return
	}
	apps, _ := s.store.ListApps(r.Context(), project.ClusterID, project.TenantID, project.ID)
	usage := types.UsageRecord{
		CPURequests:     "0",
		MemoryRequests:  "0",
		PVCStorage:      "0Gi",
		LoadBalancers:   0,
		Pods:            len(apps) * 2,
		Namespaces:      1,
		Apps:            len(apps),
		QuotaViolations: 0,
		LastReportedAt:  time.Now().UTC(),
	}
	writeJSON(w, http.StatusOK, usage)
}

func (s *Server) createApp(w http.ResponseWriter, r *http.Request) {
	if !s.requireRole(w, r, "admin", "ops", "projectDev", "tenantOwner") {
		return
	}
	clusterID := chi.URLParam(r, "clusterID")
	tenantID := chi.URLParam(r, "tenantID")
	projectID := chi.URLParam(r, "projectID")
	var req AppRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "KN-400", err.Error())
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		writeError(w, http.StatusBadRequest, "KN-400", "name is required")
		return
	}
	now := time.Now().UTC()
	app := &types.App{
		ClusterID:   clusterID,
		TenantID:    tenantID,
		ProjectID:   projectID,
		Name:        strings.TrimSpace(req.Name),
		Description: req.Description,
		Component:   req.Component,
		Image:       req.Image,
		Spec:        req.Spec,
		Traits:      req.Traits,
		Policies:    req.Policies,
		Status:      "pending",
		Revision:    1,
		Revisions: []types.AppRevision{{
			Number:    1,
			Spec:      req.Spec,
			Traits:    req.Traits,
			Policies:  req.Policies,
			CreatedAt: now,
		}},
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.store.CreateApp(r.Context(), app); err != nil {
		switch {
		case errors.Is(err, store.ErrConflict):
			writeError(w, http.StatusConflict, "KN-409", "app already exists")
		case errors.Is(err, store.ErrNotFound):
			writeError(w, http.StatusNotFound, "KN-404", "project not found")
		default:
			writeError(w, http.StatusInternalServerError, "KN-500", err.Error())
		}
		return
	}
	writeJSON(w, http.StatusCreated, app)
}

func (s *Server) listApps(w http.ResponseWriter, r *http.Request) {
	if !s.requireRole(w, r, "admin", "ops", "projectDev", "tenantOwner", "readOnly") && s.requireAuth {
		return
	}
	clusterID := chi.URLParam(r, "clusterID")
	tenantID := chi.URLParam(r, "tenantID")
	projectID := chi.URLParam(r, "projectID")
	apps, err := s.store.ListApps(r.Context(), clusterID, tenantID, projectID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "KN-500", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, apps)
}

func (s *Server) getApp(w http.ResponseWriter, r *http.Request) {
	if !s.requireRole(w, r, "admin", "ops", "projectDev", "tenantOwner", "readOnly") && s.requireAuth {
		return
	}
	clusterID := chi.URLParam(r, "clusterID")
	tenantID := chi.URLParam(r, "tenantID")
	projectID := chi.URLParam(r, "projectID")
	appID := chi.URLParam(r, "appID")
	app, err := s.store.GetApp(r.Context(), clusterID, tenantID, projectID, appID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "KN-404", "app not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "KN-500", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, app)
}

func (s *Server) updateApp(w http.ResponseWriter, r *http.Request) {
	clusterID := chi.URLParam(r, "clusterID")
	tenantID := chi.URLParam(r, "tenantID")
	projectID := chi.URLParam(r, "projectID")
	appID := chi.URLParam(r, "appID")
	app, err := s.store.GetApp(r.Context(), clusterID, tenantID, projectID, appID)
	if err != nil {
		writeError(w, http.StatusNotFound, "KN-404", "app not found")
		return
	}
	var req AppRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "KN-400", err.Error())
		return
	}
	if req.Description != "" {
		app.Description = req.Description
	}
	if req.Component != "" {
		app.Component = req.Component
	}
	if req.Image != "" {
		app.Image = req.Image
	}
	if req.Spec != nil {
		app.Spec = req.Spec
		app.Revision++
		app.Revisions = append(app.Revisions, types.AppRevision{
			Number:    app.Revision,
			Spec:      req.Spec,
			Traits:    app.Traits,
			Policies:  app.Policies,
			CreatedAt: time.Now().UTC(),
		})
	}
	app.UpdatedAt = time.Now().UTC()
	if err := s.store.UpdateApp(r.Context(), app); err != nil {
		writeError(w, http.StatusInternalServerError, "KN-500", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, app)
}

func (s *Server) deployApp(w http.ResponseWriter, r *http.Request) {
	s.changeAppStatus(w, r, "Deployed", false)
}

func (s *Server) suspendApp(w http.ResponseWriter, r *http.Request) {
	s.changeAppStatus(w, r, "Suspended", true)
}

func (s *Server) resumeApp(w http.ResponseWriter, r *http.Request) {
	s.changeAppStatus(w, r, "Deployed", false)
}

func (s *Server) deleteAppAction(w http.ResponseWriter, r *http.Request) {
	if !s.requireRole(w, r, "admin", "ops", "projectDev", "tenantOwner") {
		return
	}
	clusterID := chi.URLParam(r, "clusterID")
	tenantID := chi.URLParam(r, "tenantID")
	projectID := chi.URLParam(r, "projectID")
	appID := chi.URLParam(r, "appID")
	if err := s.store.DeleteApp(r.Context(), clusterID, tenantID, projectID, appID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "KN-404", "app not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "KN-500", err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "deleting"})
}

func (s *Server) rollbackApp(w http.ResponseWriter, r *http.Request) {
	if !s.requireRole(w, r, "admin", "ops", "projectDev", "tenantOwner") {
		return
	}
	clusterID := chi.URLParam(r, "clusterID")
	tenantID := chi.URLParam(r, "tenantID")
	projectID := chi.URLParam(r, "projectID")
	appID := chi.URLParam(r, "appID")
	app, err := s.store.GetApp(r.Context(), clusterID, tenantID, projectID, appID)
	if err != nil {
		writeError(w, http.StatusNotFound, "KN-404", "app not found")
		return
	}
	if len(app.Revisions) < 2 {
		writeError(w, http.StatusUnprocessableEntity, "KN-422", "no previous revision to roll back to")
		return
	}
	previous := app.Revisions[len(app.Revisions)-2]
	app.Spec = previous.Spec
	app.Traits = previous.Traits
	app.Policies = previous.Policies
	app.Revision = previous.Number
	app.Revisions = app.Revisions[:len(app.Revisions)-1]
	app.Status = "RolledBack"
	app.UpdatedAt = time.Now().UTC()
	if err := s.store.UpdateApp(r.Context(), app); err != nil {
		writeError(w, http.StatusInternalServerError, "KN-500", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, app)
}

func (s *Server) appStatus(w http.ResponseWriter, r *http.Request) {
	if !s.requireRole(w, r, "admin", "ops", "projectDev", "tenantOwner", "readOnly") && s.requireAuth {
		return
	}
	clusterID := chi.URLParam(r, "clusterID")
	tenantID := chi.URLParam(r, "tenantID")
	projectID := chi.URLParam(r, "projectID")
	appID := chi.URLParam(r, "appID")
	app, err := s.store.GetApp(r.Context(), clusterID, tenantID, projectID, appID)
	if err != nil {
		writeError(w, http.StatusNotFound, "KN-404", "app not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":    app.Status,
		"revision":  app.Revision,
		"suspended": app.Suspended,
	})
}

func (s *Server) appRevisions(w http.ResponseWriter, r *http.Request) {
	if !s.requireRole(w, r, "admin", "ops", "projectDev", "tenantOwner", "readOnly") && s.requireAuth {
		return
	}
	clusterID := chi.URLParam(r, "clusterID")
	tenantID := chi.URLParam(r, "tenantID")
	projectID := chi.URLParam(r, "projectID")
	appID := chi.URLParam(r, "appID")
	app, err := s.store.GetApp(r.Context(), clusterID, tenantID, projectID, appID)
	if err != nil {
		writeError(w, http.StatusNotFound, "KN-404", "app not found")
		return
	}
	writeJSON(w, http.StatusOK, app.Revisions)
}

func (s *Server) appDiff(w http.ResponseWriter, r *http.Request) {
	if !s.requireRole(w, r, "admin", "ops", "projectDev", "tenantOwner", "readOnly") && s.requireAuth {
		return
	}
	clusterID := chi.URLParam(r, "clusterID")
	tenantID := chi.URLParam(r, "tenantID")
	projectID := chi.URLParam(r, "projectID")
	appID := chi.URLParam(r, "appID")
	revA := chi.URLParam(r, "revA")
	revB := chi.URLParam(r, "revB")
	app, err := s.store.GetApp(r.Context(), clusterID, tenantID, projectID, appID)
	if err != nil {
		writeError(w, http.StatusNotFound, "KN-404", "app not found")
		return
	}
	a := findRevision(app.Revisions, revA)
	b := findRevision(app.Revisions, revB)
	if a == nil || b == nil {
		writeError(w, http.StatusNotFound, "KN-404", "revision not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"from":    a.Number,
		"to":      b.Number,
		"summary": fmt.Sprintf("changed spec from rev %d to %d", a.Number, b.Number),
	})
}

func (s *Server) appLogs(w http.ResponseWriter, r *http.Request) {
	if !s.requireRole(w, r, "admin", "ops", "projectDev", "tenantOwner", "readOnly") && s.requireAuth {
		return
	}
	component := chi.URLParam(r, "component")
	writeJSON(w, http.StatusOK, map[string]any{
		"component": component,
		"lines": []string{
			"2024-01-01T00:00:00Z starting component",
			"2024-01-01T00:00:01Z reconciled",
		},
	})
}

func (s *Server) updateAppTraits(w http.ResponseWriter, r *http.Request) {
	s.updateAppConfig(w, r, func(app *types.App, req AppRequest) {
		app.Traits = req.Traits
	})
}

func (s *Server) updateAppPolicies(w http.ResponseWriter, r *http.Request) {
	s.updateAppConfig(w, r, func(app *types.App, req AppRequest) {
		app.Policies = req.Policies
	})
}

func (s *Server) runWorkflow(w http.ResponseWriter, r *http.Request) {
	clusterID := chi.URLParam(r, "clusterID")
	tenantID := chi.URLParam(r, "tenantID")
	projectID := chi.URLParam(r, "projectID")
	appID := chi.URLParam(r, "appID")
	app, err := s.store.GetApp(r.Context(), clusterID, tenantID, projectID, appID)
	if err != nil {
		writeError(w, http.StatusNotFound, "KN-404", "app not found")
		return
	}
	var req WorkflowRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "KN-400", err.Error())
		return
	}
	run := types.WorkflowRun{
		ID:        fmt.Sprintf("run-%d", len(app.WorkflowRuns)+1),
		AppID:     app.ID,
		Status:    "Running",
		Inputs:    req.Inputs,
		StartedAt: time.Now().UTC(),
	}
	app.WorkflowRuns = append(app.WorkflowRuns, run)
	app.UpdatedAt = time.Now().UTC()
	if err := s.store.UpdateApp(r.Context(), app); err != nil {
		writeError(w, http.StatusInternalServerError, "KN-500", err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, run)
}

func (s *Server) listWorkflowRuns(w http.ResponseWriter, r *http.Request) {
	clusterID := chi.URLParam(r, "clusterID")
	tenantID := chi.URLParam(r, "tenantID")
	projectID := chi.URLParam(r, "projectID")
	appID := chi.URLParam(r, "appID")
	app, err := s.store.GetApp(r.Context(), clusterID, tenantID, projectID, appID)
	if err != nil {
		writeError(w, http.StatusNotFound, "KN-404", "app not found")
		return
	}
	writeJSON(w, http.StatusOK, app.WorkflowRuns)
}

func (s *Server) getWorkflowRun(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "runID")
	apps, _ := s.store.ListApps(r.Context(), "", "", "")
	for _, app := range apps {
		for _, run := range app.WorkflowRuns {
			if run.ID == runID {
				writeJSON(w, http.StatusOK, run)
				return
			}
		}
	}
	writeError(w, http.StatusNotFound, "KN-404", "run not found")
}

func (s *Server) updateAppConfig(w http.ResponseWriter, r *http.Request, apply func(*types.App, AppRequest)) {
	if !s.requireRole(w, r, "admin", "ops", "projectDev", "tenantOwner") {
		return
	}
	clusterID := chi.URLParam(r, "clusterID")
	tenantID := chi.URLParam(r, "tenantID")
	projectID := chi.URLParam(r, "projectID")
	appID := chi.URLParam(r, "appID")
	app, err := s.store.GetApp(r.Context(), clusterID, tenantID, projectID, appID)
	if err != nil {
		writeError(w, http.StatusNotFound, "KN-404", "app not found")
		return
	}
	var req AppRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "KN-400", err.Error())
		return
	}
	apply(app, req)
	app.Revision++
	app.Revisions = append(app.Revisions, types.AppRevision{
		Number:    app.Revision,
		Spec:      app.Spec,
		Traits:    app.Traits,
		Policies:  app.Policies,
		CreatedAt: time.Now().UTC(),
	})
	app.UpdatedAt = time.Now().UTC()
	if err := s.store.UpdateApp(r.Context(), app); err != nil {
		writeError(w, http.StatusInternalServerError, "KN-500", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, app)
}

func (s *Server) changeAppStatus(w http.ResponseWriter, r *http.Request, status string, suspended bool) {
	clusterID := chi.URLParam(r, "clusterID")
	tenantID := chi.URLParam(r, "tenantID")
	projectID := chi.URLParam(r, "projectID")
	appID := chi.URLParam(r, "appID")
	app, err := s.store.GetApp(r.Context(), clusterID, tenantID, projectID, appID)
	if err != nil {
		writeError(w, http.StatusNotFound, "KN-404", "app not found")
		return
	}
	app.Status = status
	app.Suspended = suspended
	app.UpdatedAt = time.Now().UTC()
	if err := s.store.UpdateApp(r.Context(), app); err != nil {
		writeError(w, http.StatusInternalServerError, "KN-500", err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"status": status})
}

func (s *Server) authContext(ctx context.Context) *AuthContext {
	if v, ok := ctx.Value(authContextKey).(*AuthContext); ok && v != nil {
		return v
	}
	return &AuthContext{Subject: "anonymous", Roles: []string{}}
}

func (s *Server) telemetryEvent(w http.ResponseWriter, r *http.Request) {
	var ev TelemetryEvent
	if err := decodeJSON(r, &ev); err != nil {
		writeError(w, http.StatusBadRequest, "KN-400", err.Error())
		return
	}
	logging.L.Info("telemetry_event_received",
		zap.String("stream", ev.Stream),
		zap.String("component", ev.Component),
		zap.String("status", ev.Status),
		zap.String("error", ev.Error),
		zap.String("cluster_id", ev.ClusterID),
	)
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "received"})
}

// Request DTOs
type TokenRequest struct {
	Subject    string   `json:"subject"`
	Roles      []string `json:"roles"`
	TTLMinutes int      `json:"ttlMinutes"`
}

type TokenResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type TelemetryEvent struct {
	Stream    string `json:"stream"`
	Event     string `json:"event"`
	Component string `json:"component"`
	Status    string `json:"status"`
	Error     string `json:"error"`
	ClusterID string `json:"clusterId"`
}

type ClusterRequest struct {
	Name                 string            `json:"name"`
	Datacenter           string            `json:"datacenter"`
	Kubeconfig           string            `json:"kubeconfig"`
	Labels               map[string]string `json:"labels"`
	CapsuleProxyEndpoint string            `json:"capsuleProxyEndpoint"`
}

type TenantRequest struct {
	Name            string            `json:"name"`
	Owners          []string          `json:"owners"`
	Plan            string            `json:"plan"`
	Labels          map[string]string `json:"labels"`
	Quotas          map[string]string `json:"quotas"`
	Limits          map[string]string `json:"limits"`
	NetworkPolicies []string          `json:"networkPolicies"`
}

type OwnersRequest struct {
	Owners []string `json:"owners"`
}

type NetworkPolicyRequest struct {
	Policies []string `json:"policies"`
}

type ProjectRequest struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Labels      map[string]string `json:"labels"`
	Access      []string          `json:"access"`
}

type AccessRequest struct {
	Access []string `json:"access"`
}

type AppRequest struct {
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Component   string           `json:"component"`
	Image       string           `json:"image"`
	Spec        map[string]any   `json:"spec"`
	Traits      []map[string]any `json:"traits"`
	Policies    []map[string]any `json:"policies"`
}

type WorkflowRequest struct {
	Inputs map[string]any `json:"inputs"`
}

// Helpers
func decodeJSON(r *http.Request, v any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(io.LimitReader(r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return err
	}
	if dec.More() {
		return errors.New("unexpected trailing data")
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, map[string]string{
		"code":    code,
		"message": msg,
	})
}

func findRevision(revs []types.AppRevision, number string) *types.AppRevision {
	for _, r := range revs {
		if fmt.Sprintf("%d", r.Number) == number {
			cp := r
			return &cp
		}
	}
	return nil
}

func parseBool(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "true", "1", "yes", "on", "y", "t":
		return true
	default:
		return false
	}
}

func (s *Server) findTenant(ctx context.Context, tenantID string) *types.Tenant {
	tenants, _ := s.store.ListTenants(ctx, "")
	for _, t := range tenants {
		if t.ID == tenantID {
			return t
		}
	}
	return nil
}

func (s *Server) findProject(ctx context.Context, projectID string) *types.Project {
	projects, _ := s.store.ListProjects(ctx, "", "")
	for _, p := range projects {
		if p.ID == projectID {
			return p
		}
	}
	return nil
}

func (s *Server) clusterProxyBase(ctx context.Context, clusterID string) string {
	base := strings.TrimRight(defaultCapsuleProxy, "/")
	if clusterID == "" {
		return base
	}
	c, err := s.store.GetCluster(ctx, clusterID)
	if err != nil || c == nil {
		return base
	}
	if v := strings.TrimRight(strings.TrimSpace(c.CapsuleProxyEndpoint), "/"); v != "" {
		return v
	}
	return base
}

func sanitizeCluster(c *types.Cluster) *types.Cluster {
	if c == nil {
		return nil
	}
	cp := *c
	cp.Kubeconfig = ""
	return &cp
}

func sanitizeClusters(list []*types.Cluster) []*types.Cluster {
	out := make([]*types.Cluster, 0, len(list))
	for _, c := range list {
		out = append(out, sanitizeCluster(c))
	}
	return out
}

func normalizeKubeconfig(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return trimmed
	}
	compact := strings.ReplaceAll(strings.ReplaceAll(trimmed, "\n", ""), "\r", "")
	if decoded, err := base64.StdEncoding.DecodeString(compact); err == nil && len(decoded) > 0 {
		str := string(decoded)
		if strings.Contains(str, "apiVersion") && strings.Contains(str, "clusters") {
			return str
		}
	}
	return trimmed
}

func defaultKubeClientFactory() kubeClientFactory {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(v1alpha1.AddToScheme(scheme))
	return func(ctx context.Context, kubeconfig string) (ctrlclient.Client, error) {
		cfg, err := clientcmd.RESTConfigFromKubeConfig([]byte(kubeconfig))
		if err != nil {
			return nil, fmt.Errorf("build rest config: %w", err)
		}
		return ctrlclient.New(cfg, ctrlclient.Options{Scheme: scheme})
	}
}

func (s *Server) kubeClientForCluster(ctx context.Context, c *types.Cluster) (ctrlclient.Client, error) {
	if c == nil {
		return nil, errors.New("cluster is nil")
	}
	if c.Kubeconfig == "" {
		return nil, errors.New("kubeconfig missing")
	}
	if s.kubeFactory == nil {
		s.kubeFactory = defaultKubeClientFactory()
	}
	return s.kubeFactory(ctx, c.Kubeconfig)
}

func (s *Server) syncTenant(ctx context.Context, tenant *types.Tenant) error {
	return s.syncTenantWithCluster(ctx, nil, tenant)
}

func (s *Server) syncTenantWithCluster(ctx context.Context, cluster *types.Cluster, tenant *types.Tenant) error {
	if tenant == nil {
		return errors.New("tenant is nil")
	}
	var err error
	if cluster == nil {
		cluster, err = s.store.GetCluster(ctx, tenant.ClusterID)
		if err != nil {
			return err
		}
	}
	cli, err := s.kubeClientForCluster(ctx, cluster)
	if err != nil {
		return err
	}
	return upsertNovaTenant(ctx, cli, tenant)
}

func (s *Server) syncProject(ctx context.Context, project *types.Project, tenant *types.Tenant) error {
	return s.syncProjectWithCluster(ctx, nil, tenant, project)
}

func (s *Server) syncProjectWithCluster(ctx context.Context, cluster *types.Cluster, tenant *types.Tenant, project *types.Project) error {
	if project == nil {
		return errors.New("project is nil")
	}
	var err error
	if tenant == nil {
		tenant, err = s.store.GetTenant(ctx, project.ClusterID, project.TenantID)
		if err != nil {
			return err
		}
	}
	if cluster == nil {
		cluster, err = s.store.GetCluster(ctx, project.ClusterID)
		if err != nil {
			return err
		}
	}
	cli, err := s.kubeClientForCluster(ctx, cluster)
	if err != nil {
		return err
	}
	return upsertNovaProject(ctx, cli, tenant.Name, project)
}

func (s *Server) deleteTenantResource(ctx context.Context, tenant *types.Tenant) error {
	if tenant == nil {
		return errors.New("tenant is nil")
	}
	cluster, err := s.store.GetCluster(ctx, tenant.ClusterID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil
		}
		return err
	}
	cli, err := s.kubeClientForCluster(ctx, cluster)
	if err != nil {
		return err
	}
	obj := &v1alpha1.NovaTenant{ObjectMeta: metav1.ObjectMeta{Name: tenant.Name}}
	if err := cli.Delete(ctx, obj); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	return nil
}

func (s *Server) deleteProjectResource(ctx context.Context, project *types.Project) error {
	if project == nil {
		return errors.New("project is nil")
	}
	cluster, err := s.store.GetCluster(ctx, project.ClusterID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil
		}
		return err
	}
	cli, err := s.kubeClientForCluster(ctx, cluster)
	if err != nil {
		return err
	}
	obj := &v1alpha1.NovaProject{ObjectMeta: metav1.ObjectMeta{Name: project.Name}}
	if err := cli.Delete(ctx, obj); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	return nil
}

func upsertNovaTenant(ctx context.Context, cli ctrlclient.Client, t *types.Tenant) error {
	if cli == nil {
		return errors.New("kube client is nil")
	}
	if t == nil {
		return errors.New("tenant is nil")
	}
	spec := v1alpha1.NovaTenantSpec{
		Owners:          t.Owners,
		Plan:            t.Plan,
		Labels:          t.Labels,
		OwnerNamespace:  t.OwnerNamespace,
		AppsNamespace:   t.AppsNamespace,
		NetworkPolicies: t.NetworkPolicies,
		Quotas:          t.Quotas,
		Limits:          t.Limits,
	}
	current := &v1alpha1.NovaTenant{}
	err := cli.Get(ctx, ctrlclient.ObjectKey{Name: t.Name}, current)
	if apierrors.IsNotFound(err) {
		return cli.Create(ctx, &v1alpha1.NovaTenant{
			TypeMeta: metav1.TypeMeta{
				APIVersion: v1alpha1.GroupVersion.String(),
				Kind:       "NovaTenant",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:   t.Name,
				Labels: t.Labels,
			},
			Spec: spec,
		})
	}
	if err != nil {
		return err
	}
	current.Spec = spec
	current.Labels = t.Labels
	return cli.Update(ctx, current)
}

func upsertNovaProject(ctx context.Context, cli ctrlclient.Client, tenantName string, project *types.Project) error {
	if cli == nil {
		return errors.New("kube client is nil")
	}
	if project == nil {
		return errors.New("project is nil")
	}
	if tenantName == "" {
		return errors.New("tenant name is required")
	}
	spec := v1alpha1.NovaProjectSpec{
		Tenant:      tenantName,
		Description: project.Description,
		Labels:      project.Labels,
		Access:      project.Access,
	}
	current := &v1alpha1.NovaProject{}
	err := cli.Get(ctx, ctrlclient.ObjectKey{Name: project.Name}, current)
	if apierrors.IsNotFound(err) {
		return cli.Create(ctx, &v1alpha1.NovaProject{
			TypeMeta: metav1.TypeMeta{
				APIVersion: v1alpha1.GroupVersion.String(),
				Kind:       "NovaProject",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:   project.Name,
				Labels: project.Labels,
			},
			Spec: spec,
		})
	}
	if err != nil {
		return err
	}
	current.Spec = spec
	current.Labels = project.Labels
	return cli.Update(ctx, current)
}

func (s *Server) installOperator(ctx context.Context, c *types.Cluster) error {
	if c.Kubeconfig == "" {
		return errors.New("kubeconfig missing")
	}
	cfg, err := clientcmd.RESTConfigFromKubeConfig([]byte(c.Kubeconfig))
	if err != nil {
		return fmt.Errorf("parse kubeconfig: %w", err)
	}
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	cli, err := ctrlclient.New(cfg, ctrlclient.Options{Scheme: scheme})
	if err != nil {
		return fmt.Errorf("build client: %w", err)
	}
	installer := cluster.NewInstaller(cli, scheme, []byte(c.Kubeconfig), nil, false)
	if err := installer.Bootstrap(ctx, "operator"); err != nil {
		return err
	}
	return nil
}

func (s *Server) requireRole(w http.ResponseWriter, r *http.Request, allowed ...string) bool {
	if !s.requireAuth {
		return true
	}
	auth := s.authContext(r.Context())
	for _, role := range auth.Roles {
		for _, allowedRole := range allowed {
			if role == allowedRole {
				return true
			}
		}
	}
	writeError(w, http.StatusForbidden, "KN-403", "forbidden")
	return false
}
