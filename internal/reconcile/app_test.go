package reconcile

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/vaheed/kubenova/internal/backends/vela"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type fakeVelaClient struct {
	ensured  bool
	traits   []map[string]any
	policies []map[string]any
	ns       string
	name     string
	spec     map[string]any
	meta     map[string]string
}

func (f *fakeVelaClient) EnsureApp(_ context.Context, ns, name string, spec map[string]any, meta map[string]string) error {
	f.ensured = true
	f.ns = ns
	f.name = name
	f.spec = spec
	f.meta = meta
	return nil
}
func (f *fakeVelaClient) DeleteApp(context.Context, string, string) error { return nil }
func (f *fakeVelaClient) GetApp(context.Context, string, string) (map[string]any, error) {
	return nil, nil
}
func (f *fakeVelaClient) ListApps(context.Context, string, int, string) ([]map[string]any, string, error) {
	return nil, "", nil
}
func (f *fakeVelaClient) Deploy(context.Context, string, string) error         { return nil }
func (f *fakeVelaClient) Suspend(context.Context, string, string) error        { return nil }
func (f *fakeVelaClient) Resume(context.Context, string, string) error         { return nil }
func (f *fakeVelaClient) Rollback(context.Context, string, string, *int) error { return nil }
func (f *fakeVelaClient) Status(context.Context, string, string) (map[string]any, error) {
	return nil, nil
}
func (f *fakeVelaClient) Revisions(context.Context, string, string) ([]map[string]any, error) {
	return nil, nil
}
func (f *fakeVelaClient) Diff(context.Context, string, string, int, int) (map[string]any, error) {
	return nil, nil
}
func (f *fakeVelaClient) Logs(context.Context, string, string, string, bool) ([]map[string]any, error) {
	return nil, nil
}
func (f *fakeVelaClient) SetTraits(_ context.Context, _, _ string, traits []map[string]any) error {
	f.traits = traits
	return nil
}
func (f *fakeVelaClient) SetPolicies(_ context.Context, _, _ string, policies []map[string]any) error {
	f.policies = policies
	return nil
}
func (f *fakeVelaClient) ImageUpdate(context.Context, string, string, string, string, string) error {
	return nil
}

var _ vela.Client = (*fakeVelaClient)(nil)

func TestAppReconcilerProjectsConfigMapToVela(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	c := fake.NewClientBuilder().WithScheme(scheme).Build()

	fv := &fakeVelaClient{}
	r := &AppReconciler{
		Client:  c,
		newVela: func() vela.Client { return fv },
	}

	spec := map[string]any{
		"components": []any{
			map[string]any{"name": "web", "type": "webservice"},
		},
		"source": map[string]any{
			"kind": "containerImage",
		},
	}
	specRaw, _ := json.Marshal(spec)
	traitsRaw, _ := json.Marshal([]map[string]any{{"type": "scaler"}})
	policiesRaw, _ := json.Marshal([]map[string]any{{"type": "rollout"}})

	cm := &corev1.ConfigMap{}
	cm.Name = "app-spec"
	cm.Namespace = "proj"
	cm.Labels = map[string]string{
		"kubenova.app":            "demo",
		"kubenova.tenant":         "acme",
		"kubenova.project":        "proj",
		"kubenova.io/app-id":      "app-123",
		"kubenova.io/tenant-id":   "tenant-123",
		"kubenova.io/project-id":  "project-123",
		"kubenova.io/source-kind": "containerImage",
	}
	cm.Data = map[string]string{
		"spec":     string(specRaw),
		"traits":   string(traitsRaw),
		"policies": string(policiesRaw),
	}
	if err := c.Create(context.Background(), cm); err != nil {
		t.Fatal(err)
	}

	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Namespace: cm.Namespace, Name: cm.Name}}); err != nil {
		t.Fatal(err)
	}
	if !fv.ensured {
		t.Fatalf("expected EnsureApp to be called")
	}
	if fv.ns != "proj" || fv.name != "demo" {
		t.Fatalf("unexpected ns/name: %s/%s", fv.ns, fv.name)
	}
	if len(fv.traits) != 1 || len(fv.policies) != 1 {
		t.Fatalf("expected traits and policies to be applied, got: %+v %+v", fv.traits, fv.policies)
	}
	expectedMeta := map[string]string{
		"kubenova.app":            "demo",
		"kubenova.tenant":         "acme",
		"kubenova.project":        "proj",
		"kubenova.io/app-id":      "app-123",
		"kubenova.io/tenant-id":   "tenant-123",
		"kubenova.io/project-id":  "project-123",
		"kubenova.io/source-kind": "containerImage",
	}
	for key, want := range expectedMeta {
		if fv.meta == nil || fv.meta[key] != want {
			t.Fatalf("metadata %s mismatch: expected %s got %v", key, want, fv.meta[key])
		}
	}
}

