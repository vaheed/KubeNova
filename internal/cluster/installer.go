package cluster

import (
	"context"
	"embed"
	"fmt"
	"strings"
	"time"

	"github.com/vaheed/kubenova/internal/logging"
	"github.com/vaheed/kubenova/internal/util"
	"go.uber.org/zap"
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
	logging.L.Info("agent.install.start", zap.String("image", image), zap.String("manager_url", managerURL))
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
	logging.L.Info("agent.apply.start")
	scheme := unstructuredScheme()
	dec := serializer.NewCodecFactory(scheme).UniversalDeserializer()
	// Loop embedded files
	items := []string{
		"namespace.yaml",
		"serviceaccount.yaml",
		"clusterrole.yaml",
		"clusterrolebinding.yaml",
		"role.yaml",
		"rolebinding.yaml",
		"bootstrap-serviceaccount.yaml",
		"bootstrap-clusterrolebinding.yaml",
		"deployment.yaml",
		"hpa.yaml",
	}
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
			logging.L.Info("agent.apply.ensure", zap.String("kind", gvk.Kind), zap.String("name", o.Name))
			if _, err := cset.CoreV1().Namespaces().Create(ctx, o, metav1.CreateOptions{}); err != nil {
				_ = err
			}
		case *corev1.ServiceAccount:
			logging.L.Info("agent.apply.ensure", zap.String("kind", gvk.Kind), zap.String("name", o.Name), zap.String("ns", o.Namespace))
			if _, err := cset.CoreV1().ServiceAccounts(o.Namespace).Create(ctx, o, metav1.CreateOptions{}); err != nil {
				if existing, getErr := cset.CoreV1().ServiceAccounts(o.Namespace).Get(ctx, o.Name, metav1.GetOptions{}); getErr == nil {
					o.ResourceVersion = existing.ResourceVersion
					_, _ = cset.CoreV1().ServiceAccounts(o.Namespace).Update(ctx, o, metav1.UpdateOptions{})
				}
			}
		case *rbacv1.ClusterRole:
			logging.L.Info("agent.apply.ensure", zap.String("kind", gvk.Kind), zap.String("name", o.Name))
			if _, err := cset.RbacV1().ClusterRoles().Create(ctx, o, metav1.CreateOptions{}); err != nil {
				if existing, getErr := cset.RbacV1().ClusterRoles().Get(ctx, o.Name, metav1.GetOptions{}); getErr == nil {
					o.ResourceVersion = existing.ResourceVersion
					_, _ = cset.RbacV1().ClusterRoles().Update(ctx, o, metav1.UpdateOptions{})
				}
			}
		case *rbacv1.ClusterRoleBinding:
			logging.L.Info("agent.apply.ensure", zap.String("kind", gvk.Kind), zap.String("name", o.Name))
			if _, err := cset.RbacV1().ClusterRoleBindings().Create(ctx, o, metav1.CreateOptions{}); err != nil {
				if existing, getErr := cset.RbacV1().ClusterRoleBindings().Get(ctx, o.Name, metav1.GetOptions{}); getErr == nil {
					o.ResourceVersion = existing.ResourceVersion
					_, _ = cset.RbacV1().ClusterRoleBindings().Update(ctx, o, metav1.UpdateOptions{})
				}
			}
		case *rbacv1.Role:
			logging.L.Info("agent.apply.ensure", zap.String("kind", gvk.Kind), zap.String("name", o.Name), zap.String("ns", o.Namespace))
			if _, err := cset.RbacV1().Roles(o.Namespace).Create(ctx, o, metav1.CreateOptions{}); err != nil {
				if existing, getErr := cset.RbacV1().Roles(o.Namespace).Get(ctx, o.Name, metav1.GetOptions{}); getErr == nil {
					o.ResourceVersion = existing.ResourceVersion
					_, _ = cset.RbacV1().Roles(o.Namespace).Update(ctx, o, metav1.UpdateOptions{})
				}
			}
		case *rbacv1.RoleBinding:
			logging.L.Info("agent.apply.ensure", zap.String("kind", gvk.Kind), zap.String("name", o.Name), zap.String("ns", o.Namespace))
			if _, err := cset.RbacV1().RoleBindings(o.Namespace).Create(ctx, o, metav1.CreateOptions{}); err != nil {
				if existing, getErr := cset.RbacV1().RoleBindings(o.Namespace).Get(ctx, o.Name, metav1.GetOptions{}); getErr == nil {
					o.ResourceVersion = existing.ResourceVersion
					_, _ = cset.RbacV1().RoleBindings(o.Namespace).Update(ctx, o, metav1.UpdateOptions{})
				}
			}
		case *appsv1.Deployment:
			logging.L.Info("agent.apply.ensure", zap.String("kind", gvk.Kind), zap.String("name", o.Name), zap.String("ns", o.Namespace), zap.Int32("replicas", *o.Spec.Replicas))
			if _, err := cset.AppsV1().Deployments(o.Namespace).Create(ctx, o, metav1.CreateOptions{}); err != nil {
				if existing, getErr := cset.AppsV1().Deployments(o.Namespace).Get(ctx, o.Name, metav1.GetOptions{}); getErr == nil {
					o.ResourceVersion = existing.ResourceVersion
					_, _ = cset.AppsV1().Deployments(o.Namespace).Update(ctx, o, metav1.UpdateOptions{})
				}
			}
		case *autoscalingv2.HorizontalPodAutoscaler:
			// MinReplicas is optional, guard to avoid nil deref
			var min int32
			if o.Spec.MinReplicas != nil {
				min = *o.Spec.MinReplicas
			}
			logging.L.Info("agent.apply.ensure", zap.String("kind", gvk.Kind), zap.String("name", o.Name), zap.String("ns", o.Namespace), zap.Int32("min", min))
			if _, err := cset.AutoscalingV2().HorizontalPodAutoscalers(o.Namespace).Create(ctx, o, metav1.CreateOptions{}); err != nil {
				if existing, getErr := cset.AutoscalingV2().HorizontalPodAutoscalers(o.Namespace).Get(ctx, o.Name, metav1.GetOptions{}); getErr == nil {
					o.ResourceVersion = existing.ResourceVersion
					_, _ = cset.AutoscalingV2().HorizontalPodAutoscalers(o.Namespace).Update(ctx, o, metav1.UpdateOptions{})
				}
			}
		default:
			_ = gvk // ignored
		}
	}
	// Wait for agent 2/2 ready with backoff
	var ready bool
	ns := "kubenova-system"
	logging.L.Info("agent.wait.ready", zap.String("namespace", ns), zap.String("name", "agent"))
	check := func() (bool, error) {
		dep, err := cset.AppsV1().Deployments(ns).Get(ctx, "agent", metav1.GetOptions{})
		if err == nil && dep.Status.ReadyReplicas >= 2 {
			if _, err := cset.AutoscalingV2().HorizontalPodAutoscalers(ns).Get(ctx, "agent", metav1.GetOptions{}); err == nil {
				ready = true
				return false, nil
			}
		}
		return true, nil
	}
	_ = util.Retry(10*time.Minute, check)
	if !ready {
		logging.L.Error("agent.wait.timeout")
		return fmt.Errorf("timeout waiting for agent ready")
	}
	logging.L.Info("agent.ready")
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

// no env toggles required; installer applies least-privilege runtime RBAC,
// a dedicated cluster-admin bootstrap SA, and upserts resources idempotently.
