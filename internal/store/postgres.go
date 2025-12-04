package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/vaheed/kubenova/pkg/types"
)

type postgresStore struct {
	db *sql.DB
}

// NewPostgresStore creates a Store backed by PostgreSQL.
func NewPostgresStore(dsn string) (*postgresStore, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(1 * time.Hour)

	st := &postgresStore{db: db}
	if err := st.init(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return st, nil
}

func (p *postgresStore) init(ctx context.Context) error {
	if _, err := p.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (id TEXT PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL)`); err != nil {
		return err
	}
	for _, m := range migrations {
		applied, err := p.isApplied(ctx, m.ID)
		if err != nil {
			return err
		}
		if applied {
			continue
		}
		if err := p.applyMigration(ctx, m); err != nil {
			return err
		}
	}
	return nil
}

func (p *postgresStore) Close(ctx context.Context) error {
	return p.db.Close()
}

func (p *postgresStore) Health(ctx context.Context) error {
	return p.db.PingContext(ctx)
}

func marshalPayload(v any) ([]byte, error) {
	return json.Marshal(v)
}

func unmarshalPayload(raw []byte, v any) error {
	return json.Unmarshal(raw, v)
}

func handleSQLError(err error) error {
	if err == nil {
		return nil
	}
	// pgx driver returns plain errors; string matching keeps deps small.
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	msg := err.Error()
	if containsAny(msg, "unique constraint", "duplicate key") {
		return ErrConflict
	}
	return err
}

func containsAny(msg string, tokens ...string) bool {
	for _, t := range tokens {
		if t != "" && strings.Contains(msg, t) {
			return true
		}
	}
	return false
}

func (p *postgresStore) CreateCluster(ctx context.Context, c *types.Cluster) error {
	assignIDs(c, nil, nil, nil)
	payload, err := marshalPayload(c)
	if err != nil {
		return err
	}
	_, err = p.db.ExecContext(ctx, `
		INSERT INTO clusters (id, name, payload, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5)
	`, c.ID, c.Name, payload, c.CreatedAt, c.UpdatedAt)
	return handleSQLError(err)
}

func (p *postgresStore) ListClusters(ctx context.Context) ([]*types.Cluster, error) {
	rows, err := p.db.QueryContext(ctx, `SELECT payload FROM clusters ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*types.Cluster
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var c types.Cluster
		if err := unmarshalPayload(raw, &c); err != nil {
			return nil, err
		}
		out = append(out, &c)
	}
	return out, rows.Err()
}

