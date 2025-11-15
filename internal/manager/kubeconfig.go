package manager

import (
	"fmt"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/vaheed/kubenova/pkg/types"
)

// GenerateKubeconfig builds a kubeconfig bound to the given proxy server URL.
// It embeds a signed JWT token that encodes tenant/role information, using the
// same semantics as the HTTP kubeconfig endpoints (tenantOwner/projectDev/readOnly).
func GenerateKubeconfig(grant interface{}, server string) ([]byte, error) {
	if server == "" {
		if v := os.Getenv("CAPSULE_PROXY_URL"); v != "" {
			server = v
		} else {
			server = "https://proxy.kubenova.svc"
		}
	}
	// Derive tenant, role, and expiry from the optional KubeconfigGrant.
	role := "readOnly"
	tenant := ""
	var exp *time.Time
	if g, ok := grant.(types.KubeconfigGrant); ok {
		tenant = g.Tenant
		switch g.Role {
		case "tenant-admin", "tenantOwner":
			role = "tenantOwner"
		case "tenant-dev", "projectDev":
			role = "projectDev"
		case "read-only", "readOnly":
			role = "readOnly"
		}
		if !g.Expires.IsZero() {
			e := g.Expires.UTC()
			exp = &e
		}
	}
	claims := jwt.MapClaims{
		"roles": []string{role},
	}
	if tenant != "" {
		claims["tenant"] = tenant
	}
	// Map roles to Kubernetes groups expected by capsule-proxy / RBAC.
	groupsSet := map[string]struct{}{}
	switch role {
	case "tenantOwner":
		groupsSet["tenant-admins"] = struct{}{}
	case "projectDev":
		groupsSet["tenant-maintainers"] = struct{}{}
	case "readOnly":
		groupsSet["tenant-viewers"] = struct{}{}
	}
	if len(groupsSet) > 0 {
		var groups []string
		for g := range groupsSet {
			groups = append(groups, g)
		}
		claims["groups"] = groups
	}
	if exp != nil {
		claims["exp"] = exp.Unix()
	}
	key := []byte(os.Getenv("JWT_SIGNING_KEY"))
	if len(key) == 0 {
		key = []byte("dev")
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, err := tok.SignedString(key)
	if err != nil {
		return nil, err
	}
	// Build kubeconfig that always points at the access proxy, never the cluster API.
	cfg := fmt.Sprintf(
		"apiVersion: v1\nkind: Config\nclusters:\n- name: kn-proxy\n  cluster:\n    insecure-skip-tls-verify: true\n    server: %s\ncontexts:\n- name: tenant\n  context:\n    cluster: kn-proxy\n    user: tenant-user\ncurrent-context: tenant\nusers:\n- name: tenant-user\n  user:\n    token: %s\n",
		server,
		tokenStr,
	)
	return []byte(cfg), nil
}
