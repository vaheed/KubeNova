package reconcile

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/vaheed/kubenova/internal/adapters/capsule"
	"github.com/vaheed/kubenova/internal/adapters/vela"
	capsulebackend "github.com/vaheed/kubenova/internal/backends/capsule"
	proxybackend "github.com/vaheed/kubenova/internal/backends/proxy"
	velabackend "github.com/vaheed/kubenova/internal/backends/vela"
	"github.com/vaheed/kubenova/internal/cluster"
	"github.com/vaheed/kubenova/internal/logging"
	"github.com/vaheed/kubenova/internal/telemetry"
	v1alpha1 "github.com/vaheed/kubenova/pkg/api/v1alpha1"
	"github.com/vaheed/kubenova/pkg/types"
	"go.uber.org/zap"
	authv1 "k8s.io/api/authentication/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/labels"
)

// ProjectReconciler watches NovaProjects and ensures namespaces exist and Vela projects align.
type ProjectReconciler struct {
	client.Client
	Scheme  *runtime.Scheme
	Backend velabackend.Interface
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
	if r.Backend != nil {
		manifest := map[string]any{
			"name":        proj.Name,
			"tenant":      tenantName,
			"description": proj.Spec.Description,
			"labels":      proj.Spec.Labels,
		}
		if err := r.Backend.ApplyProject(ctx, manifest); err != nil {
			if apimeta.IsNoMatchError(err) {
				logging.L.Warn("vela_project_crd_missing", zap.Error(err))
			} else {
				return ctrl.Result{}, err
			}
		}
	}
	return ctrl.Result{}, nil
}

func (r *ProjectReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.Scheme == nil {
		r.Scheme = mgr.GetScheme()
	}
	if r.Backend == nil {
		r.Backend = velabackend.NewClient(mgr.GetClient(), mgr.GetScheme())
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
	proxyServer := proxyServerEndpoint(tenantObj.Spec.ProxyEndpoint, tenantName)
	if r.Proxy != nil {
		_ = r.Proxy.Publish(ctx, tenantName, proxyServer)
	}
	if err := ensureNamespace(ctx, r.Client, ownerNS); err != nil {
		return ctrl.Result{}, err
	}
	if err := ensureNamespace(ctx, r.Client, appsNS); err != nil {
		return ctrl.Result{}, err
	}
	if err := ensureTenantAccess(ctx, r.Client, tenantName, ownerNS, appsNS, proxyServer); err != nil {
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
	// Mark status/annotations with retry to avoid conflicts
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		var latest v1alpha1.NovaApp
		if err := r.Get(ctx, req.NamespacedName, &latest); err != nil {
			if apierrors.IsNotFound(err) {
				return nil
			}
			return err
		}
		if err := setStatusReady(ctx, r.Client, &latest); err != nil {
			return err
		}
		return patchAnnotations(ctx, r.Client, &latest, map[string]string{
			"kubenova.io/last-applied": time.Now().UTC().Format(time.RFC3339),
		})
	})
	return ctrl.Result{}, err
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
	components := []string{"cert-manager", "capsule", "capsule-proxy", "kubevela", "velaux"}
	for _, comp := range components {
		logging.L.Info("bootstrap_component_start", zap.String("component", comp))
		if err := installer.Reconcile(ctx, comp); err != nil {
			logging.L.Error("bootstrap_component_error", zap.String("component", comp), zap.Error(err))
			telemetry.Emit("component_install", map[string]string{
				"component": comp,
				"status":    "error",
				"error":     err.Error(),
			})
		}
	}
	return nil
}

