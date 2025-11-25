package manager

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/vaheed/kubenova/internal/store"
	"github.com/vaheed/kubenova/pkg/types"
)

func TestManagerEndToEndLifecycle(t *testing.T) {
	t.Setenv("KUBENOVA_REQUIRE_AUTH", "false")
	t.Setenv("JWT_SIGNING_KEY", "unused")

	st := store.NewMemoryStore()
	srv := NewServer(st)
	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	client := ts.Client()
	baseURL := ts.URL + "/api/v1"

	cluster := doJSON[*types.Cluster](t, client, http.MethodPost, baseURL+"/clusters", map[string]any{
		"name":       "dev-cluster",
		"datacenter": "dc1",
		"kubeconfig": fakeKubeconfig,
		"labels": map[string]string{
			"env": "dev",
		},
	}, http.StatusCreated)
	if cluster == nil || cluster.ID == "" {
		t.Fatalf("expected cluster ID, got %#v", cluster)
	}
	if cluster.Kubeconfig != "" {
		t.Fatalf("create response must redact kubeconfig")
	}

	clusters := doJSON[[]*types.Cluster](t, client, http.MethodGet, baseURL+"/clusters", nil, http.StatusOK)
	if len(clusters) != 1 {
		t.Fatalf("expected 1 cluster, got %d", len(clusters))
	}

	tenant := doJSON[*types.Tenant](t, client, http.MethodPost,
		fmt.Sprintf("%s/clusters/%s/tenants", baseURL, cluster.ID), map[string]any{
			"name":   "acme",
			"owners": []string{"alice"},
			"plan":   "gold",
			"quotas": map[string]string{"cpu": "4"},
		}, http.StatusCreated)
	if tenant.ClusterID != cluster.ID {
		t.Fatalf("tenant cluster mismatch: %s", tenant.ClusterID)
	}
	if tenant.OwnerNamespace == "" || tenant.AppsNamespace == "" {
		t.Fatalf("namespaces should be populated: %#v", tenant)
	}

	project := doJSON[*types.Project](t, client, http.MethodPost,
		fmt.Sprintf("%s/clusters/%s/tenants/%s/projects", baseURL, cluster.ID, tenant.ID),
		map[string]any{
			"name":        "payments",
			"description": "Handles payment flows",
		}, http.StatusCreated)
	if project.TenantID != tenant.ID {
		t.Fatalf("project tenant mismatch: %s", project.TenantID)
	}

	app := doJSON[*types.App](t, client, http.MethodPost,
		fmt.Sprintf("%s/clusters/%s/tenants/%s/projects/%s/apps", baseURL, cluster.ID, tenant.ID, project.ID),
		map[string]any{
			"name":        "api",
			"description": "API service",
			"component":   "web",
			"image":       "ghcr.io/vaheed/api:latest",
			"spec": map[string]any{
				"type": "webservice",
				"properties": map[string]any{
					"image": "ghcr.io/vaheed/api:latest",
					"port":  8080,
				},
			},
			"traits": []map[string]any{
				{
					"type": "scaler",
					"properties": map[string]any{
						"min": 1,
						"max": 3,
					},
				},
			},
		}, http.StatusCreated)
	if app.ProjectID != project.ID {
		t.Fatalf("app project mismatch: %s", app.ProjectID)
	}
	if app.Status != "pending" || app.Revision != 1 {
		t.Fatalf("new app should be pending revision 1")
	}

	_ = doJSON[map[string]any](t, client, http.MethodPost,
		fmt.Sprintf("%s/clusters/%s/tenants/%s/projects/%s/apps/%s:deploy", baseURL, cluster.ID, tenant.ID, project.ID, app.ID),
		nil, http.StatusAccepted)

	status := doJSON[map[string]any](t, client, http.MethodGet,
		fmt.Sprintf("%s/clusters/%s/tenants/%s/projects/%s/apps/%s/status", baseURL, cluster.ID, tenant.ID, project.ID, app.ID),
		nil, http.StatusOK)
	if status["status"] != "Deployed" {
		t.Fatalf("expected deployed status, got %v", status["status"])
	}

	updated := doJSON[*types.App](t, client, http.MethodPut,
		fmt.Sprintf("%s/clusters/%s/tenants/%s/projects/%s/apps/%s", baseURL, cluster.ID, tenant.ID, project.ID, app.ID),
		map[string]any{
			"description": "API service v2",
			"spec": map[string]any{
				"type": "webservice",
				"properties": map[string]any{
					"image": "ghcr.io/vaheed/api:v2",
					"port":  8080,
				},
			},
		}, http.StatusOK)
	if updated.Revision != 2 {
		t.Fatalf("expected revision 2, got %d", updated.Revision)
	}

	revisions := doJSON[[]types.AppRevision](t, client, http.MethodGet,
		fmt.Sprintf("%s/clusters/%s/tenants/%s/projects/%s/apps/%s/revisions", baseURL, cluster.ID, tenant.ID, project.ID, app.ID),
		nil, http.StatusOK)
	if len(revisions) != 2 {
		t.Fatalf("expected 2 revisions, got %d", len(revisions))
	}

	summary := doJSON[*types.TenantSummary](t, client, http.MethodGet,
		fmt.Sprintf("%s/clusters/%s/tenants/%s/summary", baseURL, cluster.ID, tenant.ID),
		nil, http.StatusOK)
	if summary.Projects != 1 || summary.Apps != 1 {
		t.Fatalf("unexpected summary counts: %+v", summary)
	}

	run := doJSON[*types.WorkflowRun](t, client, http.MethodPost,
		fmt.Sprintf("%s/clusters/%s/tenants/%s/projects/%s/apps/%s/workflow/run", baseURL, cluster.ID, tenant.ID, project.ID, app.ID),
		map[string]any{
			"inputs": map[string]any{"action": "smoke-test"},
		}, http.StatusAccepted)
	if run.Status != "Running" {
		t.Fatalf("expected workflow running status, got %s", run.Status)
	}

	runs := doJSON[[]types.WorkflowRun](t, client, http.MethodGet,
		fmt.Sprintf("%s/clusters/%s/tenants/%s/projects/%s/apps/%s/workflow/runs", baseURL, cluster.ID, tenant.ID, project.ID, app.ID),
		nil, http.StatusOK)
	if len(runs) != 1 {
		t.Fatalf("expected 1 workflow run, got %d", len(runs))
	}

	runLookup := doJSON[*types.WorkflowRun](t, client, http.MethodGet,
		fmt.Sprintf("%s/apps/runs/%s", baseURL, run.ID), nil, http.StatusOK)
	if runLookup.ID != run.ID {
		t.Fatalf("expected workflow run %s, got %s", run.ID, runLookup.ID)
	}

	usage := doJSON[*types.UsageRecord](t, client, http.MethodGet,
		fmt.Sprintf("%s/tenants/%s/usage", baseURL, tenant.ID), nil, http.StatusOK)
	if usage.Apps != 1 {
		t.Fatalf("expected tenant usage apps=1, got %+v", usage)
	}

	projectUsage := doJSON[*types.UsageRecord](t, client, http.MethodGet,
		fmt.Sprintf("%s/projects/%s/usage", baseURL, project.ID), nil, http.StatusOK)
	if projectUsage.Apps != 1 {
		t.Fatalf("expected project apps=1, got %+v", projectUsage)
	}

	_ = doJSON[map[string]any](t, client, http.MethodPost,
		fmt.Sprintf("%s/clusters/%s/tenants/%s/projects/%s/apps/%s:suspend", baseURL, cluster.ID, tenant.ID, project.ID, app.ID),
		nil, http.StatusAccepted)
	suspendStatus := doJSON[map[string]any](t, client, http.MethodGet,
		fmt.Sprintf("%s/clusters/%s/tenants/%s/projects/%s/apps/%s/status", baseURL, cluster.ID, tenant.ID, project.ID, app.ID),
		nil, http.StatusOK)
	if suspendStatus["suspended"] != true {
		t.Fatalf("expected app suspended flag true, got %v", suspendStatus["suspended"])
	}

	_ = doJSON[map[string]any](t, client, http.MethodPost,
		fmt.Sprintf("%s/clusters/%s/tenants/%s/projects/%s/apps/%s:resume", baseURL, cluster.ID, tenant.ID, project.ID, app.ID),
		nil, http.StatusAccepted)

	_ = doJSON[map[string]any](t, client, http.MethodPost,
		fmt.Sprintf("%s/clusters/%s/tenants/%s/projects/%s/apps/%s:delete", baseURL, cluster.ID, tenant.ID, project.ID, app.ID),
		nil, http.StatusAccepted)

	apps := doJSON[[]*types.App](t, client, http.MethodGet,
		fmt.Sprintf("%s/clusters/%s/tenants/%s/projects/%s/apps", baseURL, cluster.ID, tenant.ID, project.ID),
		nil, http.StatusOK)
	if len(apps) != 0 {
		t.Fatalf("expected no apps after delete, got %d", len(apps))
	}

	doNoBody(t, client, http.MethodDelete,
		fmt.Sprintf("%s/clusters/%s/tenants/%s/projects/%s", baseURL, cluster.ID, tenant.ID, project.ID),
		nil, http.StatusNoContent)
}

