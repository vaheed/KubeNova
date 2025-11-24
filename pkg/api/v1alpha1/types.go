package v1alpha1

import (
	"maps"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	GroupVersion = schema.GroupVersion{Group: "kubenova.io", Version: "v1alpha1"}
)

// NovaTenantSpec defines desired state for a tenant.
type NovaTenantSpec struct {
	Owners          []string          `json:"owners,omitempty"`
	Plan            string            `json:"plan,omitempty"`
	Labels          map[string]string `json:"labels,omitempty"`
	OwnerNamespace  string            `json:"ownerNamespace,omitempty"`
	AppsNamespace   string            `json:"appsNamespace,omitempty"`
	NetworkPolicies []string          `json:"networkPolicies,omitempty"`
	Quotas          map[string]string `json:"quotas,omitempty"`
	Limits          map[string]string `json:"limits,omitempty"`
	ProxyEndpoint   string            `json:"proxyEndpoint,omitempty"`
}

func (s NovaTenantSpec) DeepCopy() NovaTenantSpec {
	out := s
	if s.Labels != nil {
		out.Labels = maps.Clone(s.Labels)
	}
	if s.Quotas != nil {
		out.Quotas = maps.Clone(s.Quotas)
	}
	if s.Limits != nil {
		out.Limits = maps.Clone(s.Limits)
	}
	if s.NetworkPolicies != nil {
		out.NetworkPolicies = append([]string{}, s.NetworkPolicies...)
	}
	if s.Owners != nil {
		out.Owners = append([]string{}, s.Owners...)
	}
	return out
}

// NovaTenant represents a logical tenant inside the cluster.
type NovaTenant struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec NovaTenantSpec `json:"spec,omitempty"`
}

func (in *NovaTenant) DeepCopyObject() runtime.Object {
	if in == nil {
		return nil
	}
	out := *in
	out.ObjectMeta = *in.ObjectMeta.DeepCopy()
	out.Spec = in.Spec.DeepCopy()
	return &out
}

// NovaTenantList contains a list of tenants.
type NovaTenantList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NovaTenant `json:"items"`
}

func (in *NovaTenantList) DeepCopyObject() runtime.Object {
	if in == nil {
		return nil
	}
	out := *in
	out.ListMeta = in.ListMeta
	if in.Items != nil {
		out.Items = make([]NovaTenant, len(in.Items))
		for i := range in.Items {
			out.Items[i] = *in.Items[i].DeepCopyObject().(*NovaTenant)
		}
	}
	return &out
}

// NovaProjectSpec holds project metadata and tenant association.
type NovaProjectSpec struct {
	Tenant      string            `json:"tenant"`
	Description string            `json:"description,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	Access      []string          `json:"access,omitempty"`
}

func (s NovaProjectSpec) DeepCopy() NovaProjectSpec {
	out := s
	if s.Labels != nil {
		out.Labels = maps.Clone(s.Labels)
	}
	if s.Access != nil {
		out.Access = append([]string{}, s.Access...)
	}
	return out
}

// NovaProject groups applications under a tenant.
type NovaProject struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec NovaProjectSpec `json:"spec,omitempty"`
}

func (in *NovaProject) DeepCopyObject() runtime.Object {
	if in == nil {
		return nil
	}
	out := *in
	out.ObjectMeta = *in.ObjectMeta.DeepCopy()
	out.Spec = in.Spec.DeepCopy()
	return &out
}

// NovaProjectList is a list of projects.
type NovaProjectList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NovaProject `json:"items"`
}

func (in *NovaProjectList) DeepCopyObject() runtime.Object {
	if in == nil {
		return nil
	}
	out := *in
	out.ListMeta = in.ListMeta
	if in.Items != nil {
		out.Items = make([]NovaProject, len(in.Items))
		for i := range in.Items {
			out.Items[i] = *in.Items[i].DeepCopyObject().(*NovaProject)
		}
	}
	return &out
}

// NovaAppSpec describes a single application deployment.
type NovaAppSpec struct {
	Tenant      string           `json:"tenant"`
	Project     string           `json:"project"`
	Namespace   string           `json:"namespace,omitempty"`
	Description string           `json:"description,omitempty"`
	Component   string           `json:"component,omitempty"`
	Image       string           `json:"image,omitempty"`
	Template    map[string]any   `json:"template,omitempty"`
	Traits      []map[string]any `json:"traits,omitempty"`
	Policies    []map[string]any `json:"policies,omitempty"`
}

func (s NovaAppSpec) DeepCopy() NovaAppSpec {
	out := s
	if s.Template != nil {
		out.Template = maps.Clone(s.Template)
	}
	if s.Traits != nil {
		out.Traits = append([]map[string]any{}, s.Traits...)
	}
	if s.Policies != nil {
		out.Policies = append([]map[string]any{}, s.Policies...)
	}
	return out
}

// NovaApp represents an app definition to be projected into KubeVela.
type NovaApp struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec NovaAppSpec `json:"spec,omitempty"`
}

func (in *NovaApp) DeepCopyObject() runtime.Object {
	if in == nil {
		return nil
	}
	out := *in
	out.ObjectMeta = *in.ObjectMeta.DeepCopy()
	out.Spec = in.Spec.DeepCopy()
	return &out
}

// NovaAppList lists apps.
type NovaAppList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NovaApp `json:"items"`
}

func (in *NovaAppList) DeepCopyObject() runtime.Object {
	if in == nil {
		return nil
	}
	out := *in
	out.ListMeta = in.ListMeta
	if in.Items != nil {
		out.Items = make([]NovaApp, len(in.Items))
		for i := range in.Items {
			out.Items[i] = *in.Items[i].DeepCopyObject().(*NovaApp)
		}
	}
	return &out
}

// AddToScheme registers all Nova API types.
func AddToScheme(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(GroupVersion,
		&NovaTenant{}, &NovaTenantList{},
		&NovaProject{}, &NovaProjectList{},
		&NovaApp{}, &NovaAppList{},
	)
	metav1.AddToGroupVersion(scheme, GroupVersion)
	return nil
}
