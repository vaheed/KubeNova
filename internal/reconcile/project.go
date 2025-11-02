package reconcile

import (
    "context"
    "time"
    corev1 "k8s.io/api/core/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
    "k8s.io/apimachinery/pkg/runtime/schema"
    "k8s.io/apimachinery/pkg/types"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
    "sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// ProjectReconciler uses a Namespace as the projection for a Project.
// It ensures the namespace exists and is labeled for tenant isolation.
type ProjectReconciler struct { client.Client }

func (r *ProjectReconciler) Reconcile(ctx context.Context, req ctrl.Request) (reconcile.Result, error) {
    const finalizer = "kubenova.io/finalizer-project"
    ns := &corev1.Namespace{}
    if err := r.Get(ctx, req.NamespacedName, ns); err == nil {
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
            if err := r.Update(ctx, ns); err != nil { return reconcile.Result{RequeueAfter: 10 * time.Second}, nil }
        }
        // labels
        if ns.Labels == nil { ns.Labels = map[string]string{} }
        changed := false
        if ns.Labels["kubenova.project"] == "" { ns.Labels["kubenova.project"] = req.Name; changed = true }
        tenant := ns.Labels["kubenova.tenant"]
        if tenant == "" { tenant = "default"; ns.Labels["kubenova.tenant"] = tenant; changed = true }
        if changed { _ = r.Update(ctx, ns) }
        // ensure Capsule Tenant exists using unstructured
        if err := ensureCapsuleTenant(ctx, r.Client, tenant); err != nil {
            return reconcile.Result{RequeueAfter: 10 * time.Second}, nil
        }
        return reconcile.Result{}, nil
    }
    // Create namespace if missing
    ns = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: req.Name, Labels: map[string]string{"kubenova.project": req.Name}}}
    if err := r.Create(ctx, ns); err != nil { return reconcile.Result{RequeueAfter: 10 * time.Second}, nil }
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
        "kind": gvk.Kind,
        "metadata": map[string]interface{}{"name": name},
        "spec": map[string]interface{}{"owners": []interface{}{}},
    }
    return c.Create(ctx, u)
}