const fakeKubeconfig = `
apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://127.0.0.1:6443
    insecure-skip-tls-verify: true
  name: fake
contexts:
- context:
    cluster: fake
    user: fake
  name: fake
current-context: fake
users:
- name: fake
  user:
    token: fake
`

func doJSON[T any](t *testing.T, client *http.Client, method, url string, body any, wantStatus int) T {
	t.Helper()
	resp := doRequest(t, client, method, url, body)
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body %s: %v", url, err)
	}
	if resp.StatusCode != wantStatus {
		t.Fatalf("%s %s: want %d got %d body=%s", method, url, wantStatus, resp.StatusCode, string(raw))
	}
	var out T
	if len(bytes.TrimSpace(raw)) == 0 {
		return out
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode %s response: %v (body=%s)", url, err, string(raw))
	}
	return out
}

func doNoBody(t *testing.T, client *http.Client, method, url string, body any, wantStatus int) {
	t.Helper()
	resp := doRequest(t, client, method, url, body)
	defer resp.Body.Close()
	if resp.StatusCode != wantStatus {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("%s %s: want %d got %d body=%s", method, url, wantStatus, resp.StatusCode, string(raw))
	}
	io.Copy(io.Discard, resp.Body)
}

func doRequest(t *testing.T, client *http.Client, method, url string, body any) *http.Response {
	t.Helper()
	var reader io.Reader
	if body != nil {
		buf := new(bytes.Buffer)
		if err := json.NewEncoder(buf).Encode(body); err != nil {
			t.Fatalf("encode body for %s: %v", url, err)
		}
		reader = buf
	}
	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		t.Fatalf("build request %s: %v", url, err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("execute %s %s: %v", method, url, err)
	}
	return resp
}
