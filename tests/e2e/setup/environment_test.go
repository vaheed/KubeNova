package setup

import (
	"testing"
	"time"
)

func TestNewHTTPClientUsesProvidedTimeout(t *testing.T) {
	want := 5 * time.Minute
	client := newHTTPClient(want)
	if client.Timeout != want {
		t.Fatalf("expected timeout %s, got %s", want, client.Timeout)
	}
}

func TestNewHTTPClientAppliesMinimumTimeout(t *testing.T) {
	client := newHTTPClient(10 * time.Second)
	if client.Timeout < time.Minute {
		t.Fatalf("expected minimum timeout of %s, got %s", time.Minute, client.Timeout)
	}
}
