package vela

import "testing"

func TestApplicationCR(t *testing.T) {
	u := ApplicationCR("demo", "app", "nginx:latest")
	if u.GetKind() != "Application" {
		t.Fatal("kind mismatch")
	}
	if u.GetNamespace() != "demo" {
		t.Fatal("ns mismatch")
	}
}
