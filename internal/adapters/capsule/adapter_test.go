package capsule

import "testing"

func TestTenantCR(t *testing.T) {
    u := TenantCR("alice", []string{"alice@example.com"}, map[string]string{"t":"1"})
    if u.GetKind() != "Tenant" { t.Fatal("kind mismatch") }
    if u.GetName() != "alice" { t.Fatal("name mismatch") }
}

