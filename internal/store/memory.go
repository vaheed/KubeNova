package store

import (
	"context"
	"fmt"
	"github.com/vaheed/kubenova/pkg/types"
	"sort"
	"strings"
	"sync"
)

type Memory struct {
	mu           sync.RWMutex
	tenants      map[string]types.Tenant
	projects     map[string]map[string]types.Project        // tenant -> name
	apps         map[string]map[string]map[string]types.App // tenant -> project -> name
	policysets   map[string]map[string]types.PolicySet      // tenantUID -> name -> policyset
	clusters     map[types.ID]memCluster
	byName       map[string]types.ID
	byUID        map[string]types.ID
	tenantByUID  map[string]string        // uid -> tenant name
	projectByUID map[string]types.Project // uid -> project (tenant name + name)
	appByUID     map[string]types.App     // uid -> app (tenant/project names + name)
	sandboxes    map[string]map[string]types.Sandbox
	sandboxByUID map[string]types.Sandbox
	evts         []memEvent
}

func NewMemory() *Memory {
	return &Memory{
		tenants:      map[string]types.Tenant{},
		projects:     map[string]map[string]types.Project{},
		apps:         map[string]map[string]map[string]types.App{},
		policysets:   map[string]map[string]types.PolicySet{},
		clusters:     map[types.ID]memCluster{},
		byName:       map[string]types.ID{},
		byUID:        map[string]types.ID{},
		tenantByUID:  map[string]string{},
		projectByUID: map[string]types.Project{},
		appByUID:     map[string]types.App{},
		sandboxes:    map[string]map[string]types.Sandbox{},
		sandboxByUID: map[string]types.Sandbox{},
	}
}

func (m *Memory) Close(ctx context.Context) error { return nil }

// Health implements a no-op readiness check for the in-memory store.
func (m *Memory) Health(ctx context.Context) error { return nil }

func (m *Memory) CreateTenant(ctx context.Context, t types.Tenant) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.tenants[t.Name]; ok {
		return nil
	}
	t.CreatedAt = stamp(t.CreatedAt)
	if t.UID == "" {
		t.UID = uuidNew()
	}
	m.tenants[t.Name] = t
	m.tenantByUID[t.UID] = t.Name
	return nil
}
func (m *Memory) GetTenant(ctx context.Context, name string) (types.Tenant, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	t, ok := m.tenants[name]
	if !ok {
		return types.Tenant{}, ErrNotFound
	}
	return t, nil
}

func (m *Memory) GetTenantByUID(ctx context.Context, uid string) (types.Tenant, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if name, ok := m.tenantByUID[uid]; ok {
		if t, ok2 := m.tenants[name]; ok2 {
			return t, nil
		}
	}
	return types.Tenant{}, ErrNotFound
}
func (m *Memory) ListTenants(ctx context.Context) ([]types.Tenant, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]types.Tenant, 0, len(m.tenants))
	for _, t := range m.tenants {
		out = append(out, t)
	}
	return out, nil
}
func (m *Memory) UpdateTenant(ctx context.Context, t types.Tenant) error {
	return m.CreateTenant(ctx, t)
}
func (m *Memory) DeleteTenant(ctx context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if t, ok := m.tenants[name]; ok {
		delete(m.tenantByUID, t.UID)
		delete(m.policysets, t.UID)
	}
	delete(m.tenants, name)
	delete(m.projects, name)
	delete(m.apps, name)
	return nil
}

// PolicySets

func (m *Memory) CreatePolicySet(ctx context.Context, ps types.PolicySet) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.policysets[ps.Tenant] == nil {
		m.policysets[ps.Tenant] = map[string]types.PolicySet{}
	}
	if existing, ok := m.policysets[ps.Tenant][ps.Name]; ok {
		// Merge policies but keep CreatedAt from existing
		ps.CreatedAt = existing.CreatedAt
	} else {
		ps.CreatedAt = stamp(ps.CreatedAt)
	}
	m.policysets[ps.Tenant][ps.Name] = ps
	return nil
}

func (m *Memory) ListPolicySets(ctx context.Context, tenantUID string) ([]types.PolicySet, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	mp := m.policysets[tenantUID]
	out := make([]types.PolicySet, 0, len(mp))
	for _, ps := range mp {
		out = append(out, ps)
	}
	return out, nil
}

func (m *Memory) GetPolicySet(ctx context.Context, tenantUID, name string) (types.PolicySet, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if mp := m.policysets[tenantUID]; mp != nil {
		if ps, ok := mp[name]; ok {
			return ps, nil
		}
	}
	return types.PolicySet{}, ErrNotFound
}

func (m *Memory) UpdatePolicySet(ctx context.Context, ps types.PolicySet) error {
	return m.CreatePolicySet(ctx, ps)
}

func (m *Memory) DeletePolicySet(ctx context.Context, tenantUID, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if mp := m.policysets[tenantUID]; mp != nil {
		delete(mp, name)
	}
	return nil
}

