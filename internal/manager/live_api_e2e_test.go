//go:build integration

package manager

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// TestLiveAPIE2E exercises the running manager against a real cluster (e.g., kind).
// Requires RUN_LIVE_E2E=1, a reachable manager, and kubeconfig for the target cluster.
func TestLiveAPIE2E(t *testing.T) {
	if os.Getenv("RUN_LIVE_E2E") != "1" {
		t.Skip("set RUN_LIVE_E2E=1 to run live API test")
	}
	base := strings.TrimSuffix(os.Getenv("KUBENOVA_E2E_BASE_URL"), "/")
	if base == "" {
		base = "http://localhost:8080"
	}
	kubeB64 := os.Getenv("KUBENOVA_E2E_KUBECONFIG_B64")
	if kubeB64 == "" {
		if path := os.Getenv("KUBENOVA_E2E_KUBECONFIG"); path != "" {
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read kubeconfig: %v", err)
			}
			kubeB64 = base64.StdEncoding.EncodeToString(raw)
		} else {
			t.Skip("KUBENOVA_E2E_KUBECONFIG or KUBENOVA_E2E_KUBECONFIG_B64 must be set")
		}
	}

	client := &http.Client{Timeout: 15 * time.Second}
	headers := map[string]string{}
	if tok := strings.TrimSpace(os.Getenv("KUBENOVA_E2E_TOKEN")); tok != "" {
		headers["Authorization"] = "Bearer " + tok
	} else {
		headers["X-KN-Roles"] = "admin"
	}

	uniq := time.Now().Unix()
	cluster := liveJSON[map[string]any](t, client, http.MethodPost, base+"/api/v1/clusters", map[string]any{
		"name":       fmt.Sprintf("kind-live-%d", uniq),
		"datacenter": "dev",
		"labels":     map[string]string{"e2e": "true"},
		"kubeconfig": kubeB64,
	}, headers, http.StatusCreated)
	clusterID := fmt.Sprint(cluster["id"])

	tenant := liveJSON[map[string]any](t, client, http.MethodPost, fmt.Sprintf("%s/api/v1/clusters/%s/tenants", base, clusterID), map[string]any{
		"name":   fmt.Sprintf("tenant-%d", uniq),
		"owners": []string{"e2e@example.com"},
		"plan":   "gold",
	}, headers, http.StatusCreated)
	tenantID := fmt.Sprint(tenant["id"])

	project := liveJSON[map[string]any](t, client, http.MethodPost, fmt.Sprintf("%s/api/v1/clusters/%s/tenants/%s/projects", base, clusterID, tenantID), map[string]any{
		"name":        fmt.Sprintf("proj-%d", uniq),
		"description": "live e2e project",
	}, headers, http.StatusCreated)
	projectID := fmt.Sprint(project["id"])

	app := liveJSON[map[string]any](t, client, http.MethodPost, fmt.Sprintf("%s/api/v1/clusters/%s/tenants/%s/projects/%s/apps", base, clusterID, tenantID, projectID), map[string]any{
		"name":        fmt.Sprintf("app-%d", uniq),
		"description": "live e2e app",
		"component":   "web",
		"image":       "ghcr.io/vaheed/kubenova/kubenova-manager:v0.1.1",
		"spec": map[string]any{
			"type": "webservice",
			"properties": map[string]any{
				"image": "ghcr.io/vaheed/kubenova/kubenova-manager:v0.1.1",
				"port":  8080,
			},
		},
		"traits": []map[string]any{
			{
				"type": "scaler",
				"properties": map[string]any{
					"min": 1,
					"max": 2,
				},
			},
		},
	}, headers, http.StatusCreated)
	appID := fmt.Sprint(app["id"])

	liveNoBody(t, client, http.MethodPost, fmt.Sprintf("%s/api/v1/clusters/%s/tenants/%s/projects/%s/apps/%s:deploy", base, clusterID, tenantID, projectID, appID), nil, headers, http.StatusAccepted)
	_ = liveJSON[map[string]any](t, client, http.MethodGet, fmt.Sprintf("%s/api/v1/clusters/%s/tenants/%s/projects/%s/apps/%s/status", base, clusterID, tenantID, projectID, appID), nil, headers, http.StatusOK)

	_ = liveJSON[map[string]any](t, client, http.MethodPost, fmt.Sprintf("%s/api/v1/clusters/%s/tenants/%s/projects/%s/apps/%s/workflow/run", base, clusterID, tenantID, projectID, appID), map[string]any{
		"inputs": map[string]any{"action": "smoke-test"},
	}, headers, http.StatusAccepted)

	_ = liveJSON[map[string]any](t, client, http.MethodGet, fmt.Sprintf("%s/api/v1/tenants/%s/usage", base, tenantID), nil, headers, http.StatusOK)
	_ = liveJSON[map[string]any](t, client, http.MethodGet, fmt.Sprintf("%s/api/v1/projects/%s/usage", base, projectID), nil, headers, http.StatusOK)
}

func liveNoBody(t *testing.T, client *http.Client, method, url string, body any, headers map[string]string, wantStatus int) {
	t.Helper()
	resp := liveRequest(t, client, method, url, body, headers)
	defer resp.Body.Close()
	if resp.StatusCode != wantStatus {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("%s %s: want %d got %d body=%s", method, url, wantStatus, resp.StatusCode, string(raw))
	}
}

func liveJSON[T any](t *testing.T, client *http.Client, method, url string, body any, headers map[string]string, wantStatus int) T {
	t.Helper()
	resp := liveRequest(t, client, method, url, body, headers)
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
		t.Fatalf("decode %s: %v (body=%s)", url, err, string(raw))
	}
	return out
}

func liveRequest(t *testing.T, client *http.Client, method, url string, body any, headers map[string]string) *http.Response {
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
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("execute %s %s: %v", method, url, err)
	}
	return resp
}
