package capsule

import (
	"context"
	"encoding/json"
	"fmt"

	capadapter "github.com/vaheed/kubenova/internal/adapters/capsule"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"
)

// Client abstracts tenant and policy operations on a target cluster.
// This interface does not expose any vendor-specific resources.
type Client interface {
	EnsureTenant(ctx context.Context, name string, owners []string, labels map[string]string) error
	DeleteTenant(ctx context.Context, name string) error
	ListTenants(ctx context.Context, labelSelector string, limit int, cursor string) ([]Tenant, string, error)
	GetTenant(ctx context.Context, name string) (Tenant, error)

	SetTenantQuotas(ctx context.Context, name string, quotas map[string]string) error
	SetTenantLimits(ctx context.Context, name string, limits map[string]string) error
	SetTenantNetworkPolicies(ctx context.Context, name string, spec map[string]any) error

	TenantSummary(ctx context.Context, name string) (Summary, error)
}

type Tenant struct {
	Name        string
	Owners      []string
	Labels      map[string]string
	Annotations map[string]string
}

// Summary represents an aggregate view of a Capsule Tenant.
// It is intentionally generic and does not leak CRD structs.
type Summary struct {
	Namespaces []string
	Quotas     map[string]string
	Usages     map[string]string
}

// New returns a Client backed by the in-repo adapter logic.
// For now this returns a no-op stub to keep the API surface stable until full wiring is completed.
var tenantGVR = schema.GroupVersionResource{Group: "capsule.clastix.io", Version: "v1beta2", Resource: "tenants"}
var namespaceGVR = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "namespaces"}

func New(kubeconfig []byte) Client { // kubeconfig (cluster-scoped)
	cfg, err := clientcmd.RESTConfigFromKubeConfig(kubeconfig)
	if err != nil {
		return &noop{}
	}
	dyn, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return &noop{}
	}
	return &client{dyn: dyn}
}

type client struct{ dyn dynamic.Interface }

type noop struct{}

func (n *noop) EnsureTenant(ctx context.Context, name string, owners []string, labels map[string]string) error {
	return nil
}
func (n *noop) DeleteTenant(ctx context.Context, name string) error { _ = ctx; _ = name; return nil }
func (n *noop) ListTenants(ctx context.Context, labelSelector string, limit int, cursor string) ([]Tenant, string, error) {
	_ = ctx
	_ = labelSelector
	_ = limit
	_ = cursor
	return []Tenant{}, "", nil
}
func (n *noop) GetTenant(ctx context.Context, name string) (Tenant, error) {
	_ = ctx
	_ = name
	return Tenant{Name: name}, nil
}
func (n *noop) SetTenantQuotas(ctx context.Context, name string, quotas map[string]string) error {
	_ = ctx
	_ = name
	_ = quotas
	return nil
}
func (n *noop) SetTenantLimits(ctx context.Context, name string, limits map[string]string) error {
	_ = ctx
	_ = name
	_ = limits
	return nil
}
func (n *noop) SetTenantNetworkPolicies(ctx context.Context, name string, spec map[string]any) error {
	_ = ctx
	_ = name
	_ = spec
	return nil
}
func (n *noop) TenantSummary(ctx context.Context, name string) (Summary, error) {
	_ = ctx
	_ = name
	return Summary{}, nil
}

func (c *client) EnsureTenant(ctx context.Context, name string, owners []string, labels map[string]string) error {
	u := capadapter.TenantCR(name, owners, labels)
	cur, err := c.dyn.Resource(tenantGVR).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			_, err = c.dyn.Resource(tenantGVR).Create(ctx, u, metav1.CreateOptions{})
			return err
		}
		return err
	}
	u.SetResourceVersion(cur.GetResourceVersion())
	_, err = c.dyn.Resource(tenantGVR).Update(ctx, u, metav1.UpdateOptions{})
	return err
}

func (c *client) DeleteTenant(ctx context.Context, name string) error {
	return c.dyn.Resource(tenantGVR).Delete(ctx, name, metav1.DeleteOptions{})
}

func (c *client) ListTenants(ctx context.Context, labelSelector string, limit int, cursor string) ([]Tenant, string, error) {
	opts := metav1.ListOptions{LabelSelector: labelSelector, Limit: int64(limit), Continue: cursor}
	list, err := c.dyn.Resource(tenantGVR).List(ctx, opts)
	if err != nil {
		return nil, "", err
	}
	out := make([]Tenant, 0, len(list.Items))
	for _, it := range list.Items {
		out = append(out, toTenant(&it))
	}
	return out, list.GetContinue(), nil
}

func (c *client) GetTenant(ctx context.Context, name string) (Tenant, error) {
	u, err := c.dyn.Resource(tenantGVR).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return Tenant{}, err
	}
	return toTenant(u), nil
}

