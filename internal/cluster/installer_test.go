package cluster

import (
	"strings"
	"testing"
)

func TestAgentManifestTemplates(t *testing.T) {
	b, err := manifests.ReadFile("manifests/deployment.yaml")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, "replicas: 2") {
		t.Fatalf("expected replicas 2: %s", s)
	}
	if !strings.Contains(s, "{{IMAGE}}") {
		t.Fatalf("image placeholder missing")
	}
}
