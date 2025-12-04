package store

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/vaheed/kubenova/pkg/types"
)

type memoryStore struct {
	mu       sync.RWMutex
	clusters map[string]*types.Cluster
	tenants  map[string]*types.Tenant
	projects map[string]*types.Project
	apps     map[string]*types.App
}

// NewMemoryStore returns an in-memory Store suitable for development and tests.
func NewMemoryStore() Store {
	return &memoryStore{
		clusters: make(map[string]*types.Cluster),
		tenants:  make(map[string]*types.Tenant),
		projects: make(map[string]*types.Project),
		apps:     make(map[string]*types.App),
	}
}

func (m *memoryStore) CreateCluster(ctx context.Context, c *types.Cluster) error {
	assignIDs(c, nil, nil, nil)
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, existing := range m.clusters {
		if existing.Name == c.Name {
			return ErrConflict
		}
	}
	m.clusters[c.ID] = clone(c)
	return nil
}

func (m *memoryStore) ListClusters(ctx context.Context) ([]*types.Cluster, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*types.Cluster, 0, len(m.clusters))
	for _, c := range m.clusters {
		out = append(out, clone(c))
	}
	return out, nil
}

func (m *memoryStore) GetCluster(ctx context.Context, id string) (*types.Cluster, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	c, ok := m.clusters[id]
	if !ok {
		return nil, ErrNotFound
	}
	return clone(c), nil
}

func (m *memoryStore) DeleteCluster(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.clusters[id]; !ok {
		return ErrNotFound
	}
	delete(m.clusters, id)
	for tid, t := range m.tenants {
		if t.ClusterID == id {
			delete(m.tenants, tid)
		}
	}
	for pid, p := range m.projects {
		if p.ClusterID == id {
			delete(m.projects, pid)
		}
	}
	for aid, a := range m.apps {
		if a.ClusterID == id {
			delete(m.apps, aid)
		}
	}
	return nil
}

func (m *memoryStore) Health(ctx context.Context) error {
	return nil
}

func (m *memoryStore) UpdateCluster(ctx context.Context, c *types.Cluster) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cur, ok := m.clusters[c.ID]
	if !ok {
		return ErrNotFound
	}
	c.CreatedAt = cur.CreatedAt
	m.clusters[c.ID] = clone(c)
	return nil
}

func (m *memoryStore) CreateTenant(ctx context.Context, t *types.Tenant) error {
	assignIDs(nil, t, nil, nil)
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.clusters[t.ClusterID]; !ok {
		return fmt.Errorf("cluster %s: %w", t.ClusterID, ErrNotFound)
	}
	for _, existing := range m.tenants {
		if existing.ClusterID == t.ClusterID && existing.Name == t.Name {
			return ErrConflict
		}
	}
	m.tenants[t.ID] = clone(t)
	return nil
}

func (m *memoryStore) ListTenants(ctx context.Context, clusterID string) ([]*types.Tenant, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := []*types.Tenant{}
	for _, t := range m.tenants {
		if clusterID == "" || t.ClusterID == clusterID {
			out = append(out, clone(t))
		}
	}
	return out, nil
}

func (m *memoryStore) GetTenant(ctx context.Context, clusterID, tenantID string) (*types.Tenant, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	t, ok := m.tenants[tenantID]
	if !ok || (clusterID != "" && t.ClusterID != clusterID) {
		return nil, ErrNotFound
	}
	return clone(t), nil
}

func (m *memoryStore) UpdateTenant(ctx context.Context, t *types.Tenant) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cur, ok := m.tenants[t.ID]
	if !ok {
		return ErrNotFound
	}
	if cur.ClusterID != t.ClusterID {
		return errors.New("cluster mismatch")
	}
	t.CreatedAt = cur.CreatedAt
	if t.UpdatedAt.IsZero() {
		t.UpdatedAt = time.Now().UTC()
	}
	m.tenants[t.ID] = clone(t)
	return nil
}

func (m *memoryStore) DeleteTenant(ctx context.Context, clusterID, tenantID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.tenants[tenantID]
	if !ok || (clusterID != "" && t.ClusterID != clusterID) {
		return ErrNotFound
	}
	delete(m.tenants, tenantID)
	for pid, p := range m.projects {
		if p.TenantID == tenantID {
			delete(m.projects, pid)
		}
	}
	for aid, a := range m.apps {
		if a.TenantID == tenantID {
			delete(m.apps, aid)
		}
	}
	return nil
}