func ensureCapsuleUserGroups(ctx context.Context, c client.Client, tenant string) error {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "capsule.clastix.io",
		Version: "v1beta2",
		Kind:    "CapsuleConfiguration",
	})
	name := "default"
	if err := c.Get(ctx, client.ObjectKey{Name: name}, obj); err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
		cfg := map[string]any{
			"userGroups": []any{
				"system:serviceaccounts",
				fmt.Sprintf("system:serviceaccounts:%s", tenant+"-owner"),
			},
		}
		obj.SetName(name)
		obj.SetLabels(map[string]string{"managed-by": "kubenova"})
		_ = unstructured.SetNestedMap(obj.Object, cfg, "spec")
		return c.Create(ctx, obj)
	}
	ugs, found, _ := unstructured.NestedStringSlice(obj.Object, "spec", "userGroups")
	set := sets.NewString(ugs...)
	updated := false
	if !found {
		set = sets.NewString()
	}
	for _, g := range []string{"system:serviceaccounts", fmt.Sprintf("system:serviceaccounts:%s", tenant+"-owner")} {
		if !set.Has(g) {
			set.Insert(g)
			updated = true
		}
	}
	if !updated {
		return nil
	}
	if err := unstructured.SetNestedStringSlice(obj.Object, set.List(), "spec", "userGroups"); err != nil {
		return err
	}
	return c.Update(ctx, obj)
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
			for _, comp := range []string{"cert-manager", "capsule", "capsule-proxy", "kubevela"} {
				start := time.Now()
				logging.L.Info("reconcile_component_start", zap.String("component", comp))
				if err := installer.Reconcile(ctx, comp); err != nil {
					logging.L.Error("reconcile_component_error", zap.String("component", comp), zap.Error(err))
					telemetry.Emit("component_install", map[string]string{
						"component": comp,
						"status":    "error",
						"error":     err.Error(),
					})
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

func proxyServerEndpoint(endpoint, tenant string) string {
	trimmed := strings.TrimSpace(endpoint)
	if trimmed == "" {
		trimmed = "https://proxy.kubenova.local"
	}
	u, err := url.Parse(trimmed)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return strings.TrimRight(trimmed, "/")
	}
	u.Path = ""
	u.RawPath = ""
	return strings.TrimRight(u.String(), "/")
}

func ensureTenantAccess(ctx context.Context, c client.Client, tenant, ownerNS, appsNS, proxyEndpoint string) error {
	ownerSA := "kubenova-owner"
	readonlySA := "kubenova-readonly"
	server := proxyServerEndpoint(proxyEndpoint, tenant)
	// ServiceAccounts
	if err := ensureServiceAccount(ctx, c, ownerNS, ownerSA); err != nil {
		return err
	}
	if err := ensureServiceAccount(ctx, c, ownerNS, readonlySA); err != nil {
		return err
	}
	// Roles and bindings in both namespaces
	for _, ns := range []string{ownerNS, appsNS} {
		if err := ensureRole(ctx, c, ns, "kubenova-owner", []string{"*"}, []string{"*"}, []string{"*"}); err != nil {
			return err
		}
		if err := ensureRole(ctx, c, ns, "kubenova-readonly", []string{"get", "list", "watch"}, []string{"*"}, []string{"*"}); err != nil {
			return err
		}
		if err := ensureRoleBinding(ctx, c, ns, "kubenova-owner-binding", "kubenova-owner", ownerSA, ownerNS); err != nil {
			return err
		}
		if err := ensureRoleBinding(ctx, c, ns, "kubenova-readonly-binding", "kubenova-readonly", readonlySA, ownerNS); err != nil {
			return err
		}
	}
	// Kubeconfigs secret (best-effort: skip if tokens not yet available)
	_ = ensureKubeconfigSecret(ctx, c, tenant, ownerNS, server, ownerSA, readonlySA)
	// Ensure capsule-proxy settings allow tenant service accounts
	_ = ensureCapsuleUserGroups(ctx, c, tenant)
	// Ensure capsule-proxy settings allow tenant service accounts
	_ = ensureProxySetting(ctx, c, tenant, ownerNS, ownerSA, readonlySA)
	return nil
}

func ensureServiceAccount(ctx context.Context, c client.Client, ns, name string) error {
	sa := &corev1.ServiceAccount{}
	err := c.Get(ctx, client.ObjectKey{Name: name, Namespace: ns}, sa)
	if apierrors.IsNotFound(err) {
		sa = &corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: ns,
				Labels:    map[string]string{"managed-by": "kubenova"},
			},
		}
		return c.Create(ctx, sa)
	}
	return err
}

func ensureRole(ctx context.Context, c client.Client, ns, name string, verbs, resources, apiGroups []string) error {
	role := &rbacv1.Role{}
	err := c.Get(ctx, client.ObjectKey{Name: name, Namespace: ns}, role)
	if apierrors.IsNotFound(err) {
		role = &rbacv1.Role{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: ns,
				Labels:    map[string]string{"managed-by": "kubenova"},
			},
			Rules: []rbacv1.PolicyRule{{
				APIGroups: apiGroups,
				Resources: resources,
				Verbs:     verbs,
			}},
		}
		return c.Create(ctx, role)
	}
	if err != nil {
		return err
	}
	role.Rules = []rbacv1.PolicyRule{{
		APIGroups: apiGroups,
		Resources: resources,
		Verbs:     verbs,
	}}
	return c.Update(ctx, role)
}

func ensureRoleBinding(ctx context.Context, c client.Client, ns, name, roleName, saName, saNamespace string) error {
	rb := &rbacv1.RoleBinding{}
	err := c.Get(ctx, client.ObjectKey{Name: name, Namespace: ns}, rb)
	subjects := []rbacv1.Subject{{
		Kind:      "ServiceAccount",
		Name:      saName,
		Namespace: saNamespace,
	}}
	if apierrors.IsNotFound(err) {
		rb = &rbacv1.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: ns,
				Labels:    map[string]string{"managed-by": "kubenova"},
			},
			Subjects: subjects,
			RoleRef: rbacv1.RoleRef{
				Kind:     "Role",
				Name:     roleName,
				APIGroup: "rbac.authorization.k8s.io",
			},
		}
		return c.Create(ctx, rb)
	}
	if err != nil {
		return err
	}
	rb.Subjects = subjects
	rb.RoleRef = rbacv1.RoleRef{
		Kind:     "Role",
		Name:     roleName,
		APIGroup: "rbac.authorization.k8s.io",
	}
	return c.Update(ctx, rb)
}

