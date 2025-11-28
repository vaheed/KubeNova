package capsule

import (
	"testing"

	"github.com/vaheed/kubenova/pkg/types"
)

func TestAdapterProducesNamespaces(t *testing.T) {
	adapter := NewTenantAdapter()
	man := adapter.ToManifests(&types.Tenant{
		Name:           "acme",
		OwnerNamespace: "acme-owner",
		AppsNamespace:  "acme-apps",
	})
	if len(man) == 0 {
		t.Fatalf("expected manifests content")
	}
}
