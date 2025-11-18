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

type fakeVelaDelete struct{ called bool }

func (f *fakeVelaDelete) EnsureApp(context.Context, string, string, map[string]any) error { return nil }
func (f *fakeVelaDelete) DeleteApp(context.Context, string, string) error {
	f.called = true
	return nil
}
func (f *fakeVelaDelete) GetApp(context.Context, string, string) (map[string]any, error) {
	return nil, nil
}
func (f *fakeVelaDelete) ListApps(context.Context, string, int, string) ([]map[string]any, string, error) {
	return nil, "", nil
}
func (f *fakeVelaDelete) Deploy(context.Context, string, string) error         { return nil }
func (f *fakeVelaDelete) Suspend(context.Context, string, string) error        { return nil }
func (f *fakeVelaDelete) Resume(context.Context, string, string) error         { return nil }
func (f *fakeVelaDelete) Rollback(context.Context, string, string, *int) error { return nil }
func (f *fakeVelaDelete) Status(context.Context, string, string) (map[string]any, error) {
	return nil, nil
}
func (f *fakeVelaDelete) Revisions(context.Context, string, string) ([]map[string]any, error) {
	return nil, nil
}
func (f *fakeVelaDelete) Diff(context.Context, string, string, int, int) (map[string]any, error) {
	return nil, nil
}
func (f *fakeVelaDelete) Logs(context.Context, string, string, string, bool) ([]map[string]any, error) {
	return nil, nil
}
func (f *fakeVelaDelete) SetTraits(context.Context, string, string, []map[string]any) error {
	return nil
}
func (f *fakeVelaDelete) SetPolicies(context.Context, string, string, []map[string]any) error {
	return nil
}
func (f *fakeVelaDelete) ImageUpdate(context.Context, string, string, string, string, string) error {
	return nil
}

func TestAppDeleteActionInvokesBackend(t *testing.T) {
	st := store.NewMemory()
	api := NewAPIServer(st)
	fv := &fakeVelaDelete{}
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

	// set up minimal objects to get UIDs
	kcfg := base64.StdEncoding.EncodeToString([]byte("apiVersion: v1\nclusters: []\ncontexts: []\n"))
	reqBody := []byte(`{"name":"c","kubeconfig":"` + kcfg + `","capsuleProxyUrl":"https://capsule-proxy.example.com:9001"}`)
	resp2, _ := http.Post(ts.URL+"/api/v1/clusters", "application/json", bytes.NewReader(reqBody))
	var c Cluster
	_ = json.NewDecoder(resp2.Body).Decode(&c)
	resp2.Body.Close()
	tb, _ := json.Marshal(Tenant{Name: "t"})
	rq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/clusters/"+*c.Uid+"/tenants", bytes.NewReader(tb))
	rq.Header.Set("Content-Type", "application/json")
	rr, _ := http.DefaultClient.Do(rq)
	var tnt Tenant
	_ = json.NewDecoder(rr.Body).Decode(&tnt)
	rr.Body.Close()
	pb, _ := json.Marshal(Project{Name: "p"})
	rq, _ = http.NewRequest(http.MethodPost, ts.URL+"/api/v1/clusters/"+*c.Uid+"/tenants/"+*tnt.Uid+"/projects", bytes.NewReader(pb))
	rq.Header.Set("Content-Type", "application/json")
	rr, _ = http.DefaultClient.Do(rq)
	var pr Project
	_ = json.NewDecoder(rr.Body).Decode(&pr)
	rr.Body.Close()
	ab, _ := json.Marshal(App{Name: "a"})
	rq, _ = http.NewRequest(http.MethodPost, ts.URL+"/api/v1/clusters/"+*c.Uid+"/tenants/"+*tnt.Uid+"/projects/"+*pr.Uid+"/apps", bytes.NewReader(ab))
	rq.Header.Set("Content-Type", "application/json")
	rr, _ = http.DefaultClient.Do(rq)
	var ap App
	_ = json.NewDecoder(rr.Body).Decode(&ap)
	rr.Body.Close()
	resp, err := http.Post(ts.URL+"/api/v1/clusters/"+*c.Uid+"/tenants/"+*tnt.Uid+"/projects/"+*pr.Uid+"/apps/"+*ap.Uid+":delete", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("status: %s", resp.Status)
	}
	resp.Body.Close()
	if !fv.called {
		t.Fatalf("backend delete not called")
	}
}