func (p *postgresStore) GetCluster(ctx context.Context, id string) (*types.Cluster, error) {
	var raw []byte
	err := p.db.QueryRowContext(ctx, `SELECT payload FROM clusters WHERE id=$1`, id).Scan(&raw)
	if err != nil {
		return nil, handleSQLError(err)
	}
	var c types.Cluster
	if err := unmarshalPayload(raw, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

func (p *postgresStore) DeleteCluster(ctx context.Context, id string) error {
	_, err := p.db.ExecContext(ctx, `DELETE FROM apps WHERE cluster_id=$1`, id)
	if err != nil {
		return err
	}
	_, err = p.db.ExecContext(ctx, `DELETE FROM projects WHERE cluster_id=$1`, id)
	if err != nil {
		return err
	}
	_, err = p.db.ExecContext(ctx, `DELETE FROM tenants WHERE cluster_id=$1`, id)
	if err != nil {
		return err
	}
	res, err := p.db.ExecContext(ctx, `DELETE FROM clusters WHERE id=$1`, id)
	if err != nil {
		return err
	}
	aff, _ := res.RowsAffected()
	if aff == 0 {
		return ErrNotFound
	}
	return nil
}

type migration struct {
	ID  string
	SQL string
}

var migrations = []migration{
	{
		ID: "0001_init",
		SQL: `
CREATE TABLE IF NOT EXISTS clusters (
	id UUID PRIMARY KEY,
	name TEXT UNIQUE NOT NULL,
	payload JSONB NOT NULL,
	created_at TIMESTAMPTZ NOT NULL,
	updated_at TIMESTAMPTZ NOT NULL
);
CREATE TABLE IF NOT EXISTS tenants (
	id UUID PRIMARY KEY,
	cluster_id UUID NOT NULL,
	name TEXT NOT NULL,
	payload JSONB NOT NULL,
	created_at TIMESTAMPTZ NOT NULL,
	updated_at TIMESTAMPTZ NOT NULL,
	UNIQUE(cluster_id, name)
);
CREATE TABLE IF NOT EXISTS projects (
	id UUID PRIMARY KEY,
	cluster_id UUID NOT NULL,
	tenant_id UUID NOT NULL,
	name TEXT NOT NULL,
	payload JSONB NOT NULL,
	created_at TIMESTAMPTZ NOT NULL,
	updated_at TIMESTAMPTZ NOT NULL,
	UNIQUE(tenant_id, name)
);
CREATE TABLE IF NOT EXISTS apps (
	id UUID PRIMARY KEY,
	cluster_id UUID NOT NULL,
	tenant_id UUID NOT NULL,
	project_id UUID NOT NULL,
	name TEXT NOT NULL,
	payload JSONB NOT NULL,
	created_at TIMESTAMPTZ NOT NULL,
	updated_at TIMESTAMPTZ NOT NULL,
	UNIQUE(project_id, name)
);
`,
	},
}

func (p *postgresStore) isApplied(ctx context.Context, id string) (bool, error) {
	var count int
	err := p.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM schema_migrations WHERE id=$1`, id).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (p *postgresStore) applyMigration(ctx context.Context, m migration) error {
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, m.SQL); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("migration %s: %w", m.ID, err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations (id, applied_at) VALUES ($1, $2)`, m.ID, time.Now().UTC()); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("migration %s: %w", m.ID, err)
	}
	return tx.Commit()
}

