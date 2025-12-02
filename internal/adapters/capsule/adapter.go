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
	owners := []map[string]any{}
	for _, o := range t.Owners {
		if o == "" {
			continue
		}
		owners = append(owners, map[string]any{"name": o, "kind": "User"})
	}
	if len(owners) == 0 {
		owners = append(owners, map[string]any{"name": t.Name, "kind": "User"})
	}
	return map[string]any{
		"tenant": t.Name,
		"owners": owners,
		"labels": t.Labels,
	}
}
