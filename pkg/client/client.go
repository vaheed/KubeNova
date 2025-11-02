package client

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "net/url"

    "github.com/vaheed/kubenova/pkg/types"
)

type Client struct {
    base string
    http *http.Client
    token string
}

func New(base, token string) *Client { return &Client{base: trim(base), http: http.DefaultClient, token: token} }

func trim(s string) string { if len(s) > 0 && s[len(s)-1] == '/' { return s[:len(s)-1] }; return s }

func (c *Client) req(ctx context.Context, method, path string, body any) (*http.Request, error) {
    var br *bytes.Reader
    if body != nil { b, _ := json.Marshal(body); br = bytes.NewReader(b) } else { br = bytes.NewReader(nil) }
    u, _ := url.Parse(c.base + path)
    req, _ := http.NewRequestWithContext(ctx, method, u.String(), br)
    req.Header.Set("Content-Type", "application/json")
    if c.token != "" { req.Header.Set("Authorization", "Bearer "+c.token) }
    return req, nil
}

// Tenants
func (c *Client) ListTenants(ctx context.Context) ([]types.Tenant, error) {
    req, _ := c.req(ctx, http.MethodGet, "/api/v1/tenants", nil)
    resp, err := c.http.Do(req); if err != nil { return nil, err }
    defer resp.Body.Close()
    var v []types.Tenant
    if err := json.NewDecoder(resp.Body).Decode(&v); err != nil { return nil, err }
    return v, nil
}
func (c *Client) CreateTenant(ctx context.Context, t types.Tenant) (types.Tenant, error) {
    req, _ := c.req(ctx, http.MethodPost, "/api/v1/tenants", t)
    resp, err := c.http.Do(req); if err != nil { return types.Tenant{}, err }
    defer resp.Body.Close()
    if resp.StatusCode >= 300 { return types.Tenant{}, fmt.Errorf("status %s", resp.Status) }
    var v types.Tenant; _ = json.NewDecoder(resp.Body).Decode(&v); return v, nil
}

// Projects
func (c *Client) CreateProject(ctx context.Context, p types.Project) (types.Project, error) {
    req, _ := c.req(ctx, http.MethodPost, "/api/v1/projects", p)
    resp, err := c.http.Do(req); if err != nil { return types.Project{}, err }
    defer resp.Body.Close()
    var v types.Project; _ = json.NewDecoder(resp.Body).Decode(&v); return v, nil
}
func (c *Client) ListProjects(ctx context.Context, tenant string) ([]types.Project, error) {
    req, _ := c.req(ctx, http.MethodGet, "/api/v1/tenants/"+tenant+"/projects", nil)
    resp, err := c.http.Do(req); if err != nil { return nil, err }
    defer resp.Body.Close()
    var v []types.Project; _ = json.NewDecoder(resp.Body).Decode(&v); return v, nil
}

// Apps
func (c *Client) CreateApp(ctx context.Context, a types.App) (types.App, error) {
    req, _ := c.req(ctx, http.MethodPost, "/api/v1/apps", a)
    resp, err := c.http.Do(req); if err != nil { return types.App{}, err }
    defer resp.Body.Close()
    var v types.App; _ = json.NewDecoder(resp.Body).Decode(&v); return v, nil
}
func (c *Client) ListApps(ctx context.Context, tenant, project string) ([]types.App, error) {
    req, _ := c.req(ctx, http.MethodGet, "/api/v1/projects/"+tenant+"/"+project+"/apps", nil)
    resp, err := c.http.Do(req); if err != nil { return nil, err }
    defer resp.Body.Close()
    var v []types.App; _ = json.NewDecoder(resp.Body).Decode(&v); return v, nil
}

func (c *Client) IssueKubeconfig(ctx context.Context, g types.KubeconfigGrant) (types.KubeconfigGrant, error) {
    req, _ := c.req(ctx, http.MethodPost, "/api/v1/kubeconfig-grants", g)
    resp, err := c.http.Do(req); if err != nil { return types.KubeconfigGrant{}, err }
    defer resp.Body.Close()
    var v types.KubeconfigGrant; _ = json.NewDecoder(resp.Body).Decode(&v); return v, nil
}

