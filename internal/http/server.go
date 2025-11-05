package httpapi

import (
    "encoding/base64"
    "encoding/json"
    "net/http"
    "os"
    "strings"
    "time"

    "github.com/vaheed/kubenova/internal/store"
    kn "github.com/vaheed/kubenova/pkg/types"
    clusterpkg "github.com/vaheed/kubenova/internal/cluster"
    capib "github.com/vaheed/kubenova/internal/backends/capsule"
)

// APIServer implements a subset of the contract (Clusters + Tenants) and embeds
// Unimplemented for the rest of the surface.
type APIServer struct {
    Unimplemented
    st          store.Store
    requireAuth bool
    jwtKey      []byte
    newCapsule  func([]byte) capib.Client
}

func NewAPIServer(st store.Store) *APIServer {
    return &APIServer{
        st:          st,
        requireAuth: parseBool(os.Getenv("KUBENOVA_REQUIRE_AUTH")),
        jwtKey:      []byte(getenv("JWT_SIGNING_KEY", "dev")),
        newCapsule:  capib.New,
    }
}

// --- helpers ---
func getenv(k, d string) string { if v := os.Getenv(k); v != "" { return v }; return d }
func parseBool(v string) bool {
    switch strings.ToLower(strings.TrimSpace(v)) {
    case "1", "t", "true", "y", "yes", "on":
        return true
    default:
        return false
    }
}

func (s *APIServer) writeError(w http.ResponseWriter, status int, code, msg string) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(status)
    _ = json.NewEncoder(w).Encode(Error{Code: code, Message: msg})
}

func (s *APIServer) requireRoles(w http.ResponseWriter, r *http.Request, allowed ...string) bool {
    if !s.requireAuth { return true }
    hdr := r.Header.Get("Authorization")
    if hdr == "" || !strings.HasPrefix(strings.ToLower(hdr), "bearer ") {
        s.writeError(w, http.StatusUnauthorized, "KN-401", "missing bearer token")
        return false
    }
    // Minimal role check: accept any non-empty token for now, as tokens are issued by legacy /api/v1/tokens
    // Future: parse HS256 and extract roles for granular checks.
    tok := strings.TrimSpace(strings.TrimPrefix(hdr, "Bearer"))
    if tok == "" {
        s.writeError(w, http.StatusUnauthorized, "KN-401", "invalid token")
        return false
    }
    // Allow when any allowed role is present in a simple comma list header (X-KN-Roles) or when allowed contains "*"
    if len(allowed) == 0 || (len(allowed) == 1 && allowed[0] == "*") {
        return true
    }
    rolesHdr := r.Header.Get("X-KN-Roles")
    if rolesHdr == "" {
        // default to readOnly
        rolesHdr = "readOnly"
    }
    have := map[string]struct{}{}
    for _, p := range strings.Split(rolesHdr, ",") { have[strings.TrimSpace(p)] = struct{}{} }
    for _, want := range allowed {
        if _, ok := have[want]; ok { return true }
    }
    s.writeError(w, http.StatusForbidden, "KN-403", "forbidden")
    return false
}

// --- Clusters ---

// (POST /api/v1/clusters)
func (s *APIServer) PostApiV1Clusters(w http.ResponseWriter, r *http.Request) {
    if !s.requireRoles(w, r, "admin", "ops") { return }
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
    if in.Labels != nil { out.Labels = in.Labels }
    w.Header().Set("Content-Type", "application/json")
    _ = json.NewEncoder(w).Encode(out)
}

// (GET /api/v1/clusters)
func (s *APIServer) GetApiV1Clusters(w http.ResponseWriter, r *http.Request, params GetApiV1ClustersParams) {
    if !s.requireRoles(w, r, "admin", "ops") { return }
    // Store does not expose list yet; return empty set for now.
    w.Header().Set("Content-Type", "application/json")
    _ = json.NewEncoder(w).Encode([]Cluster{})
}

// (GET /api/v1/clusters/{c})
func (s *APIServer) GetApiV1ClustersC(w http.ResponseWriter, r *http.Request, c ClusterParam) {
    if !s.requireRoles(w, r, "admin", "ops") { return }
    name := string(c)
    cl, enc, err := s.st.GetClusterByName(r.Context(), name)
    if err != nil { s.writeError(w, http.StatusNotFound, "KN-404", "not found"); return }
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
    if len(outConds) > 0 { out.Conditions = &outConds }
    w.Header().Set("Content-Type", "application/json")
    _ = json.NewEncoder(w).Encode(out)
}