func (c *client) SetTenantQuotas(ctx context.Context, name string, quotas map[string]string) error {
	// Capsule Tenant.spec.resourceQuotas is an object with:
	// - scope: "Tenant" | "Namespace"
	// - items: []ResourceQuotaSpec{ { hard: {...} } }
	if err := c.patchSpec(ctx, name, map[string]any{
		"resourceQuotas": map[string]any{
			"scope": "Tenant",
			"items": []any{
				map[string]any{
					"hard": toAnyMap(quotas),
				},
			},
		},
	}); err != nil {
		return err
	}

	// Also persist quotas in a KubeNova-owned annotation so summary can remain
	// stable across Capsule versions and CRD evolutions.
	u, err := c.dyn.Resource(tenantGVR).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	data, err := json.Marshal(quotas)
	if err != nil {
		return fmt.Errorf("marshal quotas: %w", err)
	}
	ann := u.GetAnnotations()
	if ann == nil {
		ann = map[string]string{}
	}
	ann["kubenova.io/quotas"] = string(data)
	u.SetAnnotations(ann)
	_, err = c.dyn.Resource(tenantGVR).Update(ctx, u, metav1.UpdateOptions{})
	return err
}

func (c *client) SetTenantLimits(ctx context.Context, name string, limits map[string]string) error {
	// Capsule Tenant.spec.limitRanges is an object with:
	// - items: []LimitRangeSpec{ { limits: []LimitRangeItem{ { type: "...", max: {...} } } } }
	return c.patchSpec(ctx, name, map[string]any{
		"limitRanges": map[string]any{
			"items": []any{
				map[string]any{
					"limits": []any{
						map[string]any{
							"type": "Container",
							"max":  toAnyMap(limits),
						},
					},
				},
			},
		},
	})
}

func (c *client) SetTenantNetworkPolicies(ctx context.Context, name string, spec map[string]any) error {
	return c.patchSpec(ctx, name, map[string]any{"networkPolicies": buildNetworkPolicies(spec)})
}

func (c *client) TenantSummary(ctx context.Context, name string) (Summary, error) {
	out := Summary{
		Namespaces: []string{},
		Quotas:     map[string]string{},
		Usages:     map[string]string{},
	}

	// 1) Discover namespaces associated with the Capsule tenant via label selector.
	nsList, err := c.dyn.Resource(namespaceGVR).List(ctx, metav1.ListOptions{
		LabelSelector: "capsule.clastix.io/tenant=" + name,
	})
	if err == nil {
		for _, ns := range nsList.Items {
			out.Namespaces = append(out.Namespaces, ns.GetName())
		}
	}

	// 2) Read resourceQuotas.hard from the Tenant spec if present.
	u, err := c.dyn.Resource(tenantGVR).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return out, nil
		}
		return Summary{}, err
	}
	items, found, _ := unstructured.NestedSlice(u.Object, "spec", "resourceQuotas", "items")
	if found {
		for _, it := range items {
			m, ok := it.(map[string]any)
			if !ok {
				continue
			}
			hard, foundHard, _ := unstructured.NestedMap(m, "hard")
			if !foundHard {
				continue
			}
			for k, v := range hard {
				out.Quotas[k] = fmt.Sprint(v)
			}
		}
	}

	// 3) Fallback to KubeNova-specific annotation if spec does not expose quotas.
	if len(out.Quotas) == 0 {
		if ann := u.GetAnnotations(); ann != nil {
			if raw, ok := ann["kubenova.io/quotas"]; ok && raw != "" {
				q := map[string]string{}
				if err := json.Unmarshal([]byte(raw), &q); err == nil {
					out.Quotas = q
				}
			}
		}
	}

	return out, nil
}

func (c *client) patchSpec(ctx context.Context, name string, fragment map[string]any) error {
	u, err := c.dyn.Resource(tenantGVR).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	spec, _, _ := unstructured.NestedMap(u.Object, "spec")
	for k, v := range fragment {
		spec[k] = v
	}
	if err := unstructured.SetNestedMap(u.Object, spec, "spec"); err != nil {
		return fmt.Errorf("set spec: %w", err)
	}
	_, err = c.dyn.Resource(tenantGVR).Update(ctx, u, metav1.UpdateOptions{})
	return err
}

func toTenant(u *unstructured.Unstructured) Tenant {
	t := Tenant{Name: u.GetName(), Labels: u.GetLabels(), Annotations: u.GetAnnotations()}
	owners := []string{}
	if arr, found, _ := unstructured.NestedSlice(u.Object, "spec", "owners"); found {
		for _, it := range arr {
			if m, ok := it.(map[string]any); ok {
				if n, ok2 := m["name"].(string); ok2 {
					owners = append(owners, n)
				}
			}
		}
	}
	t.Owners = owners
	return t
}

func toAnyMap(in map[string]string) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func buildNetworkPolicies(in map[string]any) map[string]any {
	if in == nil {
		return map[string]any{}
	}
	// If caller passed a raw "items" slice, assume it's already a NetworkPolicySpec list.
	if _, ok := in["items"]; ok {
		return in
	}
	// Map a simple defaultDeny flag into a minimal NetworkPolicySpec that denies all traffic.
	if v, ok := in["defaultDeny"]; ok {
		if b, ok2 := v.(bool); ok2 && b {
			return map[string]any{
				"items": []any{
					map[string]any{
						"podSelector": map[string]any{},
						"policyTypes": []any{"Ingress", "Egress"},
					},
				},
			}
		}
	}
	// Fallback: ignore unknown shape to avoid writing invalid fields.
	return map[string]any{}
}
