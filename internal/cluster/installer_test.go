package cluster

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestBootstrapNoop(t *testing.T) {
	t.Setenv("HELM_USE_REMOTE", "false")
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	i := NewInstaller(client, scheme, nil, nil, false)
	if err := i.Bootstrap(context.Background(), "capsule"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if i.RenderManifest("capsule") == "" {
		t.Fatalf("manifest should not be empty")
	}
}

func TestInClusterKubeconfig(t *testing.T) {
	dir := t.TempDir()
	tokenPath := filepath.Join(dir, "token")
	caPath := filepath.Join(dir, "ca.crt")
	if err := os.WriteFile(tokenPath, []byte("service-account-token\n"), 0o600); err != nil {
		t.Fatalf("write token: %v", err)
	}
	if err := os.WriteFile(caPath, []byte("cafile"), 0o600); err != nil {
		t.Fatalf("write ca: %v", err)
	}
	t.Setenv(serviceAccountDirEnv, dir)
	t.Setenv("KUBERNETES_SERVICE_HOST", "10.0.0.1")
	t.Setenv("KUBERNETES_SERVICE_PORT", "6443")

	cfg, err := inClusterKubeconfig()
	if err != nil {
		t.Fatalf("inClusterKubeconfig: %v", err)
	}
	text := string(cfg)
	if !strings.Contains(text, "https://10.0.0.1:6443") {
		t.Fatalf("expected server in kubeconfig, got %s", text)
	}
	if !strings.Contains(text, caPath) {
		t.Fatalf("expected ca path in kubeconfig, got %s", text)
	}
	if !strings.Contains(text, "token: service-account-token") {
		t.Fatalf("expected token in kubeconfig, got %s", text)
	}
}
