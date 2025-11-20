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

	// Sandboxes
	CreateSandbox(ctx context.Context, sb types.Sandbox) error
	GetSandbox(ctx context.Context, tenant, name string) (types.Sandbox, error)
	GetSandboxByUID(ctx context.Context, uid string) (types.Sandbox, error)
	ListSandboxes(ctx context.Context, tenant string) ([]types.Sandbox, error)
	DeleteSandbox(ctx context.Context, tenant, name string) error

	// Apps
	CreateApp(ctx context.Context, a types.App) error
	GetApp(ctx context.Context, tenant, project, name string) (types.App, error)
	ListApps(ctx context.Context, tenant, project string) ([]types.App, error)
	UpdateApp(ctx context.Context, a types.App) error
	DeleteApp(ctx context.Context, tenant, project, name string) error

	// Clusters
	CreateCluster(ctx context.Context, c types.Cluster, kubeconfigEnc string) (types.ID, error)
	GetCluster(ctx context.Context, id types.ID) (types.Cluster, string, error)
	// GetClusterByName returns cluster by name with stored kubeconfig encoding.
	GetClusterByName(ctx context.Context, name string) (types.Cluster, string, error)
	// GetClusterByUID returns cluster by uid with stored kubeconfig encoding.
	GetClusterByUID(ctx context.Context, uid string) (types.Cluster, string, error)
	// DeleteCluster removes a cluster by id.
	DeleteCluster(ctx context.Context, id types.ID) error

	// Events & condition history
	AddEvents(ctx context.Context, clusterID *types.ID, evts []types.Event) error
	AddConditionHistory(ctx context.Context, clusterID types.ID, conds []types.Condition) error
	ListClusterEvents(ctx context.Context, clusterID types.ID, limit int) ([]types.Event, error)

	// ListClusters returns clusters with optional pagination and label filtering.
	// Cursor is an opaque string returned by the previous call (UUID-based).
	ListClusters(ctx context.Context, limit int, cursor string, labelSelector string) ([]types.Cluster, string, error)

	// UID-based helpers for tenant/project/app resolution
	GetTenantByUID(ctx context.Context, uid string) (types.Tenant, error)
	GetProjectByUID(ctx context.Context, uid string) (types.Project, error)
	GetAppByUID(ctx context.Context, uid string) (types.App, error)

	// PolicySets
	CreatePolicySet(ctx context.Context, ps types.PolicySet) error
	ListPolicySets(ctx context.Context, tenantUID string) ([]types.PolicySet, error)
	GetPolicySet(ctx context.Context, tenantUID, name string) (types.PolicySet, error)
	UpdatePolicySet(ctx context.Context, ps types.PolicySet) error
	DeletePolicySet(ctx context.Context, tenantUID, name string) error
}

var ErrNotFound = errors.New("not found")

// Helper to stamp time fields for idempotent creates
func stamp(t time.Time) time.Time {
	if t.IsZero() {
		return time.Now().UTC()
	}
	return t.UTC()
}
