package types

import "time"

// IDs are simple strings for portability; database can map to serial if desired.

type Cluster struct {
    ID          int       `json:"id,omitempty"`
    Name        string    `json:"name"`
    DisplayName string    `json:"displayName,omitempty"`
    KubeconfigB64 string  `json:"kubeconfig,omitempty"`
    Labels      map[string]string `json:"labels,omitempty"`
    CreatedAt   time.Time `json:"createdAt"`
    Conditions  []Condition `json:"conditions,omitempty"`
}

type Condition struct {
    Type   string    `json:"type"`
    Status string    `json:"status"`
    Reason string    `json:"reason,omitempty"`
    Message string   `json:"message,omitempty"`
    LastTransitionTime time.Time `json:"lastTransitionTime"`
}

type Tenant struct {
    Name        string            `json:"name"`
    Owners      []string          `json:"owners,omitempty"` // user or group subjects
    Labels      map[string]string `json:"labels,omitempty"`
    Annotations map[string]string `json:"annotations,omitempty"`
    CreatedAt   time.Time         `json:"createdAt"`
}

type Project struct {
    Tenant      string            `json:"tenant"`
    Name        string            `json:"name"`
    Labels      map[string]string `json:"labels,omitempty"`
    Annotations map[string]string `json:"annotations,omitempty"`
    Quota       map[string]string `json:"quota,omitempty"` // cpu: "2", memory: "4Gi" etc
    CreatedAt   time.Time         `json:"createdAt"`
}

type App struct {
    Tenant      string            `json:"tenant"`
    Project     string            `json:"project"`
    Name        string            `json:"name"`
    Image       string            `json:"image,omitempty"`
    Properties  map[string]any    `json:"properties,omitempty"`
    CreatedAt   time.Time         `json:"createdAt"`
}

type PolicySet struct {
    Name      string            `json:"name"`
    Tenant    string            `json:"tenant"`
    Policies  map[string]any    `json:"policies"`
    CreatedAt time.Time         `json:"createdAt"`
}

type KubeconfigGrant struct {
    Tenant   string    `json:"tenant"`
    Project  string    `json:"project,omitempty"`
    Role     string    `json:"role"`  // tenant-admin, tenant-dev, read-only
    Expires  time.Time `json:"expires"`
    // Result holds the kubeconfig data when issued
    Kubeconfig []byte `json:"kubeconfig,omitempty"`
}

// Event represents an agent-observed item that should be ingested.
type Event struct {
    Type     string    `json:"type"`
    Resource string    `json:"resource"`
    Payload  any       `json:"payload"`
    TS       time.Time `json:"ts"`
}
