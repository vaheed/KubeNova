package reconcile

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/vaheed/kubenova/internal/backends/vela"
	"github.com/vaheed/kubenova/internal/cluster"
	"github.com/vaheed/kubenova/internal/logging"
	"github.com/vaheed/kubenova/internal/telemetry"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// AppReconciler projects the KubeNova App model, encoded in ConfigMaps, onto
// real KubeVela Application resources per tenant/project. This keeps the
// controller-runtime wiring small while still using the shared vela backend.
//
// Contract for ConfigMaps:
//   - kind: ConfigMap
//   - metadata:
//     namespace: <project-namespace>
//     labels:
//     kubenova.app:     <app-name>
//     kubenova.tenant:  <tenant-name>
//     kubenova.project: <project-name>
//   - data:
//     spec:      JSON object with the base Application spec (components, etc.)
//     traits:    JSON array of trait objects (optional)
//     policies:  JSON array of policy objects (optional)
//
// This representation can be produced by the manager or other controllers and
// is treated as the in-cluster projection of the App DTO.
type AppReconciler struct {
	client.Client
	newVela func() vela.Client
}

func (r *AppReconciler) Reconcile(ctx context.Context, req ctrl.Request) (reconcile.Result, error) {
	log := logging.FromContext(ctx).With(
		zap.String("reconciler", "app"),
		zap.String("namespace", req.Namespace),
		zap.String("name", req.Name),
	)

	cm := &corev1.ConfigMap{}
	if err := r.Get(ctx, req.NamespacedName, cm); err != nil {
		if apierrors.IsNotFound(err) {
			// ConfigMap deleted; nothing to project. Deletion semantics for the
			// corresponding Vela Application are handled via higher-level APIs.
			return reconcile.Result{}, nil
		}
		log.With(zap.Error(err)).Error("get configmap")
		return reconcile.Result{}, nil
	}
	if cm.Labels == nil {
		return reconcile.Result{}, nil
	}
	appName := cm.Labels["kubenova.app"]
	if appName == "" {
		appName = cm.Name
	}
	if appName == "" {
		return reconcile.Result{}, nil
	}
	tenant := cm.Labels["kubenova.tenant"]
	if tenant == "" {
		tenant = cm.Labels["kubenova.io/tenant-id"]
	}
	project := cm.Labels["kubenova.project"]
	if project == "" {
		project = cm.Labels["kubenova.io/project-id"]
	}
	appID := cm.Labels["kubenova.io/app-id"]
	tenantID := cm.Labels["kubenova.io/tenant-id"]
	projectID := cm.Labels["kubenova.io/project-id"]
	sourceKind := cm.Labels["kubenova.io/source-kind"]

	log = log.With(
		zap.String("tenant", tenant),
		zap.String("tenant_id", tenantID),
		zap.String("project", project),
		zap.String("project_id", projectID),
		zap.String("app", appName),
		zap.String("app_id", appID),
		zap.String("source_kind", sourceKind),
	)

	spec := map[string]any{}
	if raw := cm.Data["spec"]; raw != "" {
		if err := json.Unmarshal([]byte(raw), &spec); err != nil {
			log.With(zap.Error(err)).Error("decode app spec")
		}
	}

	var traits []map[string]any
	if raw := cm.Data["traits"]; raw != "" {
		if err := json.Unmarshal([]byte(raw), &traits); err != nil {
			log.With(zap.Error(err)).Error("decode traits")
		}
	}
	var policies []map[string]any
	if raw := cm.Data["policies"]; raw != "" {
		if err := json.Unmarshal([]byte(raw), &policies); err != nil {
			log.With(zap.Error(err)).Error("decode policies")
		}
	}

	if sourceKind == "" {
		if raw, ok := spec["source"]; ok {
			if m, ok := raw.(map[string]any); ok {
				if kind, ok := m["kind"].(string); ok && kind != "" {
					sourceKind = kind
				}
			}
		}
	}

	catalogVersion, _ := spec["catalogVersion"].(string)
	catalogItemID, _ := spec["catalogItemId"].(string)
	var catalogOverrides map[string]any
	if raw, ok := spec["catalogOverrides"]; ok {
		if m, ok := raw.(map[string]any); ok {
			catalogOverrides = m
		}
	}
	log = log.With(
		zap.String("catalog_version", catalogVersion),
		zap.String("catalog_item_id", catalogItemID),
		zap.Any("catalog_overrides", catalogOverrides),
	)

	if r.newVela == nil {
		cfg := ctrl.GetConfigOrDie()
		r.newVela = func() vela.Client { return vela.NewFromRESTConfig(cfg) }
	}
	backend := r.newVela()

	ns := cm.Namespace
	if ns == "" {
		ns = "default"
	}
	nsObj := &corev1.Namespace{}
	if err := r.Client.Get(ctx, client.ObjectKey{Name: ns}, nsObj); err == nil {
		if nsObj.Labels != nil {
			if val, ok := nsObj.Labels[cluster.LabelSandbox]; ok && val == "true" {
				return reconcile.Result{}, nil
			}
		}
	}

	meta := map[string]string{}
	addMeta := func(key, value string) {
		if v := strings.TrimSpace(value); v != "" {
			meta[key] = v
		}
	}
	addMeta("kubenova.app", cm.Labels["kubenova.app"])
	addMeta("kubenova.tenant", cm.Labels["kubenova.tenant"])
	addMeta("kubenova.project", cm.Labels["kubenova.project"])
	addMeta("kubenova.io/app-id", cm.Labels["kubenova.io/app-id"])
	addMeta("kubenova.io/tenant-id", cm.Labels["kubenova.io/tenant-id"])
	addMeta("kubenova.io/project-id", cm.Labels["kubenova.io/project-id"])
	addMeta("kubenova.io/source-kind", sourceKind)
	if len(meta) == 0 {
		meta = nil
	}
	applySourceSecretRefs(spec, ns)
	if err := backend.EnsureApp(ctx, ns, appName, spec, meta); err != nil {
		log.With(zap.Error(err)).Error("ensure app")
		return reconcile.Result{}, nil
	}
	if len(traits) > 0 {
		if err := backend.SetTraits(ctx, ns, appName, traits); err != nil {
			log.With(zap.Error(err)).Error("set traits")
			return reconcile.Result{}, nil
		}
	}
	if len(policies) > 0 {
		if err := backend.SetPolicies(ctx, ns, appName, policies); err != nil {
			log.With(zap.Error(err)).Error("set policies")
			return reconcile.Result{}, nil
		}
	}

	log.With(
		zap.String("adapter", "vela"),
		zap.String("tenant", tenant),
		zap.String("project", project),
		zap.String("app", appName),
	).Info("app projected to vela")
	telemetry.PublishEvent(map[string]any{
		"type":             "app",
		"tenant":           tenant,
		"tenantId":         tenantID,
		"project":          project,
		"projectId":        projectID,
		"app":              appName,
		"appId":            appID,
		"name":             appName,
		"namespace":        ns,
		"sourceKind":       sourceKind,
		"catalogVersion":   catalogVersion,
		"catalogItemId":    catalogItemID,
		"catalogOverrides": catalogOverrides,
		"operation":        "reconciled",
	})
	return reconcile.Result{}, nil
}

