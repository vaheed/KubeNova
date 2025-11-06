package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/vaheed/kubenova/internal/store"
)

func TestProjectsAndAppsLifecycle(t *testing.T) {
	st := store.NewMemory()
	api := NewAPIServer(st)
	r := chi.NewRouter()
	_ = HandlerWithOptions(api, ChiServerOptions{BaseRouter: r})
	ts := httptest.NewServer(r)
	defer ts.Close()

	// create tenant
	tb, _ := json.Marshal(Tenant{Name: "acme"})
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/clusters/c/tenants", bytes.NewReader(tb))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create tenant: %s", resp.Status)
	}
	resp.Body.Close()

	// create project
	pb, _ := json.Marshal(Project{Name: "web"})
	req, _ = http.NewRequest(http.MethodPost, ts.URL+"/api/v1/clusters/c/tenants/acme/projects", bytes.NewReader(pb))
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create project: %s", resp.Status)
	}
	resp.Body.Close()

	// list projects
	resp, err = http.Get(ts.URL + "/api/v1/clusters/c/tenants/acme/projects")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list projects: %s", resp.Status)
	}
	var projects []Project
	_ = json.NewDecoder(resp.Body).Decode(&projects)
	resp.Body.Close()
	if len(projects) != 1 || projects[0].Name != "web" {
		t.Fatalf("unexpected projects: %+v", projects)
	}

	// create app
	ab, _ := json.Marshal(App{Name: "hello"})
	req, _ = http.NewRequest(http.MethodPost, ts.URL+"/api/v1/clusters/c/tenants/acme/projects/web/apps", bytes.NewReader(ab))
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create app: %s", resp.Status)
	}
	resp.Body.Close()

	// list apps
	resp, err = http.Get(ts.URL + "/api/v1/clusters/c/tenants/acme/projects/web/apps")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list apps: %s", resp.Status)
	}
	var apps []App
	_ = json.NewDecoder(resp.Body).Decode(&apps)
	resp.Body.Close()
	if len(apps) != 1 || apps[0].Name != "hello" {
		t.Fatalf("unexpected apps: %+v", apps)
	}

	// get app
	resp, err = http.Get(ts.URL + "/api/v1/clusters/c/tenants/acme/projects/web/apps/hello")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get app: %s", resp.Status)
	}
	resp.Body.Close()

	// delete app
	req, _ = http.NewRequest(http.MethodDelete, ts.URL+"/api/v1/clusters/c/tenants/acme/projects/web/apps/hello", nil)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("delete app: %s", resp.Status)
	}
	resp.Body.Close()
}
