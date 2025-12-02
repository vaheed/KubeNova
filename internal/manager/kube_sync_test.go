package manager

import (
	"context"
	"testing"

	v1alpha1 "github.com/vaheed/kubenova/pkg/api/v1alpha1"
	"github.com/vaheed/kubenova/pkg/types"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestUpsertNovaTenantCreatesAndUpdates(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)
	cli := fake.NewClientBuilder().WithScheme(scheme).Build()

	ctx := context.Background()
	tenant := &types.Tenant{
		Name:            "acme",
		Owners:          []string{"alice"},
		Plan:            "gold",
		Labels:          map[string]string{"tier": "gold"},
		OwnerNamespace:  "acme-owner",
		AppsNamespace:   "acme-apps",
		NetworkPolicies: []string{"default-deny"},
		Quotas:          map[string]string{"cpu": "2"},
		Limits:          map[string]string{"memory": "1Gi"},
	}
	if err := upsertNovaTenant(ctx, cli, tenant); err != nil {
		t.Fatalf("create tenant CR: %v", err)
	}

	var created v1alpha1.NovaTenant
	if err := cli.Get(ctx, ctrlclient.ObjectKey{Name: tenant.Name}, &created); err != nil {
		t.Fatalf("get created tenant: %v", err)
	}
	if created.Spec.OwnerNamespace != tenant.OwnerNamespace || created.Spec.AppsNamespace != tenant.AppsNamespace {
		t.Fatalf("namespaces not propagated: %#v", created.Spec)
	}
	if created.Spec.Plan != "gold" || created.Labels["tier"] != "gold" {
		t.Fatalf("unexpected spec/labels: %+v labels=%+v", created.Spec, created.Labels)
	}

	tenant.Plan = "platinum"
	tenant.Labels = map[string]string{"tier": "platinum"}
	tenant.Owners = []string{"bob"}
	if err := upsertNovaTenant(ctx, cli, tenant); err != nil {
		t.Fatalf("update tenant CR: %v", err)
	}

	var updated v1alpha1.NovaTenant
	if err := cli.Get(ctx, ctrlclient.ObjectKey{Name: tenant.Name}, &updated); err != nil {
		t.Fatalf("get updated tenant: %v", err)
	}
	if updated.Spec.Plan != "platinum" || updated.Labels["tier"] != "platinum" {
		t.Fatalf("updates not applied: %+v labels=%+v", updated.Spec, updated.Labels)
	}
	if len(updated.Spec.Owners) != 1 || updated.Spec.Owners[0] != "bob" {
		t.Fatalf("owners not updated: %+v", updated.Spec.Owners)
	}
}

func TestUpsertNovaProjectCreatesAndUpdates(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)
	cli := fake.NewClientBuilder().WithScheme(scheme).Build()

	ctx := context.Background()
	project := &types.Project{
		Name:        "payments",
		Description: "initial",
		Labels:      map[string]string{"team": "core"},
		Access:      []string{"devs"},
	}

	if err := upsertNovaProject(ctx, cli, "acme", project); err != nil {
		t.Fatalf("create project CR: %v", err)
	}

	var created v1alpha1.NovaProject
	if err := cli.Get(ctx, ctrlclient.ObjectKey{Name: project.Name}, &created); err != nil {
		t.Fatalf("get created project: %v", err)
	}
	if created.Spec.Tenant != "acme" || created.Spec.Description != "initial" {
		t.Fatalf("unexpected project spec: %+v", created.Spec)
	}
	if created.Labels["team"] != "core" {
		t.Fatalf("labels not applied: %+v", created.Labels)
	}

	project.Description = "updated"
	project.Labels = map[string]string{"team": "platform"}
	project.Access = []string{"ops"}
	if err := upsertNovaProject(ctx, cli, "acme", project); err != nil {
		t.Fatalf("update project CR: %v", err)
	}

	var updated v1alpha1.NovaProject
	if err := cli.Get(ctx, ctrlclient.ObjectKey{Name: project.Name}, &updated); err != nil {
		t.Fatalf("get updated project: %v", err)
	}
	if updated.Spec.Description != "updated" || updated.Labels["team"] != "platform" {
		t.Fatalf("project updates not applied: %+v labels=%+v", updated.Spec, updated.Labels)
	}
	if len(updated.Spec.Access) != 1 || updated.Spec.Access[0] != "ops" {
		t.Fatalf("access list not updated: %+v", updated.Spec.Access)
	}
}

func TestUpsertNovaAppCreatesAndUpdates(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)
	cli := fake.NewClientBuilder().WithScheme(scheme).Build()

	tenant := &types.Tenant{Name: "acme", AppsNamespace: "acme-apps"}
	project := &types.Project{Name: "payments"}
	app := &types.App{
		Name:        "api",
		Description: "api svc",
		Component:   "web",
		Image:       "ghcr.io/example/api:dev",
		Spec: map[string]any{
			"type": "webservice",
		},
		Traits:   []map[string]any{{"type": "scaler", "properties": map[string]any{"min": 1}}},
		Policies: []map[string]any{{"type": "autoscale"}},
	}

	if err := upsertNovaApp(context.Background(), cli, tenant, project, app); err != nil {
		t.Fatalf("create novaapp: %v", err)
	}

	var created v1alpha1.NovaApp
	if err := cli.Get(context.Background(), ctrlclient.ObjectKey{Name: app.Name, Namespace: "acme-apps"}, &created); err != nil {
		t.Fatalf("get novaapp: %v", err)
	}
	if created.Spec.Tenant != "acme" || created.Spec.Project != "payments" {
		t.Fatalf("unexpected spec: %+v", created.Spec)
	}
	if created.Spec.Namespace != "acme-apps" {
		t.Fatalf("namespace not set: %s", created.Spec.Namespace)
	}

	app.Description = "v2"
	app.Image = "ghcr.io/example/api:v2"
	if err := upsertNovaApp(context.Background(), cli, tenant, project, app); err != nil {
		t.Fatalf("update novaapp: %v", err)
	}
	var updated v1alpha1.NovaApp
	if err := cli.Get(context.Background(), ctrlclient.ObjectKey{Name: app.Name, Namespace: "acme-apps"}, &updated); err != nil {
		t.Fatalf("get updated novaapp: %v", err)
	}
	if updated.Spec.Description != "v2" || updated.Spec.Image != "ghcr.io/example/api:v2" {
		t.Fatalf("fields not updated: %+v", updated.Spec)
	}
}