func (m *Memory) CreateProject(ctx context.Context, p types.Project) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.projects[p.Tenant]; !ok {
		m.projects[p.Tenant] = map[string]types.Project{}
	}
	if _, ok := m.projects[p.Tenant][p.Name]; ok {
		return nil
	}
	p.CreatedAt = stamp(p.CreatedAt)
	if p.UID == "" {
		p.UID = uuidNew()
	}
	m.projects[p.Tenant][p.Name] = p
	m.projectByUID[p.UID] = p
	return nil
}
func (m *Memory) GetProject(ctx context.Context, tenant, name string) (types.Project, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if mp, ok := m.projects[tenant]; ok {
		if p, ok2 := mp[name]; ok2 {
			return p, nil
		}
	}
	return types.Project{}, ErrNotFound
}

func (m *Memory) GetProjectByUID(ctx context.Context, uid string) (types.Project, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if pr, ok := m.projectByUID[uid]; ok {
		return pr, nil
	}
	return types.Project{}, ErrNotFound
}
func (m *Memory) ListProjects(ctx context.Context, tenant string) ([]types.Project, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	mp, ok := m.projects[tenant]
	if !ok {
		return []types.Project{}, nil
	}
	out := make([]types.Project, 0, len(mp))
	for _, p := range mp {
		out = append(out, p)
	}
	return out, nil
}
func (m *Memory) UpdateProject(ctx context.Context, p types.Project) error {
	return m.CreateProject(ctx, p)
}
func (m *Memory) DeleteProject(ctx context.Context, tenant, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if mp, ok := m.projects[tenant]; ok {
		if pr, ok2 := mp[name]; ok2 {
			delete(m.projectByUID, pr.UID)
		}
		delete(mp, name)
	}
	if ma, ok := m.apps[tenant]; ok {
		delete(ma, name)
	}
	return nil
}

func (m *Memory) CreateSandbox(ctx context.Context, sb types.Sandbox) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.sandboxes[sb.Tenant]; !ok {
		m.sandboxes[sb.Tenant] = map[string]types.Sandbox{}
	}
	if existing, ok := m.sandboxes[sb.Tenant][sb.Name]; ok {
		sb.UID = existing.UID
		sb.CreatedAt = existing.CreatedAt
	} else {
		sb.CreatedAt = stamp(sb.CreatedAt)
		if sb.UID == "" {
			sb.UID = uuidNew()
		}
	}
	m.sandboxes[sb.Tenant][sb.Name] = sb
	m.sandboxByUID[sb.UID] = sb
	return nil
}

func (m *Memory) GetSandbox(ctx context.Context, tenant, name string) (types.Sandbox, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if mp, ok := m.sandboxes[tenant]; ok {
		if sb, ok2 := mp[name]; ok2 {
			return sb, nil
		}
	}
	return types.Sandbox{}, ErrNotFound
}

func (m *Memory) GetSandboxByUID(ctx context.Context, uid string) (types.Sandbox, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if sb, ok := m.sandboxByUID[uid]; ok {
		return sb, nil
	}
	return types.Sandbox{}, ErrNotFound
}

func (m *Memory) ListSandboxes(ctx context.Context, tenant string) ([]types.Sandbox, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if mp, ok := m.sandboxes[tenant]; ok {
		out := make([]types.Sandbox, 0, len(mp))
		for _, sb := range mp {
			out = append(out, sb)
		}
		return out, nil
	}
	return []types.Sandbox{}, nil
}

func (m *Memory) DeleteSandbox(ctx context.Context, tenant, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if mp, ok := m.sandboxes[tenant]; ok {
		if sb, ok2 := mp[name]; ok2 {
			delete(m.sandboxByUID, sb.UID)
		}
		delete(mp, name)
	}
	return nil
}

func (m *Memory) CreateApp(ctx context.Context, a types.App) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.apps[a.Tenant]; !ok {
		m.apps[a.Tenant] = map[string]map[string]types.App{}
	}
	if _, ok := m.apps[a.Tenant][a.Project]; !ok {
		m.apps[a.Tenant][a.Project] = map[string]types.App{}
	}
	if _, ok := m.apps[a.Tenant][a.Project][a.Name]; ok {
		return nil
	}
	a.CreatedAt = stamp(a.CreatedAt)
	if a.UID == "" {
		a.UID = uuidNew()
	}
	m.apps[a.Tenant][a.Project][a.Name] = a
	m.appByUID[a.UID] = a
	return nil
}
func (m *Memory) GetApp(ctx context.Context, tenant, project, name string) (types.App, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ma, ok := m.apps[tenant]
	if !ok {
		return types.App{}, ErrNotFound
	}
	mp, ok := ma[project]
	if !ok {
		return types.App{}, ErrNotFound
	}
	a, ok := mp[name]
	if !ok {
		return types.App{}, ErrNotFound
	}
	return a, nil
}

