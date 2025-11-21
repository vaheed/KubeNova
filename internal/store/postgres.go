package store

import (
	"context"
	"encoding/json"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vaheed/kubenova/pkg/types"
	"strconv"
	"strings"
)

// Postgres implements Store using pgx. It applies the bootstrap migration on open.
type Postgres struct{ db *pgxpool.Pool }

func NewPostgres(ctx context.Context, url string) (*Postgres, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, err
	}
	db, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	p := &Postgres{db: db}
	if err := p.applyMigration(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return p, nil
}

func (p *Postgres) applyMigration(ctx context.Context) error {
	// small migration runner: try file, fallback to embedded baseline
	path := "db/migrations/0001_init.sql"
	b, err := os.ReadFile(path)
	if err != nil {
		b = []byte(defaultMigrationSQL)
	}
	_, err = p.db.Exec(ctx, string(b))
	return err
}

func (p *Postgres) Close(ctx context.Context) error { p.db.Close(); return nil }

// Health reports whether the underlying database connection pool is usable.
// It is safe to call frequently from readiness checks.
func (p *Postgres) Health(ctx context.Context) error {
	if p.db == nil {
		return nil
	}
	return p.db.Ping(ctx)
}

func (p *Postgres) CreateTenant(ctx context.Context, t types.Tenant) error {
	t.CreatedAt = stamp(t.CreatedAt)
	if t.ID == (types.ID{}) {
		t.ID = types.NewID()
	}
	labels := mapToJSONB(t.Labels)
	owners := t.Owners
	_, err := p.db.Exec(ctx, `
INSERT INTO tenants(id, uid, name, owners, labels, created_at)
VALUES ($1,$2,$3,$4,$5,$6)
ON CONFLICT (name) DO UPDATE
SET owners = EXCLUDED.owners,
    labels = EXCLUDED.labels
`, t.ID.String(), t.ID.String(), t.Name, owners, labels, t.CreatedAt)
	return err
}
func (p *Postgres) GetTenant(ctx context.Context, name string) (types.Tenant, error) {
	var t types.Tenant
	var labels map[string]string
	err := p.db.QueryRow(ctx, `
SELECT id, name, owners, labels, created_at
FROM tenants
WHERE name=$1
`, name).Scan(&t.ID, &t.Name, &t.Owners, &labels, &t.CreatedAt)
	if err != nil {
		return types.Tenant{}, ErrNotFound
	}
	t.Labels = labels
	return t, nil
}
func (p *Postgres) GetTenantByID(ctx context.Context, id string) (types.Tenant, error) {
	var t types.Tenant
	var labels map[string]string
	err := p.db.QueryRow(ctx, `
SELECT id, name, owners, labels, created_at
FROM tenants
WHERE id=$1
`, id).Scan(&t.ID, &t.Name, &t.Owners, &labels, &t.CreatedAt)
	if err != nil {
		return types.Tenant{}, ErrNotFound
	}
	t.Labels = labels
	return t, nil
}
func (p *Postgres) ListTenants(ctx context.Context) ([]types.Tenant, error) {
	rows, err := p.db.Query(ctx, `
SELECT id, name, owners, labels, created_at
FROM tenants
ORDER BY name
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []types.Tenant
	for rows.Next() {
		var t types.Tenant
		var labels map[string]string
		if err := rows.Scan(&t.ID, &t.Name, &t.Owners, &labels, &t.CreatedAt); err != nil {
			return nil, err
		}
		t.Labels = labels
		out = append(out, t)
	}
	return out, nil
}
func (p *Postgres) UpdateTenant(ctx context.Context, t types.Tenant) error {
	return p.CreateTenant(ctx, t)
}
func (p *Postgres) DeleteTenant(ctx context.Context, name string) error {
	_, err := p.db.Exec(ctx, `DELETE FROM tenants WHERE name=$1`, name)
	return err
}

func (p *Postgres) CreateProject(ctx context.Context, pr types.Project) error {
	pr.CreatedAt = stamp(pr.CreatedAt)
	if pr.ID == (types.ID{}) {
		pr.ID = types.NewID()
	}
	_, err := p.db.Exec(ctx, `INSERT INTO projects(id, uid, tenant, name) VALUES ($1,$2,$3,$4) ON CONFLICT (tenant,name) DO NOTHING`, pr.ID.String(), pr.ID.String(), pr.Tenant, pr.Name)
	return err
}
func (p *Postgres) GetProject(ctx context.Context, tenant, name string) (types.Project, error) {
	var pr types.Project
	err := p.db.QueryRow(ctx, `SELECT id, tenant, name FROM projects WHERE tenant=$1 AND name=$2`, tenant, name).Scan(&pr.ID, &pr.Tenant, &pr.Name)
	if err != nil {
		return types.Project{}, ErrNotFound
	}
	return pr, nil
}
func (p *Postgres) GetProjectByID(ctx context.Context, id string) (types.Project, error) {
	var pr types.Project
	err := p.db.QueryRow(ctx, `SELECT id, tenant, name FROM projects WHERE id=$1`, id).Scan(&pr.ID, &pr.Tenant, &pr.Name)
	if err != nil {
		return types.Project{}, ErrNotFound
	}
	return pr, nil
}
func (p *Postgres) ListProjects(ctx context.Context, tenant string) ([]types.Project, error) {
	rows, err := p.db.Query(ctx, `SELECT id, tenant, name FROM projects WHERE tenant=$1 ORDER BY name`, tenant)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []types.Project
	for rows.Next() {
		var pr types.Project
		if err := rows.Scan(&pr.ID, &pr.Tenant, &pr.Name); err != nil {
			return nil, err
		}
		out = append(out, pr)
	}
	return out, nil
}
func (p *Postgres) UpdateProject(ctx context.Context, pr types.Project) error {
	return p.CreateProject(ctx, pr)
}
func (p *Postgres) DeleteProject(ctx context.Context, tenant, name string) error {
	_, err := p.db.Exec(ctx, `DELETE FROM projects WHERE tenant=$1 AND name=$2`, tenant, name)
	return err
}

func (p *Postgres) CreateSandbox(ctx context.Context, sb types.Sandbox) error {
	sb.CreatedAt = stamp(sb.CreatedAt)
	if sb.ID == (types.ID{}) {
		sb.ID = types.NewID()
	}
	_, err := p.db.Exec(ctx, `
INSERT INTO sandboxes(id, uid, tenant, name, namespace, created_at)
VALUES ($1,$2,$3,$4,$5,$6)
ON CONFLICT (tenant,name) DO UPDATE
SET namespace = EXCLUDED.namespace
`, sb.ID.String(), sb.ID.String(), sb.Tenant, sb.Name, sb.Namespace, sb.CreatedAt)
	return err
}

func (p *Postgres) GetSandbox(ctx context.Context, tenant, name string) (types.Sandbox, error) {
	var sb types.Sandbox
	err := p.db.QueryRow(ctx, `SELECT id, tenant, name, namespace, created_at FROM sandboxes WHERE tenant=$1 AND name=$2`, tenant, name).
		Scan(&sb.ID, &sb.Tenant, &sb.Name, &sb.Namespace, &sb.CreatedAt)
	if err != nil {
		return types.Sandbox{}, ErrNotFound
	}
	return sb, nil
}

func (p *Postgres) GetSandboxByID(ctx context.Context, id string) (types.Sandbox, error) {
	var sb types.Sandbox
	err := p.db.QueryRow(ctx, `SELECT id, tenant, name, namespace, created_at FROM sandboxes WHERE id=$1`, id).
		Scan(&sb.ID, &sb.Tenant, &sb.Name, &sb.Namespace, &sb.CreatedAt)
	if err != nil {
		return types.Sandbox{}, ErrNotFound
	}
	return sb, nil
}

func (p *Postgres) ListSandboxes(ctx context.Context, tenant string) ([]types.Sandbox, error) {
	rows, err := p.db.Query(ctx, `SELECT id, tenant, name, namespace, created_at FROM sandboxes WHERE tenant=$1 ORDER BY name`, tenant)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []types.Sandbox{}
	for rows.Next() {
		var sb types.Sandbox
		if err := rows.Scan(&sb.ID, &sb.Tenant, &sb.Name, &sb.Namespace, &sb.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, sb)
	}
	return out, nil
}

func (p *Postgres) DeleteSandbox(ctx context.Context, tenant, name string) error {
	_, err := p.db.Exec(ctx, `DELETE FROM sandboxes WHERE tenant=$1 AND name=$2`, tenant, name)
	return err
}

func (p *Postgres) CreateApp(ctx context.Context, a types.App) error {
	a.CreatedAt = stamp(a.CreatedAt)
	if a.ID == (types.ID{}) {
		a.ID = types.NewID()
	}
	spec, err := marshalAppSpecPayload(a)
	if err != nil {
		return err
	}
	_, err = p.db.Exec(ctx, `
INSERT INTO apps(id, uid, tenant, project, name, spec, created_at)
VALUES ($1,$2,$3,$4,$5,$6,$7)
ON CONFLICT (tenant,project,name) DO UPDATE
SET spec = EXCLUDED.spec
`, a.ID.String(), a.ID.String(), a.Tenant, a.Project, a.Name, spec, a.CreatedAt)
	return err
}
func (p *Postgres) GetApp(ctx context.Context, tenant, project, name string) (types.App, error) {
	var a types.App
	var spec []byte
	err := p.db.QueryRow(ctx, `SELECT id, tenant, project, name, spec, created_at FROM apps WHERE tenant=$1 AND project=$2 AND name=$3`, tenant, project, name).Scan(&a.ID, &a.Tenant, &a.Project, &a.Name, &spec, &a.CreatedAt)
	if err != nil {
		return types.App{}, ErrNotFound
	}
	_ = applyAppSpecPayload(&a, spec)
	return a, nil
}
func (p *Postgres) GetAppByID(ctx context.Context, id string) (types.App, error) {
	var a types.App
	var spec []byte
	err := p.db.QueryRow(ctx, `SELECT id, tenant, project, name, spec, created_at FROM apps WHERE id=$1`, id).Scan(&a.ID, &a.Tenant, &a.Project, &a.Name, &spec, &a.CreatedAt)
	if err != nil {
		return types.App{}, ErrNotFound
	}
	_ = applyAppSpecPayload(&a, spec)
	return a, nil
}
func (p *Postgres) ListApps(ctx context.Context, tenant, project string) ([]types.App, error) {
	rows, err := p.db.Query(ctx, `SELECT id, tenant, project, name, spec, created_at FROM apps WHERE tenant=$1 AND project=$2 ORDER BY name`, tenant, project)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []types.App
	for rows.Next() {
		var a types.App
		var spec []byte
		if err := rows.Scan(&a.ID, &a.Tenant, &a.Project, &a.Name, &spec, &a.CreatedAt); err != nil {
			return nil, err
		}
		_ = applyAppSpecPayload(&a, spec)
		out = append(out, a)
	}
	return out, nil
}
func (p *Postgres) UpdateApp(ctx context.Context, a types.App) error { return p.CreateApp(ctx, a) }
func (p *Postgres) DeleteApp(ctx context.Context, tenant, project, name string) error {
	_, err := p.db.Exec(ctx, `DELETE FROM apps WHERE tenant=$1 AND project=$2 AND name=$3`, tenant, project, name)
	return err
}

type appSpecPayload struct {
	Description *string           `json:"description,omitempty"`
	Components  *[]map[string]any `json:"components,omitempty"`
	Traits      *[]map[string]any `json:"traits,omitempty"`
	Policies    *[]map[string]any `json:"policies,omitempty"`
	Spec        *types.AppSpec    `json:"spec,omitempty"`
}

func marshalAppSpecPayload(a types.App) ([]byte, error) {
	payload := appSpecPayload{
		Description: a.Description,
		Components:  a.Components,
		Traits:      a.Traits,
		Policies:    a.Policies,
		Spec:        a.Spec,
	}
	return json.Marshal(payload)
}

func applyAppSpecPayload(a *types.App, data []byte) error {
	if a == nil || len(data) == 0 {
		return nil
	}
	var payload appSpecPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}
	a.Description = payload.Description
	a.Components = payload.Components
	a.Traits = payload.Traits
	a.Policies = payload.Policies
	a.Spec = payload.Spec
	return nil
}

// PolicySets

func (p *Postgres) CreatePolicySet(ctx context.Context, ps types.PolicySet) error {
	ps.CreatedAt = stamp(ps.CreatedAt)
	_, err := p.db.Exec(ctx, `
INSERT INTO policysets(tenant_uid, name, spec, created_at)
VALUES ($1,$2,$3,$4)
ON CONFLICT (tenant_uid,name) DO UPDATE
SET spec = EXCLUDED.spec
`, ps.Tenant, ps.Name, ps.Policies, ps.CreatedAt)
	return err
}

func (p *Postgres) ListPolicySets(ctx context.Context, tenantID string) ([]types.PolicySet, error) {
	rows, err := p.db.Query(ctx, `
SELECT name, spec, created_at
FROM policysets
WHERE tenant_uid=$1
ORDER BY name
`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []types.PolicySet
	for rows.Next() {
		var ps types.PolicySet
		ps.Tenant = tenantID
		if err := rows.Scan(&ps.Name, &ps.Policies, &ps.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, ps)
	}
	return out, nil
}

func (p *Postgres) GetPolicySet(ctx context.Context, tenantID, name string) (types.PolicySet, error) {
	var ps types.PolicySet
	ps.Tenant = tenantID
	err := p.db.QueryRow(ctx, `
SELECT name, spec, created_at
FROM policysets
WHERE tenant_uid=$1 AND name=$2
`, tenantID, name).Scan(&ps.Name, &ps.Policies, &ps.CreatedAt)
	if err != nil {
		return types.PolicySet{}, ErrNotFound
	}
	return ps, nil
}

func (p *Postgres) UpdatePolicySet(ctx context.Context, ps types.PolicySet) error {
	return p.CreatePolicySet(ctx, ps)
}

func (p *Postgres) DeletePolicySet(ctx context.Context, tenantID, name string) error {
	_, err := p.db.Exec(ctx, `DELETE FROM policysets WHERE tenant_uid=$1 AND name=$2`, tenantID, name)
	return err
}

// Ensure interface compliance
var _ Store = (*Postgres)(nil)

// Helper for DSN building in tests
func EnvOrMemory() (Store, func(context.Context) error, error) {
	if url := os.Getenv("DATABASE_URL"); url != "" {
		p, err := NewPostgres(context.Background(), url)
		if err != nil {
			return nil, nil, err
		}
		return p, p.Close, nil
	}
	m := NewMemory()
	return m, func(context.Context) error { return nil }, nil
}

// Clusters
func (p *Postgres) CreateCluster(ctx context.Context, c types.Cluster, kubeconfigEnc string) (types.ID, error) {
	if c.ID == (types.ID{}) {
		c.ID = types.NewID()
	}
	var idStr string
	err := p.db.QueryRow(ctx, `INSERT INTO clusters(id, uid, name, kubeconfig_enc, labels) VALUES ($1,$2,$3,$4,$5) RETURNING id::text`, c.ID.String(), c.ID.String(), c.Name, kubeconfigEnc, mapToJSONB(c.Labels)).Scan(&idStr)
	if err != nil {
		return types.ID{}, err
	}
	id, perr := types.ParseID(idStr)
	if perr != nil {
		return types.ID{}, perr
	}
	return id, nil
}

func (p *Postgres) GetCluster(ctx context.Context, id types.ID) (types.Cluster, string, error) {
	var c types.Cluster
	var enc string
	var labels map[string]string
	err := p.db.QueryRow(ctx, `SELECT name, kubeconfig_enc, labels, created_at FROM clusters WHERE id=$1::uuid`, id.String()).Scan(&c.Name, &enc, &labels, &c.CreatedAt)
	if err != nil {
		return types.Cluster{}, "", ErrNotFound
	}
	c.ID = id
	c.Labels = labels
	return c, enc, nil
}

func (p *Postgres) GetClusterByName(ctx context.Context, name string) (types.Cluster, string, error) {
	var c types.Cluster
	var enc string
	var labels map[string]string
	var idStr string
	err := p.db.QueryRow(ctx, `SELECT id::text, kubeconfig_enc, labels, created_at FROM clusters WHERE name=$1`, name).Scan(&idStr, &enc, &labels, &c.CreatedAt)
	if err != nil {
		return types.Cluster{}, "", ErrNotFound
	}
	id, _ := types.ParseID(idStr)
	c.ID = id
	c.Name = name
	c.Labels = labels
	return c, enc, nil
}

func (p *Postgres) GetClusterByID(ctx context.Context, id string) (types.Cluster, string, error) {
	parsed, err := types.ParseID(id)
	if err != nil {
		return types.Cluster{}, "", ErrNotFound
	}
	return p.GetCluster(ctx, parsed)
}

func (p *Postgres) DeleteCluster(ctx context.Context, id types.ID) error {
	_, err := p.db.Exec(ctx, `DELETE FROM clusters WHERE id=$1::uuid`, id.String())
	return err
}

func mapToJSONB(m map[string]string) any {
	if m == nil {
		return map[string]string{}
	}
	return m
}

// randUID minimal generator: kn + 16 random hex bytes
// randUID is provided in memory.go (same package)

// defaultMigrationSQL mirrors db/migrations/0001_init.sql for test environments
const defaultMigrationSQL = `
CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- tenants, projects, apps, events (skeleton)
CREATE TABLE IF NOT EXISTS tenants (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  uid TEXT UNIQUE NOT NULL,
  name TEXT UNIQUE NOT NULL,
  owners TEXT[] DEFAULT '{}'::text[],
  labels JSONB DEFAULT '{}'::jsonb,
  created_at TIMESTAMP DEFAULT NOW()
);
ALTER TABLE IF EXISTS tenants ADD COLUMN IF NOT EXISTS owners TEXT[] DEFAULT '{}'::text[];
ALTER TABLE IF EXISTS tenants ADD COLUMN IF NOT EXISTS labels JSONB DEFAULT '{}'::jsonb;
CREATE TABLE IF NOT EXISTS projects (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  uid TEXT UNIQUE NOT NULL,
  tenant TEXT NOT NULL,
  name TEXT NOT NULL,
  UNIQUE(tenant,name)
);
CREATE TABLE IF NOT EXISTS apps (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  uid TEXT UNIQUE NOT NULL,
  tenant TEXT NOT NULL,
  project TEXT NOT NULL,
  name TEXT NOT NULL,
  UNIQUE(tenant,project,name)
);
CREATE TABLE IF NOT EXISTS sandboxes (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  uid TEXT UNIQUE NOT NULL,
  tenant TEXT NOT NULL,
  name TEXT NOT NULL,
  namespace TEXT NOT NULL,
  created_at TIMESTAMP DEFAULT NOW(),
  UNIQUE(tenant,name)
);
CREATE TABLE IF NOT EXISTS policysets (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_uid TEXT NOT NULL,
  name TEXT NOT NULL,
  spec JSONB NOT NULL,
  created_at TIMESTAMP DEFAULT NOW(),
  UNIQUE(tenant_uid,name)
);

CREATE TABLE IF NOT EXISTS clusters (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  uid TEXT UNIQUE NOT NULL,
  name TEXT UNIQUE NOT NULL,
  kubeconfig_enc TEXT NOT NULL,
  labels JSONB DEFAULT '{}'::jsonb,
  created_at TIMESTAMP DEFAULT NOW()
);

-- indexes for clusters listing and label filtering
CREATE INDEX IF NOT EXISTS clusters_labels_gin_idx ON clusters USING gin(labels);

CREATE TABLE IF NOT EXISTS events (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  cluster_id UUID NULL REFERENCES clusters(id) ON DELETE SET NULL,
  type TEXT NOT NULL,
  resource TEXT NOT NULL,
  payload JSONB,
  ts TIMESTAMP DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS cluster_conditions (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  cluster_id UUID NOT NULL REFERENCES clusters(id) ON DELETE CASCADE,
  type TEXT NOT NULL,
  status TEXT NOT NULL,
  reason TEXT,
  message TEXT,
  ts TIMESTAMP DEFAULT NOW()
);`

// Events & conditions
func (p *Postgres) AddEvents(ctx context.Context, clusterID *types.ID, evts []types.Event) error {
	for _, e := range evts {
		var cid any = nil
		if clusterID != nil {
			cid = clusterID.String()
		}
		_, err := p.db.Exec(ctx, `INSERT INTO events(cluster_id, type, resource, payload, ts) VALUES ($1::uuid,$2,$3,$4,COALESCE($5,NOW()))`, cid, e.Type, e.Resource, e.Payload, e.TS)
		if err != nil {
			return err
		}
	}
	return nil
}

func (p *Postgres) AddConditionHistory(ctx context.Context, clusterID types.ID, conds []types.Condition) error {
	for _, c := range conds {
		_, err := p.db.Exec(ctx, `INSERT INTO cluster_conditions(cluster_id, type, status, reason, message, ts) VALUES ($1::uuid,$2,$3,$4,$5,COALESCE($6,NOW()))`, clusterID.String(), c.Type, c.Status, c.Reason, c.Message, c.LastTransitionTime)
		if err != nil {
			return err
		}
	}
	return nil
}

func (p *Postgres) ListClusterEvents(ctx context.Context, clusterID types.ID, limit int) ([]types.Event, error) {
	rows, err := p.db.Query(ctx, `SELECT type, resource, payload, ts FROM events WHERE cluster_id=$1::uuid ORDER BY ts DESC LIMIT $2`, clusterID.String(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []types.Event
	for rows.Next() {
		var e types.Event
		if err := rows.Scan(&e.Type, &e.Resource, &e.Payload, &e.TS); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, nil
}

func (p *Postgres) ListClusters(ctx context.Context, limit int, cursor string, labelSelector string) ([]types.Cluster, string, error) {
	if limit <= 0 {
		limit = 50
	}
	// parse UUID cursor (ignore errors -> treat as zero UUID lower bound)
	lastID, _ := types.ParseID(cursor)
	// build where clause
	where := []string{"id > $1::uuid"}
	args := []any{lastID.String()}
	idx := 2
	if labelSelector != "" {
		// parse simple k=v pairs
		pairs := map[string]string{}
		for _, pz := range strings.Split(labelSelector, ",") {
			kv := strings.SplitN(strings.TrimSpace(pz), "=", 2)
			if len(kv) == 2 {
				pairs[kv[0]] = kv[1]
			}
		}
		if len(pairs) > 0 {
			// build a jsonb object string parameter
			// e.g., '{"env":"dev","tier":"gold"}'
			sb := strings.Builder{}
			sb.WriteString("{")
			i := 0
			for k, v := range pairs {
				if i > 0 {
					sb.WriteString(",")
				}
				sb.WriteString("\"")
				sb.WriteString(k)
				sb.WriteString("\":\"")
				sb.WriteString(v)
				sb.WriteString("\"")
				i++
			}
			sb.WriteString("}")
			where = append(where, "labels @> $"+strconv.Itoa(idx)+"::jsonb")
			args = append(args, sb.String())
			idx++
		}
	}
	q := "SELECT id::text, name, labels, created_at FROM clusters WHERE " + strings.Join(where, " AND ") + " ORDER BY id ASC LIMIT $" + strconv.Itoa(idx)
	args = append(args, limit)
	rows, err := p.db.Query(ctx, q, args...)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	out := []types.Cluster{}
	next := ""
	for rows.Next() {
		var c types.Cluster
		var idStr string
		if err := rows.Scan(&idStr, &c.Name, &c.Labels, &c.CreatedAt); err != nil {
			return nil, "", err
		}
		id, _ := types.ParseID(idStr)
		c.ID = id
		out = append(out, c)
	}
	if len(out) == limit {
		next = out[len(out)-1].ID.String()
	}
	return out, next, nil
}
