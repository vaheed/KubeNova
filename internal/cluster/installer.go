package cluster

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Installer coordinates bootstrap of in-cluster components.
// It can execute Helm installs when HELM_CHARTS_DIR is provided,
// otherwise it records placeholder ConfigMaps to mark bootstrap intent.
type Installer struct {
	Client    client.Client
	Scheme    *runtime.Scheme
	ChartsDir string
}

// NewInstaller returns a new Installer instance.
func NewInstaller(c client.Client, scheme *runtime.Scheme) *Installer {
	return &Installer{
		Client:    c,
		Scheme:    scheme,
		ChartsDir: os.Getenv("HELM_CHARTS_DIR"),
	}
}

// Bootstrap simulates a bootstrap action for the given component.
func (i *Installer) Bootstrap(ctx context.Context, component string) error {
	if i.Client == nil {
		return fmt.Errorf("bootstrap: client is nil")
	}
	// Ensure namespace
	if err := i.ensureNamespace(ctx, "kubenova-system"); err != nil {
		return err
	}

	if i.ChartsDir != "" {
		if err := i.runHelm(ctx, component, fmt.Sprintf("%s/%s", i.ChartsDir, component), "kubenova-system"); err != nil {
			return err
		}
		return nil
	}
	// Fallback: record intent via ConfigMap so we can track bootstrap progress.
	return i.ensurePlaceholder(ctx, component)
}

// RenderManifest returns a placeholder manifest for the requested component.
func (i *Installer) RenderManifest(component string) string {
	return fmt.Sprintf("# kubenova %s manifest would be rendered here", component)
}

func (i *Installer) ensureNamespace(ctx context.Context, ns string) error {
	obj := &corev1.Namespace{}
	err := i.Client.Get(ctx, client.ObjectKey{Name: ns}, obj)
	if apierrors.IsNotFound(err) {
		obj = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name:   ns,
				Labels: map[string]string{"managed-by": "kubenova"},
			},
		}
		return i.Client.Create(ctx, obj)
	}
	return err
}

func (i *Installer) ensurePlaceholder(ctx context.Context, component string) error {
	cm := &corev1.ConfigMap{}
	key := client.ObjectKey{Name: "kubenova-bootstrap-" + component, Namespace: "kubenova-system"}
	err := i.Client.Get(ctx, key, cm)
	if apierrors.IsNotFound(err) {
		cm = &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      key.Name,
				Namespace: key.Namespace,
				Labels: map[string]string{
					"managed-by": "kubenova",
					"component":  component,
				},
			},
			Data: map[string]string{
				"status":    "pending",
				"component": component,
			},
		}
		return i.Client.Create(ctx, cm)
	}
	if err != nil {
		return err
	}
	if cm.Data == nil {
		cm.Data = map[string]string{}
	}
	cm.Data["status"] = "ready"
	cm.Data["updatedAt"] = time.Now().UTC().Format(time.RFC3339)
	return i.Client.Update(ctx, cm)
}

func (i *Installer) runHelm(ctx context.Context, release, chart, namespace string) error {
	cmd := exec.CommandContext(ctx, "helm", "upgrade", "--install", release, chart, "--namespace", namespace, "--create-namespace")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
