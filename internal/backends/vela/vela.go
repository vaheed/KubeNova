package vela

import (
	"context"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Interface abstracts the interaction with KubeVela Application resources.
type Interface interface {
	ApplyApp(ctx context.Context, spec map[string]any) error
	ApplyProject(ctx context.Context, spec map[string]any) error
}

type clientImpl struct {
	client client.Client
	scheme *runtime.Scheme
}

var applicationGVK = schema.GroupVersionKind{
	Group:   "core.oam.dev",
	Version: "v1beta1",
	Kind:    "Application",
}

var projectGVK = schema.GroupVersionKind{
	Group:   "core.oam.dev",
	Version: "v1beta1",
	Kind:    "Project",
}

// AddToScheme registers the KubeVela Application GVK for unstructured usage.
func AddToScheme(scheme *runtime.Scheme) {
	scheme.AddKnownTypeWithName(applicationGVK, &unstructured.Unstructured{})
	scheme.AddKnownTypeWithName(projectGVK, &unstructured.Unstructured{})
}

// NewClient returns a KubeVela backend backed by the Kubernetes API.
func NewClient(c client.Client, scheme *runtime.Scheme) Interface {
	return &clientImpl{client: c, scheme: scheme}
}

func (c *clientImpl) ApplyApp(ctx context.Context, spec map[string]any) error {
	name, _ := spec["name"].(string)
	namespace, _ := spec["namespace"].(string)
	if name == "" || namespace == "" {
		return nil
	}
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		obj := &unstructured.Unstructured{}
		obj.SetGroupVersionKind(applicationGVK)
		err := c.client.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, obj)
		if apierrors.IsNotFound(err) {
			obj.SetName(name)
			obj.SetNamespace(namespace)
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
	})
}

func (c *clientImpl) ApplyProject(ctx context.Context, spec map[string]any) error {
	name, _ := spec["name"].(string)
	if name == "" {
		return nil
	}
	namespace := "vela-system"
	if ns, ok := spec["namespace"].(string); ok && ns != "" {
		namespace = ns
	}
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		obj := &unstructured.Unstructured{}
		obj.SetGroupVersionKind(projectGVK)
		err := c.client.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, obj)
		if apierrors.IsNotFound(err) {
			obj.SetName(name)
			obj.SetNamespace(namespace)
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
	})
}

func (c *clientImpl) ApplyProject(ctx context.Context, spec map[string]any) error {
	name, _ := spec["name"].(string)
	if name == "" {
		return nil
	}
	namespace := "vela-system"
	if ns, ok := spec["namespace"].(string); ok && ns != "" {
		namespace = ns
	}
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(projectGVK)
	err := c.client.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, obj)
	if apierrors.IsNotFound(err) {
		obj.SetName(name)
		obj.SetNamespace(namespace)
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
		if k == "name" || k == "namespace" {
			continue
		}
		out[k] = v
	}
	if len(out) == 0 {
		out["placeholder"] = "managed-by-kubenova"
	}
	return out
}
