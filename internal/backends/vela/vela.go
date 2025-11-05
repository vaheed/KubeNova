package vela

import (
    "context"

    vadapter "github.com/vaheed/kubenova/internal/adapters/vela"
    "k8s.io/apimachinery/pkg/api/errors"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
    "k8s.io/apimachinery/pkg/runtime/schema"
    "k8s.io/client-go/dynamic"
    "k8s.io/client-go/tools/clientcmd"
)

// Client abstracts app delivery concepts without leaking vendor constructs.
type Client interface {
	EnsureApp(ctx context.Context, ns, name string, spec map[string]any) error
	DeleteApp(ctx context.Context, ns, name string) error
	GetApp(ctx context.Context, ns, name string) (map[string]any, error)
	ListApps(ctx context.Context, ns string, limit int, cursor string) ([]map[string]any, string, error)

	Deploy(ctx context.Context, ns, name string) error
	Suspend(ctx context.Context, ns, name string) error
	Resume(ctx context.Context, ns, name string) error
	Rollback(ctx context.Context, ns, name string, toRevision *int) error

	Status(ctx context.Context, ns, name string) (map[string]any, error)
	Revisions(ctx context.Context, ns, name string) ([]map[string]any, error)
	Diff(ctx context.Context, ns, name string, a, b int) (map[string]any, error)
	Logs(ctx context.Context, ns, name, component string, follow bool) ([]map[string]any, error)

	SetTraits(ctx context.Context, ns, name string, traits []map[string]any) error
	SetPolicies(ctx context.Context, ns, name string, policies []map[string]any) error
	ImageUpdate(ctx context.Context, ns, name, component, image, tag string) error
}

type client struct { dyn dynamic.Interface }

var appGVR = schema.GroupVersionResource{Group: "core.oam.dev", Version: "v1beta1", Resource: "applications"}
var appRevGVR = schema.GroupVersionResource{Group: "core.oam.dev", Version: "v1beta1", Resource: "applicationrevisions"}

// New returns a client backed by dynamic client from kubeconfig.
func New(kubeconfig []byte) Client {
    cfg, err := clientcmd.RESTConfigFromKubeConfig(kubeconfig)
    if err != nil { return &noop{} }
    dyn, err := dynamic.NewForConfig(cfg)
    if err != nil { return &noop{} }
    return &client{dyn: dyn}
}

type noop struct{}

func (n *noop) EnsureApp(ctx context.Context, ns, name string, spec map[string]any) error {
	_ = ctx
	_ = ns
	_ = name
	_ = spec
	return nil
}
func (n *noop) DeleteApp(ctx context.Context, ns, name string) error {
	_ = ctx
	_ = ns
	_ = name
	return nil
}
func (n *noop) GetApp(ctx context.Context, ns, name string) (map[string]any, error) {
	_ = ctx
	_ = ns
	_ = name
	return map[string]any{"name": name}, nil
}
func (n *noop) ListApps(ctx context.Context, ns string, limit int, cursor string) ([]map[string]any, string, error) {
	_ = ctx
	_ = ns
	_ = limit
	_ = cursor
	return []map[string]any{}, "", nil
}
func (n *noop) Deploy(ctx context.Context, ns, name string) error {
	_ = ctx
	_ = ns
	_ = name
	return nil
}
func (n *noop) Suspend(ctx context.Context, ns, name string) error {
	_ = ctx
	_ = ns
	_ = name
	return nil
}
func (n *noop) Resume(ctx context.Context, ns, name string) error {
	_ = ctx
	_ = ns
	_ = name
	return nil
}
func (n *noop) Rollback(ctx context.Context, ns, name string, toRevision *int) error {
	_ = ctx
	_ = ns
	_ = name
	_ = toRevision
	return nil
}
func (n *noop) Status(ctx context.Context, ns, name string) (map[string]any, error) {
	_ = ctx
	_ = ns
	_ = name
	return map[string]any{"phase": "Running"}, nil
}
func (n *noop) Revisions(ctx context.Context, ns, name string) ([]map[string]any, error) {
	_ = ctx
	_ = ns
	_ = name
	return []map[string]any{}, nil
}
func (n *noop) Diff(ctx context.Context, ns, name string, a, b int) (map[string]any, error) {
	_ = ctx
	_ = ns
	_ = name
	_ = a
	_ = b
	return map[string]any{"changes": []any{}}, nil
}
func (n *noop) Logs(ctx context.Context, ns, name, component string, follow bool) ([]map[string]any, error) {
	_ = ctx
	_ = ns
	_ = name
	_ = component
	_ = follow
	return []map[string]any{}, nil
}
func (n *noop) SetTraits(ctx context.Context, ns, name string, traits []map[string]any) error {
	_ = ctx
	_ = ns
	_ = name
	_ = traits
	return nil
}
func (n *noop) SetPolicies(ctx context.Context, ns, name string, policies []map[string]any) error {
	_ = ctx
	_ = ns
	_ = name
	_ = policies
	return nil
}
func (n *noop) ImageUpdate(ctx context.Context, ns, name, component, image, tag string) error {
    _ = ctx
    _ = ns
    _ = name
    _ = component
    _ = image
    _ = tag
    return nil
}

