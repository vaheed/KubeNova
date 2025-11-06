package httpapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	capib "github.com/vaheed/kubenova/internal/backends/capsule"
	velab "github.com/vaheed/kubenova/internal/backends/vela"
	clusterpkg "github.com/vaheed/kubenova/internal/cluster"
	"github.com/vaheed/kubenova/internal/lib/httperr"
	"github.com/vaheed/kubenova/internal/store"
	kn "github.com/vaheed/kubenova/pkg/types"
)

// APIServer implements a subset of the contract (Clusters + Tenants) and embeds
// Unimplemented for the rest of the surface.
type APIServer struct {
	Unimplemented
	st          store.Store
	requireAuth bool
	jwtKey      []byte
	newCapsule  func([]byte) capib.Client
	newVela     func([]byte) interface {
		Deploy(context.Context, string, string) error
		Suspend(context.Context, string, string) error
		Resume(context.Context, string, string) error
		Rollback(context.Context, string, string, *int) error
		Status(context.Context, string, string) (map[string]any, error)
		Revisions(context.Context, string, string) ([]map[string]any, error)
		Diff(context.Context, string, string, int, int) (map[string]any, error)
		Logs(context.Context, string, string, string, bool) ([]map[string]any, error)
		SetTraits(context.Context, string, string, []map[string]any) error
		SetPolicies(context.Context, string, string, []map[string]any) error
		ImageUpdate(context.Context, string, string, string, string, string) error
	}
	psMu       sync.RWMutex
	policysets map[string]map[string]PolicySet // tenantUID -> name -> item
	runsMu     sync.RWMutex
	runsByID   map[string]WorkflowRun
	runsByApp  map[string][]WorkflowRun // key: tenantUID|projectUID|appUID
}

func NewAPIServer(st store.Store) *APIServer {
	return &APIServer{
		st:          st,
		requireAuth: parseBool(os.Getenv("KUBENOVA_REQUIRE_AUTH")),
		jwtKey:      []byte(os.Getenv("JWT_SIGNING_KEY")),
		newCapsule:  capib.New,
		newVela: func(b []byte) interface {
			Deploy(context.Context, string, string) error
			Suspend(context.Context, string, string) error
			Resume(context.Context, string, string) error
			Rollback(context.Context, string, string, *int) error
			Status(context.Context, string, string) (map[string]any, error)
			Revisions(context.Context, string, string) ([]map[string]any, error)
			Diff(context.Context, string, string, int, int) (map[string]any, error)
			Logs(context.Context, string, string, string, bool) ([]map[string]any, error)
			SetTraits(context.Context, string, string, []map[string]any) error
			SetPolicies(context.Context, string, string, []map[string]any) error
			ImageUpdate(context.Context, string, string, string, string, string) error
		} {
			return velab.New(b)
		},
		policysets: map[string]map[string]PolicySet{},
		runsByID:   map[string]WorkflowRun{},
		runsByApp:  map[string][]WorkflowRun{},
	}
}

// --- helpers ---
func parseBool(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "t", "true", "y", "yes", "on":
		return true
	default:
		return false
	}
}

func (s *APIServer) writeError(w http.ResponseWriter, status int, code, msg string) {
	httperr.Write(w, status, code, msg)
}

// (GET /api/v1/healthz)
func (s *APIServer) GetApiV1Healthz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

// (GET /api/v1/readyz)
func (s *APIServer) GetApiV1Readyz(w http.ResponseWriter, r *http.Request) {
	// For now, return 200 when the store is usable
	if _, err := s.st.ListTenants(r.Context()); err != nil {
		s.writeError(w, http.StatusServiceUnavailable, "KN-500", "store not ready")
		return
	}
	w.WriteHeader(http.StatusOK)
}

// (GET /api/v1/catalog/components)
func (s *APIServer) GetApiV1CatalogComponents(w http.ResponseWriter, r *http.Request) {
	if !s.requireRoles(w, r, "admin", "ops", "tenantOwner", "projectDev", "readOnly") {
		return
	}
	t := Component
	name := "web"
	desc := "Web service"
	items := []CatalogItem{{Name: &name, Type: &t, Description: &desc}}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(items)
}

// (GET /api/v1/catalog/traits)
func (s *APIServer) GetApiV1CatalogTraits(w http.ResponseWriter, r *http.Request) {
	if !s.requireRoles(w, r, "admin", "ops", "tenantOwner", "projectDev", "readOnly") {
		return
	}
	t := Trait
	name := "scaler"
	desc := "Scale deployments"
	items := []CatalogItem{{Name: &name, Type: &t, Description: &desc}}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(items)
}

// (GET /api/v1/catalog/workflows)
func (s *APIServer) GetApiV1CatalogWorkflows(w http.ResponseWriter, r *http.Request) {
	if !s.requireRoles(w, r, "admin", "ops", "tenantOwner", "projectDev", "readOnly") {
		return
	}
	t := Workflow
	name := "rollout"
	desc := "Rolling updates"
	items := []CatalogItem{{Name: &name, Type: &t, Description: &desc}}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(items)
}

func (s *APIServer) requireRoles(w http.ResponseWriter, r *http.Request, allowed ...string) bool {
	if !s.requireAuth {
		return true
	}
	hdr := r.Header.Get("Authorization")
	if hdr == "" || !strings.HasPrefix(strings.ToLower(hdr), "bearer ") {
		s.writeError(w, http.StatusUnauthorized, "KN-401", "missing bearer token")
		return false
	}
	tok := strings.TrimSpace(strings.TrimPrefix(hdr, "Bearer"))
	roles := s.rolesFromToken(tok)
	// Fallback to X-KN-Roles header for tests/dev
	if len(roles) == 0 {
		rolesHdr := r.Header.Get("X-KN-Roles")
		if rolesHdr != "" {
			roles = strings.Split(rolesHdr, ",")
		}
	}
	// Allow when allowed contains "*"
	if len(allowed) == 0 || (len(allowed) == 1 && allowed[0] == "*") {
		return true
	}
	have := map[string]struct{}{}
	if len(roles) == 0 {
		roles = []string{"readOnly"}
	}
	for _, p := range roles {
		have[strings.TrimSpace(p)] = struct{}{}
	}
	for _, want := range allowed {
		if _, ok := have[want]; ok {
			return true
		}
	}
	s.writeError(w, http.StatusForbidden, "KN-403", "forbidden")
	return false
}

func (s *APIServer) rolesFromReq(r *http.Request) []string {
	hdr := r.Header.Get("Authorization")
	if hdr != "" && strings.HasPrefix(strings.ToLower(hdr), "bearer ") {
		tok := strings.TrimSpace(strings.TrimPrefix(hdr, "Bearer"))
		if rs := s.rolesFromToken(tok); len(rs) > 0 {
			return rs
		}
	}
	if v := r.Header.Get("X-KN-Roles"); v != "" {
		return strings.Split(v, ",")
	}
	return nil
}

