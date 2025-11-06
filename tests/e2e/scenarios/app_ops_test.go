package scenarios

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"testing"

	"github.com/vaheed/kubenova/tests/e2e/setup"
)

// TestAppOperations exercises deploy/suspend/resume/rollback if enabled by env E2E_VELA_OPS=1.
func TestAppOperations(t *testing.T) {
	if os.Getenv("E2E_VELA_OPS") == "" || os.Getenv("E2E_VELA_OPS") == "0" {
		t.Skip("vela ops disabled (set E2E_VELA_OPS=1 to enable)")
	}
	t.Parallel()
	env := setup.SuiteEnvironment()
	if env == nil {
		t.Skip("suite environment unavailable")
	}

	// Ensure cluster registered
	info, err := env.EnsureClusterRegistered(t.Context(), env.Config().ClusterName)
	if err != nil {
		t.Fatalf("cluster registration failed: %v", err)
	}

	base := env.ManagerBaseURL()
	httpc := env.HTTPClient()

	// Create tenant
	tb := map[string]any{"name": "ops-tenant"}
	tr := doJSON200(t, httpc, http.MethodPost, base+"/api/v1/clusters/"+info.Name+"/tenants", tb)
	tenant := str(tr["uid"])
	if tenant == "" {
		tenant = "ops-tenant"
	}

	// Create project
	pr := doJSON200(t, httpc, http.MethodPost, base+"/api/v1/clusters/"+info.Name+"/tenants/"+tenant+"/projects", map[string]any{"name": "ops-proj"})
	project := str(pr["uid"])
	if project == "" {
		project = "ops-proj"
	}

	// Create app
	ar := doJSON200(t, httpc, http.MethodPost, base+"/api/v1/clusters/"+info.Name+"/tenants/"+tenant+"/projects/"+project+"/apps", map[string]any{"name": "ops-app"})
	app := str(ar["uid"])
	if app == "" {
		app = "ops-app"
	}

	// Deploy, Suspend, Resume, Rollback
	doStatus(t, httpc, http.MethodPost, base+"/api/v1/clusters/"+info.Name+"/tenants/"+tenant+"/projects/"+project+"/apps/"+app+":deploy", nil, http.StatusAccepted)
	doStatus(t, httpc, http.MethodPost, base+"/api/v1/clusters/"+info.Name+"/tenants/"+tenant+"/projects/"+project+"/apps/"+app+":suspend", nil, http.StatusAccepted)
	doStatus(t, httpc, http.MethodPost, base+"/api/v1/clusters/"+info.Name+"/tenants/"+tenant+"/projects/"+project+"/apps/"+app+":resume", nil, http.StatusAccepted)
	body, _ := json.Marshal(map[string]any{"toRevision": 1})
	doStatus(t, httpc, http.MethodPost, base+"/api/v1/clusters/"+info.Name+"/tenants/"+tenant+"/projects/"+project+"/apps/"+app+":rollback", bytes.NewReader(body), http.StatusAccepted)

	// Traits, Policies, Image Update
	traits, _ := json.Marshal([]map[string]any{{"type": "scaler", "properties": map[string]any{"replicas": 2}}})
	doStatus(t, httpc, http.MethodPut, base+"/api/v1/clusters/"+info.Name+"/tenants/"+tenant+"/projects/"+project+"/apps/"+app+"/traits", bytes.NewReader(traits), http.StatusOK)
	pols, _ := json.Marshal([]map[string]any{{"type": "rollout", "properties": map[string]any{"maxUnavailable": 1}}})
	doStatus(t, httpc, http.MethodPut, base+"/api/v1/clusters/"+info.Name+"/tenants/"+tenant+"/projects/"+project+"/apps/"+app+"/policies", bytes.NewReader(pols), http.StatusOK)
	img, _ := json.Marshal(map[string]any{"component": "web", "image": "busybox", "tag": "latest"})
	doStatus(t, httpc, http.MethodPost, base+"/api/v1/clusters/"+info.Name+"/tenants/"+tenant+"/projects/"+project+"/apps/"+app+"/image-update", bytes.NewReader(img), http.StatusAccepted)
}

func doStatus(t *testing.T, c *http.Client, method, url string, body *bytes.Reader, want int) {
	t.Helper()
	var rdr *bytes.Reader
	if body != nil {
		rdr = body
	} else {
		rdr = bytes.NewReader(nil)
	}
	req, _ := http.NewRequest(method, url, rdr)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != want {
		t.Fatalf("%s %s: got %d want %d", method, url, resp.StatusCode, want)
	}
}
