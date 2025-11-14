package store

import (
	"context"
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

func (p *Postgres) CreateTenant(ctx context.Context, t types.Tenant) error {
	t.CreatedAt = stamp(t.CreatedAt)
	if t.UID == "" {
		t.UID = types.NewID().String()
	}
	labels := mapToJSONB(t.Labels)
	owners := t.Owners
	_, err := p.db.Exec(ctx, `
INSERT INTO tenants(uid, name, owners, labels, created_at)
VALUES ($1,$2,$3,$4,$5)
ON CONFLICT (name) DO UPDATE
SET owners = EXCLUDED.owners,
    labels = EXCLUDED.labels
`, t.UID, t.Name, owners, labels, t.CreatedAt)
	return err
}
func (p *Postgres) GetTenant(ctx context.Context, name string) (types.Tenant, error) {
	var t types.Tenant
	var labels map[string]string
	err := p.db.QueryRow(ctx, `
SELECT uid, name, owners, labels, created_at
FROM tenants
WHERE name=$1
`, name).Scan(&t.UID, &t.Name, &t.Owners, &labels, &t.CreatedAt)
	if err != nil {
		return types.Tenant{}, ErrNotFound
	}
	t.Labels = labels
	return t, nil
}
func (p *Postgres) GetTenantByUID(ctx context.Context, uid string) (types.Tenant, error) {
	var t types.Tenant
	var labels map[string]string
	err := p.db.QueryRow(ctx, `
SELECT uid, name, owners, labels, created_at
FROM tenants
WHERE uid=$1
`, uid).Scan(&t.UID, &t.Name, &t.Owners, &labels, &t.CreatedAt)
	if err != nil {
		return types.Tenant{}, ErrNotFound
	}
	t.Labels = labels
	return t, nil
}
func (p *Postgres) ListTenants(ctx context.Context) ([]types.Tenant, error) {
	rows, err := p.db.Query(ctx, `
SELECT uid, name, owners, labels, created_at
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
		if err := rows.Scan(&t.UID, &t.Name, &t.Owners, &labels, &t.CreatedAt); err != nil {
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
	if pr.UID == "" {
		pr.UID = types.NewID().String()
	}
	_, err := p.db.Exec(ctx, `INSERT INTO projects(uid, tenant, name) VALUES ($1,$2,$3) ON CONFLICT (tenant,name) DO NOTHING`, pr.UID, pr.Tenant, pr.Name)
	return err
}
func (p *Postgres) GetProject(ctx context.Context, tenant, name string) (types.Project, error) {
	var pr types.Project
	err := p.db.QueryRow(ctx, `SELECT uid, tenant, name FROM projects WHERE tenant=$1 AND name=$2`, tenant, name).Scan(&pr.UID, &pr.Tenant, &pr.Name)
	if err != nil {
		return types.Project{}, ErrNotFound
	}
	return pr, nil
}
func (p *Postgres) GetProjectByUID(ctx context.Context, uid string) (types.Project, error) {
	var pr types.Project
	err := p.db.QueryRow(ctx, `SELECT uid, tenant, name FROM projects WHERE uid=$1`, uid).Scan(&pr.UID, &pr.Tenant, &pr.Name)
	if err != nil {
		return types.Project{}, ErrNotFound
	}
	return pr, nil
}
func (p *Postgres) ListProjects(ctx context.Context, tenant string) ([]types.Project, error) {
	rows, err := p.db.Query(ctx, `SELECT uid, tenant, name FROM projects WHERE tenant=$1 ORDER BY name`, tenant)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []types.Project
	for rows.Next() {
		var pr types.Project
		if err := rows.Scan(&pr.UID, &pr.Tenant, &pr.Name); err != nil {
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

func (p *Postgres) CreateApp(ctx context.Context, a types.App) error {
	a.CreatedAt = stamp(a.CreatedAt)
	if a.UID == "" {
		a.UID = types.NewID().String()
	}
	_, err := p.db.Exec(ctx, `INSERT INTO apps(uid, tenant, project, name) VALUES ($1,$2,$3,$4) ON CONFLICT (tenant,project,name) DO NOTHING`, a.UID, a.Tenant, a.Project, a.Name)
	return err
}
func (p *Postgres) GetApp(ctx context.Context, tenant, project, name string) (types.App, error) {
	var a types.App
	err := p.db.QueryRow(ctx, `SELECT uid, tenant, project, name FROM apps WHERE tenant=$1 AND project=$2 AND name=$3`, tenant, project, name).Scan(&a.UID, &a.Tenant, &a.Project, &a.Name)
	if err != nil {
		return types.App{}, ErrNotFound
	}
	return a, nil
}
func (p *Postgres) GetAppByUID(ctx context.Context, uid string) (types.App, error) {
	var a types.App
	err := p.db.QueryRow(ctx, `SELECT uid, tenant, project, name FROM apps WHERE uid=$1`, uid).Scan(&a.UID, &a.Tenant, &a.Project, &a.Name)
	if err != nil {
		return types.App{}, ErrNotFound
	}
	return a, nil
}
func (p *Postgres) ListApps(ctx context.Context, tenant, project string) ([]types.App, error) {
	rows, err := p.db.Query(ctx, `SELECT tenant, project, name FROM apps WHERE tenant=$1 AND project=$2 ORDER BY name`, tenant, project)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []types.App
	for rows.Next() {
		var a types.App
		if err := rows.Scan(&a.Tenant, &a.Project, &a.Name); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, nil
}
func (p *Postgres) UpdateApp(ctx context.Context, a types.App) error { return p.CreateApp(ctx, a) }
func (p *Postgres) DeleteApp(ctx context.Context, tenant, project, name string) error {
	_, err := p.db.Exec(ctx, `DELETE FROM apps WHERE tenant=$1 AND project=$2 AND name=$3`, tenant, project, name)
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
	if c.UID == "" {
		c.UID = types.NewID().String()
	}
	var idStr string
	err := p.db.QueryRow(ctx, `INSERT INTO clusters(uid, name, kubeconfig_enc, labels) VALUES ($1,$2,$3,$4) RETURNING id::text`, c.UID, c.Name, kubeconfigEnc, mapToJSONB(c.Labels)).Scan(&idStr)
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
	err := p.db.QueryRow(ctx, `SELECT uid, name, kubeconfig_enc, labels, created_at FROM clusters WHERE id=$1::uuid`, id.String()).Scan(&c.UID, &c.Name, &enc, &labels, &c.CreatedAt)
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
	err := p.db.QueryRow(ctx, `SELECT id::text, uid, kubeconfig_enc, labels, created_at FROM clusters WHERE name=$1`, name).Scan(&idStr, &c.UID, &enc, &labels, &c.CreatedAt)
	if err != nil {
		return types.Cluster{}, "", ErrNotFound
	}
	id, _ := types.ParseID(idStr)
	c.ID = id
	c.Name = name
	c.Labels = labels
	return c, enc, nil
}

func (p *Postgres) GetClusterByUID(ctx context.Context, uid string) (types.Cluster, string, error) {
	var c types.Cluster
	var enc string
	var labels map[string]string
	var idStr string
	err := p.db.QueryRow(ctx, `SELECT id::text, name, kubeconfig_enc, labels, created_at FROM clusters WHERE uid=$1`, uid).Scan(&idStr, &c.Name, &enc, &labels, &c.CreatedAt)
	if err != nil {
		return types.Cluster{}, "", ErrNotFound
	}
	id, _ := types.ParseID(idStr)
	c.ID = id
	c.UID = uid
	c.Labels = labels
	return c, enc, nil
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
	q := "SELECT id::text, uid, name, labels, created_at FROM clusters WHERE " + strings.Join(where, " AND ") + " ORDER BY id ASC LIMIT $" + strconv.Itoa(idx)
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
		if err := rows.Scan(&idStr, &c.UID, &c.Name, &c.Labels, &c.CreatedAt); err != nil {
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