func (m *memoryStore) CreateProject(ctx context.Context, p *types.Project) error {
	assignIDs(nil, nil, p, nil)
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.tenants[p.TenantID]
	if !ok || t.ClusterID != p.ClusterID {
		return fmt.Errorf("tenant %s: %w", p.TenantID, ErrNotFound)
	}
	for _, existing := range m.projects {
		if existing.TenantID == p.TenantID && existing.Name == p.Name {
			return ErrConflict
		}
	}
	m.projects[p.ID] = clone(p)
	return nil
}

func (m *memoryStore) ListProjects(ctx context.Context, clusterID, tenantID string) ([]*types.Project, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := []*types.Project{}
	for _, p := range m.projects {
		if (clusterID == "" || p.ClusterID == clusterID) && (tenantID == "" || p.TenantID == tenantID) {
			out = append(out, clone(p))
		}
	}
	return out, nil
}

func (m *memoryStore) GetProject(ctx context.Context, clusterID, tenantID, projectID string) (*types.Project, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	p, ok := m.projects[projectID]
	if !ok || (clusterID != "" && p.ClusterID != clusterID) || (tenantID != "" && p.TenantID != tenantID) {
		return nil, ErrNotFound
	}
	return clone(p), nil
}

func (m *memoryStore) UpdateProject(ctx context.Context, p *types.Project) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cur, ok := m.projects[p.ID]
	if !ok {
		return ErrNotFound
	}
	if cur.ClusterID != p.ClusterID || cur.TenantID != p.TenantID {
		return errors.New("parent mismatch")
	}
	p.CreatedAt = cur.CreatedAt
	if p.UpdatedAt.IsZero() {
		p.UpdatedAt = time.Now().UTC()
	}
	m.projects[p.ID] = clone(p)
	return nil
}

func (m *memoryStore) DeleteProject(ctx context.Context, clusterID, tenantID, projectID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.projects[projectID]
	if !ok || (clusterID != "" && p.ClusterID != clusterID) || (tenantID != "" && p.TenantID != tenantID) {
		return ErrNotFound
	}
	delete(m.projects, projectID)
	for aid, a := range m.apps {
		if a.ProjectID == projectID {
			delete(m.apps, aid)
		}
	}
	return nil
}

func (m *memoryStore) CreateApp(ctx context.Context, a *types.App) error {
	assignIDs(nil, nil, nil, a)
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.projects[a.ProjectID]
	if !ok || p.TenantID != a.TenantID || p.ClusterID != a.ClusterID {
		return fmt.Errorf("project %s: %w", a.ProjectID, ErrNotFound)
	}
	for _, existing := range m.apps {
		if existing.ProjectID == a.ProjectID && existing.Name == a.Name {
			return ErrConflict
		}
	}
	m.apps[a.ID] = clone(a)
	return nil
}

func (m *memoryStore) ListApps(ctx context.Context, clusterID, tenantID, projectID string) ([]*types.App, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := []*types.App{}
	for _, a := range m.apps {
		if (clusterID == "" || a.ClusterID == clusterID) && (tenantID == "" || a.TenantID == tenantID) && (projectID == "" || a.ProjectID == projectID) {
			out = append(out, clone(a))
		}
	}
	return out, nil
}

func (m *memoryStore) GetApp(ctx context.Context, clusterID, tenantID, projectID, appID string) (*types.App, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	a, ok := m.apps[appID]
	if !ok || (clusterID != "" && a.ClusterID != clusterID) || (tenantID != "" && a.TenantID != tenantID) || (projectID != "" && a.ProjectID != projectID) {
		return nil, ErrNotFound
	}
	return clone(a), nil
}

func (m *memoryStore) UpdateApp(ctx context.Context, a *types.App) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cur, ok := m.apps[a.ID]
	if !ok {
		return ErrNotFound
	}
	if cur.ClusterID != a.ClusterID || cur.TenantID != a.TenantID || cur.ProjectID != a.ProjectID {
		return errors.New("parent mismatch")
	}
	a.CreatedAt = cur.CreatedAt
	if a.UpdatedAt.IsZero() {
		a.UpdatedAt = time.Now().UTC()
	}
	if a.Revision == 0 {
		a.Revision = cur.Revision
	}
	if len(a.Revisions) == 0 {
		a.Revisions = cur.Revisions
	}
	m.apps[a.ID] = clone(a)
	return nil
}

func (m *memoryStore) DeleteApp(ctx context.Context, clusterID, tenantID, projectID, appID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	a, ok := m.apps[appID]
	if !ok || (clusterID != "" && a.ClusterID != clusterID) || (tenantID != "" && a.TenantID != tenantID) || (projectID != "" && a.ProjectID != projectID) {
		return ErrNotFound
	}
	delete(m.apps, appID)
	return nil
}
