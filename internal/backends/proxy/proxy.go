package proxy

import "context"

// Client abstracts issuing scoped kubeconfigs via a proxy endpoint.
type Client interface {
	Issue(ctx context.Context, tenant string, project *string, role string, ttlSeconds int) (kubeconfig []byte, expiresAt int64, err error)
}

// New returns a no-op stub which issues a placeholder kubeconfig pointing to the proxy URL.
func New(_ []byte, _ string) Client { return &noop{} }

type noop struct{}

func (n *noop) Issue(ctx context.Context, tenant string, project *string, role string, ttlSeconds int) ([]byte, int64, error) {
	_ = ctx
	_ = tenant
	_ = project
	_ = role
	_ = ttlSeconds
	return []byte("apiVersion: v1\nclusters: []\ncontexts: []\n"), 0, nil
}
