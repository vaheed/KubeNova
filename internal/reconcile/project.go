package reconcile

import (
	"context"

	"github.com/vaheed/kubenova/internal/cluster"
	"github.com/vaheed/kubenova/internal/logging"
	"github.com/vaheed/kubenova/internal/telemetry"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1api "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"time"
)

// ProjectReconciler uses a Namespace as the projection for a Project.
// It ensures the namespace exists and is labeled for tenant isolation.
type ProjectReconciler struct{ client.Client }

func (r *ProjectReconciler) Reconcile(ctx context.Context, req ctrl.Request) (reconcile.Result, error) {
	const finalizer = "kubenova.io/finalizer-project"
	ns := &corev1.Namespace{}
	if err := r.Get(ctx, req.NamespacedName, ns); err == nil {
		if ns.Labels != nil && ns.Labels[cluster.LabelSandbox] == "true" {
			return reconcile.Result{}, nil
		}
		if cluster.IsSandboxNamespace(ns.Name) {
			return reconcile.Result{}, nil
		}
		// Handle deletion finalizer
		if !ns.DeletionTimestamp.IsZero() {
			if controllerutil.ContainsFinalizer(ns, finalizer) {
				controllerutil.RemoveFinalizer(ns, finalizer)
				_ = r.Update(ctx, ns)
			}
			return reconcile.Result{}, nil
		}
		// ensure finalizer
		if !controllerutil.ContainsFinalizer(ns, finalizer) {
			controllerutil.AddFinalizer(ns, finalizer)
			if err := r.Update(ctx, ns); err != nil {
				return reconcile.Result{RequeueAfter: 10 * time.Second}, nil
			}
		}
		// labels
		if ns.Labels == nil {
			ns.Labels = map[string]string{}
		}
		changed := false
		project := ns.Labels[cluster.LabelProject]
		if project == "" {
			if _, parsedProject, ok := cluster.ParseAppNamespace(ns.Name); ok {
				project = parsedProject
			} else {
				project = ns.Name
			}
			ns.Labels[cluster.LabelProject] = project
			changed = true
		}
		tenant := ns.Labels[cluster.LabelTenant]
		if tenant == "" {
			if parsedTenant, _, ok := cluster.ParseAppNamespace(ns.Name); ok {
				tenant = parsedTenant
			}
			if tenant == "" {
				tenant = "default"
			}
			ns.Labels[cluster.LabelTenant] = tenant
			changed = true
		}
		if ns.Labels["capsule.clastix.io/tenant"] != tenant {
			ns.Labels["capsule.clastix.io/tenant"] = tenant
			changed = true
		}
		if ns.Labels[cluster.LabelNamespaceType] != cluster.NamespaceTypeApp {
			ns.Labels[cluster.LabelNamespaceType] = cluster.NamespaceTypeApp
			changed = true
		}
		if changed {
			_ = r.Update(ctx, ns)
		}
		// ensure Capsule Tenant exists using unstructured
		if err := ensureCapsuleTenant(ctx, r.Client, tenant); err != nil {
			// If Capsule CRDs are not installed yet, requeue quietly while bootstrap completes.
			if metav1api.IsNoMatchError(err) {
				logging.FromContext(ctx).With(zap.String("adapter", "capsule"), zap.String("tenant", tenant)).Info("waiting for Capsule CRDs; will retry")
				return reconcile.Result{RequeueAfter: 15 * time.Second}, nil
			}
			logging.FromContext(ctx).With(zap.String("adapter", "capsule"), zap.String("tenant", tenant)).Error("ensure tenant", zap.Error(err))
			return reconcile.Result{RequeueAfter: 10 * time.Second}, nil
		}
		logging.FromContext(ctx).With(zap.String("adapter", "capsule"), zap.String("tenant", tenant)).Info("tenant ensured")
		telemetry.PublishEvent(map[string]any{
			"type":      "namespace",
			"tenant":    tenant,
			"project":   ns.Labels[cluster.LabelProject],
			"name":      ns.Name,
			"operation": "reconciled",
		})
		return reconcile.Result{}, nil
	}
	// Create namespace if missing
	if cluster.IsSandboxNamespace(req.Name) {
		return reconcile.Result{}, nil
	}
	ns = &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: req.Name,
			Labels: map[string]string{
				cluster.LabelProject:        req.Name,
				cluster.LabelTenant:         "default",
				"capsule.clastix.io/tenant": "default",
				cluster.LabelNamespaceType:  cluster.NamespaceTypeApp,
			},
		},
	}
	if err := r.Create(ctx, ns); err != nil {
		return reconcile.Result{RequeueAfter: 10 * time.Second}, nil
	}
	return reconcile.Result{}, nil
}

func (r *ProjectReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Watch namespaces only; cheap and available in every cluster
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Namespace{}).
		Complete(r)
}

func ensureCapsuleTenant(ctx context.Context, c client.Client, name string) error {
	gvk := schema.GroupVersionKind{Group: "capsule.clastix.io", Version: "v1beta2", Kind: "Tenant"}
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(gvk)
	if err := c.Get(ctx, types.NamespacedName{Name: name}, u); err == nil {
		return nil
	}
	u.Object = map[string]interface{}{
		"apiVersion": gvk.Group + "/" + gvk.Version,
		"kind":       gvk.Kind,
		"metadata":   map[string]interface{}{"name": name},
		"spec":       map[string]interface{}{"owners": []interface{}{}},
	}
	return c.Create(ctx, u)
}