// real impl methods
func (c *client) EnsureApp(ctx context.Context, ns, name string, spec map[string]any) error {
    u := vadapter.ApplicationCR(ns, name, "")
    cur, err := c.dyn.Resource(appGVR).Namespace(ns).Get(ctx, name, metav1.GetOptions{})
    if err != nil {
        if errors.IsNotFound(err) {
            _, err = c.dyn.Resource(appGVR).Namespace(ns).Create(ctx, u, metav1.CreateOptions{})
            return err
        }
        return err
    }
    u.SetResourceVersion(cur.GetResourceVersion())
    _, err = c.dyn.Resource(appGVR).Namespace(ns).Update(ctx, u, metav1.UpdateOptions{})
    return err
}
func (c *client) DeleteApp(ctx context.Context, ns, name string) error { return c.dyn.Resource(appGVR).Namespace(ns).Delete(ctx, name, metav1.DeleteOptions{}) }
func (c *client) GetApp(ctx context.Context, ns, name string) (map[string]any, error) {
    u, err := c.dyn.Resource(appGVR).Namespace(ns).Get(ctx, name, metav1.GetOptions{})
    if err != nil { return nil, err }
    return u.Object, nil
}
func (c *client) ListApps(ctx context.Context, ns string, limit int, cursor string) ([]map[string]any, string, error) {
    list, err := c.dyn.Resource(appGVR).Namespace(ns).List(ctx, metav1.ListOptions{Limit: int64(limit), Continue: cursor})
    if err != nil { return nil, "", err }
    out := make([]map[string]any, 0, len(list.Items))
    for _, it := range list.Items { out = append(out, it.Object) }
    return out, list.GetContinue(), nil
}
func (c *client) Deploy(ctx context.Context, ns, name string) error { return nil }
func (c *client) Suspend(ctx context.Context, ns, name string) error { return nil }
func (c *client) Resume(ctx context.Context, ns, name string) error { return nil }
func (c *client) Rollback(ctx context.Context, ns, name string, toRevision *int) error { return nil }
func (c *client) Status(ctx context.Context, ns, name string) (map[string]any, error) {
    u, err := c.dyn.Resource(appGVR).Namespace(ns).Get(ctx, name, metav1.GetOptions{})
    if err != nil { return nil, err }
    st, _, _ := unstructured.NestedMap(u.Object, "status")
    if st == nil { st = map[string]any{"phase": "Pending"} }
    return st, nil
}
func (c *client) Revisions(ctx context.Context, ns, name string) ([]map[string]any, error) {
    list, err := c.dyn.Resource(appRevGVR).Namespace(ns).List(ctx, metav1.ListOptions{LabelSelector: "app.oam.dev/name=" + name})
    if err != nil { return nil, err }
    out := make([]map[string]any, 0, len(list.Items))
    for _, it := range list.Items { out = append(out, it.Object) }
    return out, nil
}
func (c *client) Diff(ctx context.Context, ns, name string, a, b int) (map[string]any, error) { return map[string]any{"changes": []any{}}, nil }
func (c *client) Logs(ctx context.Context, ns, name, component string, follow bool) ([]map[string]any, error) { return []map[string]any{}, nil }
func (c *client) SetTraits(ctx context.Context, ns, name string, traits []map[string]any) error { return nil }
func (c *client) SetPolicies(ctx context.Context, ns, name string, policies []map[string]any) error { return nil }
func (c *client) ImageUpdate(ctx context.Context, ns, name, component, image, tag string) error { return nil }
