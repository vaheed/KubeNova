package httpapi

import (
	"bytes"
	"context"
	"encoding/base64"
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

	// Register a cluster to provide kubeconfig to backend
	kcfg := base64.StdEncoding.EncodeToString([]byte("apiVersion: v1\nclusters: []\ncontexts: []\n"))
	_, _ = http.Post(ts.URL+"/api/v1/clusters", "application/json",
		bytes.NewReader([]byte(`{"name":"c","kubeconfig":"`+kcfg+`"}`)))

	resp, err := http.Post(ts.URL+"/api/v1/clusters/c/tenants/t/projects/p/apps/a:delete", "application/json", nil)
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
