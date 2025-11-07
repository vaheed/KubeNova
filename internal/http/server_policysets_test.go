package httpapi

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/vaheed/kubenova/internal/store"
)

func TestPolicySetsCRUDAndKubeconfigAndWorkflows(t *testing.T) {
	t.Setenv("KUBENOVA_REQUIRE_AUTH", "true")
	st := store.NewMemory()
	api := NewAPIServer(st)
	r := chi.NewRouter()
	_ = HandlerWithOptions(api, ChiServerOptions{BaseRouter: r})
	ts := httptest.NewServer(r)
	defer ts.Close()

	// Register a cluster
	kcfg := base64.StdEncoding.EncodeToString([]byte("apiVersion: v1\nclusters: []\ncontexts: []\n"))
	reg := map[string]any{"name": "kind", "kubeconfig": kcfg}
	rb, _ := json.Marshal(reg)
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/clusters", bytes.NewReader(rb))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test")
	req.Header.Set("X-KN-Roles", "admin")
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("register cluster: %v %s", err, resp.Status)
	}
	var cjson map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&cjson)
	resp.Body.Close()
	cuid := cjson["uid"].(string)

	// Create tenant
	tb := map[string]any{"name": "acme"}
	bb, _ := json.Marshal(tb)
	req, _ = http.NewRequest(http.MethodPost, ts.URL+"/api/v1/clusters/"+cuid+"/tenants", bytes.NewReader(bb))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test")
	req.Header.Set("X-KN-Roles", "tenantOwner")
	resp, err = http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("create tenant: %v %s", err, resp.Status)
	}
	var tjson map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&tjson)
	resp.Body.Close()
	ten := tjson["uid"].(string)

	// PolicySet create (allowed)
	psBody := []byte(`{"name":"baseline"}`)
	req, _ = http.NewRequest(http.MethodPost, ts.URL+"/api/v1/clusters/"+cuid+"/tenants/"+ten+"/policysets", bytes.NewReader(psBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test")
	req.Header.Set("X-KN-Roles", "tenantOwner")
	req.Header.Set("X-KN-Tenant", "acme")
	resp, err = http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("policyset create: %v %s", err, resp.Status)
	}
	resp.Body.Close()

	// PolicySet create forbidden for readOnly
	req, _ = http.NewRequest(http.MethodPost, ts.URL+"/api/v1/clusters/"+cuid+"/tenants/"+ten+"/policysets", bytes.NewReader(psBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test")
	req.Header.Set("X-KN-Roles", "readOnly")
	req.Header.Set("X-KN-Tenant", "acme")
	resp, _ = http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403, got %s", resp.Status)
	}
	resp.Body.Close()

	// List & get
	req, _ = http.NewRequest(http.MethodGet, ts.URL+"/api/v1/clusters/"+cuid+"/tenants/"+ten+"/policysets", nil)
	req.Header.Set("Authorization", "Bearer test")
	req.Header.Set("X-KN-Roles", "readOnly")
	req.Header.Set("X-KN-Tenant", "acme")
	resp, _ = http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("policyset list: %s", resp.Status)
	}
	resp.Body.Close()
	req, _ = http.NewRequest(http.MethodGet, ts.URL+"/api/v1/clusters/"+cuid+"/tenants/"+ten+"/policysets/baseline", nil)
	req.Header.Set("Authorization", "Bearer test")
	req.Header.Set("X-KN-Roles", "readOnly")
	req.Header.Set("X-KN-Tenant", "acme")
	resp, _ = http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("policyset get: %s", resp.Status)
	}
	resp.Body.Close()

	// Update & delete
	req, _ = http.NewRequest(http.MethodPut, ts.URL+"/api/v1/clusters/"+cuid+"/tenants/"+ten+"/policysets/baseline", bytes.NewReader([]byte(`{"name":"baseline","rules":[]}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test")
	req.Header.Set("X-KN-Roles", "tenantOwner")
	req.Header.Set("X-KN-Tenant", "acme")
	resp, _ = http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("policyset update: %s", resp.Status)
	}
	resp.Body.Close()
	req, _ = http.NewRequest(http.MethodDelete, ts.URL+"/api/v1/clusters/"+cuid+"/tenants/"+ten+"/policysets/baseline", nil)
	req.Header.Set("Authorization", "Bearer test")
	req.Header.Set("X-KN-Roles", "tenantOwner")
	req.Header.Set("X-KN-Tenant", "acme")
	resp, _ = http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("policyset delete: %s", resp.Status)
	}
	resp.Body.Close()

	// Project kubeconfig (create project first)
	pb := []byte(`{"name":"web"}`)
	req, _ = http.NewRequest(http.MethodPost, ts.URL+"/api/v1/clusters/"+cuid+"/tenants/"+ten+"/projects", bytes.NewReader(pb))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test")
	req.Header.Set("X-KN-Roles", "tenantOwner")
	req.Header.Set("X-KN-Tenant", "acme")
	resp, _ = http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("project create: %s", resp.Status)
	}
	var pjson map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&pjson)
	resp.Body.Close()
	pr := pjson["uid"].(string)
	req, _ = http.NewRequest(http.MethodGet, ts.URL+"/api/v1/clusters/"+cuid+"/tenants/"+ten+"/projects/"+pr+"/kubeconfig", nil)
	req.Header.Set("Authorization", "Bearer test")
	req.Header.Set("X-KN-Roles", "projectDev")
	req.Header.Set("X-KN-Tenant", "acme")
	resp, _ = http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("project kubeconfig: %s", resp.Status)
	}
	resp.Body.Close()

	// Tenant kubeconfig
	req, _ = http.NewRequest(http.MethodPost, ts.URL+"/api/v1/tenants/"+ten+"/kubeconfig", bytes.NewReader(nil))
	req.Header.Set("Authorization", "Bearer test")
	req.Header.Set("X-KN-Roles", "readOnly")
	req.Header.Set("X-KN-Tenant", "acme")
	resp, _ = http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("tenant kubeconfig: %s", resp.Status)
	}
	resp.Body.Close()

	// Workflows run/list/get
	// Create app first
	ab := []byte(`{"name":"hello"}`)
	req, _ = http.NewRequest(http.MethodPost, ts.URL+"/api/v1/clusters/"+cuid+"/tenants/"+ten+"/projects/"+pr+"/apps", bytes.NewReader(ab))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test")
	req.Header.Set("X-KN-Roles", "projectDev")
	req.Header.Set("X-KN-Tenant", "acme")
	resp, _ = http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("app create: %s", resp.Status)
	}
	var ajson map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&ajson)
	resp.Body.Close()
	app := ajson["uid"].(string)
	req, _ = http.NewRequest(http.MethodPost, ts.URL+"/api/v1/clusters/"+cuid+"/tenants/"+ten+"/projects/"+pr+"/apps/"+app+"/workflow/run", bytes.NewReader([]byte(`{"steps":["deploy"]}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test")
	req.Header.Set("X-KN-Roles", "projectDev")
	req.Header.Set("X-KN-Tenant", "acme")
	resp, _ = http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("workflow run: %s", resp.Status)
	}
	var run map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&run)
	resp.Body.Close()
	runID := run["id"].(string)
	// list
	req, _ = http.NewRequest(http.MethodGet, ts.URL+"/api/v1/clusters/"+cuid+"/tenants/"+ten+"/projects/"+pr+"/apps/"+app+"/workflow/runs", nil)
	req.Header.Set("Authorization", "Bearer test")
	req.Header.Set("X-KN-Roles", "projectDev")
	req.Header.Set("X-KN-Tenant", "acme")
	resp, _ = http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("runs list: %s", resp.Status)
	}
	resp.Body.Close()
	// get by id
	req, _ = http.NewRequest(http.MethodGet, ts.URL+"/api/v1/clusters/"+cuid+"/tenants/"+ten+"/projects/"+pr+"/apps/runs/"+runID, nil)
	req.Header.Set("Authorization", "Bearer test")
	req.Header.Set("X-KN-Roles", "projectDev")
	req.Header.Set("X-KN-Tenant", "acme")
	resp, _ = http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("run get: %s", resp.Status)
	}
	resp.Body.Close()
}