func TestAppReconcilerInjectsSecretRefs(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	c := fake.NewClientBuilder().WithScheme(scheme).Build()

	fv := &fakeVelaClient{}
	r := &AppReconciler{
		Client:  c,
		newVela: func() vela.Client { return fv },
	}

	spec := map[string]any{
		"components": []any{
			map[string]any{
				"name": "web",
				"type": "webservice",
				"properties": map[string]any{
					"image": "nginx",
				},
			},
			map[string]any{
				"name": "helm",
				"type": "helm",
				"properties": map[string]any{
					"chart": "demo",
				},
			},
			map[string]any{
				"name": "git",
				"type": "git",
				"properties": map[string]any{
					"repo": "https://example.com/repo",
				},
			},
		},
		"source": map[string]any{
			"kind": "containerImage",
			"containerImage": map[string]any{
				"credentialsSecretRef": map[string]any{
					"name": "pull-secret",
				},
			},
			"helmHttp": map[string]any{
				"credentialsSecretRef": map[string]any{
					"name":      "helm-secret",
					"namespace": "kube-system",
				},
			},
			"gitRepo": map[string]any{
				"credentialsSecretRef": map[string]any{
					"name": "git-secret",
				},
			},
		},
	}
	specRaw, _ := json.Marshal(spec)

	cm := &corev1.ConfigMap{}
	cm.Name = "app-secret-spec"
	cm.Namespace = "proj"
	cm.Labels = map[string]string{
		"kubenova.app":            "demo",
		"kubenova.tenant":         "acme",
		"kubenova.project":        "proj",
		"kubenova.io/app-id":      "app-123",
		"kubenova.io/tenant-id":   "tenant-123",
		"kubenova.io/project-id":  "project-123",
		"kubenova.io/source-kind": "containerImage",
	}
	cm.Data = map[string]string{
		"spec": string(specRaw),
	}
	if err := c.Create(context.Background(), cm); err != nil {
		t.Fatal(err)
	}

	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Namespace: cm.Namespace, Name: cm.Name}}); err != nil {
		t.Fatal(err)
	}
	if !fv.ensured {
		t.Fatalf("expected EnsureApp to be called")
	}
	components, _ := fv.spec["components"].([]any)
	if len(components) != 3 {
		t.Fatalf("expected 3 components, got %d", len(components))
	}
	web := components[0].(map[string]any)
	webProps := web["properties"].(map[string]any)
	secrets, ok := webProps["imagePullSecrets"].([]any)
	if !ok {
		if typed, ok2 := webProps["imagePullSecrets"].([]map[string]string); ok2 {
			secrets = make([]any, len(typed))
			for i, v := range typed {
				secrets[i] = v
			}
		}
	}
	if len(secrets) != 1 {
		t.Fatalf("imagePullSecrets missing: %v", webProps["imagePullSecrets"])
	}
	item := toAnyMap(secrets[0])
	if item == nil || item["name"] != "pull-secret" {
		t.Fatalf("unexpected pull secret: %v", secrets[0])
	}

	helm := components[1].(map[string]any)
	helmProps := helm["properties"].(map[string]any)
	helmSecret := toAnyMap(helmProps["credentialsSecretRef"])
	if helmSecret == nil || helmSecret["name"] != "helm-secret" || helmSecret["namespace"] != "kube-system" {
		t.Fatalf("unexpected helm secret: %v", helmProps["credentialsSecretRef"])
	}

	git := components[2].(map[string]any)
	gitProps := git["properties"].(map[string]any)
	gitSecret := toAnyMap(gitProps["credentialsSecretRef"])
	if gitSecret == nil || gitSecret["name"] != "git-secret" || gitSecret["namespace"] != "proj" {
		t.Fatalf("unexpected git secret: %v", gitProps["credentialsSecretRef"])
	}
}

func toAnyMap(input any) map[string]any {
	if input == nil {
		return nil
	}
	switch v := input.(type) {
	case map[string]any:
		return v
	case map[string]string:
		out := map[string]any{}
		for key, val := range v {
			out[key] = val
		}
		return out
	default:
		return nil
	}
}
