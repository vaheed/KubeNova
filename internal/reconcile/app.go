package reconcile

import (
    "context"

    corev1 "k8s.io/api/core/v1"
    "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
    "k8s.io/apimachinery/pkg/runtime/schema"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// AppReconciler would translate Apps to Vela Applications.
// Here it's a placeholder so we keep controller runtime wiring small.
type AppReconciler struct { client.Client }

func (r *AppReconciler) Reconcile(ctx context.Context, req ctrl.Request) (reconcile.Result, error) {
    // Watch ConfigMaps labeled kubenova.app=<name> and create a Vela Application
    cm := &corev1.ConfigMap{}
    if err := r.Get(ctx, req.NamespacedName, cm); err != nil {
        return reconcile.Result{}, nil
    }
    if cm.Labels["kubenova.app"] == "" { return reconcile.Result{}, nil }
    name := cm.Labels["kubenova.app"]
    image := cm.Data["image"]

    gvk := schema.GroupVersionKind{Group: "core.oam.dev", Version: "v1beta1", Kind: "Application"}
    app := &unstructured.Unstructured{}
    app.SetGroupVersionKind(gvk)
    app.SetNamespace(cm.Namespace)
    app.SetName(name)
    app.Object = map[string]interface{}{
        "apiVersion": gvk.Group + "/" + gvk.Version,
        "kind": gvk.Kind,
        "metadata": map[string]interface{}{ "name": name, "namespace": cm.Namespace },
        "spec": map[string]interface{}{
            "components": []interface{}{ map[string]interface{}{ "name": name, "type":"webservice", "properties": map[string]interface{}{"image": image} } },
        },
    }
    // Create or update
    err := r.Create(ctx, app)
    if err != nil {
        // On conflict try update
        _ = r.Update(ctx, app)
    }
    return reconcile.Result{}, nil
}

func (r *AppReconciler) SetupWithManager(mgr ctrl.Manager) error {
    // Watch ConfigMaps in all namespaces to drive Application projection
    return ctrl.NewControllerManagedBy(mgr).For(&corev1.ConfigMap{}).Complete(r)
}
