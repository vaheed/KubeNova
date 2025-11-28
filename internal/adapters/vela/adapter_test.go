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
}
