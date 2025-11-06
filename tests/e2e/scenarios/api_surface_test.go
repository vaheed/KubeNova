package scenarios

import (
    "bytes"
    "encoding/base64"
    "encoding/json"
    "net/http"
    "testing"

    "github.com/vaheed/kubenova/tests/e2e/setup"
)

func TestAPISurfaceBasicFlows(t *testing.T) {
    t.Parallel()
    env := setup.SuiteEnvironment()
    if env == nil {
        t.Skip("suite environment unavailable")
    }

    // Register the suite cluster (idempotent)
    info, err := env.EnsureClusterRegistered(t.Context(), env.Config().ClusterName)
    if err != nil {
        t.Fatalf("cluster registration failed: %v", err)
    }

    base := env.ManagerBaseURL()
    httpc := env.HTTPClient()

    // Capabilities
    doGet200(t, httpc, base+"/api/v1/clusters/"+info.Name+"/capabilities", nil)

    // Create tenant
    tenantName := "e2e-tenant"
    tb := map[string]any{"name": tenantName}
    tjson := doJSON200(t, httpc, http.MethodPost, base+"/api/v1/clusters/"+info.Name+"/tenants", tb)
    tenantUID := str(tjson["uid"]) // DTO may include uid

    // List tenants
    _ = doGet200(t, httpc, base+"/api/v1/clusters/"+info.Name+"/tenants", nil)

    // Resolve tenant by UID when present; fallback to name for older flows
    tenantRef := tenantUID
    if tenantRef == "" { tenantRef = tenantName }

    // Create project
    projectName := "web"
    pb := map[string]any{"name": projectName}
    pjson := doJSON200(t, httpc, http.MethodPost, base+"/api/v1/clusters/"+info.Name+"/tenants/"+tenantRef+"/projects", pb)
    projectUID := str(pjson["uid"]) // optional
    if projectUID == "" { projectUID = projectName }

    // List projects
    _ = doGet200(t, httpc, base+"/api/v1/clusters/"+info.Name+"/tenants/"+tenantRef+"/projects", nil)

    // Create app
    appName := "hello"
    ab := map[string]any{"name": appName}
    ajson := doJSON200(t, httpc, http.MethodPost, base+"/api/v1/clusters/"+info.Name+"/tenants/"+tenantRef+"/projects/"+projectUID+"/apps", ab)
    appUID := str(ajson["uid"]) // optional
    if appUID == "" { appUID = appName }

    // List apps
    _ = doGet200(t, httpc, base+"/api/v1/clusters/"+info.Name+"/tenants/"+tenantRef+"/projects/"+projectUID+"/apps", nil)

    // Get app
    _ = doGet200(t, httpc, base+"/api/v1/clusters/"+info.Name+"/tenants/"+tenantRef+"/projects/"+projectUID+"/apps/"+appUID, nil)

    // Project kubeconfig (best-effort)
    _ = doGet200(t, httpc, base+"/api/v1/clusters/"+info.Name+"/tenants/"+tenantRef+"/projects/"+projectUID+"/kubeconfig", func(m map[string]any) error {
        if v, ok := m["kubeconfig"].(string); ok && v != "" {
            if _, err := base64.StdEncoding.DecodeString(v); err != nil { t.Fatalf("invalid kubeconfig encoding: %v", err) }
        }
        return nil
    })
}

func doJSON200(t *testing.T, c *http.Client, method, url string, body any) map[string]any {
    t.Helper()
    var rdr *bytes.Reader
    if body != nil {
        b, _ := json.Marshal(body)
        rdr = bytes.NewReader(b)
    } else { rdr = bytes.NewReader(nil) }
    req, _ := http.NewRequest(method, url, rdr)
    req.Header.Set("Content-Type", "application/json")
    resp, err := c.Do(req)
    if err != nil { t.Fatalf("%s %s: %v", method, url, err) }
    defer resp.Body.Close()
    if resp.StatusCode != http.StatusOK { t.Fatalf("%s %s: %s", method, url, resp.Status) }
    var out map[string]any
    _ = json.NewDecoder(resp.Body).Decode(&out)
    return out
}

func doGet200(t *testing.T, c *http.Client, url string, check func(map[string]any) error) map[string]any {
    t.Helper()
    resp, err := c.Get(url)
    if err != nil { t.Fatalf("GET %s: %v", url, err) }
    defer resp.Body.Close()
    if resp.StatusCode != http.StatusOK { t.Fatalf("GET %s: %s", url, resp.Status) }
    var out map[string]any
    _ = json.NewDecoder(resp.Body).Decode(&out)
    if check != nil { _ = check(out) }
    return out
}

func str(v any) string {
    if s, ok := v.(string); ok { return s }
    return ""
}

