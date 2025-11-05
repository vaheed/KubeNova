package store

import (
    "context"
    "fmt"
    "sync"

    "github.com/vaheed/kubenova/pkg/types"
    "sort"
    "strings"
)

type Memory struct {
	mu       sync.RWMutex
	tenants  map[string]types.Tenant
	projects map[string]map[string]types.Project        // tenant -> name
	apps     map[string]map[string]map[string]types.App // tenant -> project -> name
    clusters map[int]memCluster
    byName   map[string]int
	nextID   int
    evts     []memEvent
}

func NewMemory() *Memory {
    return &Memory{tenants: map[string]types.Tenant{}, projects: map[string]map[string]types.Project{}, apps: map[string]map[string]map[string]types.App{}, clusters: map[int]memCluster{}, byName: map[string]int{}, nextID: 1}
}

func (m *Memory) Close(ctx context.Context) error { return nil }

func (m *Memory) CreateTenant(ctx context.Context, t types.Tenant) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.tenants[t.Name]; ok {
		return nil
	}
	t.CreatedAt = stamp(t.CreatedAt)
	m.tenants[t.Name] = t
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
	delete(m.tenants, name)
	delete(m.projects, name)
	delete(m.apps, name)
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
	m.projects[p.Tenant][p.Name] = p
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
		delete(mp, name)
	}
	if ma, ok := m.apps[tenant]; ok {
		delete(ma, name)
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
	m.apps[a.Tenant][a.Project][a.Name] = a
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

func (m *Memory) CreateCluster(ctx context.Context, c types.Cluster, kubeconfigEnc string) (int, error) {
    m.mu.Lock()
    defer m.mu.Unlock()
    id := m.nextID
    m.nextID++
    c.ID = id
    m.clusters[id] = memCluster{c: c, enc: kubeconfigEnc}
    m.byName[c.Name] = id
    return id, nil
}

func (m *Memory) GetCluster(ctx context.Context, id int) (types.Cluster, string, error) {
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

type memEvent struct {
	clusterID *int
	e         types.Event
}

func (m *Memory) AddEvents(ctx context.Context, clusterID *int, evts []types.Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, e := range evts {
		m.evts = append(m.evts, memEvent{clusterID: clusterID, e: e})
	}
	return nil
}

func (m *Memory) AddConditionHistory(ctx context.Context, clusterID int, conds []types.Condition) error {
	return nil
}

func (m *Memory) ListClusterEvents(ctx context.Context, clusterID int, limit int) ([]types.Event, error) {
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
    // parse cursor as last id
    last := 0
    for i := 0; i < len(cursor); i++ {
        c := cursor[i]
        if c < '0' || c > '9' { break }
        last = last*10 + int(c-'0')
    }
    // parse label selector
    want := map[string]string{}
    if labelSelector != "" {
        parts := strings.Split(labelSelector, ",")
        for _, p := range parts {
            p = strings.TrimSpace(p)
            if p == "" { continue }
            kv := strings.SplitN(p, "=", 2)
            if len(kv) == 2 { want[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1]) }
        }
    }
    // collect ids sorted
    ids := make([]int, 0, len(m.clusters))
    for id := range m.clusters { ids = append(ids, id) }
    sort.Ints(ids)
    out := make([]types.Cluster, 0, limit)
    var next string
    for _, id := range ids {
        if id <= last { continue }
        mc := m.clusters[id]
        if matchesLabels(mc.c.Labels, want) {
            out = append(out, mc.c)
            if len(out) == limit {
                next = itoa(id)
                break
            }
        }
    }
    return out, next, nil
}

func matchesLabels(have map[string]string, want map[string]string) bool {
    if len(want) == 0 { return true }
    for k, v := range want {
        if hv, ok := have[k]; !ok || hv != v { return false }
    }
    return true
}

func itoa(n int) string {
    if n == 0 { return "" }
    // simple int to string
    b := make([]byte, 0, 16)
    s := []byte{}
    for n > 0 { s = append(s, byte('0'+(n%10))); n/=10 }
    for i := len(s)-1; i>=0; i-- { b = append(b, s[i]) }
    return string(b)
}
