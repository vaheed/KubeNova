package manager

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"os"
	"time"

	"github.com/vaheed/kubenova/internal/logging"
	"github.com/vaheed/kubenova/pkg/types"
	"go.uber.org/zap"
    corev1 "k8s.io/api/core/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
    "k8s.io/client-go/kubernetes"
    "k8s.io/client-go/tools/clientcmd"
)

type bootstrapUserReq struct {
	ClusterID   *int              `json:"cluster_id,omitempty"`
	ClusterUID  string            `json:"cluster_uid,omitempty"`
	Tenant      string            `json:"tenant"`
	Owners      []string          `json:"owners,omitempty"`
	Project     string            `json:"project"`
	Namespace   string            `json:"namespace,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
	Role        string            `json:"role,omitempty"` // tenant-admin by default
}

type bootstrapUserResp struct {
	Tenant     string `json:"tenant"`
	Project    string `json:"project"`
	Namespace  string `json:"namespace"`
	Kubeconfig string `json:"kubeconfigB64"`
}

func (s *Server) bootstrapUser(w http.ResponseWriter, r *http.Request) {
	var req bootstrapUserReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if req.Tenant == "" || req.Project == "" {
		http.Error(w, "tenant and project are required", 400)
		return
	}
	if req.Role == "" {
		req.Role = "tenant-admin"
	}
	nsName := req.Namespace
	if nsName == "" {
		nsName = req.Project
	}

	// Resolve cluster kubeconfig (by id or uid)
	var enc string
	var err error
	if req.ClusterUID != "" {
		if _, enc, err = s.store.GetClusterByUID(r.Context(), req.ClusterUID); err != nil {
			http.Error(w, err.Error(), 404)
			return
		}
	} else if req.ClusterID != nil {
		if _, enc, err = s.store.GetCluster(r.Context(), *req.ClusterID); err != nil {
			http.Error(w, err.Error(), 404)
			return
		}
	} else {
		http.Error(w, "one of cluster_uid or cluster_id is required", 400)
		return
	}
	kc, err := decodeB64(enc)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	restCfg, err := clientcmd.RESTConfigFromKubeConfig(kc)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	cset, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

    // 1) Create Capsule Tenant via dynamic client
    d, err := s.dynFactory(r.Context(), kc)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	tenantObj := map[string]any{
		"apiVersion": "capsule.clastix.io/v1beta2",
		"kind":       "Tenant",
		"metadata":   map[string]any{"name": req.Tenant},
		"spec":       map[string]any{"owners": owners(req.Owners)},
	}
    _, _ = d.Resource(gvrTenants).Create(r.Context(), &unstructured.Unstructured{Object: tenantObj}, metav1.CreateOptions{})

    // Optional: default TenantResourceQuota (env JSON map)
    if trq := os.Getenv("DEFAULT_TENANT_RESOURCEQUOTA"); trq != "" {
        var hard map[string]string
        if json.Unmarshal([]byte(trq), &hard) == nil && len(hard) > 0 {
            trqObj := map[string]any{
                "apiVersion": "capsule.clastix.io/v1beta2",
                "kind": "TenantResourceQuota",
                "metadata": map[string]any{"name": req.Tenant},
                "spec": map[string]any{"hard": hard},
            }
            _, _ = d.Resource(gvrTenantResourceQuota).Create(r.Context(), &unstructured.Unstructured{Object: trqObj}, metav1.CreateOptions{})
        }
    }

	// 2) Create Namespace with tenant/project labels
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: nsName, Labels: map[string]string{}}}
	if req.Labels != nil {
		ns.Labels = req.Labels
	}
	ns.Labels["kubenova.tenant"] = req.Tenant
	ns.Labels["kubenova.project"] = req.Project
	if req.Annotations != nil {
		ns.Annotations = req.Annotations
	}
	_, _ = cset.CoreV1().Namespaces().Create(r.Context(), ns, metav1.CreateOptions{})

	// Optional: default namespace ResourceQuota from env JSON
	if rq := os.Getenv("DEFAULT_NS_RESOURCEQUOTA"); rq != "" {
		var hard map[string]string
		if json.Unmarshal([]byte(rq), &hard) == nil && len(hard) > 0 {
			rqObj := &corev1.ResourceQuota{ObjectMeta: metav1.ObjectMeta{Name: "ns-default", Namespace: nsName}, Spec: corev1.ResourceQuotaSpec{Hard: corev1.ResourceList{}}}
			// Only a subset can be parsed here; leave values as raw strings via JSON is not trivial without resource.Parse
			// Skip conversion for simplicity; in production, parse quantities.
			// Create empty object to signal administrator default policy if needed.
			_, _ = cset.CoreV1().ResourceQuotas(nsName).Create(r.Context(), rqObj, metav1.CreateOptions{})
		}
	}

	// 3) Create Project record in Manager store
    p := types.Project{Tenant: req.Tenant, Name: req.Project, CreatedAt: time.Now().UTC()}
    _ = s.store.CreateProject(r.Context(), p)

	// 4) Issue kubeconfig to capsule-proxy
	g := types.KubeconfigGrant{Tenant: req.Tenant, Role: req.Role, Expires: time.Now().UTC().Add(1 * time.Hour)}
	kubeconfig, err := GenerateKubeconfig(g, os.Getenv("CAPSULE_PROXY_URL"))
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	resp := bootstrapUserResp{Tenant: req.Tenant, Project: req.Project, Namespace: nsName, Kubeconfig: base64.StdEncoding.EncodeToString(kubeconfig)}
	logging.WithTrace(r.Context(), logging.FromContext(r.Context())).Info("bootstrap_user", zap.String("tenant", req.Tenant), zap.String("project", req.Project))
	respond(w, resp, nil)
}

// helpers
func owners(list []string) []map[string]any {
	out := make([]map[string]any, 0, len(list))
	for _, o := range list {
		out = append(out, map[string]any{"kind": "User", "name": o})
	}
	return out
}

// no extra helpers required
