package httpapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/vaheed/kubenova/internal/cluster"
	"github.com/vaheed/kubenova/internal/store"
	"github.com/vaheed/kubenova/pkg/types"
)

func TestGetClusterAppOrphans(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemory()
	api := NewAPIServer(st)
	// Make sure we can override Vela client with deterministic data.
	fake := &fakeVelaOrphans{}
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
		ListApps(context.Context, string, int, string) ([]map[string]any, string, error)
	} {
		return fake
	}

	clusterID, err := st.CreateCluster(ctx, types.Cluster{Name: "kind"}, base64.StdEncoding.EncodeToString([]byte("apiVersion: v1\n")))
	if err != nil {
		t.Fatalf("create cluster: %v", err)
	}
	tenant := types.Tenant{Name: "acme"}
	if err := st.CreateTenant(ctx, tenant); err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	ten, err := st.GetTenant(ctx, "acme")
	if err != nil {
		t.Fatalf("lookup tenant: %v", err)
	}
	project := types.Project{Tenant: ten.Name, Name: "shop"}
	if err := st.CreateProject(ctx, project); err != nil {
		t.Fatalf("create project: %v", err)
	}
	pr, err := st.GetProject(ctx, ten.Name, project.Name)
	if err != nil {
		t.Fatalf("lookup project: %v", err)
	}
	appID := types.NewID()
	if err := st.CreateApp(ctx, types.App{ID: appID, Tenant: ten.Name, Project: pr.Name, Name: "managed"}); err != nil {
		t.Fatalf("create app: %v", err)
	}

	fake.apps = []map[string]any{
		{
			"metadata": map[string]any{
				"name":      "managed",
				"namespace": cluster.AppNamespaceName(ten.Name, pr.Name),
				"labels": map[string]any{
					"kubenova.app":       "managed",
					"kubenova.tenant":    ten.Name,
					"kubenova.project":   pr.Name,
					"kubenova.io/app-id": appID.String(),
				},
			},
		},
		{
			"metadata": map[string]any{
				"name":      "missing-record",
				"namespace": cluster.AppNamespaceName(ten.Name, pr.Name),
				"labels": map[string]any{
					"kubenova.app":       "missing",
					"kubenova.tenant":    ten.Name,
					"kubenova.project":   pr.Name,
					"kubenova.io/app-id": types.NewID().String(),
				},
			},
		},
		{
			"metadata": map[string]any{
				"name":      "no-label",
				"namespace": cluster.AppNamespaceName(ten.Name, pr.Name),
				"labels": map[string]any{
					"kubenova.app":     "nolabel",
					"kubenova.tenant":  ten.Name,
					"kubenova.project": pr.Name,
				},
			},
		},
	}

	r := chi.NewRouter()
	_ = HandlerWithOptions(api, ChiServerOptions{BaseRouter: r})
	ts := httptest.NewServer(r)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/clusters/" + clusterID.String() + "/apps/orphans")
	if err != nil {
		t.Fatalf("request orphans: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %s", resp.Status)
	}
	var body OrphanedApplications
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Orphans == nil || len(*body.Orphans) != 2 {
		t.Fatalf("expected 2 orphans, got %d", len(*body.Orphans))
	}
	foundMissing := false
	foundLabel := false
	for _, orphan := range *body.Orphans {
		switch orphan.Name {
		case "missing-record":
			if orphan.Reason != "app record missing" {
				t.Fatalf("missing-record reason: %s", orphan.Reason)
			}
			if orphan.AppId == nil || orphan.AppId.String() == "" {
				t.Fatalf("expected appId for missing-record")
			}
			foundMissing = true
		case "no-label":
			if orphan.Reason != "missing kubenova.io/app-id label" {
				t.Fatalf("no-label reason: %s", orphan.Reason)
			}
			if orphan.AppId != nil {
				t.Fatalf("expected no appId for label-less orphan")
			}
			foundLabel = true
		default:
			t.Fatalf("unexpected orphan: %s", orphan.Name)
		}
	}
	if !foundMissing || !foundLabel {
		t.Fatalf("missing expected orphan categories")
	}
}

type fakeVelaOrphans struct {
	fakeVela
	apps []map[string]any
}

func (f *fakeVelaOrphans) ListApps(_ context.Context, _ string, _ int, _ string) ([]map[string]any, string, error) {
	return f.apps, "", nil
}
