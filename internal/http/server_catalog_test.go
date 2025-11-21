package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/vaheed/kubenova/internal/store"
	"github.com/vaheed/kubenova/pkg/types"
)

func TestCatalogEndpoints(t *testing.T) {
	st := store.NewMemory()
	api := NewAPIServer(st)
	r := chi.NewRouter()
	_ = HandlerWithOptions(api, ChiServerOptions{BaseRouter: r})
	ts := httptest.NewServer(r)
	defer ts.Close()

	cases := []struct {
		path   string
		expect string
	}{
		{"/api/v1/catalog/components", "web"},
		{"/api/v1/catalog/traits", "scaler"},
		{"/api/v1/catalog/workflows", "rollout"},
	}

	for _, c := range cases {
		resp, err := http.Get(ts.URL + c.path)
		if err != nil {
			t.Fatalf("GET %s: %v", c.path, err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status for %s: %s", c.path, resp.Status)
		}
		var arr []map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&arr)
		resp.Body.Close()
		if len(arr) == 0 {
			t.Fatalf("empty catalog for %s", c.path)
		}
		if arr[0]["name"].(string) != c.expect {
			t.Fatalf("unexpected first item name for %s: %v", c.path, arr[0])
		}
	}
}

func TestCatalogInstall(t *testing.T) {
	st := store.NewMemory()
	api := NewAPIServer(st)
	r := chi.NewRouter()
	_ = HandlerWithOptions(api, ChiServerOptions{BaseRouter: r})
	ts := httptest.NewServer(r)
	defer ts.Close()

	ctx := context.Background()
	if err := st.CreateTenant(ctx, types.Tenant{Name: "acme"}); err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	tenant, err := st.GetTenant(ctx, "acme")
	if err != nil {
		t.Fatalf("lookup tenant: %v", err)
	}
	if err := st.CreateProject(ctx, types.Project{Tenant: tenant.Name, Name: "web"}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	project, err := st.GetProject(ctx, tenant.Name, "web")
	if err != nil {
		t.Fatalf("lookup project: %v", err)
	}

	source := map[string]any{
		"kind": "containerImage",
		"containerImage": map[string]any{
			"image": "nginx",
			"tag":   "1.21.0",
		},
	}
	version := "1.21.0"
	if err := st.CreateCatalogItem(ctx, types.CatalogItem{
		Slug:        "nginx",
		Name:        "Nginx",
		Scope:       "global",
		Version:     &version,
		Source:      source,
		Description: ptr("web server"),
	}); err != nil {
		t.Fatalf("create catalog item: %v", err)
	}

	overrides := map[string]any{
		"containerImage": map[string]any{"tag": "1.22.0"},
	}
	payload := CatalogInstall{Slug: "nginx", Source: &overrides}
	body, _ := json.Marshal(payload)
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/clusters/"+testClusterID+"/tenants/"+tenant.ID.String()+"/projects/"+project.ID.String()+"/catalog/install", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("install request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("unexpected status: %s", resp.Status)
	}
	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode install response: %v", err)
	}
	if result["status"] != "accepted" || result["appSlug"] != "nginx" {
		t.Fatalf("unexpected install body: %#v", result)
	}

	apps, err := st.ListApps(ctx, tenant.Name, project.Name)
	if err != nil {
		t.Fatalf("list apps: %v", err)
	}
	if len(apps) != 1 {
		t.Fatalf("expected 1 app, got %d", len(apps))
	}
	app := apps[0]
	if app.Name != "nginx" {
		t.Fatalf("app name mismatch: %s", app.Name)
	}
	if app.Spec == nil || app.Spec.Source == nil {
		t.Fatal("app source missing")
	}
	if app.Spec.Source.Kind != types.AppSourceKindContainerImage {
		t.Fatalf("unexpected source kind %s", app.Spec.Source.Kind)
	}
	if app.Spec.Source.CatalogRef == nil || app.Spec.Source.CatalogRef.Name != "nginx" {
		t.Fatalf("catalog ref missing or wrong")
	}
	if app.Spec.Source.CatalogRef.Version == nil || *app.Spec.Source.CatalogRef.Version != version {
		t.Fatalf("catalog version mismatch: %v", app.Spec.Source.CatalogRef.Version)
	}
	if app.Spec.Source.ContainerImage == nil || app.Spec.Source.ContainerImage.Tag == nil || *app.Spec.Source.ContainerImage.Tag != "1.22.0" {
		t.Fatalf("override not applied")
	}
}

func ptr(v string) *string { return &v }
