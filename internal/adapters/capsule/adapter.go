package capsule

import (
	"github.com/vaheed/kubenova/internal/tenancy"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// TenantCR builds an unstructured Capsule Tenant CR.
// apiVersion: capsule.clastix.io/v1beta2, kind: Tenant
func TenantCR(name string, owners []string, labels map[string]string) *unstructured.Unstructured {
	u := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "capsule.clastix.io/v1beta2",
		"kind":       "Tenant",
		"metadata": map[string]interface{}{
			"name":   name,
			"labels": labels,
		},
		"spec": map[string]interface{}{
			"owners":        ownersToSpec(owners),
			"allowedGroups": []string{tenancy.TenantDevGroup(name)},
		},
	}}
	return u
}

func ownersToSpec(owners []string) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(owners))
	for _, o := range owners {
		out = append(out, map[string]interface{}{"kind": "User", "name": o})
	}
	return out
}
