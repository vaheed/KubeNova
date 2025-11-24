package capsule

import (
	"context"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Interface abstracts Capsule operations for reconciliation and testing.
type Interface interface {
	EnsureTenant(ctx context.Context, spec map[string]any) error
}

// clientImpl applies Capsule Tenants using the Kubernetes API.
type clientImpl struct {
	client client.Client
	scheme *runtime.Scheme
}

var tenantGVK = schema.GroupVersionKind{
	Group:   "capsule.clastix.io",
	Version: "v1beta2",
	Kind:    "Tenant",
}

// AddToScheme registers the Capsule Tenant GVK for unstructured usage.
func AddToScheme(scheme *runtime.Scheme) {
	scheme.AddKnownTypeWithName(tenantGVK, &unstructured.Unstructured{})
}

// NewClient returns a Capsule client backed by a controller-runtime client.
func NewClient(c client.Client, scheme *runtime.Scheme) Interface {
	return &clientImpl{client: c, scheme: scheme}
}

func (c *clientImpl) EnsureTenant(ctx context.Context, spec map[string]any) error {
	name, _ := spec["tenant"].(string)
	if name == "" {
		return nil
	}

	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(tenantGVK)
	err := c.client.Get(ctx, client.ObjectKey{Name: name}, obj)
	if apierrors.IsNotFound(err) {
		obj.SetName(name)
		obj.SetLabels(map[string]string{"managed-by": "kubenova"})
		obj.Object["spec"] = specFromMap(spec)
		return c.client.Create(ctx, obj)
	}
	if err != nil {
		return err
	}
	obj.SetLabels(map[string]string{"managed-by": "kubenova"})
	obj.Object["spec"] = specFromMap(spec)
	return c.client.Update(ctx, obj)
}

func specFromMap(spec map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range spec {
		if k == "tenant" {
			continue
		}
		out[k] = v
	}
	// provide defaults for namespaces to align with Capsule Tenant fields
	if ns, ok := spec["namespaces"]; ok {
		out["namespaces"] = ns
	}
	if owners, ok := spec["owners"]; ok {
		out["owners"] = owners
	}
	if len(out) == 0 {
		out["placeholder"] = "managed-by-kubenova"
	}
	return out
}
