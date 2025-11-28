package capsule

import "github.com/vaheed/kubenova/pkg/types"

// TenantAdapter translates a KubeNova tenant into Capsule-friendly specs.
type TenantAdapter struct{}

// NewTenantAdapter builds a new adapter.
func NewTenantAdapter() *TenantAdapter {
	return &TenantAdapter{}
}

// ToManifests returns a minimal representation of the tenant resources.
func (a *TenantAdapter) ToManifests(t *types.Tenant) map[string]any {
	if t == nil {
		return map[string]any{}
	}
	return map[string]any{
		"tenant": t.Name,
		"namespaces": []string{
			t.OwnerNamespace,
			t.AppsNamespace,
		},
		"labels": t.Labels,
	}
}
