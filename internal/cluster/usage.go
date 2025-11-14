package cluster

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

type Usage struct {
	CPU    string
	Memory string
	Pods   int64
}

func TenantUsage(ctx context.Context, kubeconfig []byte, tenant string) (Usage, error) {
	cfg, err := clientcmd.RESTConfigFromKubeConfig(kubeconfig)
	if err != nil {
		return Usage{}, err
	}
	cset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return Usage{}, err
	}
	nsList, err := cset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{
		LabelSelector: "capsule.clastix.io/tenant=" + tenant,
	})
	if err != nil {
		return Usage{}, err
	}
	return aggregateUsage(ctx, cset, nsList.Items)
}

func ProjectUsage(ctx context.Context, kubeconfig []byte, namespace string) (Usage, error) {
	cfg, err := clientcmd.RESTConfigFromKubeConfig(kubeconfig)
	if err != nil {
		return Usage{}, err
	}
	cset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return Usage{}, err
	}
	ns, err := cset.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
	if err != nil {
		return Usage{}, err
	}
	return aggregateUsage(ctx, cset, []corev1.Namespace{*ns})
}

func aggregateUsage(ctx context.Context, cset kubernetes.Interface, namespaces []corev1.Namespace) (Usage, error) {
	var cpuTotal, memTotal resource.Quantity
	var podsTotal int64
	for _, ns := range namespaces {
		rqList, err := cset.CoreV1().ResourceQuotas(ns.Name).List(ctx, metav1.ListOptions{})
		if err != nil {
			return Usage{}, err
		}
		for _, rq := range rqList.Items {
			used := rq.Status.Used
			if used == nil || len(used) == 0 {
				used = rq.Spec.Hard
			}
			if qty, ok := used[corev1.ResourceCPU]; ok {
				if cpuTotal.IsZero() {
					cpuTotal = qty.DeepCopy()
				} else {
					cpuTotal.Add(qty)
				}
			}
			if qty, ok := used[corev1.ResourceMemory]; ok {
				if memTotal.IsZero() {
					memTotal = qty.DeepCopy()
				} else {
					memTotal.Add(qty)
				}
			}
			if qty, ok := used[corev1.ResourcePods]; ok {
				podsTotal += qty.Value()
			}
		}
	}
	return Usage{
		CPU:    cpuTotal.String(),
		Memory: memTotal.String(),
		Pods:   podsTotal,
	}, nil
}
