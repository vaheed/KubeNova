package capsule

import (
	"context"
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
}

type Tenant struct {
	Name        string
	Owners      []string
	Labels      map[string]string
	Annotations map[string]string
}

// New returns a Client backed by the in-repo adapter logic.
// For now this returns a no-op stub to keep the API surface stable until full wiring is completed.
func New(_ []byte) Client { // kubeconfig (cluster-scoped)
	return &noop{}
}

type noop struct{}

func (n *noop) EnsureTenant(ctx context.Context, name string, owners []string, labels map[string]string) error {
	_ = ctx
	_ = name
	_ = owners
	_ = labels
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
