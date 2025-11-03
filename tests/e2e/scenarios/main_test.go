package scenarios

import (
	"context"
	"os"
	"testing"

	"github.com/vaheed/kubenova/tests/e2e/setup"
)

func TestMain(m *testing.M) {
	cfg := setup.LoadConfig()
	if cfg.SkipSuite {
		os.Exit(0)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	env, err := setup.InitSuiteEnvironment(ctx, cfg)
	if err == setup.ErrSuiteSkipped {
		os.Exit(0)
	}
	if err != nil {
		setup.SuiteLogger().Error("suite.setup_failed", "error", err)
		os.Exit(1)
	}
	code := m.Run()
	env.Teardown(context.Background())
	os.Exit(code)
}
