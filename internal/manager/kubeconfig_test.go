package manager

import (
	"github.com/vaheed/kubenova/pkg/types"
	"testing"
	"time"
)

func TestGenerateKubeconfig(t *testing.T) {
	g := types.KubeconfigGrant{Tenant: "alice", Role: "tenant-dev", Expires: time.Now().Add(time.Hour)}
	y, err := GenerateKubeconfig(g, "https://capsule-proxy.example", []byte("dev"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(y)
	if want := "capsule-proxy.example"; !contains(s, want) {
		t.Fatalf("expected url %s in kubeconfig", want)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || (len(sub) == 0) || (len(s) > 0 && (s[0:len(sub)] == sub || contains(s[1:], sub))))
}