func (s *APIServer) tenantFromReq(r *http.Request) string {
	hdr := r.Header.Get("Authorization")
	if hdr != "" && strings.HasPrefix(strings.ToLower(hdr), "bearer ") {
		tok := strings.TrimSpace(strings.TrimPrefix(hdr, "Bearer"))
		var claims jwt.MapClaims
		if _, err := jwt.ParseWithClaims(tok, &claims, func(token *jwt.Token) (interface{}, error) { return s.jwtKey, nil }); err == nil {
			if t, ok := claims["tenant"].(string); ok {
				return t
			}
		}
	}
	if v := r.Header.Get("X-KN-Tenant"); v != "" {
		return v
	}
	return ""
}

func (s *APIServer) requireRolesTenant(w http.ResponseWriter, r *http.Request, tenant string, allowed ...string) bool {
	if !s.requireAuth {
		return true
	}
	if !s.requireRoles(w, r, allowed...) {
		return false
	}
	roles := s.rolesFromReq(r)
	// admin/ops are cluster-scoped
	for _, ro := range roles {
		if ro == "admin" || ro == "ops" {
			return true
		}
	}
	// tenant-scoped roles must match tenant
	if tenant == "" {
		s.writeError(w, http.StatusForbidden, "KN-403", "tenant scope required")
		return false
	}
	t := s.tenantFromReq(r)
	if t == tenant {
		return true
	}
	s.writeError(w, http.StatusForbidden, "KN-403", "forbidden: tenant scope mismatch")
	return false
}

func (s *APIServer) rolesFromToken(tok string) []string {
	if tok == "" {
		return nil
	}
	var claims jwt.MapClaims
	_, err := jwt.ParseWithClaims(tok, &claims, func(token *jwt.Token) (interface{}, error) { return s.jwtKey, nil })
	if err != nil {
		return nil
	}
	if v, ok := claims["roles"]; ok {
		switch arr := v.(type) {
		case []any:
			out := make([]string, 0, len(arr))
			for _, it := range arr {
				if s, ok := it.(string); ok {
					out = append(out, s)
				}
			}
			return out
		case []string:
			return arr
		}
	}
	if v, ok := claims["role"].(string); ok && v != "" {
		return []string{v}
	}
	return nil
}

// --- Clusters ---