func (p *postgresStore) CreateTenant(ctx context.Context, t *types.Tenant) error {
	assignIDs(nil, t, nil, nil)
	payload, err := marshalPayload(t)
	if err != nil {
		return err
	}
	_, err = p.db.ExecContext(ctx, `
		INSERT INTO tenants (id, cluster_id, name, payload, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, t.ID, t.ClusterID, t.Name, payload, t.CreatedAt, t.UpdatedAt)
	return handleSQLError(err)
}

func (p *postgresStore) ListTenants(ctx context.Context, clusterID string) ([]*types.Tenant, error) {
	query := `SELECT payload FROM tenants`
	args := []any{}
	if clusterID != "" {
		query += ` WHERE cluster_id=$1`
		args = append(args, clusterID)
	}
	query += ` ORDER BY created_at`
	rows, err := p.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*types.Tenant
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var t types.Tenant
		if err := unmarshalPayload(raw, &t); err != nil {
			return nil, err
		}
		out = append(out, &t)
	}
	return out, rows.Err()
}

func (p *postgresStore) GetTenant(ctx context.Context, clusterID, tenantID string) (*types.Tenant, error) {
	var raw []byte
	err := p.db.QueryRowContext(ctx, `SELECT payload FROM tenants WHERE id=$1`, tenantID).Scan(&raw)
	if err != nil {
		return nil, handleSQLError(err)
	}
	var t types.Tenant
	if err := unmarshalPayload(raw, &t); err != nil {
		return nil, err
	}
	if clusterID != "" && t.ClusterID != clusterID {
		return nil, ErrNotFound
	}
	return &t, nil
}

func (p *postgresStore) UpdateTenant(ctx context.Context, t *types.Tenant) error {
	t.UpdatedAt = time.Now().UTC()
	payload, err := marshalPayload(t)
	if err != nil {
		return err
	}
	res, err := p.db.ExecContext(ctx, `
		UPDATE tenants SET payload=$1, updated_at=$2 WHERE id=$3
	`, payload, t.UpdatedAt, t.ID)
	if err != nil {
		return err
	}
	if aff, _ := res.RowsAffected(); aff == 0 {
		return ErrNotFound
	}
	return nil
}

func (p *postgresStore) UpdateCluster(ctx context.Context, c *types.Cluster) error {
	c.UpdatedAt = time.Now().UTC()
	payload, err := marshalPayload(c)
	if err != nil {
		return err
	}
	res, err := p.db.ExecContext(ctx, `UPDATE clusters SET payload=$1, updated_at=$2 WHERE id=$3`, payload, c.UpdatedAt, c.ID)
	if err != nil {
		return err
	}
	if aff, _ := res.RowsAffected(); aff == 0 {
		return ErrNotFound
	}
	return nil
}

func (p *postgresStore) DeleteTenant(ctx context.Context, clusterID, tenantID string) error {
	_, err := p.db.ExecContext(ctx, `DELETE FROM apps WHERE tenant_id=$1`, tenantID)
	if err != nil {
		return err
	}
	_, err = p.db.ExecContext(ctx, `DELETE FROM projects WHERE tenant_id=$1`, tenantID)
	if err != nil {
		return err
	}
	res, err := p.db.ExecContext(ctx, `DELETE FROM tenants WHERE id=$1`, tenantID)
	if err != nil {
		return err
	}
	if aff, _ := res.RowsAffected(); aff == 0 {
		return ErrNotFound
	}
	return nil
}

func (p *postgresStore) CreateProject(ctx context.Context, pr *types.Project) error {
	assignIDs(nil, nil, pr, nil)
	payload, err := marshalPayload(pr)
	if err != nil {
		return err
	}
	_, err = p.db.ExecContext(ctx, `
		INSERT INTO projects (id, cluster_id, tenant_id, name, payload, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, pr.ID, pr.ClusterID, pr.TenantID, pr.Name, payload, pr.CreatedAt, pr.UpdatedAt)
	return handleSQLError(err)
}

func (p *postgresStore) ListProjects(ctx context.Context, clusterID, tenantID string) ([]*types.Project, error) {
	query := `SELECT payload FROM projects`
	args := []any{}
	if clusterID != "" && tenantID != "" {
		query += ` WHERE cluster_id=$1 AND tenant_id=$2`
		args = append(args, clusterID, tenantID)
	} else if clusterID != "" {
		query += ` WHERE cluster_id=$1`
		args = append(args, clusterID)
	} else if tenantID != "" {
		query += ` WHERE tenant_id=$1`
		args = append(args, tenantID)
	}
	query += ` ORDER BY created_at`
	rows, err := p.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*types.Project
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var pjt types.Project
		if err := unmarshalPayload(raw, &pjt); err != nil {
			return nil, err
		}
		out = append(out, &pjt)
	}
	return out, rows.Err()
}

func (p *postgresStore) GetProject(ctx context.Context, clusterID, tenantID, projectID string) (*types.Project, error) {
	var raw []byte
	err := p.db.QueryRowContext(ctx, `SELECT payload FROM projects WHERE id=$1`, projectID).Scan(&raw)
	if err != nil {
		return nil, handleSQLError(err)
	}
	var pr types.Project
	if err := unmarshalPayload(raw, &pr); err != nil {
		return nil, err
	}
	if (clusterID != "" && pr.ClusterID != clusterID) || (tenantID != "" && pr.TenantID != tenantID) {
		return nil, ErrNotFound
	}
	return &pr, nil
}

func (p *postgresStore) UpdateProject(ctx context.Context, pr *types.Project) error {
	pr.UpdatedAt = time.Now().UTC()
	payload, err := marshalPayload(pr)
	if err != nil {
		return err
	}
	res, err := p.db.ExecContext(ctx, `
		UPDATE projects SET payload=$1, updated_at=$2 WHERE id=$3
	`, payload, pr.UpdatedAt, pr.ID)
	if err != nil {
		return err
	}
	if aff, _ := res.RowsAffected(); aff == 0 {
		return ErrNotFound
	}
	return nil
}