// (GET /api/v1/clusters/{c}/capabilities)
func (s *APIServer) GetApiV1ClustersCCapabilities(w http.ResponseWriter, r *http.Request, c ClusterParam) {
    if !s.requireRoles(w, r, "admin", "ops") { return }
    t, v, p := true, true, true
    w.Header().Set("Content-Type", "application/json")
    _ = json.NewEncoder(w).Encode(ClusterCapabilities{Tenancy: &t, Vela: &v, Proxy: &p})
}

// --- Tenants ---

// (GET /api/v1/clusters/{c}/tenants)
func (s *APIServer) GetApiV1ClustersCTenants(w http.ResponseWriter, r *http.Request, c ClusterParam, params GetApiV1ClustersCTenantsParams) {
    if !s.requireRoles(w, r, "admin", "ops") { return }
    items, err := s.st.ListTenants(r.Context())
    if err != nil { s.writeError(w, http.StatusInternalServerError, "KN-500", err.Error()); return }
    out := make([]Tenant, 0, len(items))
    for _, t := range items {
        out = append(out, Tenant{Name: t.Name, Labels: &t.Labels, Annotations: &t.Annotations})
    }
    w.Header().Set("Content-Type", "application/json")
    _ = json.NewEncoder(w).Encode(out)
}

// (POST /api/v1/clusters/{c}/tenants)
func (s *APIServer) PostApiV1ClustersCTenants(w http.ResponseWriter, r *http.Request, c ClusterParam, params PostApiV1ClustersCTenantsParams) {
    if !s.requireRoles(w, r, "admin", "ops") { return }
    var in Tenant
    if err := json.NewDecoder(r.Body).Decode(&in); err != nil { s.writeError(w, http.StatusUnprocessableEntity, "KN-422", "invalid payload"); return }
    if strings.TrimSpace(in.Name) == "" { s.writeError(w, http.StatusUnprocessableEntity, "KN-422", "name required"); return }
    t := toTypesTenant(in)
    if err := s.st.CreateTenant(r.Context(), t); err != nil { s.writeError(w, http.StatusInternalServerError, "KN-500", err.Error()); return }
    now := time.Now().UTC()
    in.CreatedAt = &now
    w.Header().Set("Content-Type", "application/json")
    _ = json.NewEncoder(w).Encode(in)
}

// (GET /api/v1/clusters/{c}/tenants/{t})
func (s *APIServer) GetApiV1ClustersCTenantsT(w http.ResponseWriter, r *http.Request, c ClusterParam, t TenantParam) {
    if !s.requireRoles(w, r, "admin", "ops") { return }
    item, err := s.st.GetTenant(r.Context(), string(t))
    if err != nil {
        if err == store.ErrNotFound { s.writeError(w, http.StatusNotFound, "KN-404", "not found") } else { s.writeError(w, http.StatusInternalServerError, "KN-500", err.Error()) }
        return
    }
    out := Tenant{Name: item.Name, Labels: &item.Labels, Annotations: &item.Annotations}
    w.Header().Set("Content-Type", "application/json")
    _ = json.NewEncoder(w).Encode(out)
}

// (DELETE /api/v1/clusters/{c}/tenants/{t})
func (s *APIServer) DeleteApiV1ClustersCTenantsT(w http.ResponseWriter, r *http.Request, c ClusterParam, t TenantParam) {
    if !s.requireRoles(w, r, "admin", "ops") { return }
    _ = s.st.DeleteTenant(r.Context(), string(t))
    w.WriteHeader(http.StatusNoContent)
}

// (PUT /api/v1/clusters/{c}/tenants/{t}/owners)
func (s *APIServer) PutApiV1ClustersCTenantsTOwners(w http.ResponseWriter, r *http.Request, c ClusterParam, t TenantParam) {
    if !s.requireRoles(w, r, "admin", "ops") { return }
    var body PutApiV1ClustersCTenantsTOwnersJSONBody
    if err := json.NewDecoder(r.Body).Decode(&body); err != nil { s.writeError(w, http.StatusUnprocessableEntity, "KN-422", "invalid payload"); return }
    item, err := s.st.GetTenant(r.Context(), string(t))
    if err != nil { s.writeError(w, http.StatusNotFound, "KN-404", "not found"); return }
    if body.Owners != nil {
        item.Owners = *body.Owners
    }
    _ = s.st.UpdateTenant(r.Context(), item)
    w.WriteHeader(http.StatusOK)
}

