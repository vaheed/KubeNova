package cluster

import (
	"context"
	"testing"
)

func TestBootstrapNoop(t *testing.T) {
	i := NewInstaller()
	if err := i.Bootstrap(context.Background(), "capsule"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if i.RenderManifest("capsule") == "" {
		t.Fatalf("manifest should not be empty")
	}
}
