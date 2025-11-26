package reconcile

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/vaheed/kubenova/internal/adapters/capsule"
	"github.com/vaheed/kubenova/internal/adapters/vela"
	capsulebackend "github.com/vaheed/kubenova/internal/backends/capsule"
	proxybackend "github.com/vaheed/kubenova/internal/backends/proxy"
	velabackend "github.com/vaheed/kubenova/internal/backends/vela"
	"github.com/vaheed/kubenova/internal/cluster"
	"github.com/vaheed/kubenova/internal/logging"
	v1alpha1 "github.com/vaheed/kubenova/pkg/api/v1alpha1"
	"github.com/vaheed/kubenova/pkg/types"
	"go.uber.org/zap"
)

// ProjectReconciler watches NovaProjects and ensures namespaces exist.
type ProjectReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

func (r *ProjectReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var proj v1alpha1.NovaProject
	if err := r.Get(ctx, req.NamespacedName, &proj); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}
	tenantName := proj.Spec.Tenant
	if tenantName == "" {
		return ctrl.Result{}, nil
	}
	var tenant v1alpha1.NovaTenant
	if err := r.Get(ctx, client.ObjectKey{Name: tenantName}, &tenant); err != nil && !apierrors.IsNotFound(err) {
		return ctrl.Result{}, err
	}
	ownerNS := tenant.Spec.OwnerNamespace
	if ownerNS == "" {
		ownerNS = fmt.Sprintf("%s-owner", tenantName)
	}
	appsNS := tenant.Spec.AppsNamespace
	if appsNS == "" {
		appsNS = fmt.Sprintf("%s-apps", tenantName)
	}
	if err := ensureNamespace(ctx, r.Client, ownerNS); err != nil {
		return ctrl.Result{}, err
	}
	if err := ensureNamespace(ctx, r.Client, appsNS); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *ProjectReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.Scheme == nil {
		r.Scheme = mgr.GetScheme()
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.NovaProject{}).
		Complete(r)
}

// TenantReconciler ensures Capsule tenant resources and proxy endpoints.
type TenantReconciler struct {
	client.Client
	Capsule capsulebackend.Interface
	Proxy   proxybackend.Interface
}

func (r *TenantReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var tenantObj v1alpha1.NovaTenant
	if err := r.Get(ctx, req.NamespacedName, &tenantObj); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}
	tenantName := tenantObj.Name
	if tenantName == "" {
		return ctrl.Result{}, nil
	}
	ownerNS := tenantObj.Spec.OwnerNamespace
	if ownerNS == "" {
		ownerNS = tenantName + "-owner"
	}
	appsNS := tenantObj.Spec.AppsNamespace
	if appsNS == "" {
		appsNS = tenantName + "-apps"
	}

	t := &types.Tenant{
		Name:            tenantName,
		OwnerNamespace:  ownerNS,
		AppsNamespace:   appsNS,
		Owners:          tenantObj.Spec.Owners,
		Plan:            tenantObj.Spec.Plan,
		Labels:          tenantObj.Spec.Labels,
		NetworkPolicies: tenantObj.Spec.NetworkPolicies,
		Quotas:          tenantObj.Spec.Quotas,
		Limits:          tenantObj.Spec.Limits,
	}
	adapter := capsule.NewTenantAdapter()
	spec := adapter.ToManifests(t)
	if r.Capsule != nil {
		if err := r.Capsule.EnsureTenant(ctx, spec); err != nil {
			return ctrl.Result{}, err
		}
	}
	if r.Proxy != nil {
		endpoint := tenantObj.Spec.ProxyEndpoint
		if endpoint == "" {
			endpoint = fmt.Sprintf("https://proxy.kubenova.local/%s", tenantName)
		}
		_ = r.Proxy.Publish(ctx, tenantName, endpoint)
	}
	if err := ensureNamespace(ctx, r.Client, ownerNS); err != nil {
		return ctrl.Result{}, err
	}
	if err := ensureNamespace(ctx, r.Client, appsNS); err != nil {
		return ctrl.Result{}, err
	}
	_ = setStatusReady(ctx, r.Client, &tenantObj)
	return ctrl.Result{}, nil
}

func (r *TenantReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.Capsule == nil {
		r.Capsule = capsulebackend.NewClient(mgr.GetClient(), mgr.GetScheme())
	}
	if r.Proxy == nil {
		r.Proxy = proxybackend.NewClient(mgr.GetClient())
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.NovaTenant{}).
		Complete(r)
}

// AppReconciler projects NovaApps into KubeVela Applications.
type AppReconciler struct {
	client.Client
	Backend velabackend.Interface
}