// (PUT /api/v1/clusters/{c}/tenants/{t}/quotas)
func (s *APIServer) PutApiV1ClustersCTenantsTQuotas(w http.ResponseWriter, r *http.Request, c ClusterParam, t TenantParam) {
    if !s.requireRoles(w, r, "admin", "ops") { return }
    var body map[string]string
    if err := json.NewDecoder(r.Body).Decode(&body); err != nil { s.writeError(w, http.StatusUnprocessableEntity, "KN-422", "invalid payload"); return }
    _, enc, err := s.st.GetClusterByName(r.Context(), string(c))
    if err != nil { s.writeError(w, http.StatusNotFound, "KN-404", "cluster not found"); return }
    kb, _ := base64.StdEncoding.DecodeString(enc)
    if err := s.newCapsule(kb).SetTenantQuotas(r.Context(), string(t), body); err != nil { s.writeError(w, http.StatusInternalServerError, "KN-500", err.Error()); return }
    w.WriteHeader(http.StatusOK)
}

// (PUT /api/v1/clusters/{c}/tenants/{t}/limits)
func (s *APIServer) PutApiV1ClustersCTenantsTLimits(w http.ResponseWriter, r *http.Request, c ClusterParam, t TenantParam) {
    if !s.requireRoles(w, r, "admin", "ops") { return }
    var body map[string]string
    if err := json.NewDecoder(r.Body).Decode(&body); err != nil { s.writeError(w, http.StatusUnprocessableEntity, "KN-422", "invalid payload"); return }
    _, enc, err := s.st.GetClusterByName(r.Context(), string(c))
    if err != nil { s.writeError(w, http.StatusNotFound, "KN-404", "cluster not found"); return }
    kb, _ := base64.StdEncoding.DecodeString(enc)
    if err := s.newCapsule(kb).SetTenantLimits(r.Context(), string(t), body); err != nil { s.writeError(w, http.StatusInternalServerError, "KN-500", err.Error()); return }
    w.WriteHeader(http.StatusOK)
}

// (PUT /api/v1/clusters/{c}/tenants/{t}/network-policies)
func (s *APIServer) PutApiV1ClustersCTenantsTNetworkPolicies(w http.ResponseWriter, r *http.Request, c ClusterParam, t TenantParam) {
    if !s.requireRoles(w, r, "admin", "ops") { return }
    var body map[string]any
    if err := json.NewDecoder(r.Body).Decode(&body); err != nil { s.writeError(w, http.StatusUnprocessableEntity, "KN-422", "invalid payload"); return }
    _, enc, err := s.st.GetClusterByName(r.Context(), string(c))
    if err != nil { s.writeError(w, http.StatusNotFound, "KN-404", "cluster not found"); return }
    kb, _ := base64.StdEncoding.DecodeString(enc)
    if err := s.newCapsule(kb).SetTenantNetworkPolicies(r.Context(), string(t), body); err != nil { s.writeError(w, http.StatusInternalServerError, "KN-500", err.Error()); return }
    w.WriteHeader(http.StatusOK)
}

// (GET /api/v1/clusters/{c}/tenants/{t}/summary)
func (s *APIServer) GetApiV1ClustersCTenantsTSummary(w http.ResponseWriter, r *http.Request, c ClusterParam, t TenantParam) {
    if !s.requireRoles(w, r, "admin", "ops") { return }
    w.Header().Set("Content-Type", "application/json")
    _ = json.NewEncoder(w).Encode(TenantSummary{})
}

// ---- mapping helpers ----
func toTypesCluster(in ClusterRegistration) kn.Cluster {
    out := kn.Cluster{Name: in.Name, Labels: map[string]string{}, CreatedAt: time.Now().UTC()}
    if in.Labels != nil { out.Labels = *in.Labels }
    return out
}

func toTypesTenant(in Tenant) kn.Tenant {
    out := kn.Tenant{Name: in.Name, Labels: map[string]string{}, Annotations: map[string]string{}, CreatedAt: time.Now().UTC()}
    if in.Labels != nil { out.Labels = *in.Labels }
    if in.Annotations != nil { out.Annotations = *in.Annotations }
    if in.Owners != nil { out.Owners = *in.Owners }
    return out
}
