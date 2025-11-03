package scenarios

import (
	"context"
	"fmt"
	"testing"

	"github.com/vaheed/kubenova/tests/e2e/assertions"
	"github.com/vaheed/kubenova/tests/e2e/setup"
)

func TestClusterLifecycle(t *testing.T) {
	t.Parallel()
	env := setup.SuiteEnvironment()
	if env == nil {
		t.Skip("suite environment unavailable")
	}
	cfg := env.Config()
	clusterName := fmt.Sprintf("%s-suite", cfg.ClusterName)
	env.Logger().Info("scenario.cluster_lifecycle.start", "cluster", clusterName)

	info, err := env.EnsureClusterRegistered(context.Background(), clusterName)
	if err != nil {
		t.Fatalf("cluster registration failed: %v", err)
	}
	env.Logger().Info("scenario.cluster_lifecycle.registered", "cluster_id", info.ID)

	assertions.RequireManagerConditions(t, env, info.ID)
	env.Logger().Info("scenario.cluster_lifecycle.manager_conditions_ok")

	assertions.RequireAgentDeploymentReady(t, env, 2)
	env.Logger().Info("scenario.cluster_lifecycle.agent_ready")

	assertions.RequireAddonHealth(t, env)
	env.Logger().Info("scenario.cluster_lifecycle.addons_ready")
}
