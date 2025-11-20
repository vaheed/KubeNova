package httpapi

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/vaheed/kubenova/internal/store"
)

type fakeVela struct {
	deploy, suspend, resume, rollback, status, revisions, diff, logs bool
	traits, policies, image                                          bool
}

func (f *fakeVela) EnsureApp(_ context.Context, _, _ string, _ map[string]any) error { return nil }
func (f *fakeVela) DeleteApp(_ context.Context, _, _ string) error                   { return nil }
func (f *fakeVela) GetApp(_ context.Context, _, _ string) (map[string]any, error)    { return nil, nil }
func (f *fakeVela) ListApps(_ context.Context, _ string, _ int, _ string) ([]map[string]any, string, error) {
	return nil, "", nil
}
func (f *fakeVela) Deploy(_ context.Context, ns, name string) error  { f.deploy = true; return nil }
func (f *fakeVela) Suspend(_ context.Context, ns, name string) error { f.suspend = true; return nil }
func (f *fakeVela) Resume(_ context.Context, ns, name string) error  { f.resume = true; return nil }
func (f *fakeVela) Rollback(_ context.Context, ns, name string, _ *int) error {
	f.rollback = true
	return nil
}
func (f *fakeVela) Status(_ context.Context, ns, name string) (map[string]any, error) {
	f.status = true
	return map[string]any{"phase": "Running"}, nil
}
func (f *fakeVela) Revisions(_ context.Context, ns, name string) ([]map[string]any, error) {
	f.revisions = true
	return []map[string]any{}, nil
}
func (f *fakeVela) Diff(_ context.Context, ns, name string, _, _ int) (map[string]any, error) {
	f.diff = true
	return map[string]any{"changes": []any{}}, nil
}
func (f *fakeVela) Logs(_ context.Context, ns, name, component string, follow bool) ([]map[string]any, error) {
	f.logs = true
	return []map[string]any{}, nil
}
func (f *fakeVela) SetTraits(_ context.Context, _, _ string, _ []map[string]any) error {
	f.traits = true
	return nil
}
func (f *fakeVela) SetPolicies(_ context.Context, _, _ string, _ []map[string]any) error {
	f.policies = true
	return nil
}
func (f *fakeVela) ImageUpdate(_ context.Context, _, _, _, _, _ string) error {
	f.image = true
	return nil
}

