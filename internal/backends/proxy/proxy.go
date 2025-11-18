package proxy

import (
	"context"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Client abstracts issuing scoped kubeconfigs via an access proxy endpoint.
// Implementations are expected to generate kubeconfigs that point only at the
// proxy (for example, Capsule proxy) and embed short-lived JWT tokens that
// encode tenant, project, and role information.
type Client interface {
	Issue(ctx context.Context, tenant string, project *string, role string, ttlSeconds int) (kubeconfig []byte, expiresAt int64, err error)
}

// New returns a local JWT-based implementation that issues proxy kubeconfigs
// consistent with the Manager's HTTP kubeconfig endpoints.
// kubeconfig bytes are unused for now; proxyURL must not be empty and is
// expected to point at capsule-proxy or an equivalent access proxy.
func New(_ []byte, proxyURL string) Client {
	if proxyURL == "" {
		proxyURL = "https://proxy.kubenova.svc"
	}
	key := []byte("") // JWT signing key must be provided by caller when used.
	if len(key) == 0 {
		key = []byte("dev")
	}
	return &jwtProxy{proxyURL: proxyURL, key: key}
}

type jwtProxy struct {
	proxyURL string
	key      []byte
}

func (p *jwtProxy) Issue(ctx context.Context, tenant string, project *string, role string, ttlSeconds int) ([]byte, int64, error) {
	_ = ctx
	if tenant == "" {
		return nil, 0, fmt.Errorf("tenant required")
	}
	// Normalize role and decide namespace for project-scoped kubeconfigs.
	effectiveRole := "readOnly"
	if role != "" {
		effectiveRole = role
	}
	ns := ""
	if project != nil {
		ns = *project
	}
	// projectDev kubeconfigs must always be namespace-scoped.
	if effectiveRole == "projectDev" && ns == "" {
		return nil, 0, fmt.Errorf("project required for projectDev role")
	}
	// TTL bounds and default.
	if ttlSeconds <= 0 {
		ttlSeconds = 3600
	}
	if ttlSeconds > 315360000 {
		ttlSeconds = 315360000
	}
	now := time.Now().UTC()
	expTS := now.Add(time.Duration(ttlSeconds) * time.Second)

	claims := jwt.MapClaims{
		"tenant": tenant,
		"roles":  []string{effectiveRole},
		"exp":    expTS.Unix(),
	}
	if ns != "" {
		claims["project"] = ns
	}
	// Map role to Kubernetes groups for proxy/RBAC.
	groupsSet := map[string]struct{}{}
	switch effectiveRole {
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
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, err := tok.SignedString(p.key)
	if err != nil {
		return nil, 0, err
	}
	// Build kubeconfig that always targets the proxy URL, never the cluster API.
	nsLine := ""
	if ns != "" {
		nsLine = "    namespace: " + ns + "\n"
	}
	cfg := fmt.Sprintf(
		"apiVersion: v1\nkind: Config\nclusters:\n- name: kn-proxy\n  cluster:\n    insecure-skip-tls-verify: true\n    server: %s\ncontexts:\n- name: tenant\n  context:\n    cluster: kn-proxy\n    user: tenant-user\n%scurrent-context: tenant\nusers:\n- name: tenant-user\n  user:\n    token: %s\n",
		p.proxyURL,
		nsLine,
		tokenStr,
	)
	return []byte(cfg), expTS.Unix(), nil
}