func (m *Memory) GetAppByUID(ctx context.Context, uid string) (types.App, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if ap, ok := m.appByUID[uid]; ok {
		return ap, nil
	}
	return types.App{}, ErrNotFound
}
func (m *Memory) ListApps(ctx context.Context, tenant, project string) ([]types.App, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ma, ok := m.apps[tenant]
	if !ok {
		return []types.App{}, nil
	}
	mp, ok := ma[project]
	if !ok {
		return []types.App{}, nil
	}
	out := make([]types.App, 0, len(mp))
	for _, a := range mp {
		out = append(out, a)
	}
	return out, nil
}
func (m *Memory) UpdateApp(ctx context.Context, a types.App) error { return m.CreateApp(ctx, a) }
func (m *Memory) DeleteApp(ctx context.Context, tenant, project, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if ma, ok := m.apps[tenant]; ok {
		if mp, ok2 := ma[project]; ok2 {
			if ap, ok3 := mp[name]; ok3 {
				delete(m.appByUID, ap.UID)
			}
			delete(mp, name)
		}
	}
	return nil
}

// DebugDump is convenient for tests
func (m *Memory) DebugDump() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return fmt.Sprintf("tenants=%d projects=%d apps=%d", len(m.tenants), len(m.projects), len(m.apps))
}

type memCluster struct {
	c   types.Cluster
	enc string
}

func (m *Memory) CreateCluster(ctx context.Context, c types.Cluster, kubeconfigEnc string) (types.ID, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id := types.NewID()
	c.ID = id
	if c.UID == "" {
		c.UID = uuidNew()
	}
	m.clusters[id] = memCluster{c: c, enc: kubeconfigEnc}
	m.byName[c.Name] = id
	if m.byUID == nil {
		m.byUID = map[string]types.ID{}
	}
	m.byUID[c.UID] = id
	return id, nil
}

func (m *Memory) GetCluster(ctx context.Context, id types.ID) (types.Cluster, string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	mc, ok := m.clusters[id]
	if !ok {
		return types.Cluster{}, "", ErrNotFound
	}
	return mc.c, mc.enc, nil
}

func (m *Memory) GetClusterByName(ctx context.Context, name string) (types.Cluster, string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if id, ok := m.byName[name]; ok {
		mc := m.clusters[id]
		return mc.c, mc.enc, nil
	}
	return types.Cluster{}, "", ErrNotFound
}

func (m *Memory) GetClusterByUID(ctx context.Context, uid string) (types.Cluster, string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if id, ok := m.byUID[uid]; ok {
		mc := m.clusters[id]
		return mc.c, mc.enc, nil
	}
	return types.Cluster{}, "", ErrNotFound
}

func (m *Memory) DeleteCluster(ctx context.Context, id types.ID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	mc, ok := m.clusters[id]
	if !ok {
		return nil
	}
	delete(m.clusters, id)
	delete(m.byName, mc.c.Name)
	return nil
}

type memEvent struct {
	clusterID *types.ID
	e         types.Event
}

func (m *Memory) AddEvents(ctx context.Context, clusterID *types.ID, evts []types.Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, e := range evts {
		m.evts = append(m.evts, memEvent{clusterID: clusterID, e: e})
	}
	return nil
}

func (m *Memory) AddConditionHistory(ctx context.Context, clusterID types.ID, conds []types.Condition) error {
	return nil
}

func (m *Memory) ListClusterEvents(ctx context.Context, clusterID types.ID, limit int) ([]types.Event, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := []types.Event{}
	for i := len(m.evts) - 1; i >= 0 && len(out) < limit; i-- {
		me := m.evts[i]
		if me.clusterID != nil && *me.clusterID == clusterID {
			out = append(out, me.e)
		}
	}
	return out, nil
}

// ListClusters implements id-based pagination and simple labelSelector filtering (k=v[,k2=v2]).
func (m *Memory) ListClusters(ctx context.Context, limit int, cursor string, labelSelector string) ([]types.Cluster, string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	// parse cursor as UUID; if invalid treat as zero UUID (no lower bound)
	lastID, _ := types.ParseID(cursor)
	// parse label selector
	want := map[string]string{}
	if labelSelector != "" {
		parts := strings.Split(labelSelector, ",")
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			kv := strings.SplitN(p, "=", 2)
			if len(kv) == 2 {
				want[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
			}
		}
	}
	// collect ids sorted lexicographically by string
	ids := make([]string, 0, len(m.clusters))
	idByStr := make(map[string]types.ID, len(m.clusters))
	for id := range m.clusters {
		s := id.String()
		ids = append(ids, s)
		idByStr[s] = id
	}
	sort.Strings(ids)
	out := make([]types.Cluster, 0, limit)
	var next string
	for _, sid := range ids {
		id := idByStr[sid]
		if !types.IsZeroID(lastID) && sid <= lastID.String() {
			continue
		}
		mc := m.clusters[id]
		if matchesLabels(mc.c.Labels, want) {
			out = append(out, mc.c)
			if len(out) == limit {
				next = sid
				break
			}
		}
	}
	return out, next, nil
}

func matchesLabels(have map[string]string, want map[string]string) bool {
	if len(want) == 0 {
		return true
	}
	for k, v := range want {
		if hv, ok := have[k]; !ok || hv != v {
			return false
		}
	}
	return true
}

// helper for consistent UID generation (UUIDv4, lowercase)
func uuidNew() string { return types.NewID().String() }
