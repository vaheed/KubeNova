package store

import (
	"context"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vaheed/kubenova/pkg/types"
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
	_, err := p.db.Exec(ctx, `INSERT INTO tenants(name, created_at) VALUES ($1,$2) ON CONFLICT (name) DO NOTHING`, t.Name, t.CreatedAt)
	return err
}
func (p *Postgres) GetTenant(ctx context.Context, name string) (types.Tenant, error) {
	var t types.Tenant
	err := p.db.QueryRow(ctx, `SELECT name, created_at FROM tenants WHERE name=$1`, name).Scan(&t.Name, &t.CreatedAt)
	if err != nil {
		return types.Tenant{}, ErrNotFound
	}
	return t, nil
}
func (p *Postgres) ListTenants(ctx context.Context) ([]types.Tenant, error) {
	rows, err := p.db.Query(ctx, `SELECT name, created_at FROM tenants ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []types.Tenant
	for rows.Next() {
		var t types.Tenant
		if err := rows.Scan(&t.Name, &t.CreatedAt); err != nil {
			return nil, err
		}
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
	_, err := p.db.Exec(ctx, `INSERT INTO projects(tenant, name) VALUES ($1,$2) ON CONFLICT (tenant,name) DO NOTHING`, pr.Tenant, pr.Name)
	return err
}
func (p *Postgres) GetProject(ctx context.Context, tenant, name string) (types.Project, error) {
	var pr types.Project
	err := p.db.QueryRow(ctx, `SELECT tenant, name FROM projects WHERE tenant=$1 AND name=$2`, tenant, name).Scan(&pr.Tenant, &pr.Name)
	if err != nil {
		return types.Project{}, ErrNotFound
	}
	return pr, nil
}
func (p *Postgres) ListProjects(ctx context.Context, tenant string) ([]types.Project, error) {
	rows, err := p.db.Query(ctx, `SELECT tenant, name FROM projects WHERE tenant=$1 ORDER BY name`, tenant)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []types.Project
	for rows.Next() {
		var pr types.Project
		if err := rows.Scan(&pr.Tenant, &pr.Name); err != nil {
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
	_, err := p.db.Exec(ctx, `INSERT INTO apps(tenant, project, name) VALUES ($1,$2,$3) ON CONFLICT (tenant,project,name) DO NOTHING`, a.Tenant, a.Project, a.Name)
	return err
}
func (p *Postgres) GetApp(ctx context.Context, tenant, project, name string) (types.App, error) {
	var a types.App
	err := p.db.QueryRow(ctx, `SELECT tenant, project, name FROM apps WHERE tenant=$1 AND project=$2 AND name=$3`, tenant, project, name).Scan(&a.Tenant, &a.Project, &a.Name)
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
func (p *Postgres) CreateCluster(ctx context.Context, c types.Cluster, kubeconfigEnc string) (int, error) {
	var id int
	err := p.db.QueryRow(ctx, `INSERT INTO clusters(name, kubeconfig_enc, labels) VALUES ($1,$2,$3) RETURNING id`, c.Name, kubeconfigEnc, mapToJSONB(c.Labels)).Scan(&id)
	return id, err
}

func (p *Postgres) GetCluster(ctx context.Context, id int) (types.Cluster, string, error) {
	var c types.Cluster
	var enc string
	var labels map[string]string
	err := p.db.QueryRow(ctx, `SELECT name, kubeconfig_enc, labels, created_at FROM clusters WHERE id=$1`, id).Scan(&c.Name, &enc, &labels, &c.CreatedAt)
	if err != nil {
		return types.Cluster{}, "", ErrNotFound
	}
	c.ID = id
	c.Labels = labels
	return c, enc, nil
}

func (p *Postgres) GetClusterByUID(ctx context.Context, uid string) (types.Cluster, string, error) {
	// Postgres schema does not yet store UID; fall back to numeric id lookup failure
	return types.Cluster{}, "", ErrNotFound
}

func mapToJSONB(m map[string]string) any {
	if m == nil {
		return map[string]string{}
	}
	return m
}

// defaultMigrationSQL mirrors db/migrations/0001_init.sql for test environments
const defaultMigrationSQL = `-- tenants, projects, apps, events (skeleton)
CREATE TABLE IF NOT EXISTS tenants (
  id SERIAL PRIMARY KEY,
  name TEXT UNIQUE NOT NULL,
  created_at TIMESTAMP DEFAULT NOW()
);
CREATE TABLE IF NOT EXISTS projects (
  id SERIAL PRIMARY KEY,
  tenant TEXT NOT NULL,
  name TEXT NOT NULL,
  UNIQUE(tenant,name)
);
CREATE TABLE IF NOT EXISTS apps (
  id SERIAL PRIMARY KEY,
  tenant TEXT NOT NULL,
  project TEXT NOT NULL,
  name TEXT NOT NULL,
  UNIQUE(tenant,project,name)
);

CREATE TABLE IF NOT EXISTS clusters (
  id SERIAL PRIMARY KEY,
  name TEXT UNIQUE NOT NULL,
  kubeconfig_enc TEXT NOT NULL,
  labels JSONB DEFAULT '{}'::jsonb,
  created_at TIMESTAMP DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS events (
  id BIGSERIAL PRIMARY KEY,
  cluster_id INT NULL REFERENCES clusters(id) ON DELETE SET NULL,
  type TEXT NOT NULL,
  resource TEXT NOT NULL,
  payload JSONB,
  ts TIMESTAMP DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS cluster_conditions (
  id BIGSERIAL PRIMARY KEY,
  cluster_id INT NOT NULL REFERENCES clusters(id) ON DELETE CASCADE,
  type TEXT NOT NULL,
  status TEXT NOT NULL,
  reason TEXT,
  message TEXT,
  ts TIMESTAMP DEFAULT NOW()
);`

// Events & conditions
func (p *Postgres) AddEvents(ctx context.Context, clusterID *int, evts []types.Event) error {
	for _, e := range evts {
		var cid any = nil
		if clusterID != nil {
			cid = *clusterID
		}
		_, err := p.db.Exec(ctx, `INSERT INTO events(cluster_id, type, resource, payload, ts) VALUES ($1,$2,$3,$4,COALESCE($5,NOW()))`, cid, e.Type, e.Resource, e.Payload, e.TS)
		if err != nil {
			return err
		}
	}
	return nil
}

func (p *Postgres) AddConditionHistory(ctx context.Context, clusterID int, conds []types.Condition) error {
	for _, c := range conds {
		_, err := p.db.Exec(ctx, `INSERT INTO cluster_conditions(cluster_id, type, status, reason, message, ts) VALUES ($1,$2,$3,$4,$5,COALESCE($6,NOW()))`, clusterID, c.Type, c.Status, c.Reason, c.Message, c.LastTransitionTime)
		if err != nil {
			return err
		}
	}
	return nil
}

func (p *Postgres) ListClusterEvents(ctx context.Context, clusterID int, limit int) ([]types.Event, error) {
	rows, err := p.db.Query(ctx, `SELECT type, resource, payload, ts FROM events WHERE cluster_id=$1 ORDER BY ts DESC LIMIT $2`, clusterID, limit)
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
