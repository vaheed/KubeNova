package cluster

import (
	"context"
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
	i := NewInstaller(client, scheme)
	if err := i.Bootstrap(context.Background(), "capsule"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if i.RenderManifest("capsule") == "" {
		t.Fatalf("manifest should not be empty")
	}
}
