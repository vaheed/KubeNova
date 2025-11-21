package httpapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"
	capib "github.com/vaheed/kubenova/internal/backends/capsule"
	velab "github.com/vaheed/kubenova/internal/backends/vela"
	clusterpkg "github.com/vaheed/kubenova/internal/cluster"
	"github.com/vaheed/kubenova/internal/lib/httperr"
	"github.com/vaheed/kubenova/internal/logging"
	"github.com/vaheed/kubenova/internal/store"
	"github.com/vaheed/kubenova/internal/telemetry"
	catalogdata "github.com/vaheed/kubenova/pkg/catalog"
	kn "github.com/vaheed/kubenova/pkg/types"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
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
		DeleteApp(context.Context, string, string) error
	}
	policysetCatalog []PolicySet
	planCatalog      []TenantPlan
	runsMu           sync.RWMutex
	runsByID         map[string]WorkflowRun
	runsByApp        map[string][]WorkflowRun // key: tenantID|projectID|appID
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
			DeleteApp(context.Context, string, string) error
		} {
			return velab.New(b)
		},
		policysetCatalog: loadPolicySetCatalog(),
		planCatalog:      loadPlanCatalog(),
		runsByID:         map[string]WorkflowRun{},
		runsByApp:        map[string][]WorkflowRun{},
	}
}

// ensureAppConfigMap creates or updates a ConfigMap in the project namespace
// that encodes the App model for the in-cluster AppReconciler. Best-effort:
// failures are logged and do not affect the HTTP response. The ConfigMap includes
// both human-friendly labels (`kubenova.app/tenant/project`) and machine IDs
// (`kubenova.io/{app,tenant,project}-id`) so Agents can correlate tenant Apps.
func (s *APIServer) ensureAppConfigMap(ctx context.Context, clusterID, projectNS, tenantName, tenantID, projectID, appID, sourceKind string, in App) error {
	_, enc, err := s.st.GetClusterByID(ctx, clusterID)
	if err != nil || enc == "" {
		return nil
	}
	kb, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		return nil
	}
	cfg, err := clientcmd.RESTConfigFromKubeConfig(kb)
	if err != nil {
		return nil
	}
	cset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil
	}
	name := in.Name
	if strings.TrimSpace(name) == "" {
		return nil
	}
	cmClient := cset.CoreV1().ConfigMaps(projectNS)
	spec := map[string]any{}
	if in.Components != nil {
		spec["components"] = *in.Components
	}
	if in.Description != nil {
		spec["description"] = *in.Description
	}
	if in.Spec != nil && in.Spec.Source != nil {
		spec["source"] = in.Spec.Source
	}
	if in.Spec != nil {
		if in.Spec.CatalogItemId != nil {
			spec["catalogItemId"] = in.Spec.CatalogItemId.String()
		}
		if in.Spec.CatalogVersion != nil {
			spec["catalogVersion"] = *in.Spec.CatalogVersion
		}
		if in.Spec.CatalogOverrides != nil {
			spec["catalogOverrides"] = *in.Spec.CatalogOverrides
		}
	}
	rawSpec, _ := json.Marshal(spec)
	rawTraits, _ := json.Marshal(zeroOrSlice(in.Traits))
	rawPolicies, _ := json.Marshal(zeroOrSlice(in.Policies))
	data := map[string]string{
		"spec":     string(rawSpec),
		"traits":   string(rawTraits),
		"policies": string(rawPolicies),
	}
	labels := map[string]string{
		"kubenova.app":     name,
		"kubenova.tenant":  tenantName,
		"kubenova.project": projectNS,
	}
	if tenantID != "" {
		labels["kubenova.io/tenant-id"] = tenantID
	}
	if projectID != "" {
		labels["kubenova.io/project-id"] = projectID
	}
	if appID != "" {
		labels["kubenova.io/app-id"] = appID
	}
	if sourceKind != "" {
		labels["kubenova.io/source-kind"] = sourceKind
	}

	if cm, err := cmClient.Get(ctx, name, metav1.GetOptions{}); err == nil {
		if cm.Labels == nil {
			cm.Labels = map[string]string{}
		}
		for k, v := range labels {
			cm.Labels[k] = v
		}
		if cm.Data == nil {
			cm.Data = map[string]string{}
		}
		for k, v := range data {
			cm.Data[k] = v
		}
		if _, err := cmClient.Update(ctx, cm, metav1.UpdateOptions{}); err != nil {
			logging.FromContext(ctx).Error("update app configmap", zap.Error(err))
		}
		return nil
	}
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: projectNS,
			Labels:    labels,
		},
		Data: data,
	}
	if _, err := cmClient.Create(ctx, cm, metav1.CreateOptions{}); err != nil {
		logging.FromContext(ctx).Error("create app configmap", zap.Error(err))
	}
	return nil
}

func zeroOrSlice(ptr *[]map[string]any) []map[string]any {
	if ptr == nil {
		return []map[string]any{}
	}
	return *ptr
}

func appToDTO(a kn.App) App {
	dto := App{
		Name:        a.Name,
		Description: a.Description,
		Components:  a.Components,
		Traits:      a.Traits,
		Policies:    a.Policies,
		Spec:        appSpecToHTTP(a.Spec),
		CreatedAt:   &a.CreatedAt,
	}
	if uid := uuidFromString(a.ID.String()); uid != nil {
		dto.Id = uid
	}
	return dto
}

func appSpecFromHTTP(spec *AppSpec) *kn.AppSpec {
	if spec == nil {
		return nil
	}
	raw, err := json.Marshal(spec)
	if err != nil {
		return nil
	}
	var out kn.AppSpec
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil
	}
	return &out
}

func appSpecToHTTP(spec *kn.AppSpec) *AppSpec {
	if spec == nil {
		return nil
	}
	raw, err := json.Marshal(spec)
	if err != nil {
		return nil
	}
	var out AppSpec
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil
	}
	return &out
}

func catalogItemToDTO(item kn.CatalogItem) CatalogItem {
	dto := CatalogItem{
		Id:          openapi_types.UUID(item.ID),
		Slug:        item.Slug,
		Name:        item.Name,
		Description: item.Description,
		Icon:        item.Icon,
		Category:    item.Category,
		Version:     item.Version,
		Scope:       CatalogItemScope(item.Scope),
		Source:      item.Source,
	}
	if item.TenantID != nil && *item.TenantID != (kn.ID{}) {
		tid := openapi_types.UUID(*item.TenantID)
		dto.TenantId = &tid
	}
	return dto
}

func mergeSourceMaps(base map[string]any, overrides *map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range base {
		out[k] = v
	}
	if overrides != nil {
		for k, v := range *overrides {
			out[k] = v
		}
	}
	return out
}

func catalogSourceToAppSpec(source map[string]any) *kn.AppSpec {
	if len(source) == 0 {
		return nil
	}
	payload := map[string]any{"source": source}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil
	}
	var spec kn.AppSpec
	if err := json.Unmarshal(raw, &spec); err != nil {
		return nil
	}
	return &spec
}

func publishKubeconfigEvent(ctx context.Context, scope, clusterID, tenant, tenantID, project, projectID, role, status, reason string, expiresAt *time.Time) {
	payload := map[string]any{
		"component": "kubeconfig",
		"scope":     scope,
		"clusterId": clusterID,
		"tenant":    tenant,
		"tenantId":  tenantID,
		"project":   project,
		"projectId": projectID,
		"role":      role,
		"status":    status,
	}
	if reason != "" {
		payload["error"] = reason
	}
	if expiresAt != nil {
		payload["expiresAt"] = expiresAt.UTC().Format(time.RFC3339Nano)
	}
	telemetry.PublishEvent(payload)
}

func uuidFromString(id string) *openapi_types.UUID {
	if strings.TrimSpace(id) == "" {
		return nil
	}
	parsed, err := uuid.Parse(id)
	if err != nil {
		return nil
	}
	u := openapi_types.UUID(parsed)
	return &u
}

