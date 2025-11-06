package cluster

import (
	"context"
	"os"
	"time"

	"github.com/vaheed/kubenova/pkg/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// ComputeClusterConditions inspects the target cluster and returns readiness conditions.
func ComputeClusterConditions(ctx context.Context, kubeconfig []byte, e2eFake bool) []types.Condition {
	conds := []types.Condition{}
	if e2eFake || parseBool(os.Getenv("KUBENOVA_E2E_FAKE")) {
		now := time.Now().UTC()
		return []types.Condition{
			{Type: "AgentReady", Status: "True", LastTransitionTime: now},
			{Type: "AddonsReady", Status: "True", LastTransitionTime: now},
		}
	}
	cfg, err := clientcmd.RESTConfigFromKubeConfig(kubeconfig)
	if err != nil {
		return failConds(err)
	}
	cset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return failConds(err)
	}
	now := time.Now().UTC()
	// AgentReady
	agentReady := false
	if dep, err := cset.AppsV1().Deployments("kubenova-system").Get(ctx, "kubenova-agent", metav1.GetOptions{}); err == nil {
		if dep.Status.ReadyReplicas >= 2 {
			if _, err := cset.AutoscalingV2().HorizontalPodAutoscalers("kubenova-system").Get(ctx, "kubenova-agent", metav1.GetOptions{}); err == nil {
				agentReady = true
			}
		}
	}
	conds = append(conds, types.Condition{Type: "AgentReady", Status: boolstr(agentReady), LastTransitionTime: now})
	// AddonsReady (neutral checks; controllers and CRDs present)
	addonReady := false
	// For now, probe namespaces and known deployments used by the platform; if all present, we consider ready.
	if _, err := cset.AppsV1().Deployments("capsule-system").Get(ctx, "capsule-controller-manager", metav1.GetOptions{}); err == nil {
		if _, err := cset.AppsV1().Deployments("capsule-system").Get(ctx, "capsule-proxy", metav1.GetOptions{}); err == nil {
			if _, err := cset.AppsV1().Deployments("vela-system").Get(ctx, "vela-core", metav1.GetOptions{}); err == nil {
				// CRDs present check
				discs, _ := cset.Discovery().ServerPreferredNamespacedResources()
				crdCapsule := false
				crdVela := false
				for _, rl := range discs {
					if rl.GroupVersion == "capsule.clastix.io/v1beta2" {
						crdCapsule = true
					}
					if rl.GroupVersion == "core.oam.dev/v1beta1" {
						crdVela = true
					}
				}
				if crdCapsule && crdVela {
					addonReady = true
				}
			}
		}
	}
	conds = append(conds, types.Condition{Type: "AddonsReady", Status: boolstr(addonReady), LastTransitionTime: now})
	return conds
}

func boolstr(b bool) string {
	if b {
		return "True"
	}
	return "False"
}

func failConds(err error) []types.Condition {
	_ = err
	return []types.Condition{{Type: "AgentReady", Status: "False", Reason: "Error"}, {Type: "AddonsReady", Status: "False", Reason: "Error"}}
}

func parseBool(v string) bool {
	switch v {
	case "1", "t", "T", "true", "TRUE", "True", "y", "yes", "on":
		return true
	default:
		return false
	}
}
