package assertions

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/vaheed/kubenova/tests/e2e/setup"
)

type clusterResponse struct {
	ID         int                `json:"id"`
	Conditions []clusterCondition `json:"conditions"`
}

type clusterCondition struct {
	Type   string `json:"type"`
	Status string `json:"status"`
	Reason string `json:"reason"`
}

// RequireManagerConditions waits for the manager-reported AgentReady/AddonsReady conditions to become True.
func RequireManagerConditions(t *testing.T, env *setup.Environment, clusterID int) {
	t.Helper()
	cfg := env.Config()
	ctx, cancel := context.WithTimeout(context.Background(), cfg.WaitTimeout)
	defer cancel()

	err := wait.PollUntilContextTimeout(ctx, 3*time.Second, cfg.WaitTimeout, true, func(ctx context.Context) (bool, error) {
		status, err := fetchCluster(ctx, env, clusterID)
		if err != nil {
			return false, err
		}
		ready := map[string]string{}
		for _, cond := range status.Conditions {
			ready[cond.Type] = cond.Status
		}
		return ready["AgentReady"] == "True" && ready["AddonsReady"] == "True", nil
	})
	if err != nil {
		t.Fatalf("manager conditions did not become ready: %v", err)
	}
}

func fetchCluster(ctx context.Context, env *setup.Environment, clusterID int) (clusterResponse, error) {
	url := fmt.Sprintf("%s/api/v1/clusters/%d", env.ManagerBaseURL(), clusterID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return clusterResponse{}, err
	}
	resp, err := env.HTTPClient().Do(req)
	if err != nil {
		return clusterResponse{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return clusterResponse{}, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	var out clusterResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return clusterResponse{}, err
	}
	return out, nil
}

// RequireAgentDeploymentReady checks the Agent Deployment reports the expected ready replicas.
func RequireAgentDeploymentReady(t *testing.T, env *setup.Environment, replicas int32) {
	t.Helper()
	cfg := env.Config()
	ctx, cancel := context.WithTimeout(context.Background(), cfg.WaitTimeout)
	defer cancel()
	err := wait.PollUntilContextTimeout(ctx, 5*time.Second, cfg.WaitTimeout, true, func(ctx context.Context) (bool, error) {
		dep, err := env.KubeClient().AppsV1().Deployments(cfg.ManagerNamespace).Get(ctx, "agent", metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return false, nil
			}
			return false, err
		}
		if dep.Status.ReadyReplicas >= replicas {
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		t.Fatalf("agent deployment not ready: %v", err)
	}
}

// RequireAddonHealth validates Capsule, capsule-proxy, and KubeVela deployments exist and are ready.
func RequireAddonHealth(t *testing.T, env *setup.Environment) {
	t.Helper()
	cfg := env.Config()
	ctx, cancel := context.WithTimeout(context.Background(), cfg.WaitTimeout)
	defer cancel()
	// Capsule controller
	waitForDeployment(t, ctx, env, cfg.WaitTimeout, "capsule-system", "capsule-controller-manager")
	waitForDeployment(t, ctx, env, cfg.WaitTimeout, "capsule-system", "capsule-proxy")
	waitForDeployment(t, ctx, env, cfg.WaitTimeout, "vela-system", "vela-core")

	discovery := env.KubeClient().Discovery()
	if _, err := discovery.ServerResourcesForGroupVersion("capsule.clastix.io/v1beta2"); err != nil {
		t.Fatalf("capsule API resources missing: %v", err)
	}
	if _, err := discovery.ServerResourcesForGroupVersion("core.oam.dev/v1beta1"); err != nil {
		t.Fatalf("kubevela API resources missing: %v", err)
	}
}

func waitForDeployment(t *testing.T, ctx context.Context, env *setup.Environment, timeout time.Duration, namespace, name string) {
	t.Helper()
	err := wait.PollUntilContextTimeout(ctx, 5*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
		dep, err := env.KubeClient().AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return false, nil
			}
			return false, err
		}
		if dep.Status.ReadyReplicas >= *dep.Spec.Replicas {
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		t.Fatalf("deployment %s/%s not ready: %v", namespace, name, err)
	}
}