func TestAppsOpsInvokeBackend(t *testing.T) {
	st := store.NewMemory()
	api := NewAPIServer(st)
	fv := &fakeVela{}
	api.newVela = func([]byte) interface {
		Deploy(context.Context, string, string) error
		Suspend(context.Context, string, string) error
		Resume(context.Context, string, string) error
		Rollback(context.Context, string, string, *int) error
		Status(context.Context, string, string) (map[string]any, error)
		Revisions(context.Context, string, string) ([]map[string]any, error)
		Diff(context.Context, string, string, int, int) (map[string]any, error)
		Logs(context.Context, string, string, string, bool) ([]map[string]any, error)
		SetTraits(context.Context, string, string, []map[string]any) error
		SetPolicies(context.Context, string, string, []map[string]any) error
		ImageUpdate(context.Context, string, string, string, string, string) error
		DeleteApp(context.Context, string, string) error
	} {
		return fv
	}

	r := chi.NewRouter()
	_ = HandlerWithOptions(api, ChiServerOptions{BaseRouter: r})
	ts := httptest.NewServer(r)
	defer ts.Close()

	// Register a cluster
	kcfg := base64.StdEncoding.EncodeToString([]byte("apiVersion: v1\nclusters: []\ncontexts: []\n"))
	reqBody := []byte(`{"name":"c","kubeconfig":"` + kcfg + `","capsuleProxyUrl":"https://capsule-proxy.example.com:9001"}`)
	resp, err := http.Post(ts.URL+"/api/v1/clusters", "application/json", bytes.NewReader(reqBody))
	if err != nil {
		t.Fatal(err)
	}
	var c Cluster
	_ = json.NewDecoder(resp.Body).Decode(&c)
	resp.Body.Close()
	if c.Uid == nil {
		t.Fatalf("cluster uid missing")
	}

	// Create tenant
	tb, _ := json.Marshal(Tenant{Name: "t"})
	rq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/clusters/"+uidStr(c.Uid)+"/tenants", bytes.NewReader(tb))
	rq.Header.Set("Content-Type", "application/json")
	rr, _ := http.DefaultClient.Do(rq)
	var tnt Tenant
	_ = json.NewDecoder(rr.Body).Decode(&tnt)
	rr.Body.Close()
	if tnt.Uid == nil {
		t.Fatalf("tenant uid missing")
	}

	// Create project
	pb, _ := json.Marshal(Project{Name: "p"})
	rq, _ = http.NewRequest(http.MethodPost, ts.URL+"/api/v1/clusters/"+uidStr(c.Uid)+"/tenants/"+uidStr(tnt.Uid)+"/projects", bytes.NewReader(pb))
	rq.Header.Set("Content-Type", "application/json")
	rr, _ = http.DefaultClient.Do(rq)
	var pr Project
	_ = json.NewDecoder(rr.Body).Decode(&pr)
	rr.Body.Close()
	if pr.Uid == nil {
		t.Fatalf("project uid missing")
	}

	// Create app
	ab, _ := json.Marshal(App{Name: "a"})
	rq, _ = http.NewRequest(http.MethodPost, ts.URL+"/api/v1/clusters/"+uidStr(c.Uid)+"/tenants/"+uidStr(tnt.Uid)+"/projects/"+uidStr(pr.Uid)+"/apps", bytes.NewReader(ab))
	rq.Header.Set("Content-Type", "application/json")
	rr, _ = http.DefaultClient.Do(rq)
	var ap App
	_ = json.NewDecoder(rr.Body).Decode(&ap)
	rr.Body.Close()
	if ap.Uid == nil {
		t.Fatalf("app uid missing")
	}

	// Operations
	op := func(method, path string, body []byte) {
		req, _ := http.NewRequest(method, ts.URL+path, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer test")
		req.Header.Set("X-KN-Roles", "projectDev")
		req.Header.Set("X-KN-Tenant", "t")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode >= 400 {
			t.Fatalf("%s %s: %s", method, path, resp.Status)
		}
		resp.Body.Close()
	}
	op(http.MethodPost, "/api/v1/clusters/"+uidStr(c.Uid)+"/tenants/"+uidStr(tnt.Uid)+"/projects/"+uidStr(pr.Uid)+"/apps/"+uidStr(ap.Uid)+":deploy", nil)
	op(http.MethodPost, "/api/v1/clusters/"+uidStr(c.Uid)+"/tenants/"+uidStr(tnt.Uid)+"/projects/"+uidStr(pr.Uid)+"/apps/"+uidStr(ap.Uid)+":suspend", nil)
	op(http.MethodPost, "/api/v1/clusters/"+uidStr(c.Uid)+"/tenants/"+uidStr(tnt.Uid)+"/projects/"+uidStr(pr.Uid)+"/apps/"+uidStr(ap.Uid)+":resume", nil)
	op(http.MethodPost, "/api/v1/clusters/"+uidStr(c.Uid)+"/tenants/"+uidStr(tnt.Uid)+"/projects/"+uidStr(pr.Uid)+"/apps/"+uidStr(ap.Uid)+":rollback", []byte(`{"toRevision":1}`))
	resp, err = http.Get(ts.URL + "/api/v1/clusters/" + uidStr(c.Uid) + "/tenants/" + uidStr(tnt.Uid) + "/projects/" + uidStr(pr.Uid) + "/apps/" + uidStr(ap.Uid) + "/status")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: %s", resp.Status)
	}
	resp.Body.Close()
	resp, err = http.Get(ts.URL + "/api/v1/clusters/" + uidStr(c.Uid) + "/tenants/" + uidStr(tnt.Uid) + "/projects/" + uidStr(pr.Uid) + "/apps/" + uidStr(ap.Uid) + "/revisions")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("revisions: %s", resp.Status)
	}
	resp.Body.Close()
	resp, err = http.Get(ts.URL + "/api/v1/clusters/" + uidStr(c.Uid) + "/tenants/" + uidStr(tnt.Uid) + "/projects/" + uidStr(pr.Uid) + "/apps/" + uidStr(ap.Uid) + "/diff/1/2")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("diff: %s", resp.Status)
	}
	resp.Body.Close()
	resp, err = http.Get(ts.URL + "/api/v1/clusters/" + uidStr(c.Uid) + "/tenants/" + uidStr(tnt.Uid) + "/projects/" + uidStr(pr.Uid) + "/apps/" + uidStr(ap.Uid) + "/logs/web")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("logs: %s", resp.Status)
	}
	resp.Body.Close()

	// traits
	rq, _ = http.NewRequest(http.MethodPut, ts.URL+"/api/v1/clusters/"+uidStr(c.Uid)+"/tenants/"+uidStr(tnt.Uid)+"/projects/"+uidStr(pr.Uid)+"/apps/"+uidStr(ap.Uid)+"/traits", bytes.NewReader([]byte(`[]`)))
	rq.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(rq)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("traits: %s", resp.Status)
	}
	resp.Body.Close()
	// policies
	rq, _ = http.NewRequest(http.MethodPut, ts.URL+"/api/v1/clusters/"+uidStr(c.Uid)+"/tenants/"+uidStr(tnt.Uid)+"/projects/"+uidStr(pr.Uid)+"/apps/"+uidStr(ap.Uid)+"/policies", bytes.NewReader([]byte(`[]`)))
	rq.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(rq)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("policies: %s", resp.Status)
	}
	resp.Body.Close()
	// image update
	rq, _ = http.NewRequest(http.MethodPost, ts.URL+"/api/v1/clusters/"+uidStr(c.Uid)+"/tenants/"+uidStr(tnt.Uid)+"/projects/"+uidStr(pr.Uid)+"/apps/"+uidStr(ap.Uid)+"/image-update", bytes.NewReader([]byte(`{"component":"web","image":"busybox","tag":"latest"}`)))
	rq.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(rq)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("image update: %s", resp.Status)
	}
	resp.Body.Close()

	if !fv.deploy || !fv.suspend || !fv.resume || !fv.rollback || !fv.status || !fv.revisions || !fv.diff || !fv.logs || !fv.traits || !fv.policies || !fv.image {
		t.Fatalf("backend not invoked: %+v", fv)
	}
}
