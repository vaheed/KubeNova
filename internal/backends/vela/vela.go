package vela

import "context"

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

// New returns a no-op client placeholder until fully wired.
func New(_ []byte) Client { return &noop{} }

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
