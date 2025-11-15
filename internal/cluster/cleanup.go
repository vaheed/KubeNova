package cluster

import (
	"context"
	"regexp"
	"strings"
	"time"

	"github.com/vaheed/kubenova/internal/logging"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// CleanPlatform removes previously installed platform components (cert-manager, Capsule, capsule-proxy, Vela)
// and clears stuck webhooks/CRDs to prepare for a fresh bootstrap. Idempotent and best-effort.
func CleanPlatform(ctx context.Context, kubeconfig []byte) error {
	cfg, err := clientcmd.RESTConfigFromKubeConfig(kubeconfig)
	if err != nil {
		return err
	}
	cset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return err
	}
	d, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return err
	}
	lg := logging.FromContext(ctx)
	// 1) Delete known problematic aggregated APIService (Vela cluster-gateway)
	apisGVR := schema.GroupVersionResource{Group: "apiregistration.k8s.io", Version: "v1", Resource: "apiservices"}
	_ = d.Resource(apisGVR).Delete(ctx, "v1alpha1.cluster.core.oam.dev", metav1.DeleteOptions{})
	lg.Info("cleanup.apiservice.deleted", zap.String("name", "v1alpha1.cluster.core.oam.dev"))

	// 2) Delete broken webhooks (capsule / cert-manager / kyverno / vela)
	re := regexp.MustCompile(`(?i)(capsule|projectcapsule|cert-manager|kyverno|vela)`) // case-insensitive
	if vws, err := cset.AdmissionregistrationV1().ValidatingWebhookConfigurations().List(ctx, metav1.ListOptions{}); err == nil {
		for _, vw := range vws.Items {
			if re.MatchString(vw.Name) {
				_ = cset.AdmissionregistrationV1().ValidatingWebhookConfigurations().Delete(ctx, vw.Name, metav1.DeleteOptions{})
				lg.Info("cleanup.vwc.deleted", zap.String("name", vw.Name))
			}
		}
	}
	if mws, err := cset.AdmissionregistrationV1().MutatingWebhookConfigurations().List(ctx, metav1.ListOptions{}); err == nil {
		for _, mw := range mws.Items {
			if re.MatchString(mw.Name) {
				_ = cset.AdmissionregistrationV1().MutatingWebhookConfigurations().Delete(ctx, mw.Name, metav1.DeleteOptions{})
				lg.Info("cleanup.mwc.deleted", zap.String("name", mw.Name))
			}
		}
	}

	// 3) Delete CRDs for a clean slate (capsule/cert-manager/vela)
	crdGVR := schema.GroupVersionResource{Group: "apiextensions.k8s.io", Version: "v1", Resource: "customresourcedefinitions"}
	if list, err := d.Resource(crdGVR).List(ctx, metav1.ListOptions{}); err == nil {
		for _, crd := range list.Items {
			name := crd.GetName()
			if re.MatchString(name) {
				_ = d.Resource(crdGVR).Delete(ctx, name, metav1.DeleteOptions{})
				lg.Info("cleanup.crd.deleted", zap.String("name", name))
			}
		}
	}

	// 4) Delete component namespaces but keep kubenova-system intact
	for _, ns := range []string{"cert-manager", "capsule-system", "vela-system"} {
		_ = cset.CoreV1().Namespaces().Delete(ctx, ns, metav1.DeleteOptions{})
		lg.Info("cleanup.ns.delete_requested", zap.String("namespace", ns))
	}

	// 5) Remove previous bootstrap Job if any so Agent can recreate fresh
	_ = cset.BatchV1().Jobs("kubenova-system").Delete(ctx, "kubenova-bootstrap", metav1.DeleteOptions{})
	lg.Info("cleanup.job.delete_requested", zap.String("namespace", "kubenova-system"), zap.String("name", "kubenova-bootstrap"))

	// 6) Best-effort: wait briefly for deletes to apply
	time.Sleep(3 * time.Second)

	// Done (asynchronous operations may still be in progress)
	return nil
}

// CleanNonCoreNamespaces deletes all non-core namespaces except kube-* defaults and well-known infra.
// Use with caution. This is not used by default cleanup.
func CleanNonCoreNamespaces(ctx context.Context, kubeconfig []byte) error {
	cfg, err := clientcmd.RESTConfigFromKubeConfig(kubeconfig)
	if err != nil {
		return err
	}
	cset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return err
	}
	keep := regexp.MustCompile(`^(kube-system|kube-public|kube-node-lease|default|local-path-storage|metallb-system|kubenova-system)$`)
	nsl, err := cset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	lg := logging.FromContext(ctx)
	for _, ns := range nsl.Items {
		if keep.MatchString(ns.Name) {
			continue
		}
		if strings.HasPrefix(ns.Name, "kube-") {
			continue
		}
		_ = cset.CoreV1().Namespaces().Delete(ctx, ns.Name, metav1.DeleteOptions{})
		lg.Info("cleanup.ns.delete_requested", zap.String("namespace", ns.Name))
	}
	return nil
}
