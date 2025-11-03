package cluster

import (
	"context"
	"embed"
	"fmt"
	"strings"
	"time"

	"github.com/vaheed/kubenova/internal/util"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

//go:embed manifests/*
var manifests embed.FS

// InstallAgent applies RBAC, Deployment, and HPA to the target cluster.
// image: agent container image
// managerURL: base URL of manager for agent HEARTBEAT
func InstallAgent(ctx context.Context, kubeconfig []byte, image, managerURL string) error {
	cfg, err := clientcmd.RESTConfigFromKubeConfig(kubeconfig)
	if err != nil {
		return err
	}
	return applyAll(ctx, cfg, image, managerURL)
}

func applyAll(ctx context.Context, cfg *rest.Config, image, managerURL string) error {
	cset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return err
	}
	scheme := unstructuredScheme()
	dec := serializer.NewCodecFactory(scheme).UniversalDeserializer()
	// Loop embedded files
	items := []string{"namespace.yaml", "serviceaccount.yaml", "clusterrole.yaml", "clusterrolebinding.yaml", "deployment.yaml", "hpa.yaml"}
	for _, name := range items {
		b, err := manifests.ReadFile("manifests/" + name)
		if err != nil {
			return err
		}
		s := strings.ReplaceAll(string(b), "{{IMAGE}}", image)
		s = strings.ReplaceAll(s, "{{MANAGER_URL}}", managerURL)
		obj, gvk, err := dec.Decode([]byte(s), nil, nil)
		if err != nil {
			return fmt.Errorf("decode %s: %w", name, err)
		}
		switch o := obj.(type) {
		case *corev1.Namespace:
			_, _ = cset.CoreV1().Namespaces().Create(ctx, o, metav1.CreateOptions{})
		case *corev1.ServiceAccount:
			_, _ = cset.CoreV1().ServiceAccounts(o.Namespace).Create(ctx, o, metav1.CreateOptions{})
		case *rbacv1.ClusterRole:
			_, _ = cset.RbacV1().ClusterRoles().Create(ctx, o, metav1.CreateOptions{})
		case *rbacv1.ClusterRoleBinding:
			_, _ = cset.RbacV1().ClusterRoleBindings().Create(ctx, o, metav1.CreateOptions{})
		case *appsv1.Deployment:
			_, _ = cset.AppsV1().Deployments(o.Namespace).Create(ctx, o, metav1.CreateOptions{})
		case *autoscalingv2.HorizontalPodAutoscaler:
			_, _ = cset.AutoscalingV2().HorizontalPodAutoscalers(o.Namespace).Create(ctx, o, metav1.CreateOptions{})
		default:
			_ = gvk // ignored
		}
	}
	// Wait for agent 2/2 ready with backoff
	var ready bool
	check := func() (bool, error) {
		dep, err := cset.AppsV1().Deployments("kubenova").Get(ctx, "kubenova-agent", metav1.GetOptions{})
		if err == nil && dep.Status.ReadyReplicas >= 2 {
			if _, err := cset.AutoscalingV2().HorizontalPodAutoscalers("kubenova").Get(ctx, "kubenova-agent", metav1.GetOptions{}); err == nil {
				ready = true
				return false, nil
			}
		}
		return true, nil
	}
	_ = util.Retry(10*time.Minute, check)
	if !ready {
		return fmt.Errorf("timeout waiting for agent ready")
	}
	return nil
}

func unstructuredScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = corev1.AddToScheme(s)
	_ = appsv1.AddToScheme(s)
	_ = rbacv1.AddToScheme(s)
	_ = autoscalingv2.AddToScheme(s)
	_ = metav1.AddMetaToScheme(s)
	return s
}
