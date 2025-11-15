package reconcile

import (
	"context"
	"encoding/json"

	"github.com/vaheed/kubenova/internal/backends/vela"
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
//       namespace: <project-namespace>
//       labels:
//         kubenova.app:     <app-name>
//         kubenova.tenant:  <tenant-name>
//         kubenova.project: <project-name>
//   - data:
//       spec:      JSON object with the base Application spec (components, etc.)
//       traits:    JSON array of trait objects (optional)
//       policies:  JSON array of policy objects (optional)
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
		return reconcile.Result{}, nil
	}
	tenant := cm.Labels["kubenova.tenant"]
	project := cm.Labels["kubenova.project"]

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

	if r.newVela == nil {
		cfg := ctrl.GetConfigOrDie()
		r.newVela = func() vela.Client { return vela.NewFromRESTConfig(cfg) }
	}
	backend := r.newVela()

	ns := cm.Namespace
	if ns == "" {
		ns = "default"
	}

	if err := backend.EnsureApp(ctx, ns, appName, spec); err != nil {
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
		"type":      "app",
		"tenant":    tenant,
		"project":   project,
		"name":      appName,
		"namespace": ns,
		"operation": "reconciled",
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