// envOrDefault returns the trimmed value of the given environment variable or
// the provided default when unset/empty.
func envOrDefault(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

// signingKey returns the JWT signing/verification key. When the configured key
// is empty (dev/test), it falls back to a small default so issued tokens can be
// parsed by the same process.
func (s *APIServer) signingKey() []byte {
	if len(s.jwtKey) > 0 {
		return s.jwtKey
	}
	return []byte("dev")
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

// encodePolicySet converts an HTTP PolicySet DTO into the internal types.PolicySet
// representation by preserving the entire JSON structure in the Policies map.
func encodePolicySet(tenantID string, dto PolicySet) (kn.PolicySet, error) {
	b, err := json.Marshal(dto)
	if err != nil {
		return kn.PolicySet{}, err
	}
	spec := map[string]any{}
	if err := json.Unmarshal(b, &spec); err != nil {
		return kn.PolicySet{}, err
	}
	return kn.PolicySet{
		Name:     dto.Name,
		Tenant:   tenantID,
		Policies: spec,
	}, nil
}

// decodePolicySet converts a stored types.PolicySet back into the HTTP DTO.
func decodePolicySet(stored kn.PolicySet) (PolicySet, error) {
	if stored.Policies == nil {
		return PolicySet{Name: stored.Name}, nil
	}
	b, err := json.Marshal(stored.Policies)
	if err != nil {
		return PolicySet{}, err
	}
	var dto PolicySet
	if err := json.Unmarshal(b, &dto); err != nil {
		return PolicySet{}, err
	}
	if strings.TrimSpace(dto.Name) == "" {
		dto.Name = stored.Name
	}
	return dto, nil
}

// applyPlanToTenant attaches quotas/limits and PolicySets for the given plan name to the tenant ID.
func (s *APIServer) applyPlanToTenant(ctx context.Context, tenantID, planName string) (TenantPlan, error) {
	var plan *TenantPlan
	for i := range s.planCatalog {
		if s.planCatalog[i].Name == planName {
			plan = &s.planCatalog[i]
			break
		}
	}
	if plan == nil {
		return TenantPlan{}, fmt.Errorf("plan not found")
	}
	ten, err := s.st.GetTenantByID(ctx, tenantID)
	if err != nil {
		return TenantPlan{}, err
	}
	if ten.Labels == nil {
		ten.Labels = map[string]string{}
	}
	clusterID := ten.Labels["kubenova.cluster"]
	if strings.TrimSpace(clusterID) == "" {
		return TenantPlan{}, fmt.Errorf("tenant has no primary cluster")
	}
	_, enc, err := s.st.GetClusterByID(ctx, clusterID)
	if err != nil {
		return TenantPlan{}, err
	}
	kb, _ := base64.StdEncoding.DecodeString(enc)
	caps := s.newCapsule(kb)
	if err := caps.EnsureTenant(ctx, ten.Name, ten.Owners, ten.Labels); err != nil {
		return TenantPlan{}, err
	}
	// Apply quotas from plan (cpu/memory) and optional pods limits.
	if len(plan.TenantQuotas) > 0 {
		quotas := map[string]string{}
		for k, v := range plan.TenantQuotas {
			if k == "pods" {
				continue
			}
			quotas[k] = v
		}
		if len(quotas) > 0 {
			if err := caps.SetTenantQuotas(ctx, ten.Name, quotas); err != nil {
				return TenantPlan{}, err
			}
		}
		if pods, ok := plan.TenantQuotas["pods"]; ok {
			if err := caps.SetTenantLimits(ctx, ten.Name, map[string]string{"pods": pods}); err != nil {
				return TenantPlan{}, err
			}
		}
	}
	// Ensure referenced PolicySets exist and are attached to this tenant.
	if len(plan.Policysets) > 0 {
		for _, psName := range plan.Policysets {
			var cat *PolicySet
			for i := range s.policysetCatalog {
				if s.policysetCatalog[i].Name == psName {
					cat = &s.policysetCatalog[i]
					break
				}
			}
			if cat == nil {
				return TenantPlan{}, fmt.Errorf("plan references unknown policyset")
			}
			dto := *cat
			// Attach to tenant by name so policies/traits apply across all projects.
			tenantName := ten.Name
			attached := []struct {
				Project *string `json:"project,omitempty"`
				Tenant  *string `json:"tenant,omitempty"`
			}{}
			if dto.AttachedTo != nil {
				attached = *dto.AttachedTo
			}
			found := false
			for _, at := range attached {
				if at.Tenant != nil && *at.Tenant == tenantName {
					found = true
					break
				}
			}
			if !found {
				attached = append(attached, struct {
					Project *string `json:"project,omitempty"`
					Tenant  *string `json:"tenant,omitempty"`
				}{Tenant: &tenantName})
			}
			dto.AttachedTo = &attached
			ps, err := encodePolicySet(ten.ID.String(), dto)
			if err != nil {
				return TenantPlan{}, err
			}
			if err := s.st.CreatePolicySet(ctx, ps); err != nil {
				return TenantPlan{}, err
			}
		}
	}
	// Persist selected plan on tenant annotations.
	if ten.Annotations == nil {
		ten.Annotations = map[string]string{}
	}
	ten.Annotations["kubenova.io/plan"] = plan.Name
	if err := s.st.UpdateTenant(ctx, ten); err != nil {
		return TenantPlan{}, err
	}
	return *plan, nil
}

// ApplyPlanToTenant is an exported wrapper around applyPlanToTenant for
// operational code paths (for example, PaaS bootstrap) that run outside the
// generated router but still need to respect the same plan semantics.
func (s *APIServer) ApplyPlanToTenant(ctx context.Context, tenantID string, planName string) (TenantPlan, error) {
	return s.applyPlanToTenant(ctx, tenantID, planName)
}

// (PUT /api/v1/tenants/{t}/plan)
func (s *APIServer) PutApiV1TenantsTPlan(w http.ResponseWriter, r *http.Request, t TenantParam) {
	ten, err := s.st.GetTenantByID(r.Context(), t.String())
	if err != nil {
		s.writeError(w, http.StatusNotFound, "KN-404", "not found")
		return
	}
	if !s.requireRolesTenant(w, r, ten.Name, "admin", "ops", "tenantOwner") {
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.writeError(w, http.StatusUnprocessableEntity, "KN-422", "invalid payload")
		return
	}
	if strings.TrimSpace(body.Name) == "" {
		s.writeError(w, http.StatusUnprocessableEntity, "KN-422", "invalid payload")
		return
	}
	plan, err := s.applyPlanToTenant(r.Context(), ten.ID.String(), strings.TrimSpace(body.Name))
	if err != nil {
		// Map known error cases to KN-4xx where possible.
		if strings.Contains(err.Error(), "plan not found") {
			s.writeError(w, http.StatusNotFound, "KN-404", "plan not found")
			return
		}
		if strings.Contains(err.Error(), "primary cluster") {
			s.writeError(w, http.StatusUnprocessableEntity, "KN-422", "tenant has no primary cluster")
			return
		}
		s.writeError(w, http.StatusInternalServerError, "KN-500", err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(plan)
}

// applyPolicySets inspects tenant-level PolicySets that are attached to the given
// tenant/project and materializes their Vela traits/policies before a deploy.
// It is best-effort: failures are surfaced as errors, but absence of policysets is allowed.
func (s *APIServer) applyPolicySets(ctx context.Context, kubeconfig []byte, tenantID, namespace, appName string) error {
	// Resolve tenant by ID to get its name for matching.
	ten, err := s.st.GetTenantByID(ctx, tenantID)
	if err != nil {
		return nil
	}
	items, err := s.st.ListPolicySets(ctx, tenantID)
	if err != nil {
		return err
	}
	if len(items) == 0 {
		return nil
	}
	var traits []map[string]any
	var policies []map[string]any

	for _, ps := range items {
		dto, derr := decodePolicySet(ps)
		if derr != nil {
			return derr
		}
		// Filter by attachedTo: match when attached tenant or project is this app's scope.
		if dto.AttachedTo != nil && len(*dto.AttachedTo) > 0 {
			match := false
			for _, at := range *dto.AttachedTo {
				if at.Tenant != nil && *at.Tenant == ten.Name {
					match = true
				}
				if at.Project != nil && *at.Project == namespace {
					match = true
				}
			}
			if !match {
				continue
			}
		}
		if dto.Rules == nil {
			continue
		}
		for _, r := range *dto.Rules {
			kind, _ := r["kind"].(string)
			switch kind {
			case "vela.trait":
				if spec, ok := r["spec"].(map[string]any); ok {
					traits = append(traits, spec)
				}
			case "vela.policy":
				if spec, ok := r["spec"].(map[string]any); ok {
					policies = append(policies, spec)
				}
			}
		}
	}
	if len(traits) == 0 && len(policies) == 0 {
		return nil
	}
	backend := s.newVela(kubeconfig)
	if len(traits) > 0 {
		if err := backend.SetTraits(ctx, namespace, appName, traits); err != nil {
			return err
		}
	}
	if len(policies) > 0 {
		if err := backend.SetPolicies(ctx, namespace, appName, policies); err != nil {
			return err
		}
	}
	return nil
}

// loadPolicySetCatalog loads the cluster-wide PolicySet catalog from data.
// If the file is missing or invalid, it falls back to a minimal built-in baseline.
func loadPolicySetCatalog() []PolicySet {
	// Prefer embedded catalog (works in containers).
	b, err := catalogdata.FS.ReadFile("policysets.json")
	if err != nil {
		b = nil
	}
	if len(b) == 0 {
		desc := "Base guardrails"
		return []PolicySet{{Name: "baseline", Description: &desc}}
	}
	var items []PolicySet
	if err := json.Unmarshal(b, &items); err != nil || len(items) == 0 {
		desc := "Base guardrails"
		return []PolicySet{{Name: "baseline", Description: &desc}}
	}
	return items
}

// TenantPlan describes a tenant plan (quotas + PolicySets) loaded from the catalog.
type TenantPlan struct {
	Name         string            `json:"name"`
	Description  *string           `json:"description,omitempty"`
	TenantQuotas map[string]string `json:"tenantQuotas,omitempty"`
	Policysets   []string          `json:"policysets,omitempty"`
}

// defaultTenantPlanName is the plan that is applied automatically on tenant
// creation when the caller does not specify an explicit plan and the catalog
// contains a matching entry.
var defaultTenantPlanName = strings.TrimSpace(envOrDefault("KUBENOVA_DEFAULT_TENANT_PLAN", "baseline"))

// loadPlanCatalog loads the tenant plans catalog from data.
func loadPlanCatalog() []TenantPlan {
	// Prefer embedded catalog (works in containers).
	b, err := catalogdata.FS.ReadFile("plans.json")
	if err != nil {
		b = nil
	}
	if len(b) == 0 {
		return nil
	}
	var items []TenantPlan
	if err := json.Unmarshal(b, &items); err != nil {
		return nil
	}
	return items
}

// (GET /api/v1/healthz)
func (s *APIServer) GetApiV1Healthz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

// (GET /api/v1/readyz)
func (s *APIServer) GetApiV1Readyz(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	// Prefer an explicit health check when the store exposes one.
	if h, ok := s.st.(interface{ Health(context.Context) error }); ok {
		if err := h.Health(ctx); err != nil {
			s.writeError(w, http.StatusServiceUnavailable, "KN-500", "store not ready")
			return
		}
	} else {
		// Fallback: simple list call to verify basic store usability.
		if _, err := s.st.ListTenants(ctx); err != nil {
			s.writeError(w, http.StatusServiceUnavailable, "KN-500", "store not ready")
			return
		}
	}
	// Optional telemetry/external check: when OTEL exporter endpoint is configured,
	// ensure it is reachable and not returning 5xx responses.
	if endpoint := strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")); endpoint != "" {
		tctx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		req, _ := http.NewRequestWithContext(tctx, http.MethodGet, endpoint, nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			s.writeError(w, http.StatusServiceUnavailable, "KN-500", "telemetry not ready")
			return
		}
		_ = resp.Body.Close()
		if resp.StatusCode >= 500 {
			s.writeError(w, http.StatusServiceUnavailable, "KN-500", "telemetry not ready")
			return
		}
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
	items := []CatalogEntity{{Name: &name, Type: &t, Description: &desc}}
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
	items := []CatalogEntity{{Name: &name, Type: &t, Description: &desc}}
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
	items := []CatalogEntity{{Name: &name, Type: &t, Description: &desc}}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(items)
}

// (GET /api/v1/catalog)
func (s *APIServer) GetApiV1Catalog(w http.ResponseWriter, r *http.Request, params GetApiV1CatalogParams) {
	if !s.requireRoles(w, r, "admin", "ops", "tenantOwner", "projectDev", "readOnly") {
		return
	}
	scope := CatalogItemScopeGlobal
	if params.Scope != nil {
		scope = CatalogItemScope(*params.Scope)
	}
	tenantID := ""
	if params.TenantId != nil {
		tenantID = params.TenantId.String()
	}
	if scope == CatalogItemScopeTenant && tenantID == "" {
		s.writeError(w, http.StatusUnprocessableEntity, "KN-422", "tenantId required when scope=tenant")
		return
	}
	items, err := s.st.ListCatalogItems(r.Context(), string(scope), tenantID)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "KN-500", err.Error())
		return
	}
	out := make([]CatalogItem, 0, len(items))
	for _, it := range items {
		out = append(out, catalogItemToDTO(it))
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

// (GET /api/v1/catalog/{slug})
func (s *APIServer) GetApiV1CatalogSlug(w http.ResponseWriter, r *http.Request, slug string) {
	if !s.requireRoles(w, r, "admin", "ops", "tenantOwner", "projectDev", "readOnly") {
		return
	}
	if strings.TrimSpace(slug) == "" {
		s.writeError(w, http.StatusUnprocessableEntity, "KN-422", "slug required")
		return
	}
	item, err := s.st.GetCatalogItem(r.Context(), slug)
	if err != nil {
		s.writeError(w, http.StatusNotFound, "KN-404", "catalog item not found")
		return
	}
	dto := catalogItemToDTO(item)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(dto)
}

// (POST /api/v1/clusters/{c}/tenants/{t}/projects/{p}/catalog/install)
func (s *APIServer) PostApiV1ClustersCTenantsTProjectsPCatalogInstall(w http.ResponseWriter, r *http.Request, c ClusterParam, t TenantParam, p ProjectParam) {
	pr, err := s.st.GetProjectByID(r.Context(), p.String())
	if err != nil {
		s.writeError(w, http.StatusNotFound, "KN-404", "project not found")
		return
	}
	if !s.requireRolesTenant(w, r, pr.Tenant, "admin", "ops", "tenantOwner", "projectDev") {
		return
	}
	var in CatalogInstall
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		s.writeError(w, http.StatusUnprocessableEntity, "KN-422", "invalid payload")
		return
	}
	slug := strings.TrimSpace(in.Slug)
	if slug == "" {
		s.writeError(w, http.StatusUnprocessableEntity, "KN-422", "slug required")
		return
	}
	item, err := s.st.GetCatalogItem(r.Context(), slug)
	if err != nil {
		s.writeError(w, http.StatusNotFound, "KN-404", "catalog item not found")
		return
	}
	if item.Scope == string(CatalogItemScopeTenant) {
		if item.TenantID == nil || item.TenantID.String() != pr.Tenant {
			s.writeError(w, http.StatusNotFound, "KN-404", "catalog item not found")
			return
		}
	}
	source := mergeSourceMaps(item.Source, in.Source)
	if len(source) == 0 {
		s.writeError(w, http.StatusUnprocessableEntity, "KN-422", "source definition required")
		return
	}
	spec := catalogSourceToAppSpec(source)
	if spec == nil || spec.Source == nil {
		s.writeError(w, http.StatusUnprocessableEntity, "KN-422", "invalid catalog source")
		return
	}
	spec.Source.CatalogRef = &kn.AppCatalogRef{
		Name: slug,
	}
	if item.Version != nil {
		spec.Source.CatalogRef.Version = item.Version
	}
	catalogID := openapi_types.UUID(item.ID)
	spec.CatalogItemID = &catalogID
	spec.CatalogVersion = item.Version
	spec.CatalogOverrides = in.Source
	app := kn.App{
		ID:          kn.NewID(),
		Tenant:      pr.Tenant,
		Project:     pr.Name,
		Name:        slug,
		Description: item.Description,
		Spec:        spec,
		CreatedAt:   time.Now().UTC(),
	}
	if err := s.st.CreateApp(r.Context(), app); err != nil {
		s.writeError(w, http.StatusInternalServerError, "KN-500", err.Error())
		return
	}
	sourceKind := ""
	if spec.Source != nil {
		sourceKind = string(spec.Source.Kind)
	}
	projectNS := clusterpkg.AppNamespaceName(pr.Tenant, pr.Name)
	_ = s.ensureAppConfigMap(r.Context(), c.String(), projectNS, pr.Tenant, t.String(), p.String(), app.ID.String(), sourceKind, appToDTO(app))
	resp := map[string]string{"status": "accepted", "appSlug": slug}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(resp)
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

func generateProxyKubeconfig(server, namespace, token string) []byte {
	if strings.TrimSpace(server) == "" {
		server = "https://proxy.kubenova.svc"
	}
	nsLine := ""
	if strings.TrimSpace(namespace) != "" {
		nsLine = "    namespace: " + namespace + "\n"
	}
	cfg := "apiVersion: v1\nkind: Config\nclusters:\n- name: kn-proxy\n  cluster:\n    insecure-skip-tls-verify: true\n    server: " + server + "\ncontexts:\n- name: tenant\n  context:\n    cluster: kn-proxy\n    user: tenant-user\n" + nsLine + "current-context: tenant\nusers:\n- name: tenant-user\n  user:\n    token: " + token + "\n"
	return []byte(cfg)
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

func (s *APIServer) subjectFromReq(r *http.Request) string {
	hdr := r.Header.Get("Authorization")
	if hdr != "" && strings.HasPrefix(strings.ToLower(hdr), "bearer ") {
		tok := strings.TrimSpace(strings.TrimPrefix(hdr, "Bearer"))
		var claims jwt.MapClaims
		if _, err := jwt.ParseWithClaims(tok, &claims, func(token *jwt.Token) (interface{}, error) { return s.signingKey(), nil }); err == nil {
			if ssub, ok := claims["sub"].(string); ok {
				return ssub
			}
		}
	}
	if v := r.Header.Get("X-KN-Subject"); v != "" {
		return v
	}
	return ""
}

func (s *APIServer) tenantFromReq(r *http.Request) string {
	hdr := r.Header.Get("Authorization")
	if hdr != "" && strings.HasPrefix(strings.ToLower(hdr), "bearer ") {
		tok := strings.TrimSpace(strings.TrimPrefix(hdr, "Bearer"))
		var claims jwt.MapClaims
		if _, err := jwt.ParseWithClaims(tok, &claims, func(token *jwt.Token) (interface{}, error) { return s.signingKey(), nil }); err == nil {
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
	_, err := jwt.ParseWithClaims(tok, &claims, func(token *jwt.Token) (interface{}, error) { return s.signingKey(), nil })
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
	if strings.TrimSpace(in.Name) == "" || len(in.Kubeconfig) == 0 || strings.TrimSpace(in.CapsuleProxyUrl) == "" {
		s.writeError(w, http.StatusUnprocessableEntity, "KN-422", "name, kubeconfig and capsuleProxyUrl required")
		return
	}
	// Store encoded kubeconfig
	enc := base64.StdEncoding.EncodeToString(in.Kubeconfig)
	// Persist via store
	id, err := s.st.CreateCluster(r.Context(), toTypesCluster(in), enc)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "KN-500", err.Error())
		return
	}
	// Read back to include assigned ID/labels
	cl, encBack, _ := s.st.GetClusterByName(r.Context(), in.Name)
	now := time.Now().UTC()
	out := Cluster{Name: in.Name, CreatedAt: &now}
	if uid := uuidFromString(cl.ID.String()); uid != nil {
		out.Id = uid
	}
	if in.Labels != nil {
		out.Labels = in.Labels
	}
	// Kick off async Agent install; do not block API response
	if encBack != "" {
		kb, _ := base64.StdEncoding.DecodeString(encBack)
		image := strings.TrimSpace(os.Getenv("AGENT_IMAGE"))
		mgrURL := strings.TrimSpace(os.Getenv("MANAGER_URL_PUBLIC"))
		go func(cid kn.ID, kubeconfig []byte, img, url string) {
			// Best-effort eventing
			_ = s.st.AddEvents(context.Background(), &cid, []kn.Event{{Type: "cluster", Resource: "agent", Payload: map[string]any{"phase": "install_start"}, TS: time.Now().UTC()}})
			if img == "" {
				img = "ghcr.io/vaheed/kubenova/agent:latest"
			}
			if err := clusterpkg.InstallAgent(context.Background(), kubeconfig, img, url); err != nil {
				_ = s.st.AddEvents(context.Background(), &cid, []kn.Event{{Type: "cluster", Resource: "agent", Payload: map[string]any{"phase": "install_error", "error": err.Error()}, TS: time.Now().UTC()}})
				return
			}
			_ = s.st.AddEvents(context.Background(), &cid, []kn.Event{{Type: "cluster", Resource: "agent", Payload: map[string]any{"phase": "install_done"}, TS: time.Now().UTC()}})
		}(id, kb, image, mgrURL)
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
		if uid := uuidFromString(it.ID.String()); uid != nil {
			dto.Id = uid
		}
		if len(it.Labels) > 0 {
			m := it.Labels
			dto.Labels = &m
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
	uid := c.String()
	cl, enc, err := s.st.GetClusterByID(r.Context(), uid)
	if err != nil {
		s.writeError(w, http.StatusNotFound, "KN-404", "not found")
		return
	}
	kb, _ := base64.StdEncoding.DecodeString(enc)
	conds := clusterpkg.ComputeClusterConditions(r.Context(), kb, parseBool(os.Getenv("KUBENOVA_E2E_FAKE")))
	// map to DTO
	out := Cluster{Name: cl.Name}
	if uid := uuidFromString(cl.ID.String()); uid != nil {
		out.Id = uid
	}
	if !cl.CreatedAt.IsZero() {
		t := cl.CreatedAt
		out.CreatedAt = &t
	}
	if len(cl.Labels) > 0 {
		m := cl.Labels
		out.Labels = &m
	}
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
	// Treat path param as cluster ID for consistency with GET and other routes
	uid := c.String()
	cl, enc, err := s.st.GetClusterByID(r.Context(), uid)
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
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(s.policysetCatalog)
}

// (GET /api/v1/plans)
func (s *APIServer) GetApiV1Plans(w http.ResponseWriter, r *http.Request) {
	if !s.requireRoles(w, r, "admin", "ops", "tenantOwner", "projectDev", "readOnly") {
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(s.planCatalog)
}

// (GET /api/v1/plans/{name})
func (s *APIServer) GetApiV1PlansName(w http.ResponseWriter, r *http.Request, name string) {
	if !s.requireRoles(w, r, "admin", "ops", "tenantOwner", "projectDev", "readOnly") {
		return
	}
	for _, p := range s.planCatalog {
		if p.Name == name {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(p)
			return
		}
	}
	s.writeError(w, http.StatusNotFound, "KN-404", "not found")
}

// (GET /api/v1/clusters/{c}/tenants/{t}/policysets)
func (s *APIServer) GetApiV1ClustersCTenantsTPolicysets(w http.ResponseWriter, r *http.Request, c ClusterParam, t TenantParam) {
	ten, err := s.st.GetTenantByID(r.Context(), t.String())
	if err != nil {
		s.writeError(w, http.StatusNotFound, "KN-404", "not found")
		return
	}
	if !s.requireRolesTenant(w, r, ten.Name, "admin", "ops", "tenantOwner", "projectDev", "readOnly") {
		return
	}
	items, err := s.st.ListPolicySets(r.Context(), ten.ID.String())
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "KN-500", "store error")
		return
	}
	out := make([]PolicySet, 0, len(items))
	for _, ps := range items {
		dto, derr := decodePolicySet(ps)
		if derr != nil {
			s.writeError(w, http.StatusInternalServerError, "KN-500", "policyset decode error")
			return
		}
		out = append(out, dto)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

// (POST /api/v1/clusters/{c}/tenants/{t}/policysets)
func (s *APIServer) PostApiV1ClustersCTenantsTPolicysets(w http.ResponseWriter, r *http.Request, c ClusterParam, t TenantParam) {
	ten, err := s.st.GetTenantByID(r.Context(), t.String())
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
	ps, err := encodePolicySet(ten.ID.String(), body)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "KN-500", "policyset encode error")
		return
	}
	if err := s.st.CreatePolicySet(r.Context(), ps); err != nil {
		s.writeError(w, http.StatusInternalServerError, "KN-500", "store error")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(body)
}

// (GET /api/v1/clusters/{c}/tenants/{t}/policysets/{name})
func (s *APIServer) GetApiV1ClustersCTenantsTPolicysetsName(w http.ResponseWriter, r *http.Request, c ClusterParam, t TenantParam, name string) {
	ten, err := s.st.GetTenantByID(r.Context(), t.String())
	if err != nil {
		s.writeError(w, http.StatusNotFound, "KN-404", "not found")
		return
	}
	if !s.requireRolesTenant(w, r, ten.Name, "admin", "ops", "tenantOwner", "projectDev", "readOnly") {
		return
	}
	ps, err := s.st.GetPolicySet(r.Context(), ten.ID.String(), name)
	if err != nil {
		if err == store.ErrNotFound {
			s.writeError(w, http.StatusNotFound, "KN-404", "not found")
		} else {
			s.writeError(w, http.StatusInternalServerError, "KN-500", "store error")
		}
		return
	}
	dto, derr := decodePolicySet(ps)
	if derr != nil {
		s.writeError(w, http.StatusInternalServerError, "KN-500", "policyset decode error")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(dto)
}

// (PUT /api/v1/clusters/{c}/tenants/{t}/policysets/{name})
func (s *APIServer) PutApiV1ClustersCTenantsTPolicysetsName(w http.ResponseWriter, r *http.Request, c ClusterParam, t TenantParam, name string) {
	ten, err := s.st.GetTenantByID(r.Context(), t.String())
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
	// Path parameter is canonical for name
	if strings.TrimSpace(name) == "" {
		s.writeError(w, http.StatusUnprocessableEntity, "KN-422", "invalid payload")
		return
	}
	body.Name = name
	ps, err := encodePolicySet(ten.ID.String(), body)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "KN-500", "policyset encode error")
		return
	}
	if err := s.st.UpdatePolicySet(r.Context(), ps); err != nil {
		s.writeError(w, http.StatusInternalServerError, "KN-500", "store error")
		return
	}
	w.WriteHeader(http.StatusOK)
}

// (DELETE /api/v1/clusters/{c}/tenants/{t}/policysets/{name})
func (s *APIServer) DeleteApiV1ClustersCTenantsTPolicysetsName(w http.ResponseWriter, r *http.Request, c ClusterParam, t TenantParam, name string) {
	ten, err := s.st.GetTenantByID(r.Context(), t.String())
	if err != nil {
		s.writeError(w, http.StatusNotFound, "KN-404", "not found")
		return
	}
	if !s.requireRolesTenant(w, r, ten.Name, "admin", "ops", "tenantOwner") {
		return
	}
	_ = s.st.DeletePolicySet(r.Context(), ten.ID.String(), name)
	w.WriteHeader(http.StatusNoContent)
}

// --- Bootstrap ---

// (POST /api/v1/clusters/{c}/bootstrap/{component})
func (s *APIServer) PostApiV1ClustersCBootstrapComponent(w http.ResponseWriter, r *http.Request, c ClusterParam, component string) {
	if !s.requireRoles(w, r, "admin", "ops") {
		return
	}
	// Validate cluster exists
	if _, _, err := s.st.GetClusterByID(r.Context(), c.String()); err != nil {
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
	clusterID := c.String()
	role := "projectDev"
	scope := "project"
	tenantName := ""
	projectName := ""
	tenantIDStr := ""
	projectIDStr := ""
	report := func(status, reason string, expires *time.Time) {
		publishKubeconfigEvent(r.Context(), scope, clusterID, tenantName, tenantIDStr, projectName, projectIDStr, role, status, reason, expires)
	}

	// ensure cluster, tenant, and project exist (uid-based lookups)
	cl, enc, err := s.st.GetClusterByID(r.Context(), clusterID)
	if err != nil || enc == "" {
		s.writeError(w, http.StatusNotFound, "KN-404", "cluster not found")
		report("failed", "cluster not found", nil)
		return
	}
	ten, err := s.st.GetTenantByID(r.Context(), t.String())
	if err != nil {
		s.writeError(w, http.StatusNotFound, "KN-404", "not found")
		report("failed", "tenant not found", nil)
		return
	}
	tenantName = ten.Name
	tenantIDStr = ten.ID.String()
	if !s.requireRolesTenant(w, r, ten.Name, "admin", "ops", "tenantOwner", "projectDev") {
		report("failed", "forbidden", nil)
		return
	}
	pr, err := s.st.GetProjectByID(r.Context(), p.String())
	if err != nil {
		s.writeError(w, http.StatusNotFound, "KN-404", "not found")
		report("failed", "project not found", nil)
		return
	}
	projectName = pr.Name
	projectIDStr = pr.ID.String()
	kb, derr := base64.StdEncoding.DecodeString(enc)
	if derr != nil {
		s.writeError(w, http.StatusInternalServerError, "KN-500", "cluster kubeconfig decode error")
		report("failed", "cluster kubeconfig decode error", nil)
		return
	}
	// Derive proxy configuration from cluster labels.
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
	ttl := 3600
	cfgBytes, expTime, err := clusterpkg.IssueProjectKubeconfig(r.Context(), kb, proxyURL, proxyCA, ten.Name, pr.Name, role, ttl)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "KN-500", err.Error())
		report("failed", err.Error(), nil)
		return
	}
	exp := expTime.UTC()
	report("issued", "", &exp)
	resp := KubeconfigResponse{Kubeconfig: &cfgBytes, ExpiresAt: &exp}
	filename := fmt.Sprintf("kubeconfig-%s-%s-%s.yaml", ten.Name, pr.Name, cl.Name)
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// (POST /api/v1/tenants/{t}/kubeconfig)
func (s *APIServer) PostApiV1TenantsTKubeconfig(w http.ResponseWriter, r *http.Request, t TenantParam) {
	scope := "project"
	role := "readOnly"
	tenantName := ""
	projectName := ""
	tenantIDStr := ""
	projectIDStr := ""
	clusterID := ""
	report := func(status, reason string, expires *time.Time) {
		publishKubeconfigEvent(r.Context(), scope, clusterID, tenantName, tenantIDStr, projectName, projectIDStr, role, status, reason, expires)
	}

	ten, err := s.st.GetTenantByID(r.Context(), t.String())
	if err != nil {
		s.writeError(w, http.StatusNotFound, "KN-404", "not found")
		report("failed", "tenant not found", nil)
		return
	}
	tenantName = ten.Name
	tenantIDStr = ten.ID.String()
	if !s.requireRolesTenant(w, r, ten.Name, "admin", "ops", "tenantOwner", "projectDev") {
		report("failed", "forbidden", nil)
		return
	}
	// Optional grant parameters: project (by name), role, ttlSeconds
	var body struct {
		Project    *string `json:"project,omitempty"`
		Role       *string `json:"role,omitempty"`
		TtlSeconds *int    `json:"ttlSeconds,omitempty"`
	}
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			s.writeError(w, http.StatusUnprocessableEntity, "KN-422", "invalid payload")
			return
		}
	}
	// Validate role when provided; actual RBAC enforcement happens in capsule-proxy.
	if body.Role != nil {
		role = strings.TrimSpace(*body.Role)
		switch role {
		case "tenantOwner", "projectDev", "readOnly":
			// ok
		case "":
			// treat empty as omitted
		default:
			s.writeError(w, http.StatusUnprocessableEntity, "KN-422", "invalid payload")
			report("failed", "invalid role", nil)
			return
		}
	}
	ttl := 0
	if body.TtlSeconds != nil {
		if *body.TtlSeconds < 0 || *body.TtlSeconds > 315360000 {
			s.writeError(w, http.StatusUnprocessableEntity, "KN-422", "invalid payload")
			return
		}
		ttl = *body.TtlSeconds
	}
	// If a project name is provided, scope the kubeconfig to that project's namespace.
	ns := ""
	if body.Project != nil && strings.TrimSpace(*body.Project) != "" {
		prName := strings.TrimSpace(*body.Project)
		pr, perr := s.st.GetProject(r.Context(), ten.Name, prName)
		if perr != nil {
			s.writeError(w, http.StatusNotFound, "KN-404", "project not found")
			return
		}
		ns = pr.Name
		projectName = pr.Name
		projectIDStr = pr.ID.String()
	}
	// Enforce that projectDev kubeconfigs are always scoped to a project namespace.
	if body.Role != nil && strings.TrimSpace(*body.Role) == "projectDev" && ns == "" {
		s.writeError(w, http.StatusUnprocessableEntity, "KN-422", "project required for projectDev role")
		return
	}
	// Determine effective role for the proxy token.
	if body.Role != nil && strings.TrimSpace(*body.Role) != "" {
		role = strings.TrimSpace(*body.Role)
	}
	if ns == "" {
		s.writeError(w, http.StatusUnprocessableEntity, "KN-422", "project required for kubeconfig issuance")
		report("failed", "project required", nil)
		return
	}
	// Resolve primary cluster for the tenant and derive proxy configuration.
	clusterID = ten.Labels["kubenova.cluster"]
	if strings.TrimSpace(clusterID) == "" {
		s.writeError(w, http.StatusUnprocessableEntity, "KN-422", "tenant has no primary cluster")
		report("failed", "tenant has no primary cluster", nil)
		return
	}
	cl, enc, err := s.st.GetClusterByID(r.Context(), clusterID)
	if err != nil || enc == "" {
		s.writeError(w, http.StatusNotFound, "KN-404", "cluster not found")
		report("failed", "cluster not found", nil)
		return
	}
	kb, derr := base64.StdEncoding.DecodeString(enc)
	if derr != nil {
		s.writeError(w, http.StatusInternalServerError, "KN-500", "cluster kubeconfig decode error")
		report("failed", "cluster kubeconfig decode error", nil)
		return
	}
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
	cfgBytes, expTime, err := clusterpkg.IssueProjectKubeconfig(r.Context(), kb, proxyURL, proxyCA, ten.Name, ns, role, ttl)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "KN-500", err.Error())
		report("failed", err.Error(), nil)
		return
	}
	expPtr := expTime.UTC()
	report("issued", "", &expPtr)
	resp := KubeconfigResponse{Kubeconfig: &cfgBytes, ExpiresAt: &expPtr}
	filename := fmt.Sprintf("kubeconfig-%s-%s-%s.yaml", ten.Name, ns, cl.Name)
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// (POST /api/v1/tenants/{t}/sandbox)
func (s *APIServer) PostApiV1TenantsTSandbox(w http.ResponseWriter, r *http.Request, t TenantParam) {
	ten, err := s.st.GetTenantByID(r.Context(), t.String())
	if err != nil {
		s.writeError(w, http.StatusNotFound, "KN-404", "not found")
		return
	}
	if !s.requireRolesTenant(w, r, ten.Name, "admin", "ops", "tenantOwner") {
		return
	}
	var body struct {
		Name       string `json:"name"`
		TtlSeconds *int   `json:"ttlSeconds,omitempty"`
	}
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			s.writeError(w, http.StatusUnprocessableEntity, "KN-422", "invalid payload")
			return
		}
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		s.writeError(w, http.StatusUnprocessableEntity, "KN-422", "sandbox name required")
		return
	}
	if _, err := s.st.GetSandbox(r.Context(), ten.Name, name); err == nil {
		s.writeError(w, http.StatusConflict, "KN-409", "sandbox already exists")
		return
	} else if err != nil && err != store.ErrNotFound {
		s.writeError(w, http.StatusInternalServerError, "KN-500", err.Error())
		return
	}
	clusterID := ten.Labels["kubenova.cluster"]
	if strings.TrimSpace(clusterID) == "" {
		s.writeError(w, http.StatusUnprocessableEntity, "KN-422", "tenant has no primary cluster")
		return
	}
	cl, enc, err := s.st.GetClusterByID(r.Context(), clusterID)
	if err != nil || enc == "" {
		s.writeError(w, http.StatusNotFound, "KN-404", "cluster not found")
		return
	}
	kb, derr := base64.StdEncoding.DecodeString(enc)
	if derr != nil {
		s.writeError(w, http.StatusInternalServerError, "KN-500", "cluster kubeconfig decode error")
		return
	}
	if err := clusterpkg.EnsureSandboxNamespace(r.Context(), kb, ten.Name, name); err != nil {
		s.writeError(w, http.StatusInternalServerError, "KN-500", err.Error())
		return
	}
	sb := kn.Sandbox{
		Tenant:    ten.Name,
		Name:      name,
		Namespace: clusterpkg.SandboxNamespaceName(ten.Name, name),
		CreatedAt: time.Now().UTC(),
	}
	if err := s.st.CreateSandbox(r.Context(), sb); err != nil {
		s.writeError(w, http.StatusInternalServerError, "KN-500", err.Error())
		return
	}
	ttl := 0
	if body.TtlSeconds != nil && *body.TtlSeconds >= 0 {
		ttl = *body.TtlSeconds
	}
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
	kcfg, exp, err := clusterpkg.IssueSandboxKubeconfig(r.Context(), kb, proxyURL, proxyCA, ten.Name, name, ttl)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "KN-500", err.Error())
		return
	}
	resp := map[string]any{
		"id":         sb.ID.String(),
		"tenant":     sb.Tenant,
		"name":       sb.Name,
		"namespace":  sb.Namespace,
		"kubeconfig": kcfg,
		"expiresAt":  exp.UTC(),
		"createdAt":  sb.CreatedAt.UTC(),
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// --- Usage ---

// (GET /api/v1/tenants/{t}/usage)
func (s *APIServer) GetApiV1TenantsTUsage(w http.ResponseWriter, r *http.Request, t TenantParam, params GetApiV1TenantsTUsageParams) {
	ten, err := s.st.GetTenantByID(r.Context(), t.String())
	if err != nil {
		s.writeError(w, http.StatusNotFound, "KN-404", "not found")
		return
	}
	window := "24h"
	if params.Range != nil {
		window = string(*params.Range)
	}
	resp := UsageReport{Window: &window}
	// Prefer the tenant's primary cluster (kubenova.cluster label), fall back to first registered cluster.
	clusterID := ten.Labels["kubenova.cluster"]
	if clusterID != "" {
		if _, enc, err2 := s.st.GetClusterByID(r.Context(), clusterID); err2 == nil {
			kb, _ := base64.StdEncoding.DecodeString(enc)
			if u, err3 := clusterpkg.TenantUsage(r.Context(), kb, ten.Name); err3 == nil {
				if u.CPU != "" {
					resp.Cpu = &u.CPU
				}
				if u.Memory != "" {
					resp.Memory = &u.Memory
				}
				if u.Pods > 0 {
					p := int(u.Pods)
					resp.Pods = &p
				}
			}
		}
	} else if clusters, _, err := s.st.ListClusters(r.Context(), 100, "", ""); err == nil && len(clusters) > 0 {
		if _, enc, err2 := s.st.GetClusterByID(r.Context(), clusters[0].ID.String()); err2 == nil {
			kb, _ := base64.StdEncoding.DecodeString(enc)
			if u, err3 := clusterpkg.TenantUsage(r.Context(), kb, ten.Name); err3 == nil {
				if u.CPU != "" {
					resp.Cpu = &u.CPU
				}
				if u.Memory != "" {
					resp.Memory = &u.Memory
				}
				if u.Pods > 0 {
					p := int(u.Pods)
					resp.Pods = &p
				}
			}
		}
	}
	// Fallback stub values if no real usage data was populated.
	if resp.Cpu == nil && resp.Memory == nil && resp.Pods == nil {
		cpu, mem := "2", "4Gi"
		pods := 12
		resp.Cpu = &cpu
		resp.Memory = &mem
		resp.Pods = &pods
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// (GET /api/v1/projects/{p}/usage)
func (s *APIServer) GetApiV1ProjectsPUsage(w http.ResponseWriter, r *http.Request, p ProjectParam, params GetApiV1ProjectsPUsageParams) {
	pr, err := s.st.GetProjectByID(r.Context(), p.String())
	if err != nil {
		s.writeError(w, http.StatusNotFound, "KN-404", "not found")
		return
	}
	window := "24h"
	if params.Range != nil {
		window = string(*params.Range)
	}
	resp := UsageReport{Window: &window}
	// Resolve primary cluster via tenant label when available; fallback to first cluster.
	clusterID := ""
	if ten, err := s.st.GetTenant(r.Context(), pr.Tenant); err == nil {
		clusterID = ten.Labels["kubenova.cluster"]
	}
	if clusterID != "" {
		if _, enc, err2 := s.st.GetClusterByID(r.Context(), clusterID); err2 == nil {
			kb, _ := base64.StdEncoding.DecodeString(enc)
			if u, err3 := clusterpkg.ProjectUsage(r.Context(), kb, clusterpkg.AppNamespaceName(pr.Tenant, pr.Name)); err3 == nil {
				if u.CPU != "" {
					resp.Cpu = &u.CPU
				}
				if u.Memory != "" {
					resp.Memory = &u.Memory
				}
				if u.Pods > 0 {
					pp := int(u.Pods)
					resp.Pods = &pp
				}
			}
		}
	} else if clusters, _, err := s.st.ListClusters(r.Context(), 100, "", ""); err == nil && len(clusters) > 0 {
		if _, enc, err2 := s.st.GetClusterByID(r.Context(), clusters[0].ID.String()); err2 == nil {
			kb, _ := base64.StdEncoding.DecodeString(enc)
			if u, err3 := clusterpkg.ProjectUsage(r.Context(), kb, clusterpkg.AppNamespaceName(pr.Tenant, pr.Name)); err3 == nil {
				if u.CPU != "" {
					resp.Cpu = &u.CPU
				}
				if u.Memory != "" {
					resp.Memory = &u.Memory
				}
				if u.Pods > 0 {
					pp := int(u.Pods)
					resp.Pods = &pp
				}
			}
		}
	}
	if resp.Cpu == nil && resp.Memory == nil && resp.Pods == nil {
		cpu, mem := "1", "1Gi"
		pods := 5
		resp.Cpu = &cpu
		resp.Memory = &mem
		resp.Pods = &pods
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// --- Workflows ---

// (POST /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}/workflow/run)
func (s *APIServer) PostApiV1ClustersCTenantsTProjectsPAppsAWorkflowRun(w http.ResponseWriter, r *http.Request, c ClusterParam, t TenantParam, p ProjectParam, a AppParam) {
	id := "run-" + strings.ReplaceAll(time.Now().UTC().Format("20060102T150405.000000000"), ".", "")
	now := time.Now().UTC()
	run := WorkflowRun{Id: &id, Status: ptrWorkflowRunStatus(WorkflowRunStatusRunning), StartedAt: &now}
	key := t.String() + "|" + p.String() + "|" + a.String()
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
	key := t.String() + "|" + p.String() + "|" + a.String()
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
	if !s.requireRolesTenant(w, r, t.String(), "admin", "ops", "tenantOwner", "projectDev") {
		return
	}
	var kb []byte
	if _, enc, err := s.st.GetClusterByID(r.Context(), c.String()); err == nil {
		kb, _ = base64.StdEncoding.DecodeString(enc)
	}
	if err := s.newVela(kb).DeleteApp(r.Context(), p.String(), a.String()); err != nil {
		s.writeError(w, http.StatusInternalServerError, "KN-500", err.Error())
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
	owner := ""
	if params.Owner != nil {
		owner = string(*params.Owner)
	}
	selectors := map[string]string{}
	if params.LabelSelector != nil {
		raw := string(*params.LabelSelector)
		if raw != "" {
			parts := strings.Split(raw, ",")
			for _, p := range parts {
				kv := strings.SplitN(strings.TrimSpace(p), "=", 2)
				if len(kv) == 2 {
					selectors[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
				}
			}
		}
	}
	matches := func(t kn.Tenant) bool {
		if owner != "" {
			ok := false
			for _, o := range t.Owners {
				if o == owner {
					ok = true
					break
				}
			}
			if !ok {
				return false
			}
		}
		if len(selectors) > 0 {
			for k, v := range selectors {
				if t.Labels[k] != v {
					return false
				}
			}
		}
		return true
	}
	out := make([]Tenant, 0, len(items))
	for _, t := range items {
		if !matches(t) {
			continue
		}
		tn := Tenant{Name: t.Name, Labels: &t.Labels, Annotations: &t.Annotations}
		if uid := uuidFromString(t.ID.String()); uid != nil {
			tn.Id = uid
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
	if t.Labels == nil {
		t.Labels = map[string]string{}
	}
	// Record the primary cluster ID for this tenant for usage/kubeconfig lookups.
	t.Labels["kubenova.cluster"] = c.String()
	if err := s.st.CreateTenant(r.Context(), t); err != nil {
		s.writeError(w, http.StatusInternalServerError, "KN-500", err.Error())
		return
	}
	// read back to capture ID and optionally apply a tenant plan / bootstrap Capsule
	if t.Name != "" {
		if tt, e := s.st.GetTenant(r.Context(), t.Name); e == nil {
			if uid := uuidFromString(tt.ID.String()); uid != nil {
				in.Id = uid
			}

			// Track which plan (if any) we actually applied, so we can surface it in the response.
			appliedPlan := ""

			// If a plan was requested at creation time, apply it now and surface errors to the caller.
			if in.Plan != nil && strings.TrimSpace(*in.Plan) != "" {
				planName := strings.TrimSpace(*in.Plan)
				if _, err := s.applyPlanToTenant(r.Context(), tt.ID.String(), planName); err != nil {
					s.writeError(w, http.StatusInternalServerError, "KN-500", err.Error())
					return
				}
				appliedPlan = planName
			} else if defaultTenantPlanName != "" {
				// No explicit plan provided: best-effort apply the default plan when present in the catalog.
				if _, err := s.applyPlanToTenant(r.Context(), tt.ID.String(), defaultTenantPlanName); err != nil {
					// Default plan application should not break tenant creation; log and continue.
					logging.FromContext(r.Context()).Error("tenant.default_plan.apply_failed", zap.Error(err))
				} else {
					appliedPlan = defaultTenantPlanName
				}
			}

			// If no plan was applied (explicit or default), best-effort ensure a Capsule Tenant exists.
			if appliedPlan == "" {
				var kb []byte
				if _, enc, err := s.st.GetClusterByID(r.Context(), c.String()); err == nil {
					kb, _ = base64.StdEncoding.DecodeString(enc)
				}
				if len(kb) > 0 {
					caps := s.newCapsule(kb)
					if err := caps.EnsureTenant(r.Context(), tt.Name, tt.Owners, tt.Labels); err != nil {
						logging.FromContext(r.Context()).Error("tenant.ensure_capsule_failed", zap.Error(err))
					}
				}
			}

			// Reflect the effective plan (if any) on the response DTO.
			if appliedPlan != "" {
				planName := appliedPlan
				in.Plan = &planName
			}
		}
	}
	now := time.Now().UTC()
	in.CreatedAt = &now
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(in)
}

// (GET /api/v1/clusters/{c}/tenants/{t})
func (s *APIServer) GetApiV1ClustersCTenantsT(w http.ResponseWriter, r *http.Request, c ClusterParam, t TenantParam) {
	item, err := s.st.GetTenantByID(r.Context(), t.String())
	if err != nil {
		s.writeError(w, http.StatusNotFound, "KN-404", "not found")
		return
	}
	if !s.requireRolesTenant(w, r, item.Name, "admin", "ops", "tenantOwner", "projectDev", "readOnly") {
		return
	}
	out := Tenant{Name: item.Name, Labels: &item.Labels, Annotations: &item.Annotations}
	if uid := uuidFromString(item.ID.String()); uid != nil {
		out.Id = uid
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

// (DELETE /api/v1/clusters/{c}/tenants/{t})
func (s *APIServer) DeleteApiV1ClustersCTenantsT(w http.ResponseWriter, r *http.Request, c ClusterParam, t TenantParam) {
	item, err := s.st.GetTenantByID(r.Context(), t.String())
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
	item, err := s.st.GetTenantByID(r.Context(), t.String())
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
	ten, err := s.st.GetTenantByID(r.Context(), t.String())
	if err != nil {
		s.writeError(w, http.StatusNotFound, "KN-404", "not found")
		return
	}
	if !s.requireRolesTenant(w, r, ten.Name, "admin", "ops", "tenantOwner") {
		return
	}
	var body map[string]string
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.writeError(w, http.StatusUnprocessableEntity, "KN-422", "invalid payload")
		return
	}
	var kb []byte
	if _, enc, err := s.st.GetClusterByID(r.Context(), c.String()); err == nil {
		kb, _ = base64.StdEncoding.DecodeString(enc)
	}
	caps := s.newCapsule(kb)
	if err := caps.EnsureTenant(r.Context(), ten.Name, ten.Owners, ten.Labels); err != nil {
		s.writeError(w, http.StatusInternalServerError, "KN-500", err.Error())
		return
	}
	if err := caps.SetTenantQuotas(r.Context(), ten.Name, body); err != nil {
		s.writeError(w, http.StatusInternalServerError, "KN-500", err.Error())
		return
	}
	w.WriteHeader(http.StatusOK)
}

// (PUT /api/v1/clusters/{c}/tenants/{t}/limits)
func (s *APIServer) PutApiV1ClustersCTenantsTLimits(w http.ResponseWriter, r *http.Request, c ClusterParam, t TenantParam) {
	ten, err := s.st.GetTenantByID(r.Context(), t.String())
	if err != nil {
		s.writeError(w, http.StatusNotFound, "KN-404", "not found")
		return
	}
	if !s.requireRolesTenant(w, r, ten.Name, "admin", "ops", "tenantOwner") {
		return
	}
	var body map[string]string
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.writeError(w, http.StatusUnprocessableEntity, "KN-422", "invalid payload")
		return
	}
	var kb []byte
	if _, enc, err := s.st.GetClusterByID(r.Context(), c.String()); err == nil {
		kb, _ = base64.StdEncoding.DecodeString(enc)
	}
	caps := s.newCapsule(kb)
	if err := caps.EnsureTenant(r.Context(), ten.Name, ten.Owners, ten.Labels); err != nil {
		s.writeError(w, http.StatusInternalServerError, "KN-500", err.Error())
		return
	}
	if err := caps.SetTenantLimits(r.Context(), ten.Name, body); err != nil {
		s.writeError(w, http.StatusInternalServerError, "KN-500", err.Error())
		return
	}
	w.WriteHeader(http.StatusOK)
}

// (PUT /api/v1/clusters/{c}/tenants/{t}/network-policies)
func (s *APIServer) PutApiV1ClustersCTenantsTNetworkPolicies(w http.ResponseWriter, r *http.Request, c ClusterParam, t TenantParam) {
	ten, err := s.st.GetTenantByID(r.Context(), t.String())
	if err != nil {
		s.writeError(w, http.StatusNotFound, "KN-404", "not found")
		return
	}
	if !s.requireRolesTenant(w, r, ten.Name, "admin", "ops", "tenantOwner") {
		return
	}
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.writeError(w, http.StatusUnprocessableEntity, "KN-422", "invalid payload")
		return
	}
	var kb []byte
	if _, enc, err := s.st.GetClusterByID(r.Context(), c.String()); err == nil {
		kb, _ = base64.StdEncoding.DecodeString(enc)
	}
	caps := s.newCapsule(kb)
	if err := caps.EnsureTenant(r.Context(), ten.Name, ten.Owners, ten.Labels); err != nil {
		s.writeError(w, http.StatusInternalServerError, "KN-500", err.Error())
		return
	}
	if err := caps.SetTenantNetworkPolicies(r.Context(), ten.Name, body); err != nil {
		s.writeError(w, http.StatusInternalServerError, "KN-500", err.Error())
		return
	}
	w.WriteHeader(http.StatusOK)
}

// (GET /api/v1/clusters/{c}/tenants/{t}/summary)
func (s *APIServer) GetApiV1ClustersCTenantsTSummary(w http.ResponseWriter, r *http.Request, c ClusterParam, t TenantParam) {
	ten, err := s.st.GetTenantByID(r.Context(), t.String())
	if err != nil {
		s.writeError(w, http.StatusNotFound, "KN-404", "not found")
		return
	}
	if !s.requireRolesTenant(w, r, ten.Name, "admin", "ops", "tenantOwner", "projectDev", "readOnly") {
		return
	}
	_, enc, err := s.st.GetClusterByID(r.Context(), c.String())
	if err != nil {
		s.writeError(w, http.StatusNotFound, "KN-404", "cluster not found")
		return
	}
	kb, _ := base64.StdEncoding.DecodeString(enc)
	sum, err := s.newCapsule(kb).TenantSummary(r.Context(), ten.Name)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "KN-500", err.Error())
		return
	}
	// Best-effort usage aggregation: reuse the same cluster kubeconfig and compute usage across tenant namespaces.
	usageMap := map[string]string{}
	if u, err := clusterpkg.TenantUsage(r.Context(), kb, ten.Name); err == nil {
		if u.CPU != "" {
			usageMap["cpu"] = u.CPU
		}
		if u.Memory != "" {
			usageMap["memory"] = u.Memory
		}
		if u.Pods > 0 {
			usageMap["pods"] = fmt.Sprintf("%d", u.Pods)
		}
	}
	resp := TenantSummary{}
	if len(sum.Namespaces) > 0 {
		ns := sum.Namespaces
		resp.Namespaces = &ns
	}
	if len(sum.Quotas) > 0 {
		q := sum.Quotas
		resp.Quotas = &q
	}
	if len(usageMap) > 0 {
		u := usageMap
		resp.Usages = &u
	}
	// Include plan name if present on tenant annotations.
	if ten.Annotations != nil {
		if p, ok := ten.Annotations["kubenova.io/plan"]; ok && strings.TrimSpace(p) != "" {
			plan := strings.TrimSpace(p)
			resp.Plan = &plan
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// --- Projects ---

// (GET /api/v1/clusters/{c}/tenants/{t}/projects)
func (s *APIServer) GetApiV1ClustersCTenantsTProjects(w http.ResponseWriter, r *http.Request, c ClusterParam, t TenantParam) {
	ten, err := s.st.GetTenantByID(r.Context(), t.String())
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
		if uid := uuidFromString(pc.ID.String()); uid != nil {
			pr.Id = uid
		}
		out = append(out, pr)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

// (POST /api/v1/clusters/{c}/tenants/{t}/projects)
func (s *APIServer) PostApiV1ClustersCTenantsTProjects(w http.ResponseWriter, r *http.Request, c ClusterParam, t TenantParam) {
	ten, err := s.st.GetTenantByID(r.Context(), t.String())
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
	// Best-effort: ensure a project namespace exists and is labeled for Capsule.
	if _, enc, err := s.st.GetClusterByID(r.Context(), c.String()); err == nil {
		if kb, decErr := base64.StdEncoding.DecodeString(enc); decErr == nil {
			_ = clusterpkg.EnsureProjectNamespace(r.Context(), kb, ten.Name, in.Name)
		}
	}
	if pr2, e := s.st.GetProject(r.Context(), pr.Tenant, pr.Name); e == nil {
		if uid := uuidFromString(pr2.ID.String()); uid != nil {
			in.Id = uid
		}
	}
	in.CreatedAt = &pr.CreatedAt
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(in)
}

// (GET /api/v1/clusters/{c}/tenants/{t}/projects/{p})
func (s *APIServer) GetApiV1ClustersCTenantsTProjectsP(w http.ResponseWriter, r *http.Request, c ClusterParam, t TenantParam, p ProjectParam) {
	ten, err := s.st.GetTenantByID(r.Context(), t.String())
	if err != nil {
		s.writeError(w, http.StatusNotFound, "KN-404", "not found")
		return
	}
	if !s.requireRolesTenant(w, r, ten.Name, "admin", "ops", "tenantOwner", "projectDev", "readOnly") {
		return
	}
	pr, err := s.st.GetProjectByID(r.Context(), p.String())
	if err != nil {
		s.writeError(w, http.StatusNotFound, "KN-404", "not found")
		return
	}
	out := Project{Name: pr.Name, CreatedAt: &pr.CreatedAt}
	if uid := uuidFromString(pr.ID.String()); uid != nil {
		out.Id = uid
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

// (PUT /api/v1/clusters/{c}/tenants/{t}/projects/{p})
func (s *APIServer) PutApiV1ClustersCTenantsTProjectsP(w http.ResponseWriter, r *http.Request, c ClusterParam, t TenantParam, p ProjectParam) {
	ten, err := s.st.GetTenantByID(r.Context(), t.String())
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
	prResolved, _ := s.st.GetProjectByID(r.Context(), p.String())
	pr := kn.Project{Tenant: ten.Name, Name: prResolved.Name, CreatedAt: time.Now().UTC()}
	_ = s.st.UpdateProject(r.Context(), pr)
	w.WriteHeader(http.StatusOK)
}

// (DELETE /api/v1/clusters/{c}/tenants/{t}/projects/{p})
func (s *APIServer) DeleteApiV1ClustersCTenantsTProjectsP(w http.ResponseWriter, r *http.Request, c ClusterParam, t TenantParam, p ProjectParam) {
	ten, err := s.st.GetTenantByID(r.Context(), t.String())
	if err != nil {
		s.writeError(w, http.StatusNotFound, "KN-404", "not found")
		return
	}
	if !s.requireRolesTenant(w, r, ten.Name, "admin", "ops", "tenantOwner") {
		return
	}
	pr, err := s.st.GetProjectByID(r.Context(), p.String())
	if err == nil {
		_ = s.st.DeleteProject(r.Context(), ten.Name, pr.Name)
	}
	w.WriteHeader(http.StatusNoContent)
}

// (PUT /api/v1/clusters/{c}/tenants/{t}/projects/{p}/access)
func (s *APIServer) PutApiV1ClustersCTenantsTProjectsPAccess(w http.ResponseWriter, r *http.Request, c ClusterParam, t TenantParam, p ProjectParam) {
	ten, err := s.st.GetTenantByID(r.Context(), t.String())
	if err != nil {
		s.writeError(w, http.StatusNotFound, "KN-404", "not found")
		return
	}
	if !s.requireRolesTenant(w, r, ten.Name, "admin", "ops", "tenantOwner") {
		return
	}
	var body ProjectAccess
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.writeError(w, http.StatusUnprocessableEntity, "KN-422", "invalid payload")
		return
	}
	members := []clusterpkg.ProjectAccessMember{}
	if body.Members != nil {
		for _, m := range *body.Members {
			if strings.TrimSpace(m.Subject) == "" {
				continue
			}
			members = append(members, clusterpkg.ProjectAccessMember{
				Subject: m.Subject,
				Role:    string(m.Role),
			})
		}
	}
	_, enc, err := s.st.GetClusterByID(r.Context(), c.String())
	if err != nil {
		s.writeError(w, http.StatusNotFound, "KN-404", "cluster not found")
		return
	}
	pr, err := s.st.GetProjectByID(r.Context(), p.String())
	if err != nil {
		s.writeError(w, http.StatusNotFound, "KN-404", "not found")
		return
	}
	kb, _ := base64.StdEncoding.DecodeString(enc)
	nsName := clusterpkg.AppNamespaceName(ten.Name, pr.Name)
	if err := clusterpkg.EnsureProjectAccess(r.Context(), kb, nsName, ten.Name, pr.Name, members); err != nil {
		s.writeError(w, http.StatusInternalServerError, "KN-500", err.Error())
		return
	}
	w.WriteHeader(http.StatusOK)
}

// --- Apps ---

// (GET /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps)
func (s *APIServer) GetApiV1ClustersCTenantsTProjectsPApps(w http.ResponseWriter, r *http.Request, c ClusterParam, t TenantParam, p ProjectParam, params GetApiV1ClustersCTenantsTProjectsPAppsParams) {
	ten, err := s.st.GetTenantByID(r.Context(), t.String())
	if err != nil {
		s.writeError(w, http.StatusNotFound, "KN-404", "not found")
		return
	}
	if !s.requireRolesTenant(w, r, ten.Name, "admin", "ops", "tenantOwner", "projectDev", "readOnly") {
		return
	}
	pr, err := s.st.GetProjectByID(r.Context(), p.String())
	if err != nil {
		s.writeError(w, http.StatusNotFound, "KN-404", "not found")
		return
	}
	items, err := s.st.ListApps(r.Context(), ten.Name, pr.Name)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "KN-500", err.Error())
		return
	}
	out := make([]App, 0, len(items))
	for _, a := range items {
		out = append(out, appToDTO(a))
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

// (POST /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps)
func (s *APIServer) PostApiV1ClustersCTenantsTProjectsPApps(w http.ResponseWriter, r *http.Request, c ClusterParam, t TenantParam, p ProjectParam) {
	pr, err := s.st.GetProjectByID(r.Context(), p.String())
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
	a := kn.App{
		ID:          kn.NewID(),
		Tenant:      pr.Tenant,
		Project:     pr.Name,
		Name:        in.Name,
		Description: in.Description,
		Components:  in.Components,
		Traits:      in.Traits,
		Policies:    in.Policies,
		Spec:        appSpecFromHTTP(in.Spec),
		CreatedAt:   time.Now().UTC(),
	}
	if err := s.st.CreateApp(r.Context(), a); err != nil {
		s.writeError(w, http.StatusInternalServerError, "KN-500", err.Error())
		return
	}
	sourceKind := ""
	if in.Spec != nil && in.Spec.Source != nil {
		sourceKind = string(in.Spec.Source.Kind)
	}
	projectNS := clusterpkg.AppNamespaceName(pr.Tenant, pr.Name)
	// Best-effort: materialize an App ConfigMap in the project namespace so the
	// in-cluster AppReconciler can project it into a Vela Application.
	_ = s.ensureAppConfigMap(r.Context(), c.String(), projectNS, pr.Tenant, t.String(), p.String(), a.ID.String(), sourceKind, in)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(appToDTO(a))
}

// (GET /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a})
func (s *APIServer) GetApiV1ClustersCTenantsTProjectsPAppsA(w http.ResponseWriter, r *http.Request, c ClusterParam, t TenantParam, p ProjectParam, a AppParam) {
	it, err := s.st.GetAppByID(r.Context(), a.String())
	if err != nil {
		s.writeError(w, http.StatusNotFound, "KN-404", "not found")
		return
	}
	if !s.requireRolesTenant(w, r, it.Tenant, "admin", "ops", "tenantOwner", "projectDev", "readOnly") {
		return
	}
	out := appToDTO(it)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

// (PUT /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a})
func (s *APIServer) PutApiV1ClustersCTenantsTProjectsPAppsA(w http.ResponseWriter, r *http.Request, c ClusterParam, t TenantParam, p ProjectParam, a AppParam) {
	ap, err := s.st.GetAppByID(r.Context(), a.String())
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
	item := kn.App{
		ID:          ap.ID,
		Tenant:      ap.Tenant,
		Project:     ap.Project,
		Name:        ap.Name,
		Description: in.Description,
		Components:  in.Components,
		Traits:      in.Traits,
		Policies:    in.Policies,
		Spec:        appSpecFromHTTP(in.Spec),
		CreatedAt:   ap.CreatedAt,
	}
	_ = s.st.UpdateApp(r.Context(), item)
	// Best-effort update of the App ConfigMap so AppReconciler sees the latest
	// components/traits/policies.
	sourceKind := ""
	if in.Spec != nil && in.Spec.Source != nil {
		sourceKind = string(in.Spec.Source.Kind)
	}
	projectNS := clusterpkg.AppNamespaceName(ap.Tenant, ap.Project)
	_ = s.ensureAppConfigMap(r.Context(), c.String(), projectNS, ap.Tenant, t.String(), p.String(), ap.ID.String(), sourceKind, in)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(appToDTO(item))
}

// (DELETE /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a})
func (s *APIServer) DeleteApiV1ClustersCTenantsTProjectsPAppsA(w http.ResponseWriter, r *http.Request, c ClusterParam, t TenantParam, p ProjectParam, a AppParam) {
	ap, err := s.st.GetAppByID(r.Context(), a.String())
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
	if !s.requireRolesTenant(w, r, t.String(), "admin", "ops", "tenantOwner", "projectDev") {
		return
	}
	_, enc, err := s.st.GetClusterByID(r.Context(), c.String())
	if err != nil {
		s.writeError(w, http.StatusNotFound, "KN-404", "cluster not found")
		return
	}
	kb, _ := base64.StdEncoding.DecodeString(enc)
	pr, err := s.st.GetProjectByID(r.Context(), p.String())
	if err != nil {
		s.writeError(w, http.StatusNotFound, "KN-404", "project not found")
		return
	}
	ap, err := s.st.GetAppByID(r.Context(), a.String())
	if err != nil {
		s.writeError(w, http.StatusNotFound, "KN-404", "app not found")
		return
	}
	namespace := clusterpkg.AppNamespaceName(pr.Tenant, pr.Name)
	// Apply any attached PolicySets as traits/policies before deploy.
	if err := s.applyPolicySets(r.Context(), kb, t.String(), pr.Name, ap.Name); err != nil {
		s.writeError(w, http.StatusInternalServerError, "KN-500", err.Error())
		return
	}
	if err := s.newVela(kb).Deploy(r.Context(), namespace, ap.Name); err != nil {
		s.writeError(w, http.StatusInternalServerError, "KN-500", err.Error())
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

// (POST /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}:suspend)
func (s *APIServer) PostApiV1ClustersCTenantsTProjectsPAppsASuspend(w http.ResponseWriter, r *http.Request, c ClusterParam, t TenantParam, p ProjectParam, a AppParam) {
	if !s.requireRolesTenant(w, r, t.String(), "admin", "ops", "tenantOwner", "projectDev") {
		return
	}
	var kb []byte
	if _, enc, err := s.st.GetClusterByID(r.Context(), c.String()); err == nil {
		kb, _ = base64.StdEncoding.DecodeString(enc)
	}
	pr, _ := s.st.GetProjectByID(r.Context(), p.String())
	ap, _ := s.st.GetAppByID(r.Context(), a.String())
	namespace := clusterpkg.AppNamespaceName(pr.Tenant, pr.Name)
	if err := s.newVela(kb).Suspend(r.Context(), namespace, ap.Name); err != nil {
		s.writeError(w, http.StatusInternalServerError, "KN-500", err.Error())
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

// (POST /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}:resume)
func (s *APIServer) PostApiV1ClustersCTenantsTProjectsPAppsAResume(w http.ResponseWriter, r *http.Request, c ClusterParam, t TenantParam, p ProjectParam, a AppParam) {
	if !s.requireRolesTenant(w, r, t.String(), "admin", "ops", "tenantOwner", "projectDev") {
		return
	}
	var kb []byte
	if _, enc, err := s.st.GetClusterByID(r.Context(), c.String()); err == nil {
		kb, _ = base64.StdEncoding.DecodeString(enc)
	}
	pr, _ := s.st.GetProjectByID(r.Context(), p.String())
	ap, _ := s.st.GetAppByID(r.Context(), a.String())
	namespace := clusterpkg.AppNamespaceName(pr.Tenant, pr.Name)
	if err := s.newVela(kb).Resume(r.Context(), namespace, ap.Name); err != nil {
		s.writeError(w, http.StatusInternalServerError, "KN-500", err.Error())
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

// (POST /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}:rollback)
func (s *APIServer) PostApiV1ClustersCTenantsTProjectsPAppsARollback(w http.ResponseWriter, r *http.Request, c ClusterParam, t TenantParam, p ProjectParam, a AppParam) {
	if !s.requireRolesTenant(w, r, t.String(), "admin", "ops", "tenantOwner", "projectDev") {
		return
	}
	var kb []byte
	if _, enc, err := s.st.GetClusterByID(r.Context(), c.String()); err == nil {
		kb, _ = base64.StdEncoding.DecodeString(enc)
	}
	var body struct {
		ToRevision *int `json:"toRevision"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	pr, _ := s.st.GetProjectByID(r.Context(), p.String())
	ap, _ := s.st.GetAppByID(r.Context(), a.String())
	namespace := clusterpkg.AppNamespaceName(pr.Tenant, pr.Name)
	if err := s.newVela(kb).Rollback(r.Context(), namespace, ap.Name, body.ToRevision); err != nil {
		s.writeError(w, http.StatusInternalServerError, "KN-500", err.Error())
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

// (GET /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}/status)
func (s *APIServer) GetApiV1ClustersCTenantsTProjectsPAppsAStatus(w http.ResponseWriter, r *http.Request, c ClusterParam, t TenantParam, p ProjectParam, a AppParam) {
	if !s.requireRolesTenant(w, r, t.String(), "admin", "ops", "tenantOwner", "projectDev", "readOnly") {
		return
	}
	_, enc, err := s.st.GetClusterByID(r.Context(), c.String())
	if err != nil {
		s.writeError(w, http.StatusNotFound, "KN-404", "cluster not found")
		return
	}
	kb, _ := base64.StdEncoding.DecodeString(enc)
	pr, _ := s.st.GetProjectByID(r.Context(), p.String())
	ap, _ := s.st.GetAppByID(r.Context(), a.String())
	namespace := clusterpkg.AppNamespaceName(pr.Tenant, pr.Name)
	st, err := s.newVela(kb).Status(r.Context(), namespace, ap.Name)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "KN-500", err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(st)
}

// (GET /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}/revisions)
func (s *APIServer) GetApiV1ClustersCTenantsTProjectsPAppsARevisions(w http.ResponseWriter, r *http.Request, c ClusterParam, t TenantParam, p ProjectParam, a AppParam) {
	if !s.requireRolesTenant(w, r, t.String(), "admin", "ops", "tenantOwner", "projectDev", "readOnly") {
		return
	}
	_, enc, err := s.st.GetClusterByID(r.Context(), c.String())
	if err != nil {
		s.writeError(w, http.StatusNotFound, "KN-404", "cluster not found")
		return
	}
	kb, _ := base64.StdEncoding.DecodeString(enc)
	pr, _ := s.st.GetProjectByID(r.Context(), p.String())
	ap, _ := s.st.GetAppByID(r.Context(), a.String())
	namespace := clusterpkg.AppNamespaceName(pr.Tenant, pr.Name)
	revs, err := s.newVela(kb).Revisions(r.Context(), namespace, ap.Name)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "KN-500", err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(revs)
}

// (GET /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}/diff/{revA}/{revB})
func (s *APIServer) GetApiV1ClustersCTenantsTProjectsPAppsADiffRevARevB(w http.ResponseWriter, r *http.Request, c ClusterParam, t TenantParam, p ProjectParam, a AppParam, revA int, revB int) {
	if !s.requireRolesTenant(w, r, t.String(), "admin", "ops", "tenantOwner", "projectDev", "readOnly") {
		return
	}
	_, enc, err := s.st.GetClusterByID(r.Context(), c.String())
	if err != nil {
		s.writeError(w, http.StatusNotFound, "KN-404", "cluster not found")
		return
	}
	kb, _ := base64.StdEncoding.DecodeString(enc)
	pr, _ := s.st.GetProjectByID(r.Context(), p.String())
	ap, _ := s.st.GetAppByID(r.Context(), a.String())
	namespace := clusterpkg.AppNamespaceName(pr.Tenant, pr.Name)
	d, err := s.newVela(kb).Diff(r.Context(), namespace, ap.Name, revA, revB)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "KN-500", err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(d)
}

// (GET /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}/logs/{component})
func (s *APIServer) GetApiV1ClustersCTenantsTProjectsPAppsALogsComponent(w http.ResponseWriter, r *http.Request, c ClusterParam, t TenantParam, p ProjectParam, a AppParam, component string, params GetApiV1ClustersCTenantsTProjectsPAppsALogsComponentParams) {
	if !s.requireRolesTenant(w, r, t.String(), "admin", "ops", "tenantOwner", "projectDev", "readOnly") {
		return
	}
	_, enc, err := s.st.GetClusterByID(r.Context(), c.String())
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
	pr, _ := s.st.GetProjectByID(r.Context(), p.String())
	ap, _ := s.st.GetAppByID(r.Context(), a.String())
	namespace := clusterpkg.AppNamespaceName(pr.Tenant, pr.Name)
	lines, err := s.newVela(kb).Logs(ctx, namespace, ap.Name, component, follow)
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
	_ = json.NewEncoder(w).Encode(map[string]any{"version": "0.9.6"})
}

func (s *APIServer) GetApiV1Features(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	resp := map[string]any{
		"tenancy": true,
		"vela":    true,
		"proxy":   true,
	}
	if strings.TrimSpace(defaultTenantPlanName) != "" {
		resp["defaultTenantPlan"] = defaultTenantPlanName
	}
	if len(s.planCatalog) > 0 {
		names := make([]string, 0, len(s.planCatalog))
		for _, p := range s.planCatalog {
			if strings.TrimSpace(p.Name) != "" {
				names = append(names, p.Name)
			}
		}
		if len(names) > 0 {
			resp["availablePlans"] = names
		}
	}
	_ = json.NewEncoder(w).Encode(resp)
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
	// Map KubeNova roles to Kubernetes group names expected by capsule-proxy and tenant discovery RBAC.
	groupsSet := map[string]struct{}{}
	for _, ro := range roles {
		switch ro {
		case "admin", "ops", "tenantOwner":
			groupsSet["tenant-admins"] = struct{}{}
		case "projectDev":
			groupsSet["tenant-maintainers"] = struct{}{}
		case "readOnly":
			groupsSet["tenant-viewers"] = struct{}{}
		}
	}
	var groups []string
	for g := range groupsSet {
		groups = append(groups, g)
	}
	c := jwt.MapClaims{"sub": req.Subject, "roles": roles, "exp": time.Now().Add(time.Duration(ttl) * time.Second).Unix()}
	if len(groups) > 0 {
		c["groups"] = groups
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, c)
	ss, err := tok.SignedString(s.signingKey())
	if err != nil {
		s.writeError(w, 500, "KN-500", "sign failure")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"token": ss, "expiresAt": time.Now().Add(time.Duration(ttl) * time.Second).UTC()})
}

func (s *APIServer) GetApiV1Me(w http.ResponseWriter, r *http.Request) {
	roles := s.rolesFromReq(r)
	subject := ""
	hdr := r.Header.Get("Authorization")
	if hdr != "" && strings.HasPrefix(strings.ToLower(hdr), "bearer ") {
		tok := strings.TrimSpace(strings.TrimPrefix(hdr, "Bearer"))
		var claims jwt.MapClaims
		if _, err := jwt.ParseWithClaims(tok, &claims, func(token *jwt.Token) (interface{}, error) { return s.signingKey(), nil }); err == nil {
			if ssub, ok := claims["sub"].(string); ok {
				subject = ssub
			}
		}
	}
	// Allow overriding subject in tests/dev without a real JWT.
	if subject == "" {
		if v := r.Header.Get("X-KN-Subject"); v != "" {
			subject = v
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"subject": subject, "roles": roles})
}

// (PUT /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}/traits)
func (s *APIServer) PutApiV1ClustersCTenantsTProjectsPAppsATraits(w http.ResponseWriter, r *http.Request, c ClusterParam, t TenantParam, p ProjectParam, a AppParam) {
	if !s.requireRolesTenant(w, r, t.String(), "admin", "ops", "tenantOwner", "projectDev") {
		return
	}
	var body []map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.writeError(w, http.StatusUnprocessableEntity, "KN-422", "invalid payload")
		return
	}
	_, enc, err := s.st.GetClusterByID(r.Context(), c.String())
	if err != nil {
		s.writeError(w, http.StatusNotFound, "KN-404", "cluster not found")
		return
	}
	kb, _ := base64.StdEncoding.DecodeString(enc)
	pr, _ := s.st.GetProjectByID(r.Context(), p.String())
	ap, _ := s.st.GetAppByID(r.Context(), a.String())
	namespace := clusterpkg.AppNamespaceName(pr.Tenant, pr.Name)
	if err := s.newVela(kb).SetTraits(r.Context(), namespace, ap.Name, body); err != nil {
		s.writeError(w, http.StatusInternalServerError, "KN-500", err.Error())
		return
	}
	w.WriteHeader(http.StatusOK)
}

// (PUT /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}/policies)
func (s *APIServer) PutApiV1ClustersCTenantsTProjectsPAppsAPolicies(w http.ResponseWriter, r *http.Request, c ClusterParam, t TenantParam, p ProjectParam, a AppParam) {
	if !s.requireRolesTenant(w, r, t.String(), "admin", "ops", "tenantOwner", "projectDev") {
		return
	}
	var body []map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.writeError(w, http.StatusUnprocessableEntity, "KN-422", "invalid payload")
		return
	}
	_, enc, err := s.st.GetClusterByID(r.Context(), c.String())
	if err != nil {
		s.writeError(w, http.StatusNotFound, "KN-404", "cluster not found")
		return
	}
	kb, _ := base64.StdEncoding.DecodeString(enc)
	pr2, _ := s.st.GetProjectByID(r.Context(), p.String())
	ap2, _ := s.st.GetAppByID(r.Context(), a.String())
	namespace := clusterpkg.AppNamespaceName(pr2.Tenant, pr2.Name)
	if err := s.newVela(kb).SetPolicies(r.Context(), namespace, ap2.Name, body); err != nil {
		s.writeError(w, http.StatusInternalServerError, "KN-500", err.Error())
		return
	}
	w.WriteHeader(http.StatusOK)
}

// (POST /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}/image-update)
func (s *APIServer) PostApiV1ClustersCTenantsTProjectsPAppsAImageUpdate(w http.ResponseWriter, r *http.Request, c ClusterParam, t TenantParam, p ProjectParam, a AppParam) {
	if !s.requireRolesTenant(w, r, t.String(), "admin", "ops", "tenantOwner", "projectDev") {
		return
	}
	var body PostApiV1ClustersCTenantsTProjectsPAppsAImageUpdateJSONBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.writeError(w, http.StatusUnprocessableEntity, "KN-422", "invalid payload")
		return
	}
	_, enc, err := s.st.GetClusterByID(r.Context(), c.String())
	if err != nil {
		s.writeError(w, http.StatusNotFound, "KN-404", "cluster not found")
		return
	}
	kb, _ := base64.StdEncoding.DecodeString(enc)
	tag := ""
	if body.Tag != nil {
		tag = *body.Tag
	}
	pr3, _ := s.st.GetProjectByID(r.Context(), p.String())
	ap3, _ := s.st.GetAppByID(r.Context(), a.String())
	namespace := clusterpkg.AppNamespaceName(pr3.Tenant, pr3.Name)
	if err := s.newVela(kb).ImageUpdate(r.Context(), namespace, ap3.Name, body.Component, body.Image, tag); err != nil {
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
	if strings.TrimSpace(in.CapsuleProxyUrl) != "" {
		out.Labels["kubenova.capsuleProxyUrl"] = strings.TrimSpace(in.CapsuleProxyUrl)
	}
	if in.CapsuleProxyCa != nil && len(*in.CapsuleProxyCa) > 0 {
		out.Labels["kubenova.capsuleProxyCa"] = base64.StdEncoding.EncodeToString(*in.CapsuleProxyCa)
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
	if in.Plan != nil && strings.TrimSpace(*in.Plan) != "" {
		if out.Annotations == nil {
			out.Annotations = map[string]string{}
		}
		out.Annotations["kubenova.io/plan"] = strings.TrimSpace(*in.Plan)
	}
	return out
}
