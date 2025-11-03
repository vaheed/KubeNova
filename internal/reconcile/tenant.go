package reconcile

import (
	"context"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// TenantReconciler mirrors KubeNova Tenant state to Capsule Tenant.
// In this lightweight implementation we simply no-op to keep the
// controller-runtime manager alive for smoke tests while maintaining
// the pluggable structure for future work.
type TenantReconciler struct{ client.Client }

func (r *TenantReconciler) Reconcile(ctx context.Context, req ctrl.Request) (reconcile.Result, error) {
	// TODO: integrate Capsule tenant via unstructured object
	return reconcile.Result{}, nil
}

func (r *TenantReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// No direct watches; Tenants ensured by Project reconciler via ensureCapsuleTenant.
	return nil
}