func (p *postgresStore) DeleteProject(ctx context.Context, clusterID, tenantID, projectID string) error {
	_, err := p.db.ExecContext(ctx, `DELETE FROM apps WHERE project_id=$1`, projectID)
	if err != nil {
		return err
	}
	res, err := p.db.ExecContext(ctx, `DELETE FROM projects WHERE id=$1`, projectID)
	if err != nil {
		return err
	}
	if aff, _ := res.RowsAffected(); aff == 0 {
		return ErrNotFound
	}
	return nil
}

func (p *postgresStore) CreateApp(ctx context.Context, a *types.App) error {
	assignIDs(nil, nil, nil, a)
	payload, err := marshalPayload(a)
	if err != nil {
		return err
	}
	_, err = p.db.ExecContext(ctx, `
		INSERT INTO apps (id, cluster_id, tenant_id, project_id, name, payload, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, a.ID, a.ClusterID, a.TenantID, a.ProjectID, a.Name, payload, a.CreatedAt, a.UpdatedAt)
	return handleSQLError(err)
}

func (p *postgresStore) ListApps(ctx context.Context, clusterID, tenantID, projectID string) ([]*types.App, error) {
	query := `SELECT payload FROM apps`
	args := []any{}
	switch {
	case clusterID != "" && tenantID != "" && projectID != "":
		query += ` WHERE cluster_id=$1 AND tenant_id=$2 AND project_id=$3`
		args = append(args, clusterID, tenantID, projectID)
	case clusterID != "" && tenantID != "":
		query += ` WHERE cluster_id=$1 AND tenant_id=$2`
		args = append(args, clusterID, tenantID)
	case tenantID != "" && projectID != "":
		query += ` WHERE tenant_id=$1 AND project_id=$2`
		args = append(args, tenantID, projectID)
	case clusterID != "":
		query += ` WHERE cluster_id=$1`
		args = append(args, clusterID)
	case tenantID != "":
		query += ` WHERE tenant_id=$1`
		args = append(args, tenantID)
	case projectID != "":
		query += ` WHERE project_id=$1`
		args = append(args, projectID)
	}
	query += ` ORDER BY created_at`
	rows, err := p.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*types.App
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var app types.App
		if err := unmarshalPayload(raw, &app); err != nil {
			return nil, err
		}
		out = append(out, &app)
	}
	return out, rows.Err()
}

func (p *postgresStore) GetApp(ctx context.Context, clusterID, tenantID, projectID, appID string) (*types.App, error) {
	var raw []byte
	err := p.db.QueryRowContext(ctx, `SELECT payload FROM apps WHERE id=$1`, appID).Scan(&raw)
	if err != nil {
		return nil, handleSQLError(err)
	}
	var app types.App
	if err := unmarshalPayload(raw, &app); err != nil {
		return nil, err
	}
	if (clusterID != "" && app.ClusterID != clusterID) || (tenantID != "" && app.TenantID != tenantID) || (projectID != "" && app.ProjectID != projectID) {
		return nil, ErrNotFound
	}
	return &app, nil
}

func (p *postgresStore) UpdateApp(ctx context.Context, a *types.App) error {
	a.UpdatedAt = time.Now().UTC()
	payload, err := marshalPayload(a)
	if err != nil {
		return err
	}
	res, err := p.db.ExecContext(ctx, `
		UPDATE apps SET payload=$1, updated_at=$2 WHERE id=$3
	`, payload, a.UpdatedAt, a.ID)
	if err != nil {
		return err
	}
	if aff, _ := res.RowsAffected(); aff == 0 {
		return ErrNotFound
	}
	return nil
}

func (p *postgresStore) DeleteApp(ctx context.Context, clusterID, tenantID, projectID, appID string) error {
	res, err := p.db.ExecContext(ctx, `DELETE FROM apps WHERE id=$1`, appID)
	if err != nil {
		return err
	}
	if aff, _ := res.RowsAffected(); aff == 0 {
		return ErrNotFound
	}
	return nil
}
