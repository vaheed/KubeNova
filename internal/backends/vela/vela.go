package vela

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	vadapter "github.com/vaheed/kubenova/internal/adapters/vela"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
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

type client struct {
	dyn  dynamic.Interface
	cset kubernetes.Interface
}

var appGVR = schema.GroupVersionResource{Group: "core.oam.dev", Version: "v1beta1", Resource: "applications"}
var appRevGVR = schema.GroupVersionResource{Group: "core.oam.dev", Version: "v1beta1", Resource: "applicationrevisions"}

// New returns a client backed by dynamic client from kubeconfig.
func New(kubeconfig []byte) Client {
	cfg, err := clientcmd.RESTConfigFromKubeConfig(kubeconfig)
	if err != nil {
		return &noop{}
	}
	return newFromRESTConfig(cfg)
}

// NewFromRESTConfig returns a client using an in-cluster or external REST config.
// This is used by the in-cluster Agent reconcilers so they project Apps without
// needing raw kubeconfig bytes.
func NewFromRESTConfig(cfg *rest.Config) Client {
	if cfg == nil {
		return &noop{}
	}
	return newFromRESTConfig(cfg)
}

func newFromRESTConfig(cfg *rest.Config) Client {
	dyn, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return &noop{}
	}
	cset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return &noop{}
	}
	return &client{dyn: dyn, cset: cset}
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
func (c *client) DeleteApp(ctx context.Context, ns, name string) error {
	return c.dyn.Resource(appGVR).Namespace(ns).Delete(ctx, name, metav1.DeleteOptions{})
}
func (c *client) GetApp(ctx context.Context, ns, name string) (map[string]any, error) {
	u, err := c.dyn.Resource(appGVR).Namespace(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return u.Object, nil
}
func (c *client) ListApps(ctx context.Context, ns string, limit int, cursor string) ([]map[string]any, string, error) {
	list, err := c.dyn.Resource(appGVR).Namespace(ns).List(ctx, metav1.ListOptions{Limit: int64(limit), Continue: cursor})
	if err != nil {
		return nil, "", err
	}
	out := make([]map[string]any, 0, len(list.Items))
	for _, it := range list.Items {
		out = append(out, it.Object)
	}
	return out, list.GetContinue(), nil
}
func (c *client) Deploy(ctx context.Context, ns, name string) error {
	// Nudge controller by updating an annotation (idempotent)
	u, err := c.dyn.Resource(appGVR).Namespace(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	anns := u.GetAnnotations()
	if anns == nil {
		anns = map[string]string{}
	}
	anns["kubenova.io/redeploy-ts"] = time.Now().UTC().Format(time.RFC3339Nano)
	u.SetAnnotations(anns)
	_, err = c.dyn.Resource(appGVR).Namespace(ns).Update(ctx, u, metav1.UpdateOptions{})
	return err
}
func (c *client) Suspend(ctx context.Context, ns, name string) error {
	return c.patchSpec(ctx, ns, name, map[string]any{"suspend": true})
}
func (c *client) Resume(ctx context.Context, ns, name string) error {
	return c.patchSpec(ctx, ns, name, map[string]any{"suspend": false})
}
func (c *client) Rollback(ctx context.Context, ns, name string, toRevision *int) error {
	// Record desired rollback target via annotation for an external controller to act upon
	u, err := c.dyn.Resource(appGVR).Namespace(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	anns := u.GetAnnotations()
	if anns == nil {
		anns = map[string]string{}
	}
	if toRevision != nil {
		anns["kubenova.io/rollback-to-revision"] = jsonNumber(*toRevision)
	} else {
		delete(anns, "kubenova.io/rollback-to-revision")
	}
	u.SetAnnotations(anns)
	_, err = c.dyn.Resource(appGVR).Namespace(ns).Update(ctx, u, metav1.UpdateOptions{})
	return err
}
func (c *client) Status(ctx context.Context, ns, name string) (map[string]any, error) {
	u, err := c.dyn.Resource(appGVR).Namespace(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	st, _, _ := unstructured.NestedMap(u.Object, "status")
	if st == nil {
		st = map[string]any{"phase": "Pending"}
	}
	return st, nil
}
func (c *client) Revisions(ctx context.Context, ns, name string) ([]map[string]any, error) {
	list, err := c.dyn.Resource(appRevGVR).Namespace(ns).List(ctx, metav1.ListOptions{LabelSelector: "app.oam.dev/name=" + name})
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(list.Items))
	for _, it := range list.Items {
		out = append(out, it.Object)
	}
	return out, nil
}
func (c *client) Diff(ctx context.Context, ns, name string, a, b int) (map[string]any, error) {
	// Fetch two revisions and diff their spec
	list, err := c.dyn.Resource(appRevGVR).Namespace(ns).List(ctx, metav1.ListOptions{LabelSelector: "app.oam.dev/name=" + name})
	if err != nil {
		return nil, err
	}
	var A, B *unstructured.Unstructured
	for i := range list.Items {
		it := &list.Items[i]
		if v, _, _ := unstructured.NestedInt64(it.Object, "spec", "revision"); v == int64(a) {
			A = it
		}
		if v, _, _ := unstructured.NestedInt64(it.Object, "spec", "revision"); v == int64(b) {
			B = it
		}
	}
	if A == nil || B == nil {
		return map[string]any{"changes": []any{}}, nil
	}
	sa, _, _ := unstructured.NestedMap(A.Object, "spec")
	sb, _, _ := unstructured.NestedMap(B.Object, "spec")
	changes := shallowDiff(sa, sb)
	return map[string]any{"from": a, "to": b, "changes": changes}, nil
}
func (c *client) Logs(ctx context.Context, ns, name, component string, follow bool) ([]map[string]any, error) {
	// Select pods with app.oam.dev/name label, optionally filtered by component label
	sel := "app.oam.dev/name=" + name
	if component != "" {
		sel += ",app.oam.dev/component=" + component
	}
	pods, err := c.cset.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{LabelSelector: sel})
	if err != nil {
		return nil, err
	}
	out := []map[string]any{}
	for _, p := range pods.Items {
		// best-effort: read a small chunk of logs
		req := c.cset.CoreV1().Pods(ns).GetLogs(p.Name, &corev1.PodLogOptions{TailLines: int64Ptr(50), Follow: follow})
		bs, _ := req.DoRaw(ctx)
		if len(bs) > 0 {
			out = append(out, map[string]any{"component": component, "message": string(bs)})
		}
	}
	return out, nil
}
func (c *client) SetTraits(ctx context.Context, ns, name string, traits []map[string]any) error {
	u, err := c.dyn.Resource(appGVR).Namespace(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	spec, _, _ := unstructured.NestedMap(u.Object, "spec")
	// set traits as-is (array of objects)
	arr := make([]interface{}, 0, len(traits))
	for _, t := range traits {
		arr = append(arr, t)
	}
	spec["traits"] = arr
	if err := unstructured.SetNestedMap(u.Object, spec, "spec"); err != nil {
		return err
	}
	_, err = c.dyn.Resource(appGVR).Namespace(ns).Update(ctx, u, metav1.UpdateOptions{})
	return err
}
func (c *client) SetPolicies(ctx context.Context, ns, name string, policies []map[string]any) error {
	u, err := c.dyn.Resource(appGVR).Namespace(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	spec, _, _ := unstructured.NestedMap(u.Object, "spec")
	arr := make([]interface{}, 0, len(policies))
	for _, p := range policies {
		arr = append(arr, p)
	}
	spec["policies"] = arr
	if err := unstructured.SetNestedMap(u.Object, spec, "spec"); err != nil {
		return err
	}
	_, err = c.dyn.Resource(appGVR).Namespace(ns).Update(ctx, u, metav1.UpdateOptions{})
	return err
}
func (c *client) ImageUpdate(ctx context.Context, ns, name, component, image, tag string) error {
	u, err := c.dyn.Resource(appGVR).Namespace(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	spec, _, _ := unstructured.NestedMap(u.Object, "spec")
	comps, _, _ := unstructured.NestedSlice(spec, "components")
	// Build image reference
	ref := image
	if tag != "" {
		ref = image + ":" + tag
	}
	found := false
	for i := range comps {
		if m, ok := comps[i].(map[string]any); ok {
			if nm, ok2 := m["name"].(string); ok2 && nm == component {
				// ensure properties map exists
				props, _ := m["properties"].(map[string]any)
				if props == nil {
					props = map[string]any{}
				}
				props["image"] = ref
				m["properties"] = props
				comps[i] = m
				found = true
				break
			}
		}
	}
	if !found {
		comps = append(comps, map[string]any{
			"name":       component,
			"type":       "webservice",
			"properties": map[string]any{"image": ref},
		})
	}
	// write back
	if err := unstructured.SetNestedSlice(spec, comps, "components"); err != nil {
		return err
	}
	if err := unstructured.SetNestedMap(u.Object, spec, "spec"); err != nil {
		return err
	}
	_, err = c.dyn.Resource(appGVR).Namespace(ns).Update(ctx, u, metav1.UpdateOptions{})
	return err
}

func (c *client) patchSpec(ctx context.Context, ns, name string, fragment map[string]any) error {
	u, err := c.dyn.Resource(appGVR).Namespace(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	spec, _, _ := unstructured.NestedMap(u.Object, "spec")
	for k, v := range fragment {
		spec[k] = v
	}
	if err := unstructured.SetNestedMap(u.Object, spec, "spec"); err != nil {
		return err
	}
	_, err = c.dyn.Resource(appGVR).Namespace(ns).Update(ctx, u, metav1.UpdateOptions{})
	return err
}

func shallowDiff(a, b map[string]any) []map[string]any {
	changes := []map[string]any{}
	keys := map[string]struct{}{}
	for k := range a {
		keys[k] = struct{}{}
	}
	for k := range b {
		keys[k] = struct{}{}
	}
	for k := range keys {
		va, oka := a[k]
		vb, okb := b[k]
		if !oka {
			changes = append(changes, map[string]any{"path": k, "change": "added", "value": vb})
			continue
		}
		if !okb {
			changes = append(changes, map[string]any{"path": k, "change": "removed", "value": va})
			continue
		}
		if !equalJSON(va, vb) {
			changes = append(changes, map[string]any{"path": k, "change": "modified"})
		}
	}
	return changes
}

func equalJSON(a, b any) bool {
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	return string(ja) == string(jb)
}

func jsonNumber(n int) string { return strconv.Itoa(n) }
func int64Ptr(n int64) *int64 { return &n }
