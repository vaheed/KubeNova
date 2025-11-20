package types

import (
	"time"

	"github.com/google/uuid"
)

// ID is the canonical identifier type (UUIDv4, lowercase)
type ID = uuid.UUID

type Cluster struct {
	UID           string            `json:"uid,omitempty"`
	ID            ID                `json:"id,omitempty"`
	Name          string            `json:"name"`
	DisplayName   string            `json:"displayName,omitempty"`
	KubeconfigB64 string            `json:"kubeconfig,omitempty"`
	Labels        map[string]string `json:"labels,omitempty"`
	CreatedAt     time.Time         `json:"createdAt,omitempty"`
	Conditions    []Condition       `json:"conditions,omitempty"`
}

type Condition struct {
	Type               string    `json:"type"`
	Status             string    `json:"status"`
	Reason             string    `json:"reason,omitempty"`
	Message            string    `json:"message,omitempty"`
	LastTransitionTime time.Time `json:"lastTransitionTime"`
}

type Tenant struct {
	UID         string            `json:"uid,omitempty"`
	Name        string            `json:"name"`
	Owners      []string          `json:"owners,omitempty"` // user or group subjects
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
	CreatedAt   time.Time         `json:"createdAt,omitempty"`
}

type Project struct {
	UID         string            `json:"uid,omitempty"`
	Tenant      string            `json:"tenant"`
	Name        string            `json:"name"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
	Quota       map[string]string `json:"quota,omitempty"` // cpu: "2", memory: "4Gi" etc
	CreatedAt   time.Time         `json:"createdAt,omitempty"`
}

type Sandbox struct {
	UID       string    `json:"uid,omitempty"`
	Tenant    string    `json:"tenant"`
	Name      string    `json:"name"`
	Namespace string    `json:"namespace"`
	CreatedAt time.Time `json:"createdAt,omitempty"`
}

type App struct {
	UID         string            `json:"uid,omitempty"`
	Tenant      string            `json:"tenant"`
	Project     string            `json:"project"`
	Name        string            `json:"name"`
	Description *string           `json:"description,omitempty"`
	Components  *[]map[string]any `json:"components,omitempty"`
	Traits      *[]map[string]any `json:"traits,omitempty"`
	Policies    *[]map[string]any `json:"policies,omitempty"`
	Spec        *AppSpec          `json:"spec,omitempty"`
	CreatedAt   time.Time         `json:"createdAt,omitempty"`
}

type AppSpec struct {
	Source *AppSource `json:"source,omitempty"`
}

type AppSource struct {
	Kind           AppSourceKind            `json:"kind"`
	CatalogRef     *AppCatalogRef           `json:"catalogRef,omitempty"`
	HelmHttp       *AppSourceHelm           `json:"helmHttp,omitempty"`
	HelmOci        *AppSourceHelm           `json:"helmOci,omitempty"`
	VelaTemplate   *AppSourceVelaTemplate   `json:"velaTemplate,omitempty"`
	KubeManifest   *AppSourceKubeManifest   `json:"kubeManifest,omitempty"`
	ContainerImage *AppSourceContainerImage `json:"containerImage,omitempty"`
	GitRepo        *AppSourceGitRepo        `json:"gitRepo,omitempty"`
}

type AppSourceKind string

const (
	AppSourceKindHelmHttp       AppSourceKind = "helmHttp"
	AppSourceKindHelmOci        AppSourceKind = "helmOci"
	AppSourceKindVelaTemplate   AppSourceKind = "velaTemplate"
	AppSourceKindKubeManifest   AppSourceKind = "kubeManifest"
	AppSourceKindContainerImage AppSourceKind = "containerImage"
	AppSourceKindGitRepo        AppSourceKind = "gitRepo"
)

type AppCatalogRef struct {
	Name    string  `json:"name"`
	Version *string `json:"version,omitempty"`
}

type AppSourceHelm struct {
	RepoURL              *string         `json:"repoUrl,omitempty"`
	Registry             *string         `json:"registry,omitempty"`
	Chart                *string         `json:"chart,omitempty"`
	Version              *string         `json:"version,omitempty"`
	Values               *map[string]any `json:"values,omitempty"`
	CredentialsSecretRef *SecretRef      `json:"credentialsSecretRef,omitempty"`
}

type AppSourceVelaTemplate struct {
	Template             *string         `json:"template,omitempty"`
	Version              *string         `json:"version,omitempty"`
	Parameters           *map[string]any `json:"parameters,omitempty"`
	CredentialsSecretRef *SecretRef      `json:"credentialsSecretRef,omitempty"`
}

type AppSourceKubeManifest struct {
	Manifest *map[string]any `json:"manifest,omitempty"`
}

type AppSourceContainerImage struct {
	Image                *string             `json:"image,omitempty"`
	Tag                  *string             `json:"tag,omitempty"`
	Ports                *[]int              `json:"ports,omitempty"`
	Env                  *[]AppSourceEnvVar  `json:"env,omitempty"`
	Resources            *AppSourceResources `json:"resources,omitempty"`
	CredentialsSecretRef *SecretRef          `json:"credentialsSecretRef,omitempty"`
}

type AppSourceGitRepo struct {
	URL                  *string    `json:"url,omitempty"`
	Revision             *string    `json:"revision,omitempty"`
	Path                 *string    `json:"path,omitempty"`
	CredentialsSecretRef *SecretRef `json:"credentialsSecretRef,omitempty"`
}

type AppSourceEnvVar struct {
	Name  *string `json:"name,omitempty"`
	Value *string `json:"value,omitempty"`
}

type AppSourceResources struct {
	Requests map[string]string `json:"requests,omitempty"`
	Limits   map[string]string `json:"limits,omitempty"`
}

type SecretRef struct {
	Name      *string `json:"name,omitempty"`
	Namespace *string `json:"namespace,omitempty"`
}

type PolicySet struct {
	Name      string         `json:"name"`
	Tenant    string         `json:"tenant"`
	Policies  map[string]any `json:"policies"`
	CreatedAt time.Time      `json:"createdAt"`
}

type KubeconfigGrant struct {
	Tenant  string    `json:"tenant"`
	Project string    `json:"project,omitempty"`
	Role    string    `json:"role"` // tenant-admin, tenant-dev, read-only
	Expires time.Time `json:"expires"`
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
