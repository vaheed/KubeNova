package store

import (
	"context"
	"errors"
	"github.com/vaheed/kubenova/pkg/types"
	"time"
)

// Store defines the persistence boundary for the API Manager.
type Store interface {
	Close(ctx context.Context) error

	// Tenants
	CreateTenant(ctx context.Context, t types.Tenant) error
	GetTenant(ctx context.Context, name string) (types.Tenant, error)
	ListTenants(ctx context.Context) ([]types.Tenant, error)
	UpdateTenant(ctx context.Context, t types.Tenant) error
	DeleteTenant(ctx context.Context, name string) error

	// Projects
	CreateProject(ctx context.Context, p types.Project) error
	GetProject(ctx context.Context, tenant, name string) (types.Project, error)
	ListProjects(ctx context.Context, tenant string) ([]types.Project, error)
	UpdateProject(ctx context.Context, p types.Project) error
	DeleteProject(ctx context.Context, tenant, name string) error

	// Apps
	CreateApp(ctx context.Context, a types.App) error
	GetApp(ctx context.Context, tenant, project, name string) (types.App, error)
	ListApps(ctx context.Context, tenant, project string) ([]types.App, error)
	UpdateApp(ctx context.Context, a types.App) error
	DeleteApp(ctx context.Context, tenant, project, name string) error

	// Clusters
	CreateCluster(ctx context.Context, c types.Cluster, kubeconfigEnc string) (int, error)
	GetCluster(ctx context.Context, id int) (types.Cluster, string, error)
	// GetClusterByName returns cluster by name with stored kubeconfig encoding.
	GetClusterByName(ctx context.Context, name string) (types.Cluster, string, error)

	// Events & condition history
	AddEvents(ctx context.Context, clusterID *int, evts []types.Event) error
	AddConditionHistory(ctx context.Context, clusterID int, conds []types.Condition) error
	ListClusterEvents(ctx context.Context, clusterID int, limit int) ([]types.Event, error)

	// ListClusters returns clusters with optional pagination and label filtering.
	// Cursor is an opaque string returned by the previous call (id-based).
	ListClusters(ctx context.Context, limit int, cursor string, labelSelector string) ([]types.Cluster, string, error)
}

var ErrNotFound = errors.New("not found")

// Helper to stamp time fields for idempotent creates
func stamp(t time.Time) time.Time {
	if t.IsZero() {
		return time.Now().UTC()
	}
	return t.UTC()
}
