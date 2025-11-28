package store

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/vaheed/kubenova/pkg/types"
)

// ErrNotFound is returned when a record does not exist.
var ErrNotFound = errors.New("not found")

// ErrConflict is returned when a resource already exists.
var ErrConflict = errors.New("conflict")

// Store represents the persistence surface used by the Manager.
type Store interface {
	CreateCluster(ctx context.Context, c *types.Cluster) error
	UpdateCluster(ctx context.Context, c *types.Cluster) error
	ListClusters(ctx context.Context) ([]*types.Cluster, error)
	GetCluster(ctx context.Context, id string) (*types.Cluster, error)
	DeleteCluster(ctx context.Context, id string) error
	Health(ctx context.Context) error

	CreateTenant(ctx context.Context, t *types.Tenant) error
	ListTenants(ctx context.Context, clusterID string) ([]*types.Tenant, error)
	GetTenant(ctx context.Context, clusterID, tenantID string) (*types.Tenant, error)
	UpdateTenant(ctx context.Context, t *types.Tenant) error
	DeleteTenant(ctx context.Context, clusterID, tenantID string) error

	CreateProject(ctx context.Context, p *types.Project) error
	ListProjects(ctx context.Context, clusterID, tenantID string) ([]*types.Project, error)
	GetProject(ctx context.Context, clusterID, tenantID, projectID string) (*types.Project, error)
	UpdateProject(ctx context.Context, p *types.Project) error
	DeleteProject(ctx context.Context, clusterID, tenantID, projectID string) error

	CreateApp(ctx context.Context, a *types.App) error
	ListApps(ctx context.Context, clusterID, tenantID, projectID string) ([]*types.App, error)
	GetApp(ctx context.Context, clusterID, tenantID, projectID, appID string) (*types.App, error)
	UpdateApp(ctx context.Context, a *types.App) error
	DeleteApp(ctx context.Context, clusterID, tenantID, projectID, appID string) error
}

// EnvOrMemory builds a store using DATABASE_URL when provided; otherwise
// it returns a new in-memory store. The caller is responsible for invoking
// the returned closer.
func EnvOrMemory() (Store, func(context.Context) error, error) {
	dsn := os.Getenv("DATABASE_URL")
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		st, err := NewPostgresStore(dsn)
		if err != nil {
			return nil, func(context.Context) error { return nil }, err
		}
		return st, func(ctx context.Context) error { return st.Close(ctx) }, nil
	}
	// allow explicit memory:// opt-in for local development
	if dsn == "memory://default" || dsn == "memory" || dsn == "" {
		st := NewMemoryStore()
		return st, func(context.Context) error { return nil }, nil
	}
	// Unknown scheme -> default to in-memory
	st := NewMemoryStore()
	return st, func(context.Context) error { return nil }, nil
}

// assignIDs normalizes IDs and timestamps for new resources.
func assignIDs(c *types.Cluster, t *types.Tenant, p *types.Project, a *types.App) {
	now := time.Now().UTC()
	if c != nil {
		if c.ID == "" {
			c.ID = uuid.NewString()
		}
		if c.Status == "" {
			c.Status = "pending"
		}
		c.CreatedAt = now
		c.UpdatedAt = now
		if c.NovaClusterID == "" {
			c.NovaClusterID = "nova-" + strings.ReplaceAll(c.ID, "-", "")
		}
	}
	if t != nil {
		if t.ID == "" {
			t.ID = uuid.NewString()
		}
		if t.OwnerNamespace == "" {
			t.OwnerNamespace = sanitizeNS(t.Name, "owner")
		}
		if t.AppsNamespace == "" {
			t.AppsNamespace = sanitizeNS(t.Name, "apps")
		}
		t.CreatedAt = now
		t.UpdatedAt = now
	}
	if p != nil {
		if p.ID == "" {
			p.ID = uuid.NewString()
		}
		p.CreatedAt = now
		p.UpdatedAt = now
	}
	if a != nil {
		if a.ID == "" {
			a.ID = uuid.NewString()
		}
		a.CreatedAt = now
		a.UpdatedAt = now
		if a.Revision == 0 {
			a.Revision = 1
		}
		if len(a.Revisions) == 0 {
			a.Revisions = []types.AppRevision{{
				Number:    1,
				Spec:      a.Spec,
				Traits:    a.Traits,
				Policies:  a.Policies,
				CreatedAt: now,
			}}
		}
		if a.Status == "" {
			a.Status = "pending"
		}
	}
}

func sanitizeNS(name, suffix string) string {
	base := strings.TrimSpace(strings.ToLower(name))
	if base == "" {
		base = "tenant"
	}
	builder := strings.Builder{}
	for _, r := range base {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			builder.WriteRune(r)
		} else {
			builder.WriteRune('-')
		}
	}
	ns := strings.Trim(builder.String(), "-")
	if len(ns) > 40 {
		ns = ns[:40]
		ns = strings.Trim(ns, "-")
	}
	if ns == "" {
		ns = "tenant"
	}
	return ns + "-" + suffix
}

// clone copies a struct via JSON to avoid callers mutating stored references.
func clone[T any](in *T) *T {
	if in == nil {
		return nil
	}
	raw, _ := json.Marshal(in)
	var out T
	_ = json.Unmarshal(raw, &out)
	return &out
}

// Register the pgx driver for database/sql usage.
func init() {
	stdlib.GetDefaultDriver()
}