func (r *AppReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.newVela == nil {
		cfg := mgr.GetConfig()
		r.newVela = func() vela.Client { return vela.NewFromRESTConfig(cfg) }
	}
	// Watch ConfigMaps in all namespaces that describe KubeNova Apps.
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.ConfigMap{}).
		Complete(r)
}

func applySourceSecretRefs(spec map[string]any, namespace string) {
	if spec == nil {
		return
	}
	source, _ := spec["source"].(map[string]any)
	if source == nil {
		return
	}
	if secretRef, ok := findSecretRef(source, "containerImage"); ok {
		applyImagePullSecrets(spec, secretRef)
	}
	if secretRef, ok := findSecretRef(source, "helmHttp"); ok {
		applyComponentSecretRef(spec, namespace, secretRef, isHelmComponent)
	}
	if secretRef, ok := findSecretRef(source, "helmOci"); ok {
		applyComponentSecretRef(spec, namespace, secretRef, isHelmComponent)
	}
	if secretRef, ok := findSecretRef(source, "gitRepo"); ok {
		applyComponentSecretRef(spec, namespace, secretRef, isGitComponent)
	}
}

func findSecretRef(source map[string]any, key string) (map[string]any, bool) {
	if raw, ok := source[key].(map[string]any); ok {
		if ref, ok2 := raw["credentialsSecretRef"].(map[string]any); ok2 && ref != nil {
			if name, _ := ref["name"].(string); strings.TrimSpace(name) != "" {
				return ref, true
			}
		}
	}
	return nil, false
}

func applyImagePullSecrets(spec map[string]any, ref map[string]any) {
	name := strings.TrimSpace(getString(ref, "name"))
	if name == "" {
		return
	}
	secretEntry := map[string]string{"name": name}
	if ns := strings.TrimSpace(getString(ref, "namespace")); ns != "" {
		secretEntry["namespace"] = ns
	}
	components, ok := spec["components"].([]any)
	if !ok {
		return
	}
	for _, compRaw := range components {
		if comp, ok := compRaw.(map[string]any); ok {
			props := ensureMap(comp, "properties")
			props["imagePullSecrets"] = []map[string]string{secretEntry}
		}
	}
}

func applyComponentSecretRef(spec map[string]any, namespace string, ref map[string]any, matches func(string) bool) {
	name := strings.TrimSpace(getString(ref, "name"))
	if name == "" {
		return
	}
	propsSecret := map[string]any{"name": name}
	if ns := strings.TrimSpace(getString(ref, "namespace")); ns != "" {
		propsSecret["namespace"] = ns
	} else if namespace != "" {
		propsSecret["namespace"] = namespace
	}
	components, ok := spec["components"].([]any)
	if !ok {
		return
	}
	for _, compRaw := range components {
		comp, ok := compRaw.(map[string]any)
		if !ok {
			continue
		}
		if compType, _ := comp["type"].(string); matches(compType) {
			props := ensureMap(comp, "properties")
			props["credentialsSecretRef"] = propsSecret
			break
		}
	}
}

func ensureMap(parent map[string]any, key string) map[string]any {
	if parent[key] == nil {
		parent[key] = map[string]any{}
	}
	if m, ok := parent[key].(map[string]any); ok {
		return m
	}
	return map[string]any{}
}

func getString(m map[string]any, key string) string {
	v, _ := m[key].(string)
	return v
}

func isHelmComponent(t string) bool {
	return strings.Contains(strings.ToLower(t), "helm")
}

func isGitComponent(t string) bool {
	return strings.Contains(strings.ToLower(t), "git")
}
