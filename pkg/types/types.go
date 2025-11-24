package types

import "time"

// Cluster represents a registered Kubernetes cluster managed by KubeNova.
type Cluster struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	Datacenter    string            `json:"datacenter,omitempty"`
	Labels        map[string]string `json:"labels,omitempty"`
	Status        string            `json:"status"`
	NovaClusterID string            `json:"novaClusterId,omitempty"`
	Capabilities  Capabilities      `json:"capabilities,omitempty"`
	CreatedAt     time.Time         `json:"createdAt"`
	UpdatedAt     time.Time         `json:"updatedAt"`
}

// Capabilities captures optional cluster feature flags returned to clients.
type Capabilities struct {
	Capsule      bool `json:"capsule"`
	CapsuleProxy bool `json:"capsuleProxy"`
	KubeVela     bool `json:"kubeVela"`
}

// Tenant models a Capsule-backed tenant inside a cluster.
type Tenant struct {
	ID              string            `json:"id"`
	ClusterID       string            `json:"clusterId"`
	Name            string            `json:"name"`
	Owners          []string          `json:"owners,omitempty"`
	Plan            string            `json:"plan,omitempty"`
	Labels          map[string]string `json:"labels,omitempty"`
	Quotas          map[string]string `json:"quotas,omitempty"`
	Limits          map[string]string `json:"limits,omitempty"`
	NetworkPolicies []string          `json:"networkPolicies,omitempty"`
	OwnerNamespace  string            `json:"ownerNamespace,omitempty"`
	AppsNamespace   string            `json:"appsNamespace,omitempty"`
	CreatedAt       time.Time         `json:"createdAt"`
	UpdatedAt       time.Time         `json:"updatedAt"`
}

// TenantSummary aggregates tenant-scoped status and counts.
type TenantSummary struct {
	TenantID        string `json:"tenantId"`
	ClusterID       string `json:"clusterId"`
	Projects        int    `json:"projects"`
	Apps            int    `json:"apps"`
	Namespaces      int    `json:"namespaces"`
	LoadBalancers   int    `json:"loadBalancers"`
	QuotaViolations int    `json:"quotaViolations"`
}

// Project captures an application project under a tenant.
type Project struct {
	ID          string            `json:"id"`
	ClusterID   string            `json:"clusterId"`
	TenantID    string            `json:"tenantId"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	Access      []string          `json:"access,omitempty"`
	CreatedAt   time.Time         `json:"createdAt"`
	UpdatedAt   time.Time         `json:"updatedAt"`
}

// App models a KubeVela application.
type App struct {
	ID           string           `json:"id"`
	ClusterID    string           `json:"clusterId"`
	TenantID     string           `json:"tenantId"`
	ProjectID    string           `json:"projectId"`
	Name         string           `json:"name"`
	Description  string           `json:"description,omitempty"`
	Component    string           `json:"component,omitempty"`
	Image        string           `json:"image,omitempty"`
	Spec         map[string]any   `json:"spec,omitempty"`
	Traits       []map[string]any `json:"traits,omitempty"`
	Policies     []map[string]any `json:"policies,omitempty"`
	Revision     int              `json:"revision"`
	Revisions    []AppRevision    `json:"revisions,omitempty"`
	Status       string           `json:"status"`
	Suspended    bool             `json:"suspended"`
	CreatedAt    time.Time        `json:"createdAt"`
	UpdatedAt    time.Time        `json:"updatedAt"`
	WorkflowRuns []WorkflowRun    `json:"workflowRuns,omitempty"`
}

// AppRevision keeps a lightweight history of changes.
type AppRevision struct {
	Number    int              `json:"number"`
	Spec      map[string]any   `json:"spec,omitempty"`
	Traits    []map[string]any `json:"traits,omitempty"`
	Policies  []map[string]any `json:"policies,omitempty"`
	CreatedAt time.Time        `json:"createdAt"`
}

// WorkflowRun tracks a workflow execution for an app.
type WorkflowRun struct {
	ID        string         `json:"id"`
	AppID     string         `json:"appId"`
	Status    string         `json:"status"`
	Inputs    map[string]any `json:"inputs,omitempty"`
	Result    map[string]any `json:"result,omitempty"`
	StartedAt time.Time      `json:"startedAt"`
	EndedAt   *time.Time     `json:"endedAt,omitempty"`
}

// UsageRecord represents aggregated usage metrics for a tenant or project.
type UsageRecord struct {
	CPURequests     string    `json:"cpuRequests"`
	MemoryRequests  string    `json:"memoryRequests"`
	PVCStorage      string    `json:"pvcStorage"`
	LoadBalancers   int       `json:"loadBalancers"`
	Pods            int       `json:"pods"`
	Namespaces      int       `json:"namespaces"`
	Apps            int       `json:"apps"`
	QuotaViolations int       `json:"quotaViolations"`
	LastReportedAt  time.Time `json:"lastReportedAt"`
}
