package cluster

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
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
	UseRemote bool
}

// NewInstaller returns a new Installer instance.
func NewInstaller(c client.Client, scheme *runtime.Scheme) *Installer {
	charts := os.Getenv("HELM_CHARTS_DIR")
	if charts == "" {
		charts = "/charts"
	}
	if _, err := os.Stat(charts); err != nil {
		charts = ""
	}
	useRemote := false
	if v, ok := os.LookupEnv("HELM_USE_REMOTE"); ok {
		useRemote = parseBool(v)
	} else if charts == "" {
		// Default to remote charts when none are baked in and no override provided.
		useRemote = true
	}
	return &Installer{
		Client:    c,
		Scheme:    scheme,
		ChartsDir: charts,
		UseRemote: useRemote,
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
		if err := i.runHelmLocal(ctx, component, fmt.Sprintf("%s/%s", i.ChartsDir, component), "kubenova-system"); err != nil {
			return err
		}
		return i.waitForReady(ctx, component)
	}
	if i.UseRemote {
		if err := i.runHelmRemote(ctx, component, "kubenova-system"); err != nil {
			return err
		}
		return i.waitForReady(ctx, component)
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

func (i *Installer) runHelmLocal(ctx context.Context, release, chart, namespace string) error {
	cmd := exec.CommandContext(ctx, "helm", "upgrade", "--install", release, chart, "--namespace", namespace, "--create-namespace") // #nosec G204 -- arguments are manager-defined constants
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (i *Installer) runHelmRemote(ctx context.Context, component, namespace string) error {
	meta, ok := componentRepos[component]
	if !ok {
		return fmt.Errorf("no remote repo for component %s", component)
	}
	args := []string{"upgrade", "--install", component, meta.Chart, "--namespace", namespace, "--create-namespace", "--repo", meta.Repo}
	if meta.Version != "" {
		args = append(args, "--version", meta.Version)
	}
	cmd := exec.CommandContext(ctx, "helm", args...) // #nosec G204 -- arguments are manager-defined constants
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (i *Installer) waitForReady(ctx context.Context, component string) error {
	deploy := deploymentName(component)
	if deploy == "" {
		return nil
	}
	var dep appsv1.Deployment
	key := client.ObjectKey{Name: deploy, Namespace: "kubenova-system"}
	timeout := time.After(2 * time.Minute)
	tick := time.Tick(5 * time.Second)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout:
			return fmt.Errorf("timeout waiting for %s ready", deploy)
		case <-tick:
			err := i.Client.Get(ctx, key, &dep)
			if err != nil {
				if apierrors.IsNotFound(err) {
					continue
				}
				return err
			}
			if dep.Status.AvailableReplicas > 0 {
				return nil
			}
		}
	}
}

func deploymentName(component string) string {
	switch component {
	case "cert-manager":
		return "cert-manager"
	case "capsule":
		return "capsule-controller-manager"
	case "capsule-proxy":
		return "capsule-proxy"
	case "kubevela":
		return "vela-core"
	case "operator":
		return "kubenova-operator"
	default:
		return ""
	}
}

type repoMeta struct {
	Repo    string
	Chart   string
	Version string
}

var componentRepos = map[string]repoMeta{
	"cert-manager":  {Repo: "https://charts.jetstack.io", Chart: "cert-manager", Version: "v1.14.4"},
	"capsule":       {Repo: "https://clastix.github.io/charts", Chart: "capsule", Version: "0.5.0"},
	"capsule-proxy": {Repo: "https://clastix.github.io/charts", Chart: "capsule-proxy", Version: "0.3.1"},
	"kubevela":      {Repo: "https://kubevela.github.io/charts", Chart: "vela-core", Version: "1.9.11"},
}

func parseBool(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "true", "1", "yes", "on", "y":
		return true
	default:
		return false
	}
}