// (POST /api/v1/clusters)
func (s *APIServer) PostApiV1Clusters(w http.ResponseWriter, r *http.Request) {
	if !s.requireRoles(w, r, "admin", "ops") {
		return
	}
	var in ClusterRegistration
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		s.writeError(w, http.StatusUnprocessableEntity, "KN-422", "invalid payload")
		return
	}
	if strings.TrimSpace(in.Name) == "" || len(in.Kubeconfig) == 0 {
		s.writeError(w, http.StatusUnprocessableEntity, "KN-422", "name and kubeconfig required")
		return
	}
	// Store encoded kubeconfig
	enc := base64.StdEncoding.EncodeToString(in.Kubeconfig)
	// Persist via store (id is opaque; not returned on API)
	_, err := s.st.CreateCluster(r.Context(), toTypesCluster(in), enc)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "KN-500", err.Error())
		return
	}
	now := time.Now().UTC()
	out := Cluster{Name: in.Name, CreatedAt: &now}
	if in.Labels != nil {
		out.Labels = in.Labels
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

// (GET /api/v1/clusters)
func (s *APIServer) GetApiV1Clusters(w http.ResponseWriter, r *http.Request, params GetApiV1ClustersParams) {
	if !s.requireRoles(w, r, "admin", "ops", "tenantOwner", "projectDev", "readOnly") {
		return
	}
	lim := 50
	if params.Limit != nil {
		lim = int(*params.Limit)
	}
	cursor := ""
	if params.Cursor != nil {
		cursor = string(*params.Cursor)
	}
	sel := ""
	if params.LabelSelector != nil {
		sel = string(*params.LabelSelector)
	}
	items, next, err := s.st.ListClusters(r.Context(), lim, cursor, sel)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "KN-500", err.Error())
		return
	}
	out := make([]Cluster, 0, len(items))
	for _, it := range items {
		dto := Cluster{Name: it.Name, CreatedAt: &it.CreatedAt}
		if it.UID != "" {
			u := it.UID
			dto.Uid = &u
		}
		out = append(out, dto)
	}
	if next != "" {
		w.Header().Set("X-Next-Cursor", next)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

// (GET /api/v1/clusters/{c})
func (s *APIServer) GetApiV1ClustersC(w http.ResponseWriter, r *http.Request, c ClusterParam) {
	if !s.requireRoles(w, r, "admin", "ops", "tenantOwner", "projectDev", "readOnly") {
		return
	}
	uid := string(c)
	cl, enc, err := s.st.GetClusterByUID(r.Context(), uid)
	if err != nil {
		s.writeError(w, http.StatusNotFound, "KN-404", "not found")
		return
	}
	kb, _ := base64.StdEncoding.DecodeString(enc)
	conds := clusterpkg.ComputeClusterConditions(r.Context(), kb, parseBool(os.Getenv("KUBENOVA_E2E_FAKE")))
	// map to DTO
	out := Cluster{Name: cl.Name}
	outConds := make([]Condition, 0, len(conds))
	for _, x := range conds {
		typ := x.Type
		st := ConditionStatus(x.Status)
		t := x.LastTransitionTime
		reason := x.Reason
		message := x.Message
		outConds = append(outConds, Condition{Type: &typ, Status: &st, LastTransitionTime: &t, Reason: &reason, Message: &message})
	}
	if len(outConds) > 0 {
		out.Conditions = &outConds
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

// (DELETE /api/v1/clusters/{c})
func (s *APIServer) DeleteApiV1ClustersC(w http.ResponseWriter, r *http.Request, c ClusterParam) {
	if !s.requireRoles(w, r, "admin", "ops") {
		return
	}
	ident := string(c)
	// Require UUID id in path
	id, err := kn.ParseID(ident)
	if err != nil {
		s.writeError(w, http.StatusUnprocessableEntity, "KN-422", "invalid UUID")
		return
	}
	cl, enc, err := s.st.GetCluster(r.Context(), id)
	if err != nil {
		s.writeError(w, http.StatusNotFound, "KN-404", "not found")
		return
	}
	// Attempt to uninstall agent and related resources from target cluster
	if enc != "" {
		kb, _ := base64.StdEncoding.DecodeString(enc)
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
		defer cancel()
		if err := clusterpkg.UninstallAgent(ctx, kb); err != nil {
			s.writeError(w, http.StatusInternalServerError, "KN-500", "failed to remove cluster dependencies")
			return
		}
	}
	if err := s.st.DeleteCluster(r.Context(), cl.ID); err != nil {
		s.writeError(w, http.StatusInternalServerError, "KN-500", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// (GET /api/v1/clusters/{c}/capabilities)
func (s *APIServer) GetApiV1ClustersCCapabilities(w http.ResponseWriter, r *http.Request, c ClusterParam) {
	if !s.requireRoles(w, r, "admin", "ops", "tenantOwner", "projectDev", "readOnly") {
		return
	}
	t, v, p := true, true, true
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(ClusterCapabilities{Tenancy: &t, Vela: &v, Proxy: &p})
}

// --- PolicySets & Catalog ---

// (GET /api/v1/clusters/{c}/policysets/catalog)
func (s *APIServer) GetApiV1ClustersCPolicysetsCatalog(w http.ResponseWriter, r *http.Request, c ClusterParam) {
	if !s.requireRoles(w, r, "admin", "ops", "tenantOwner", "projectDev", "readOnly") {
		return
	}
	desc := "Base guardrails"
	items := []PolicySet{{Name: "baseline", Description: &desc}}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(items)
}

// (GET /api/v1/clusters/{c}/tenants/{t}/policysets)
func (s *APIServer) GetApiV1ClustersCTenantsTPolicysets(w http.ResponseWriter, r *http.Request, c ClusterParam, t TenantParam) {
	ten, err := s.st.GetTenantByUID(r.Context(), string(t))
	if err != nil {
		s.writeError(w, http.StatusNotFound, "KN-404", "not found")
		return
	}
	if !s.requireRolesTenant(w, r, ten.Name, "admin", "ops", "tenantOwner", "projectDev", "readOnly") {
		return
	}
	s.psMu.RLock()
	defer s.psMu.RUnlock()
	psMap := s.policysets[ten.UID]
	out := []PolicySet{}
	for _, v := range psMap {
		vv := v
		out = append(out, vv)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

// (POST /api/v1/clusters/{c}/tenants/{t}/policysets)
func (s *APIServer) PostApiV1ClustersCTenantsTPolicysets(w http.ResponseWriter, r *http.Request, c ClusterParam, t TenantParam) {
	ten, err := s.st.GetTenantByUID(r.Context(), string(t))
	if err != nil {
		s.writeError(w, http.StatusNotFound, "KN-404", "not found")
		return
	}
	if !s.requireRolesTenant(w, r, ten.Name, "admin", "ops", "tenantOwner") {
		return
	}
	var body PolicySet
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.writeError(w, http.StatusUnprocessableEntity, "KN-422", "invalid payload")
		return
	}
	if strings.TrimSpace(body.Name) == "" {
		s.writeError(w, http.StatusUnprocessableEntity, "KN-422", "invalid payload")
		return
	}
	s.psMu.Lock()
	if s.policysets[ten.UID] == nil {
		s.policysets[ten.UID] = map[string]PolicySet{}
	}
	s.policysets[ten.UID][body.Name] = body
	s.psMu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(body)
}

// (GET /api/v1/clusters/{c}/tenants/{t}/policysets/{name})
func (s *APIServer) GetApiV1ClustersCTenantsTPolicysetsName(w http.ResponseWriter, r *http.Request, c ClusterParam, t TenantParam, name string) {
	ten, err := s.st.GetTenantByUID(r.Context(), string(t))
	if err != nil {
		s.writeError(w, http.StatusNotFound, "KN-404", "not found")
		return
	}
	if !s.requireRolesTenant(w, r, ten.Name, "admin", "ops", "tenantOwner", "projectDev", "readOnly") {
		return
	}
	s.psMu.RLock()
	defer s.psMu.RUnlock()
	if m := s.policysets[ten.UID]; m != nil {
		if v, ok := m[name]; ok {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(v)
			return
		}
	}
	s.writeError(w, http.StatusNotFound, "KN-404", "not found")
}

// (PUT /api/v1/clusters/{c}/tenants/{t}/policysets/{name})
func (s *APIServer) PutApiV1ClustersCTenantsTPolicysetsName(w http.ResponseWriter, r *http.Request, c ClusterParam, t TenantParam, name string) {
	ten, err := s.st.GetTenantByUID(r.Context(), string(t))
	if err != nil {
		s.writeError(w, http.StatusNotFound, "KN-404", "not found")
		return
	}
	if !s.requireRolesTenant(w, r, ten.Name, "admin", "ops", "tenantOwner") {
		return
	}
	var body PolicySet
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.writeError(w, http.StatusUnprocessableEntity, "KN-422", "invalid payload")
		return
	}
	s.psMu.Lock()
	defer s.psMu.Unlock()
	if s.policysets[ten.UID] == nil {
		s.policysets[ten.UID] = map[string]PolicySet{}
	}
	s.policysets[ten.UID][name] = body
	w.WriteHeader(http.StatusOK)
}

// (DELETE /api/v1/clusters/{c}/tenants/{t}/policysets/{name})
func (s *APIServer) DeleteApiV1ClustersCTenantsTPolicysetsName(w http.ResponseWriter, r *http.Request, c ClusterParam, t TenantParam, name string) {
	ten, err := s.st.GetTenantByUID(r.Context(), string(t))
	if err != nil {
		s.writeError(w, http.StatusNotFound, "KN-404", "not found")
		return
	}
	if !s.requireRolesTenant(w, r, ten.Name, "admin", "ops", "tenantOwner") {
		return
	}
	s.psMu.Lock()
	if m := s.policysets[ten.UID]; m != nil {
		delete(m, name)
	}
	s.psMu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

// --- Bootstrap ---

// (POST /api/v1/clusters/{c}/bootstrap/{component})
func (s *APIServer) PostApiV1ClustersCBootstrapComponent(w http.ResponseWriter, r *http.Request, c ClusterParam, component string) {
	if !s.requireRoles(w, r, "admin", "ops") {
		return
	}
	// Validate cluster exists
	if _, _, err := s.st.GetClusterByUID(r.Context(), string(c)); err != nil {
		s.writeError(w, http.StatusNotFound, "KN-404", "cluster not found")
		return
	}
	switch component {
	case "tenancy", "proxy", "app-delivery":
		w.WriteHeader(http.StatusAccepted)
	default:
		s.writeError(w, http.StatusUnprocessableEntity, "KN-422", "invalid payload")
	}
}

// --- Project/Tenant kubeconfig ---

// (GET /api/v1/clusters/{c}/tenants/{t}/projects/{p}/kubeconfig)
func (s *APIServer) GetApiV1ClustersCTenantsTProjectsPKubeconfig(w http.ResponseWriter, r *http.Request, c ClusterParam, t TenantParam, p ProjectParam) {
	// resolve cluster
	_, enc, err := s.st.GetClusterByUID(r.Context(), string(c))
	if err != nil {
		s.writeError(w, http.StatusNotFound, "KN-404", "cluster not found")
		return
	}
	// ensure tenant/project exist (uid-based lookups)
	if _, err := s.st.GetTenantByUID(r.Context(), string(t)); err != nil {
		s.writeError(w, http.StatusNotFound, "KN-404", "not found")
		return
	}
	if _, err := s.st.GetProjectByUID(r.Context(), string(p)); err != nil {
		s.writeError(w, http.StatusNotFound, "KN-404", "not found")
		return
	}
	kb, _ := base64.StdEncoding.DecodeString(enc)
	exp := time.Now().UTC().Add(time.Hour)
	resp := KubeconfigResponse{Kubeconfig: &kb, ExpiresAt: &exp}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// (POST /api/v1/tenants/{t}/kubeconfig)
func (s *APIServer) PostApiV1TenantsTKubeconfig(w http.ResponseWriter, r *http.Request, t TenantParam) {
	// best-effort: return short-lived minimal kubeconfig
	if _, err := s.st.GetTenantByUID(r.Context(), string(t)); err != nil {
		s.writeError(w, http.StatusNotFound, "KN-404", "not found")
		return
	}
	raw := []byte("apiVersion: v1\nclusters: []\ncontexts: []\nusers: []\n")
	exp := time.Now().UTC().Add(time.Hour)
	resp := KubeconfigResponse{Kubeconfig: &raw, ExpiresAt: &exp}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// --- Usage ---

// (GET /api/v1/tenants/{t}/usage)
func (s *APIServer) GetApiV1TenantsTUsage(w http.ResponseWriter, r *http.Request, t TenantParam, params GetApiV1TenantsTUsageParams) {
	if _, err := s.st.GetTenantByUID(r.Context(), string(t)); err != nil {
		s.writeError(w, http.StatusNotFound, "KN-404", "not found")
		return
	}
	window := "24h"
	if params.Range != nil {
		window = string(*params.Range)
	}
	resp := UsageReport{Window: &window}
	cpu, mem := "2", "4Gi"
	pods := 12
	resp.Cpu = &cpu
	resp.Memory = &mem
	resp.Pods = &pods
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// (GET /api/v1/projects/{p}/usage)
func (s *APIServer) GetApiV1ProjectsPUsage(w http.ResponseWriter, r *http.Request, p ProjectParam, params GetApiV1ProjectsPUsageParams) {
	if _, err := s.st.GetProjectByUID(r.Context(), string(p)); err != nil {
		s.writeError(w, http.StatusNotFound, "KN-404", "not found")
		return
	}
	window := "24h"
	if params.Range != nil {
		window = string(*params.Range)
	}
	resp := UsageReport{Window: &window}
	cpu, mem := "1", "1Gi"
	pods := 5
	resp.Cpu = &cpu
	resp.Memory = &mem
	resp.Pods = &pods
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// --- Workflows ---

// (POST /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}/workflow/run)
func (s *APIServer) PostApiV1ClustersCTenantsTProjectsPAppsAWorkflowRun(w http.ResponseWriter, r *http.Request, c ClusterParam, t TenantParam, p ProjectParam, a AppParam) {
	id := "run-" + strings.ReplaceAll(time.Now().UTC().Format("20060102T150405.000000000"), ".", "")
	now := time.Now().UTC()
	run := WorkflowRun{Id: &id, Status: ptrWorkflowRunStatus(WorkflowRunStatusRunning), StartedAt: &now}
	key := string(t) + "|" + string(p) + "|" + string(a)
	s.runsMu.Lock()
	s.runsByID[id] = run
	s.runsByApp[key] = append(s.runsByApp[key], run)
	s.runsMu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(run)
}

// (GET /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}/workflow/runs)
func (s *APIServer) GetApiV1ClustersCTenantsTProjectsPAppsAWorkflowRuns(w http.ResponseWriter, r *http.Request, c ClusterParam, t TenantParam, p ProjectParam, a AppParam, params GetApiV1ClustersCTenantsTProjectsPAppsAWorkflowRunsParams) {
	key := string(t) + "|" + string(p) + "|" + string(a)
	s.runsMu.RLock()
	out := s.runsByApp[key]
	s.runsMu.RUnlock()
	if out == nil {
		out = []WorkflowRun{}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

// (GET /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/runs/{id})
func (s *APIServer) GetApiV1ClustersCTenantsTProjectsPAppsRunsId(w http.ResponseWriter, r *http.Request, c ClusterParam, t TenantParam, p ProjectParam, id string) {
	s.runsMu.RLock()
	run, ok := s.runsByID[id]
	s.runsMu.RUnlock()
	if !ok {
		s.writeError(w, http.StatusNotFound, "KN-404", "not found")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(run)
}

// (POST /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}:delete)
func (s *APIServer) PostApiV1ClustersCTenantsTProjectsPAppsADelete(w http.ResponseWriter, r *http.Request, c ClusterParam, t TenantParam, p ProjectParam, a AppParam) {
	if !s.requireRolesTenant(w, r, string(t), "admin", "ops", "tenantOwner", "projectDev") {
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

// helpers
func ptrWorkflowRunStatus(s WorkflowRunStatus) *WorkflowRunStatus { return &s }

// --- Tenants ---

// (GET /api/v1/clusters/{c}/tenants)
func (s *APIServer) GetApiV1ClustersCTenants(w http.ResponseWriter, r *http.Request, c ClusterParam, params GetApiV1ClustersCTenantsParams) {
	if !s.requireRoles(w, r, "admin", "ops", "tenantOwner", "projectDev", "readOnly") {
		return
	}
	items, err := s.st.ListTenants(r.Context())
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "KN-500", err.Error())
		return
	}
	out := make([]Tenant, 0, len(items))
	for _, t := range items {
		tn := Tenant{Name: t.Name, Labels: &t.Labels, Annotations: &t.Annotations}
		if t.UID != "" {
			u := t.UID
			tn.Uid = &u
		}
		out = append(out, tn)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

// (POST /api/v1/clusters/{c}/tenants)
func (s *APIServer) PostApiV1ClustersCTenants(w http.ResponseWriter, r *http.Request, c ClusterParam, params PostApiV1ClustersCTenantsParams) {
	if !s.requireRoles(w, r, "admin", "ops", "tenantOwner") {
		return
	}
	var in Tenant
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		s.writeError(w, http.StatusUnprocessableEntity, "KN-422", "invalid payload")
		return
	}
	if strings.TrimSpace(in.Name) == "" {
		s.writeError(w, http.StatusUnprocessableEntity, "KN-422", "name required")
		return
	}
	t := toTypesTenant(in)
	if err := s.st.CreateTenant(r.Context(), t); err != nil {
		s.writeError(w, http.StatusInternalServerError, "KN-500", err.Error())
		return
	}
	// read back to capture UID
	if t.Name != "" {
		if tt, e := s.st.GetTenant(r.Context(), t.Name); e == nil && tt.UID != "" {
			u := tt.UID
			in.Uid = &u
		}
	}
	now := time.Now().UTC()
	in.CreatedAt = &now
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(in)
}

// (GET /api/v1/clusters/{c}/tenants/{t})
func (s *APIServer) GetApiV1ClustersCTenantsT(w http.ResponseWriter, r *http.Request, c ClusterParam, t TenantParam) {
	item, err := s.st.GetTenantByUID(r.Context(), string(t))
	if err != nil {
		s.writeError(w, http.StatusNotFound, "KN-404", "not found")
		return
	}
	if !s.requireRolesTenant(w, r, item.Name, "admin", "ops", "tenantOwner", "projectDev", "readOnly") {
		return
	}
	out := Tenant{Name: item.Name, Labels: &item.Labels, Annotations: &item.Annotations}
	if item.UID != "" {
		u := item.UID
		out.Uid = &u
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

// (DELETE /api/v1/clusters/{c}/tenants/{t})
func (s *APIServer) DeleteApiV1ClustersCTenantsT(w http.ResponseWriter, r *http.Request, c ClusterParam, t TenantParam) {
	item, err := s.st.GetTenantByUID(r.Context(), string(t))
	if err != nil {
		s.writeError(w, http.StatusNotFound, "KN-404", "not found")
		return
	}
	if !s.requireRolesTenant(w, r, item.Name, "admin", "ops", "tenantOwner") {
		return
	}
	_ = s.st.DeleteTenant(r.Context(), item.Name)
	w.WriteHeader(http.StatusNoContent)
}

// (PUT /api/v1/clusters/{c}/tenants/{t}/owners)
func (s *APIServer) PutApiV1ClustersCTenantsTOwners(w http.ResponseWriter, r *http.Request, c ClusterParam, t TenantParam) {
	item, err := s.st.GetTenantByUID(r.Context(), string(t))
	if err != nil {
		s.writeError(w, http.StatusNotFound, "KN-404", "not found")
		return
	}
	if !s.requireRolesTenant(w, r, item.Name, "admin", "ops", "tenantOwner") {
		return
	}
	var body PutApiV1ClustersCTenantsTOwnersJSONBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.writeError(w, http.StatusUnprocessableEntity, "KN-422", "invalid payload")
		return
	}
	if body.Owners != nil {
		item.Owners = *body.Owners
	}
	_ = s.st.UpdateTenant(r.Context(), item)
	w.WriteHeader(http.StatusOK)
}

// (PUT /api/v1/clusters/{c}/tenants/{t}/quotas)
func (s *APIServer) PutApiV1ClustersCTenantsTQuotas(w http.ResponseWriter, r *http.Request, c ClusterParam, t TenantParam) {
	if !s.requireRolesTenant(w, r, string(t), "admin", "ops", "tenantOwner") {
		return
	}
	var body map[string]string
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.writeError(w, http.StatusUnprocessableEntity, "KN-422", "invalid payload")
		return
	}
	var kb []byte
	if _, enc, err := s.st.GetClusterByUID(r.Context(), string(c)); err == nil {
		kb, _ = base64.StdEncoding.DecodeString(enc)
	}
	if err := s.newCapsule(kb).SetTenantQuotas(r.Context(), string(t), body); err != nil {
		s.writeError(w, http.StatusInternalServerError, "KN-500", err.Error())
		return
	}
	w.WriteHeader(http.StatusOK)
}

// (PUT /api/v1/clusters/{c}/tenants/{t}/limits)
func (s *APIServer) PutApiV1ClustersCTenantsTLimits(w http.ResponseWriter, r *http.Request, c ClusterParam, t TenantParam) {
	if !s.requireRolesTenant(w, r, string(t), "admin", "ops", "tenantOwner") {
		return
	}
	var body map[string]string
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.writeError(w, http.StatusUnprocessableEntity, "KN-422", "invalid payload")
		return
	}
	var kb []byte
	if _, enc, err := s.st.GetClusterByUID(r.Context(), string(c)); err == nil {
		kb, _ = base64.StdEncoding.DecodeString(enc)
	}
	if err := s.newCapsule(kb).SetTenantLimits(r.Context(), string(t), body); err != nil {
		s.writeError(w, http.StatusInternalServerError, "KN-500", err.Error())
		return
	}
	w.WriteHeader(http.StatusOK)
}

// (PUT /api/v1/clusters/{c}/tenants/{t}/network-policies)
func (s *APIServer) PutApiV1ClustersCTenantsTNetworkPolicies(w http.ResponseWriter, r *http.Request, c ClusterParam, t TenantParam) {
	if !s.requireRolesTenant(w, r, string(t), "admin", "ops", "tenantOwner") {
		return
	}
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.writeError(w, http.StatusUnprocessableEntity, "KN-422", "invalid payload")
		return
	}
	var kb []byte
	if _, enc, err := s.st.GetClusterByUID(r.Context(), string(c)); err == nil {
		kb, _ = base64.StdEncoding.DecodeString(enc)
	}
	if err := s.newCapsule(kb).SetTenantNetworkPolicies(r.Context(), string(t), body); err != nil {
		s.writeError(w, http.StatusInternalServerError, "KN-500", err.Error())
		return
	}
	w.WriteHeader(http.StatusOK)
}

// (GET /api/v1/clusters/{c}/tenants/{t}/summary)
func (s *APIServer) GetApiV1ClustersCTenantsTSummary(w http.ResponseWriter, r *http.Request, c ClusterParam, t TenantParam) {
	if !s.requireRolesTenant(w, r, string(t), "admin", "ops", "tenantOwner", "projectDev", "readOnly") {
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(TenantSummary{})
}

// --- Projects ---

// (GET /api/v1/clusters/{c}/tenants/{t}/projects)
func (s *APIServer) GetApiV1ClustersCTenantsTProjects(w http.ResponseWriter, r *http.Request, c ClusterParam, t TenantParam) {
	ten, err := s.st.GetTenantByUID(r.Context(), string(t))
	if err != nil {
		s.writeError(w, http.StatusNotFound, "KN-404", "not found")
		return
	}
	if !s.requireRolesTenant(w, r, ten.Name, "admin", "ops", "tenantOwner", "projectDev", "readOnly") {
		return
	}
	items, err := s.st.ListProjects(r.Context(), ten.Name)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "KN-500", err.Error())
		return
	}
	out := make([]Project, 0, len(items))
	for _, p := range items {
		pc := p
		pr := Project{Name: pc.Name, CreatedAt: &pc.CreatedAt}
		if pc.UID != "" {
			u := pc.UID
			pr.Uid = &u
		}
		out = append(out, pr)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

// (POST /api/v1/clusters/{c}/tenants/{t}/projects)
func (s *APIServer) PostApiV1ClustersCTenantsTProjects(w http.ResponseWriter, r *http.Request, c ClusterParam, t TenantParam) {
	ten, err := s.st.GetTenantByUID(r.Context(), string(t))
	if err != nil {
		s.writeError(w, http.StatusNotFound, "KN-404", "not found")
		return
	}
	if !s.requireRolesTenant(w, r, ten.Name, "admin", "ops", "tenantOwner", "projectDev") {
		return
	}
	var in Project
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		s.writeError(w, http.StatusUnprocessableEntity, "KN-422", "invalid payload")
		return
	}
	if strings.TrimSpace(in.Name) == "" {
		s.writeError(w, http.StatusUnprocessableEntity, "KN-422", "name required")
		return
	}
	pr := kn.Project{Tenant: ten.Name, Name: in.Name, CreatedAt: time.Now().UTC()}
	if err := s.st.CreateProject(r.Context(), pr); err != nil {
		s.writeError(w, http.StatusInternalServerError, "KN-500", err.Error())
		return
	}
	if pr2, e := s.st.GetProject(r.Context(), pr.Tenant, pr.Name); e == nil && pr2.UID != "" {
		u := pr2.UID
		in.Uid = &u
	}
	in.CreatedAt = &pr.CreatedAt
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(in)
}

// (GET /api/v1/clusters/{c}/tenants/{t}/projects/{p})
func (s *APIServer) GetApiV1ClustersCTenantsTProjectsP(w http.ResponseWriter, r *http.Request, c ClusterParam, t TenantParam, p ProjectParam) {
	ten, err := s.st.GetTenantByUID(r.Context(), string(t))
	if err != nil {
		s.writeError(w, http.StatusNotFound, "KN-404", "not found")
		return
	}
	if !s.requireRolesTenant(w, r, ten.Name, "admin", "ops", "tenantOwner", "projectDev", "readOnly") {
		return
	}
	pr, err := s.st.GetProjectByUID(r.Context(), string(p))
	if err != nil {
		s.writeError(w, http.StatusNotFound, "KN-404", "not found")
		return
	}
	out := Project{Name: pr.Name, CreatedAt: &pr.CreatedAt}
	if pr.UID != "" {
		u := pr.UID
		out.Uid = &u
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

// (PUT /api/v1/clusters/{c}/tenants/{t}/projects/{p})
func (s *APIServer) PutApiV1ClustersCTenantsTProjectsP(w http.ResponseWriter, r *http.Request, c ClusterParam, t TenantParam, p ProjectParam) {
	ten, err := s.st.GetTenantByUID(r.Context(), string(t))
	if err != nil {
		s.writeError(w, http.StatusNotFound, "KN-404", "not found")
		return
	}
	if !s.requireRolesTenant(w, r, ten.Name, "admin", "ops", "tenantOwner", "projectDev") {
		return
	}
	var in Project
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		s.writeError(w, http.StatusUnprocessableEntity, "KN-422", "invalid payload")
		return
	}
	prResolved, _ := s.st.GetProjectByUID(r.Context(), string(p))
	pr := kn.Project{Tenant: ten.Name, Name: prResolved.Name, CreatedAt: time.Now().UTC()}
	_ = s.st.UpdateProject(r.Context(), pr)
	w.WriteHeader(http.StatusOK)
}

// (DELETE /api/v1/clusters/{c}/tenants/{t}/projects/{p})
func (s *APIServer) DeleteApiV1ClustersCTenantsTProjectsP(w http.ResponseWriter, r *http.Request, c ClusterParam, t TenantParam, p ProjectParam) {
	ten, err := s.st.GetTenantByUID(r.Context(), string(t))
	if err != nil {
		s.writeError(w, http.StatusNotFound, "KN-404", "not found")
		return
	}
	if !s.requireRolesTenant(w, r, ten.Name, "admin", "ops", "tenantOwner") {
		return
	}
	pr, err := s.st.GetProjectByUID(r.Context(), string(p))
	if err == nil {
		_ = s.st.DeleteProject(r.Context(), ten.Name, pr.Name)
	}
	w.WriteHeader(http.StatusNoContent)
}

// (PUT /api/v1/clusters/{c}/tenants/{t}/projects/{p}/access)
func (s *APIServer) PutApiV1ClustersCTenantsTProjectsPAccess(w http.ResponseWriter, r *http.Request, c ClusterParam, t TenantParam, p ProjectParam) {
	if !s.requireRolesTenant(w, r, string(t), "admin", "ops", "tenantOwner") {
		return
	}
	w.WriteHeader(http.StatusOK)
}

// --- Apps ---

// (GET /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps)
func (s *APIServer) GetApiV1ClustersCTenantsTProjectsPApps(w http.ResponseWriter, r *http.Request, c ClusterParam, t TenantParam, p ProjectParam, params GetApiV1ClustersCTenantsTProjectsPAppsParams) {
	if !s.requireRolesTenant(w, r, string(t), "admin", "ops", "tenantOwner", "projectDev", "readOnly") {
		return
	}
	items, err := s.st.ListApps(r.Context(), string(t), string(p))
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "KN-500", err.Error())
		return
	}
	out := make([]App, 0, len(items))
	for _, a := range items {
		aa := a
		dto := App{Name: aa.Name, CreatedAt: &aa.CreatedAt}
		if aa.UID != "" {
			u := aa.UID
			dto.Uid = &u
		}
		out = append(out, dto)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

// (POST /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps)
func (s *APIServer) PostApiV1ClustersCTenantsTProjectsPApps(w http.ResponseWriter, r *http.Request, c ClusterParam, t TenantParam, p ProjectParam) {
	pr, err := s.st.GetProjectByUID(r.Context(), string(p))
	if err != nil {
		s.writeError(w, http.StatusNotFound, "KN-404", "project not found")
		return
	}
	if !s.requireRolesTenant(w, r, pr.Tenant, "admin", "ops", "tenantOwner", "projectDev") {
		return
	}
	var in App
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		s.writeError(w, http.StatusUnprocessableEntity, "KN-422", "invalid payload")
		return
	}
	if strings.TrimSpace(in.Name) == "" {
		s.writeError(w, http.StatusUnprocessableEntity, "KN-422", "name required")
		return
	}
	a := kn.App{Tenant: pr.Tenant, Project: pr.Name, Name: in.Name, CreatedAt: time.Now().UTC()}
	if err := s.st.CreateApp(r.Context(), a); err != nil {
		s.writeError(w, http.StatusInternalServerError, "KN-500", err.Error())
		return
	}
	if aa, e := s.st.GetApp(r.Context(), a.Tenant, a.Project, a.Name); e == nil && aa.UID != "" {
		u := aa.UID
		in.Uid = &u
	}
	in.CreatedAt = &a.CreatedAt
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(in)
}

// (GET /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a})
func (s *APIServer) GetApiV1ClustersCTenantsTProjectsPAppsA(w http.ResponseWriter, r *http.Request, c ClusterParam, t TenantParam, p ProjectParam, a AppParam) {
	it, err := s.st.GetAppByUID(r.Context(), string(a))
	if err != nil {
		s.writeError(w, http.StatusNotFound, "KN-404", "not found")
		return
	}
	if !s.requireRolesTenant(w, r, it.Tenant, "admin", "ops", "tenantOwner", "projectDev", "readOnly") {
		return
	}
	out := App{Name: it.Name, CreatedAt: &it.CreatedAt}
	if it.UID != "" {
		u := it.UID
		out.Uid = &u
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

// (PUT /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a})
func (s *APIServer) PutApiV1ClustersCTenantsTProjectsPAppsA(w http.ResponseWriter, r *http.Request, c ClusterParam, t TenantParam, p ProjectParam, a AppParam) {
	ap, err := s.st.GetAppByUID(r.Context(), string(a))
	if err != nil {
		s.writeError(w, http.StatusNotFound, "KN-404", "not found")
		return
	}
	if !s.requireRolesTenant(w, r, ap.Tenant, "admin", "ops", "tenantOwner", "projectDev") {
		return
	}
	var in App
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		s.writeError(w, http.StatusUnprocessableEntity, "KN-422", "invalid payload")
		return
	}
	item := kn.App{Tenant: ap.Tenant, Project: ap.Project, Name: ap.Name, CreatedAt: time.Now().UTC()}
	_ = s.st.UpdateApp(r.Context(), item)
	w.WriteHeader(http.StatusOK)
}

// (DELETE /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a})
func (s *APIServer) DeleteApiV1ClustersCTenantsTProjectsPAppsA(w http.ResponseWriter, r *http.Request, c ClusterParam, t TenantParam, p ProjectParam, a AppParam) {
	ap, err := s.st.GetAppByUID(r.Context(), string(a))
	if err != nil {
		s.writeError(w, http.StatusNotFound, "KN-404", "not found")
		return
	}
	if !s.requireRolesTenant(w, r, ap.Tenant, "admin", "ops", "tenantOwner", "projectDev") {
		return
	}
	_ = s.st.DeleteApp(r.Context(), ap.Tenant, ap.Project, ap.Name)
	w.WriteHeader(http.StatusAccepted)
}

// (POST /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}:deploy)
func (s *APIServer) PostApiV1ClustersCTenantsTProjectsPAppsADeploy(w http.ResponseWriter, r *http.Request, c ClusterParam, t TenantParam, p ProjectParam, a AppParam) {
	if !s.requireRolesTenant(w, r, string(t), "admin", "ops", "tenantOwner", "projectDev") {
		return
	}
	var kb []byte
	if _, enc, err := s.st.GetClusterByUID(r.Context(), string(c)); err == nil {
		kb, _ = base64.StdEncoding.DecodeString(enc)
	}
	if err := s.newVela(kb).Deploy(r.Context(), string(p), string(a)); err != nil {
		s.writeError(w, http.StatusInternalServerError, "KN-500", err.Error())
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

// (POST /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}:suspend)
func (s *APIServer) PostApiV1ClustersCTenantsTProjectsPAppsASuspend(w http.ResponseWriter, r *http.Request, c ClusterParam, t TenantParam, p ProjectParam, a AppParam) {
	if !s.requireRolesTenant(w, r, string(t), "admin", "ops", "tenantOwner", "projectDev") {
		return
	}
	var kb []byte
	if _, enc, err := s.st.GetClusterByUID(r.Context(), string(c)); err == nil {
		kb, _ = base64.StdEncoding.DecodeString(enc)
	}
	if err := s.newVela(kb).Suspend(r.Context(), string(p), string(a)); err != nil {
		s.writeError(w, http.StatusInternalServerError, "KN-500", err.Error())
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

// (POST /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}:resume)
func (s *APIServer) PostApiV1ClustersCTenantsTProjectsPAppsAResume(w http.ResponseWriter, r *http.Request, c ClusterParam, t TenantParam, p ProjectParam, a AppParam) {
	if !s.requireRolesTenant(w, r, string(t), "admin", "ops", "tenantOwner", "projectDev") {
		return
	}
	var kb []byte
	if _, enc, err := s.st.GetClusterByUID(r.Context(), string(c)); err == nil {
		kb, _ = base64.StdEncoding.DecodeString(enc)
	}
	if err := s.newVela(kb).Resume(r.Context(), string(p), string(a)); err != nil {
		s.writeError(w, http.StatusInternalServerError, "KN-500", err.Error())
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

// (POST /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}:rollback)
func (s *APIServer) PostApiV1ClustersCTenantsTProjectsPAppsARollback(w http.ResponseWriter, r *http.Request, c ClusterParam, t TenantParam, p ProjectParam, a AppParam) {
	if !s.requireRolesTenant(w, r, string(t), "admin", "ops", "tenantOwner", "projectDev") {
		return
	}
	var kb []byte
	if _, enc, err := s.st.GetClusterByUID(r.Context(), string(c)); err == nil {
		kb, _ = base64.StdEncoding.DecodeString(enc)
	}
	var body struct {
		ToRevision *int `json:"toRevision"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if err := s.newVela(kb).Rollback(r.Context(), string(p), string(a), body.ToRevision); err != nil {
		s.writeError(w, http.StatusInternalServerError, "KN-500", err.Error())
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

// (GET /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}/status)
func (s *APIServer) GetApiV1ClustersCTenantsTProjectsPAppsAStatus(w http.ResponseWriter, r *http.Request, c ClusterParam, t TenantParam, p ProjectParam, a AppParam) {
	if !s.requireRolesTenant(w, r, string(t), "admin", "ops", "tenantOwner", "projectDev", "readOnly") {
		return
	}
	_, enc, err := s.st.GetClusterByUID(r.Context(), string(c))
	if err != nil {
		s.writeError(w, http.StatusNotFound, "KN-404", "cluster not found")
		return
	}
	kb, _ := base64.StdEncoding.DecodeString(enc)
	st, err := s.newVela(kb).Status(r.Context(), string(p), string(a))
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "KN-500", err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(st)
}

// (GET /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}/revisions)
func (s *APIServer) GetApiV1ClustersCTenantsTProjectsPAppsARevisions(w http.ResponseWriter, r *http.Request, c ClusterParam, t TenantParam, p ProjectParam, a AppParam) {
	if !s.requireRolesTenant(w, r, string(t), "admin", "ops", "tenantOwner", "projectDev", "readOnly") {
		return
	}
	_, enc, err := s.st.GetClusterByUID(r.Context(), string(c))
	if err != nil {
		s.writeError(w, http.StatusNotFound, "KN-404", "cluster not found")
		return
	}
	kb, _ := base64.StdEncoding.DecodeString(enc)
	revs, err := s.newVela(kb).Revisions(r.Context(), string(p), string(a))
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "KN-500", err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(revs)
}

// (GET /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}/diff/{revA}/{revB})
func (s *APIServer) GetApiV1ClustersCTenantsTProjectsPAppsADiffRevARevB(w http.ResponseWriter, r *http.Request, c ClusterParam, t TenantParam, p ProjectParam, a AppParam, revA int, revB int) {
	if !s.requireRolesTenant(w, r, string(t), "admin", "ops", "tenantOwner", "projectDev", "readOnly") {
		return
	}
	_, enc, err := s.st.GetClusterByUID(r.Context(), string(c))
	if err != nil {
		s.writeError(w, http.StatusNotFound, "KN-404", "cluster not found")
		return
	}
	kb, _ := base64.StdEncoding.DecodeString(enc)
	d, err := s.newVela(kb).Diff(r.Context(), string(p), string(a), revA, revB)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "KN-500", err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(d)
}

// (GET /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}/logs/{component})
func (s *APIServer) GetApiV1ClustersCTenantsTProjectsPAppsALogsComponent(w http.ResponseWriter, r *http.Request, c ClusterParam, t TenantParam, p ProjectParam, a AppParam, component string, params GetApiV1ClustersCTenantsTProjectsPAppsALogsComponentParams) {
	if !s.requireRolesTenant(w, r, string(t), "admin", "ops", "tenantOwner", "projectDev", "readOnly") {
		return
	}
	_, enc, err := s.st.GetClusterByUID(r.Context(), string(c))
	if err != nil {
		s.writeError(w, http.StatusNotFound, "KN-404", "cluster not found")
		return
	}
	kb, _ := base64.StdEncoding.DecodeString(enc)
	follow := false
	if params.Follow != nil {
		follow = bool(*params.Follow)
	}
	// pass optional pagination in context for backend to optionally honor
	ctx := r.Context()
	if params.Tail != nil {
		ctx = context.WithValue(ctx, struct{ k string }{"tail"}, int(*params.Tail))
	}
	if params.SinceSeconds != nil {
		ctx = context.WithValue(ctx, struct{ k string }{"sinceSeconds"}, int(*params.SinceSeconds))
	}
	lines, err := s.newVela(kb).Logs(ctx, string(p), string(a), component, follow)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "KN-500", err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(lines)
}

// System endpoints under /api/v1
func (s *APIServer) GetApiV1Version(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"version": "1.0.0"})
}

func (s *APIServer) GetApiV1Features(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]bool{"tenancy": true, "vela": true, "proxy": true})
}

// Access & Tokens
func (s *APIServer) PostApiV1Tokens(w http.ResponseWriter, r *http.Request) {
	var req TokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusUnprocessableEntity, "KN-422", "invalid payload")
		return
	}
	if strings.TrimSpace(req.Subject) == "" {
		s.writeError(w, http.StatusUnprocessableEntity, "KN-422", "subject required")
		return
	}
	ttl := 3600
	if req.TtlSeconds != nil && *req.TtlSeconds > 0 && *req.TtlSeconds <= 2592000 {
		ttl = *req.TtlSeconds
	}
	roles := []string{"readOnly"}
	if req.Roles != nil && len(*req.Roles) > 0 {
		roles = make([]string, 0, len(*req.Roles))
		for _, r := range *req.Roles {
			roles = append(roles, string(r))
		}
	}
	c := jwt.MapClaims{"sub": req.Subject, "roles": roles, "exp": time.Now().Add(time.Duration(ttl) * time.Second).Unix()}
	key := s.jwtKey
	if len(key) == 0 {
		key = []byte("dev")
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, c)
	ss, err := tok.SignedString(key)
	if err != nil {
		s.writeError(w, 500, "KN-500", "sign failure")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"token": ss, "expiresAt": time.Now().Add(time.Duration(ttl) * time.Second).UTC()})
}

func (s *APIServer) GetApiV1Me(w http.ResponseWriter, r *http.Request) {
	roles := s.rolesFromReq(r)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"subject": "", "roles": roles})
}

// (PUT /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}/traits)
func (s *APIServer) PutApiV1ClustersCTenantsTProjectsPAppsATraits(w http.ResponseWriter, r *http.Request, c ClusterParam, t TenantParam, p ProjectParam, a AppParam) {
	if !s.requireRolesTenant(w, r, string(t), "admin", "ops", "tenantOwner", "projectDev") {
		return
	}
	var body []map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.writeError(w, http.StatusUnprocessableEntity, "KN-422", "invalid payload")
		return
	}
	_, enc, err := s.st.GetClusterByName(r.Context(), string(c))
	if err != nil {
		s.writeError(w, http.StatusNotFound, "KN-404", "cluster not found")
		return
	}
	kb, _ := base64.StdEncoding.DecodeString(enc)
	if err := s.newVela(kb).SetTraits(r.Context(), string(p), string(a), body); err != nil {
		s.writeError(w, http.StatusInternalServerError, "KN-500", err.Error())
		return
	}
	w.WriteHeader(http.StatusOK)
}

// (PUT /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}/policies)
func (s *APIServer) PutApiV1ClustersCTenantsTProjectsPAppsAPolicies(w http.ResponseWriter, r *http.Request, c ClusterParam, t TenantParam, p ProjectParam, a AppParam) {
	if !s.requireRolesTenant(w, r, string(t), "admin", "ops", "tenantOwner", "projectDev") {
		return
	}
	var body []map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.writeError(w, http.StatusUnprocessableEntity, "KN-422", "invalid payload")
		return
	}
	_, enc, err := s.st.GetClusterByName(r.Context(), string(c))
	if err != nil {
		s.writeError(w, http.StatusNotFound, "KN-404", "cluster not found")
		return
	}
	kb, _ := base64.StdEncoding.DecodeString(enc)
	if err := s.newVela(kb).SetPolicies(r.Context(), string(p), string(a), body); err != nil {
		s.writeError(w, http.StatusInternalServerError, "KN-500", err.Error())
		return
	}
	w.WriteHeader(http.StatusOK)
}

// (POST /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}/image-update)
func (s *APIServer) PostApiV1ClustersCTenantsTProjectsPAppsAImageUpdate(w http.ResponseWriter, r *http.Request, c ClusterParam, t TenantParam, p ProjectParam, a AppParam) {
	if !s.requireRolesTenant(w, r, string(t), "admin", "ops", "tenantOwner", "projectDev") {
		return
	}
	var body PostApiV1ClustersCTenantsTProjectsPAppsAImageUpdateJSONBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.writeError(w, http.StatusUnprocessableEntity, "KN-422", "invalid payload")
		return
	}
	_, enc, err := s.st.GetClusterByName(r.Context(), string(c))
	if err != nil {
		s.writeError(w, http.StatusNotFound, "KN-404", "cluster not found")
		return
	}
	kb, _ := base64.StdEncoding.DecodeString(enc)
	tag := ""
	if body.Tag != nil {
		tag = *body.Tag
	}
	if err := s.newVela(kb).ImageUpdate(r.Context(), string(p), string(a), body.Component, body.Image, tag); err != nil {
		s.writeError(w, http.StatusInternalServerError, "KN-500", err.Error())
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

// ---- mapping helpers ----
func toTypesCluster(in ClusterRegistration) kn.Cluster {
	out := kn.Cluster{Name: in.Name, Labels: map[string]string{}, CreatedAt: time.Now().UTC()}
	if in.Labels != nil {
		out.Labels = *in.Labels
	}
	return out
}

func toTypesTenant(in Tenant) kn.Tenant {
	out := kn.Tenant{Name: in.Name, Labels: map[string]string{}, Annotations: map[string]string{}, CreatedAt: time.Now().UTC()}
	if in.Labels != nil {
		out.Labels = *in.Labels
	}
	if in.Annotations != nil {
		out.Annotations = *in.Annotations
	}
	if in.Owners != nil {
		out.Owners = *in.Owners
	}
	return out
}
