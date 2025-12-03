package vela

import (
	"testing"

	"github.com/vaheed/kubenova/pkg/types"
)

func TestAppAdapter(t *testing.T) {
	a := NewAppAdapter()
	result := a.ToApplication(&types.App{
		Name:      "web",
		ProjectID: "proj1",
		ClusterID: "c1",
		TenantID:  "t1",
	})
	if result["name"] != "web" {
		t.Fatalf("expected name")
	}
	if _, ok := result["spec"]; ok {
		t.Fatalf("unexpected nested spec field")
	}
	components, ok := result["components"].([]map[string]any)
	if !ok || len(components) != 1 {
		t.Fatalf("expected one component, got %#v", result["components"])
	}
	if components[0]["name"] != "web" {
		t.Fatalf("component name mismatch: %#v", components[0])
	}
	if result["project"] != "proj1" {
		t.Fatalf("project not propagated: %#v", result)
	}
}