func (r *AppReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var app v1alpha1.NovaApp
	if err := r.Get(ctx, req.NamespacedName, &app); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}
	if app.Spec.Tenant == "" || app.Spec.Project == "" {
		return ctrl.Result{}, nil
	}
	namespace := app.Spec.Namespace
	if namespace == "" {
		var tenant v1alpha1.NovaTenant
		if err := r.Get(ctx, client.ObjectKey{Name: app.Spec.Tenant}, &tenant); err == nil {
			namespace = tenant.Spec.AppsNamespace
		}
		if namespace == "" {
			namespace = app.Spec.Tenant + "-apps"
		}
	}

	appModel := &types.App{
		Name:        app.Name,
		ProjectID:   app.Spec.Project,
		TenantID:    app.Spec.Tenant,
		Description: app.Spec.Description,
		Component:   app.Spec.Component,
		Image:       app.Spec.Image,
		Spec:        app.Spec.Template,
		Traits:      app.Spec.Traits,
		Policies:    app.Spec.Policies,
		Revision:    1,
	}

	adapter := vela.NewAppAdapter()
	manifest := adapter.ToApplication(appModel)
	manifest["namespace"] = namespace
	if r.Backend != nil {
		if err := r.Backend.ApplyApp(ctx, manifest); err != nil {
			return ctrl.Result{}, err
		}
	}
	// Mark configmap with last reconciled timestamp for visibility
	_ = setStatusReady(ctx, r.Client, &app)
	return ctrl.Result{}, patchAnnotations(ctx, r.Client, &app, map[string]string{
		"kubenova.io/last-applied": time.Now().UTC().Format(time.RFC3339),
	})
}

func (r *AppReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.Backend == nil {
		r.Backend = velabackend.NewClient(mgr.GetClient(), mgr.GetScheme())
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.NovaApp{}).
		Complete(r)
}

// BootstrapHelmJob installs foundational components (cert-manager, capsule, capsule-proxy, kubevela).
func BootstrapHelmJob(ctx context.Context, c client.Client, reader client.Reader, scheme *runtime.Scheme) error {
	installer := cluster.NewInstaller(c, scheme, nil, reader, false)
	components := []string{"cert-manager", "capsule", "capsule-proxy", "kubevela", "fluxcd", "velaux"}
	for _, comp := range components {
		logging.L.Info("bootstrap_component_start", zap.String("component", comp))
		if err := installer.Reconcile(ctx, comp); err != nil {
			logging.L.Error("bootstrap_component_error", zap.String("component", comp), zap.Error(err))
		}
	}
	return nil
}

// PeriodicComponentReconciler ensures add-ons stay aligned with desired versions.
func PeriodicComponentReconciler(ctx context.Context, c client.Client, reader client.Reader, scheme *runtime.Scheme, interval time.Duration) error {
	installer := cluster.NewInstaller(c, scheme, nil, reader, false)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			for _, comp := range []string{"cert-manager", "capsule", "capsule-proxy", "kubevela", "fluxcd", "velaux"} {
				start := time.Now()
				logging.L.Info("reconcile_component_start", zap.String("component", comp))
				if err := installer.Reconcile(ctx, comp); err != nil {
					logging.L.Error("reconcile_component_error", zap.String("component", comp), zap.Error(err))
					continue
				}
				logging.L.Info("reconcile_component_done", zap.String("component", comp), zap.Duration("duration", time.Since(start)))
			}
		}
	}
}

func ensureNamespace(ctx context.Context, c client.Client, name string) error {
	if name == "" {
		return nil
	}
	var ns corev1.Namespace
	err := c.Get(ctx, client.ObjectKey{Name: name}, &ns)
	if apierrors.IsNotFound(err) {
		ns = corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name:   name,
				Labels: map[string]string{"managed-by": "kubenova"},
			},
		}
		return c.Create(ctx, &ns)
	}
	return err
}

func patchAnnotations(ctx context.Context, c client.Client, obj client.Object, annotations map[string]string) error {
	current := obj.GetAnnotations()
	if current == nil {
		current = map[string]string{}
	}
	for k, v := range annotations {
		current[k] = v
	}
	obj.SetAnnotations(current)
	return c.Update(ctx, obj)
}

func setStatusReady(ctx context.Context, c client.Client, obj client.Object) error {
	switch o := obj.(type) {
	case *v1alpha1.NovaTenant:
		o.Status.Phase = "Ready"
		o.Status.ObservedGeneration = o.Generation
		o.Status.Conditions = []metav1.Condition{{
			Type:               "Ready",
			Status:             metav1.ConditionTrue,
			Reason:             "Reconciled",
			LastTransitionTime: metav1.Now(),
		}}
		return c.Status().Update(ctx, o)
	case *v1alpha1.NovaProject:
		o.Status.Phase = "Ready"
		o.Status.ObservedGeneration = o.Generation
		o.Status.Conditions = []metav1.Condition{{
			Type:               "Ready",
			Status:             metav1.ConditionTrue,
			Reason:             "Reconciled",
			LastTransitionTime: metav1.Now(),
		}}
		return c.Status().Update(ctx, o)
	case *v1alpha1.NovaApp:
		o.Status.Phase = "Ready"
		o.Status.ObservedGeneration = o.Generation
		o.Status.Conditions = []metav1.Condition{{
			Type:               "Ready",
			Status:             metav1.ConditionTrue,
			Reason:             "Reconciled",
			LastTransitionTime: metav1.Now(),
		}}
		return c.Status().Update(ctx, o)
	default:
		return nil
	}
}