func ensureKubeconfigSecret(ctx context.Context, c client.Client, tenant, ns, proxyEndpoint, ownerSA, readonlySA string) error {
	proxyEndpoint = proxyServerEndpoint(proxyEndpoint, tenant)
	secret := &corev1.Secret{}
	err := c.Get(ctx, client.ObjectKey{Name: "kubenova-kubeconfigs", Namespace: ns}, secret)
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	ownerToken, _ := requestServiceAccountToken(ctx, c, ns, ownerSA)
	if ownerToken == "" {
		ownerToken, _ = findServiceAccountToken(ctx, c, ns, ownerSA)
	}
	readonlyToken, _ := requestServiceAccountToken(ctx, c, ns, readonlySA)
	if readonlyToken == "" {
		readonlyToken, _ = findServiceAccountToken(ctx, c, ns, readonlySA)
	}
	data := map[string][]byte{}
	if ownerToken != "" {
		data["owner"] = []byte(buildProxyKubeconfig(proxyEndpoint, ownerToken, tenant, ns, "owner"))
	}
	if readonlyToken != "" {
		data["readonly"] = []byte(buildProxyKubeconfig(proxyEndpoint, readonlyToken, tenant, ns, "readonly"))
	}
	if len(data) == 0 {
		return nil
	}
	if apierrors.IsNotFound(err) {
		secret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "kubenova-kubeconfigs",
				Namespace: ns,
				Labels:    map[string]string{"managed-by": "kubenova", "tenant": tenant},
			},
			Type: corev1.SecretTypeOpaque,
			Data: data,
		}
		return c.Create(ctx, secret)
	}
	if secret.Data == nil {
		secret.Data = map[string][]byte{}
	}
	updated := false
	for key, val := range data {
		if !bytes.Equal(secret.Data[key], val) {
			secret.Data[key] = val
			updated = true
		}
	}
	if !updated {
		return nil
	}
	return c.Update(ctx, secret)
}

func findServiceAccountToken(ctx context.Context, c client.Client, ns, saName string) (string, error) {
	var secrets corev1.SecretList
	selector := labels.Set{"kubernetes.io/service-account.name": saName}.AsSelector()
	if err := c.List(ctx, &secrets, client.InNamespace(ns), client.MatchingLabelsSelector{Selector: selector}); err != nil {
		return "", err
	}
	for _, s := range secrets.Items {
		if s.Type == corev1.SecretTypeServiceAccountToken {
			if token, ok := s.Data["token"]; ok {
				return string(token), nil
			}
		}
	}
	return "", fmt.Errorf("token secret for sa %s not found", saName)
}

func requestServiceAccountToken(ctx context.Context, c client.Client, ns, saName string) (string, error) {
	sa := &corev1.ServiceAccount{}
	if err := c.Get(ctx, client.ObjectKey{Name: saName, Namespace: ns}, sa); err != nil {
		return "", err
	}
	ttl := int64(3600)
	audiences := []string{"https://kubernetes.default.svc.cluster.local", "kubernetes"}
	tr := &authv1.TokenRequest{
		Spec: authv1.TokenRequestSpec{
			Audiences:         audiences,
			ExpirationSeconds: &ttl,
		},
	}
	if err := c.SubResource("token").Create(ctx, sa, tr); err != nil {
		return "", err
	}
	return tr.Status.Token, nil
}

func buildProxyKubeconfig(serverURL, token, tenant, namespace, role string) string {
	serverURL = strings.TrimRight(serverURL, "/")
	clusterName := fmt.Sprintf("%s-proxy", tenant)
	userName := fmt.Sprintf("%s-%s", tenant, role)
	contextName := fmt.Sprintf("%s-%s", tenant, role)
	return fmt.Sprintf(`apiVersion: v1
kind: Config
clusters:
- cluster:
    server: %s
    insecure-skip-tls-verify: true
  name: %s
contexts:
- context:
    cluster: %s
    user: %s
    namespace: %s
  name: %s
current-context: %s
users:
- name: %s
  user:
    token: %s
`, serverURL, clusterName, clusterName, userName, namespace, contextName, contextName, userName, token)
}

func ensureProxySetting(ctx context.Context, c client.Client, tenant, ownerNS, ownerSA, readonlySA string) error {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "capsule.clastix.io",
		Version: "v1beta1",
		Kind:    "ProxySetting",
	})
	name := "kubenova-proxy-" + tenant
	if err := c.Get(ctx, client.ObjectKey{Name: name, Namespace: ownerNS}, obj); err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
		ps := map[string]any{
			"subjects": []any{
				map[string]any{"kind": "ServiceAccount", "name": ownerSA},
				map[string]any{"kind": "ServiceAccount", "name": readonlySA},
				map[string]any{"kind": "Group", "name": fmt.Sprintf("system:serviceaccounts:%s", ownerNS)},
			},
		}
		obj.SetName(name)
		obj.SetNamespace(ownerNS)
		obj.SetLabels(map[string]string{"managed-by": "kubenova", "tenant": tenant})
		_ = unstructured.SetNestedMap(obj.Object, ps, "spec")
		return c.Create(ctx, obj)
	}
	return nil
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
