package manager

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"
)

var (
	gvrTenants             = schema.GroupVersionResource{Group: "capsule.clastix.io", Version: "v1beta2", Resource: "tenants"}
	gvrTenantResourceQuota = schema.GroupVersionResource{Group: "capsule.clastix.io", Version: "v1beta2", Resource: "tenantresourcequotas"}
	gvrNamespaceOptions    = schema.GroupVersionResource{Group: "capsule.clastix.io", Version: "v1beta2", Resource: "namespaceoptions"}
	gvrCapsuleConfig       = schema.GroupVersionResource{Group: "capsule.clastix.io", Version: "v1beta2", Resource: "capsuleconfigurations"}
)

// dynFactory abstracts dynamic client creation (overridden in tests).
func defaultDynFactory(ctx context.Context, kubeconfig []byte) (dynamic.Interface, error) {
	cfg, err := clientcmd.RESTConfigFromKubeConfig(kubeconfig)
	if err != nil {
		return nil, err
	}
	return dynamic.NewForConfig(cfg)
}

func (s *Server) dynamicForCluster(ctx context.Context, clusterID int) (dynamic.Interface, error) {
	_, enc, err := s.store.GetCluster(ctx, clusterID)
	if err != nil {
		return nil, err
	}
	kc, err := decodeB64(enc)
	if err != nil {
		return nil, err
	}
	return s.dynFactory(ctx, kc)
}

func (s *Server) parseClusterID(r *http.Request) (int, bool, error) {
	q := strings.TrimSpace(r.URL.Query().Get("cluster_id"))
	if q == "" {
		return 0, false, nil
	}
	id, err := strconv.Atoi(q)
	if err != nil {
		return 0, false, err
	}
	return id, true, nil
}

func (s *Server) dynamicForClusterUID(ctx context.Context, uid string) (dynamic.Interface, error) {
	_, enc, err := s.store.GetClusterByUID(ctx, uid)
	if err != nil {
		return nil, err
	}
	kc, err := decodeB64(enc)
	if err != nil {
		return nil, err
	}
	return s.dynFactory(ctx, kc)
}

func (s *Server) capsuleList(w http.ResponseWriter, r *http.Request, gvr schema.GroupVersionResource) {
	id, _, err := s.parseClusterID(r)
	if err != nil {
		http.Error(w, "invalid cluster_id", 400)
		return
	}
	d, err := s.dynamicForCluster(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	opts := metav1.ListOptions{LabelSelector: r.URL.Query().Get("labelSelector"), FieldSelector: r.URL.Query().Get("fieldSelector")}
	if lim := r.URL.Query().Get("limit"); lim != "" {
		if v, e := strconv.Atoi(lim); e == nil {
			opts.Limit = int64(v)
		}
	}
	lst, err := d.Resource(gvr).List(r.Context(), opts)
	if err != nil {
		respond(w, nil, err)
		return
	}
	items := make([]interface{}, 0, len(lst.Items))
	for i := range lst.Items {
		items = append(items, lst.Items[i].Object)
	}
	out := map[string]interface{}{
		"apiVersion": lst.GetAPIVersion(),
		"kind":       lst.GetKind(),
		"items":      items,
	}
	respond(w, out, nil)
}

func (s *Server) capsuleGet(w http.ResponseWriter, r *http.Request, gvr schema.GroupVersionResource, name string) {
	id, _, err := s.parseClusterID(r)
	if err != nil {
		http.Error(w, "invalid cluster_id", 400)
		return
	}
	d, err := s.dynamicForCluster(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	obj, err := d.Resource(gvr).Get(r.Context(), name, metav1.GetOptions{})
	if err != nil {
		respond(w, nil, err)
		return
	}
	respond(w, obj.Object, nil)
}

func (s *Server) capsuleCreate(w http.ResponseWriter, r *http.Request, gvr schema.GroupVersionResource) {
	id, _, err := s.parseClusterID(r)
	if err != nil {
		http.Error(w, "invalid cluster_id", 400)
		return
	}
	d, err := s.dynamicForCluster(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	var body map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	obj := &unstructured.Unstructured{Object: body}
	created, err := d.Resource(gvr).Create(r.Context(), obj, metav1.CreateOptions{})
	if err != nil {
		respond(w, nil, err)
		return
	}
	respond(w, created.Object, nil)
}

func (s *Server) capsuleUpdate(w http.ResponseWriter, r *http.Request, gvr schema.GroupVersionResource, name string) {
	id, _, err := s.parseClusterID(r)
	if err != nil {
		http.Error(w, "invalid cluster_id", 400)
		return
	}
	d, err := s.dynamicForCluster(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	var body map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	// ensure metadata.name matches path
	md, _ := body["metadata"].(map[string]interface{})
	if md == nil {
		md = map[string]interface{}{}
		body["metadata"] = md
	}
	md["name"] = name
	obj := &unstructured.Unstructured{Object: body}
	updated, err := d.Resource(gvr).Update(r.Context(), obj, metav1.UpdateOptions{})
	if err != nil {
		respond(w, nil, err)
		return
	}
	respond(w, updated.Object, nil)
}

func (s *Server) capsuleDelete(w http.ResponseWriter, r *http.Request, gvr schema.GroupVersionResource, name string) {
	id, _, err := s.parseClusterID(r)
	if err != nil {
		http.Error(w, "invalid cluster_id", 400)
		return
	}
	d, err := s.dynamicForCluster(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	err = d.Resource(gvr).Delete(r.Context(), name, metav1.DeleteOptions{})
	if err != nil {
		respond(w, nil, err)
		return
	}
	respond(w, map[string]string{"status": "ok"}, nil)
}

// no extra helpers required
