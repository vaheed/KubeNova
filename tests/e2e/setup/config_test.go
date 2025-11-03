package setup

import "testing"

func TestLoadConfigSkipsSuiteByDefault(t *testing.T) {
	t.Setenv("E2E_RUN", "")
	t.Setenv("E2E_SKIP", "")
	cfg := LoadConfig()
	if !cfg.SkipSuite {
		t.Fatalf("expected suite to skip by default")
	}
}

func TestLoadConfigRunsSuiteWhenExplicitlyEnabled(t *testing.T) {
	t.Setenv("E2E_RUN", "1")
	t.Setenv("E2E_SKIP", "")
	cfg := LoadConfig()
	if cfg.SkipSuite {
		t.Fatalf("expected suite to run when E2E_RUN=1")
	}
}

func TestLoadConfigHonoursExplicitSkip(t *testing.T) {
	t.Setenv("E2E_RUN", "1")
	t.Setenv("E2E_SKIP", "true")
	cfg := LoadConfig()
	if !cfg.SkipSuite {
		t.Fatalf("expected suite to skip when E2E_SKIP=true even if E2E_RUN=1")
	}
}
