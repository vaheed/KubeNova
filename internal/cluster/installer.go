package cluster

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/vaheed/kubenova/internal/logging"
	"go.uber.org/zap"
)

// Installer coordinates bootstrap of in-cluster components.
// It can execute Helm installs when HELM_CHARTS_DIR is provided,
// otherwise it records placeholder ConfigMaps to mark bootstrap intent.
type Installer struct {
	Client         client.Client
	Reader         client.Reader
	Scheme         *runtime.Scheme
	ChartsDir      string
	UseRemote      bool
	SkipWait       bool
	kubeconfigData []byte
}

const (
	defaultServiceAccountDir = "/var/run/secrets/kubernetes.io/serviceaccount"
	serviceAccountDirEnv     = "KUBENOVA_SERVICEACCOUNT_DIR"
	envCertManagerVersion    = "CERT_MANAGER_VERSION"
	envCapsuleVersion        = "CAPSULE_VERSION"
	envCapsuleProxyVersion   = "CAPSULE_PROXY_VERSION"
	envVelaVersion           = "VELA_VERSION"
	envFluxVersion           = "FLUXCD_VERSION"
	envVelauxVersion         = "VELAUX_VERSION"
	envVelauxRepo            = "VELAUX_REPO"
	envFluxRepo              = "FLUXCD_REPO"
	envOperatorRepo          = "OPERATOR_REPO"
	envVelaCLIVersion        = "VELA_CLI_VERSION"
	envBootstrapCertManager  = "BOOTSTRAP_CERT_MANAGER"
	envBootstrapCapsule      = "BOOTSTRAP_CAPSULE"
	envBootstrapCapsuleProxy = "BOOTSTRAP_CAPSULE_PROXY"
	envBootstrapKubeVela     = "BOOTSTRAP_KUBEVELA"
	envBootstrapVelaux       = "BOOTSTRAP_VELAUX"
	envBootstrapFluxCD       = "BOOTSTRAP_FLUXCD"
	envReconcileInterval     = "COMPONENT_RECONCILE_SECONDS"
	envManagerURL            = "MANAGER_URL"
)

// NewInstaller returns a new Installer instance.
func NewInstaller(c client.Client, scheme *runtime.Scheme, kubeconfig []byte, reader client.Reader, skipWait bool) *Installer {
	charts := os.Getenv("HELM_CHARTS_DIR")
	if charts == "" {
		charts = "/charts"
	}
	if _, err := os.Stat(charts); err != nil {
		charts = ""
	}
	if reader == nil {
		reader = c
	}
	useRemote := false
	if v, ok := os.LookupEnv("HELM_USE_REMOTE"); ok {
		useRemote = parseBool(v)
	} else if charts == "" {
		// Default to remote charts when none are baked in and no override provided.
		useRemote = true
	}
	return &Installer{
		Client:         c,
		Reader:         reader,
		Scheme:         scheme,
		ChartsDir:      charts,
		UseRemote:      useRemote,
		SkipWait:       skipWait,
		kubeconfigData: kubeconfig,
	}
}

// Bootstrap simulates a bootstrap action for the given component.
func (i *Installer) Bootstrap(ctx context.Context, component string) error {
	if i.Client == nil {
		return fmt.Errorf("bootstrap: client is nil")
	}
	if !i.shouldBootstrap(component) {
		logging.L.Info("bootstrap_component_skipped", zap.String("component", component))
		return nil
	}
	return i.bootstrapAndSummarize(ctx, component)
}

// Reconcile ensures the desired state for a component; when disabled it uninstalls.
func (i *Installer) Reconcile(ctx context.Context, component string) error {
	if i.shouldBootstrap(component) {
		return i.bootstrapAndSummarize(ctx, component)
	}
	if err := i.uninstall(ctx, component); err != nil {
		return err
	}
	return nil
}

func (i *Installer) bootstrapAndSummarize(ctx context.Context, component string) error {
	start := time.Now()
	meta := i.componentMeta(component)
	if component == "velaux" || component == "fluxcd" {
		if err := i.enableVelaAddon(ctx, component); err != nil {
			return err
		}
		i.logSummary(component, meta, start, "enabled")
		return i.waitForComponent(ctx, component)
	}
	// Ensure namespace
	if err := i.ensureNamespace(ctx, "kubenova-system"); err != nil {
		return err
	}

	if i.ChartsDir != "" {
		if err := i.runHelmLocal(ctx, component, fmt.Sprintf("%s/%s", i.ChartsDir, component), "kubenova-system"); err != nil {
			return err
		}
		return i.waitForComponent(ctx, component)
	}
	if i.UseRemote {
		if err := i.runHelmRemote(ctx, component, "kubenova-system"); err != nil {
			return err
		}
		return i.waitForComponent(ctx, component)
	}
	// Fallback: record intent via ConfigMap so we can track bootstrap progress.
	if err := i.ensurePlaceholder(ctx, component); err != nil {
		return err
	}
	i.logSummary(component, meta, start, "placeholder")
	return nil
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

func (i *Installer) waitForComponent(ctx context.Context, component string) error {
	if i.SkipWait && component != "operator" {
		logging.L.Info("bootstrap_component_wait_skipped", zap.String("component", component))
		return nil
	}
	return i.waitForReady(ctx, component)
}

func (i *Installer) runHelmLocal(ctx context.Context, release, chart, namespace string) error {
	args := []string{"upgrade", "--install", release, chart, "--namespace", namespace, "--create-namespace"}
	args = append(args, i.componentSetFlags(release)...)
	cmd, cleanup, err := i.prepareHelmCommand(ctx, args...)
	if err != nil {
		return err
	}
	defer cleanup()
	return cmd.Run()
}

func (i *Installer) runHelmRemote(ctx context.Context, component, namespace string) error {
	meta := i.componentMeta(component)
	if meta == nil {
		return fmt.Errorf("no remote repo for component %s", component)
	}
	chartRef := meta.Chart
	repo := meta.Repo
	// Allow OCI registries that do not expose an index.
	if strings.HasPrefix(meta.Chart, "oci://") {
		repo = ""
	} else if strings.HasPrefix(meta.Repo, "oci://") {
		repo = ""
		chartRef = strings.TrimSuffix(meta.Repo, "/") + "/" + strings.TrimPrefix(meta.Chart, "/")
	}
	args := []string{"upgrade", "--install", component, chartRef, "--namespace", namespace, "--create-namespace"}
	if repo != "" {
		args = append(args, "--repo", repo)
	}
	if version := i.versionOverride(component, meta.Version); version != "" {
		args = append(args, "--version", version)
	}
	args = append(args, i.componentSetFlags(component)...)
	cmd, cleanup, err := i.prepareHelmCommand(ctx, args...)
	if err != nil {
		return err
	}
	defer cleanup()
	return cmd.Run()
}

func (i *Installer) prepareHelmCommand(ctx context.Context, args ...string) (*exec.Cmd, func(), error) {
	cmd := exec.CommandContext(ctx, "helm", args...) // #nosec G204 -- arguments are manager-defined constants
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	env := os.Environ()
	kubeconfig, err := i.kubeconfigBytes()
	if err != nil {
		return nil, func() {}, err
	}
	var kubeconfigFile string
	var helmHome string
	cleanup := func() {
		if kubeconfigFile != "" {
			_ = os.Remove(kubeconfigFile)
		}
		if helmHome != "" {
			_ = os.RemoveAll(helmHome)
		}
	}
	if len(kubeconfig) > 0 {
		tmp, err := os.CreateTemp("", "kubenova-kubeconfig-*.yaml")
		if err != nil {
			cleanup()
			return nil, func() {}, err
		}
		if _, err := tmp.Write(kubeconfig); err != nil {
			_ = tmp.Close()
			_ = os.Remove(tmp.Name())
			return nil, func() {}, err
		}
		if err := tmp.Close(); err != nil {
			_ = os.Remove(tmp.Name())
			return nil, func() {}, err
		}
		kubeconfigFile = tmp.Name()
		env = append(env, fmt.Sprintf("KUBECONFIG=%s", tmp.Name()))
	}
	home, err := os.MkdirTemp("", "kubenova-helm-*")
	if err != nil {
		cleanup()
		return nil, func() {}, err
	}
	helmHome = home
	cacheDir := filepath.Join(home, "cache")
	configDir := filepath.Join(home, "config")
	dataDir := filepath.Join(home, "data")
	for _, d := range []string{cacheDir, configDir, dataDir} {
		if err := os.MkdirAll(d, 0o750); err != nil {
			cleanup()
			return nil, func() {}, err
		}
	}
	env = append(env,
		fmt.Sprintf("HELM_CACHE_HOME=%s", cacheDir),
		fmt.Sprintf("HELM_CONFIG_HOME=%s", configDir),
		fmt.Sprintf("HELM_DATA_HOME=%s", dataDir),
	)
	cmd.Env = env
	return cmd, cleanup, nil
}

func (i *Installer) kubeconfigBytes() ([]byte, error) {
	if len(i.kubeconfigData) > 0 {
		return i.kubeconfigData, nil
	}
	kcfg, err := inClusterKubeconfig()
	if err != nil {
		return nil, err
	}
	i.kubeconfigData = kcfg
	return kcfg, nil
}

func inClusterKubeconfig() ([]byte, error) {
	root := os.Getenv(serviceAccountDirEnv)
	if root == "" {
		root = defaultServiceAccountDir
	}
	root = filepath.Clean(root)
	fsys := os.DirFS(root)
	tokenBytes, err := fs.ReadFile(fsys, "token")
	if err != nil {
		return nil, fmt.Errorf("read sa token: %w", err)
	}
	if _, err := fs.Stat(fsys, "ca.crt"); err != nil {
		return nil, fmt.Errorf("read sa ca: %w", err)
	}
	caPath := filepath.Join(root, "ca.crt")
	host := os.Getenv("KUBERNETES_SERVICE_HOST")
	if host == "" {
		host = "kubernetes.default.svc"
	}
	port := os.Getenv("KUBERNETES_SERVICE_PORT")
	if port == "" {
		port = "443"
	}
	server := fmt.Sprintf("https://%s:%s", strings.TrimSpace(host), strings.TrimSpace(port))
	token := strings.TrimSpace(string(tokenBytes))
	config := fmt.Sprintf(`apiVersion: v1
kind: Config
clusters:
- cluster:
    server: %s
    certificate-authority: %s
  name: in-cluster
contexts:
- context:
    cluster: in-cluster
    user: in-cluster
  name: in-cluster
current-context: in-cluster
preferences: {}
users:
- name: in-cluster
  user:
    token: %s
`, server, caPath, token)
	return []byte(config), nil
}

func (i *Installer) waitForReady(ctx context.Context, component string) error {
	deploy := deploymentName(component)
	if deploy == "" {
		return nil
	}
	var dep appsv1.Deployment
	key := client.ObjectKey{Name: deploy, Namespace: "kubenova-system"}
	reader := i.statusReader()
	timeout := time.After(5 * time.Minute)
	tick := time.Tick(5 * time.Second)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout:
			return fmt.Errorf("timeout waiting for %s ready", deploy)
		case <-tick:
			err := reader.Get(ctx, key, &dep)
			if err != nil {
				if apierrors.IsNotFound(err) {
					logging.L.Info("bootstrap_component_pending", zap.String("component", component))
					continue
				}
				logging.L.Error("bootstrap_component_get_failed", zap.String("component", component), zap.Error(err))
				return err
			}
			logging.L.Info("bootstrap_component_status",
				zap.String("component", component),
				zap.Int32("available", dep.Status.AvailableReplicas),
				zap.Int32("ready", dep.Status.ReadyReplicas),
			)
			if dep.Status.AvailableReplicas > 0 {
				return nil
			}
		}
	}
}

func (i *Installer) statusReader() client.Reader {
	if i.Reader != nil {
		return i.Reader
	}
	return i.Client
}

func (i *Installer) versionOverride(component, fallback string) string {
	switch component {
	case "cert-manager":
		if v := os.Getenv(envCertManagerVersion); v != "" {
			return v
		}
	case "capsule":
		if v := os.Getenv(envCapsuleVersion); v != "" {
			return v
		}
	case "capsule-proxy":
		if v := os.Getenv(envCapsuleProxyVersion); v != "" {
			return v
		}
	case "kubevela", "velaux":
		if v := os.Getenv(envVelaVersion); v != "" {
			return v
		}
		if component == "velaux" {
			if v := os.Getenv(envVelauxVersion); v != "" {
				return v
			}
		}
	case "fluxcd":
		if v := os.Getenv(envFluxVersion); v != "" {
			return v
		}
	}
	return fallback
}

func (i *Installer) componentSetFlags(component string) []string {
	switch component {
	case "cert-manager":
		return []string{"--set", "installCRDs=true"}
	case "capsule-proxy":
		return []string{"--set", "service.type=LoadBalancer"}
	case "operator":
		if url := strings.TrimSpace(os.Getenv(envManagerURL)); url != "" {
			return []string{"--set", fmt.Sprintf("manager.url=%s", url)}
		}
		return nil
	default:
		return nil
	}
}

func (i *Installer) componentMeta(component string) *repoMeta {
	meta, ok := componentRepos[component]
	if !ok {
		return nil
	}
	switch component {
	case "velaux":
		if repo := os.Getenv(envVelauxRepo); repo != "" {
			meta.Repo = repo
		}
	case "fluxcd":
		if repo := os.Getenv(envFluxRepo); repo != "" {
			meta.Repo = repo
		}
	case "operator":
		if repo := os.Getenv(envOperatorRepo); repo != "" {
			meta.Repo = repo
		}
	}
	return &meta
}

func (i *Installer) shouldBootstrap(component string) bool {
	switch component {
	case "cert-manager":
		return parseBoolWithDefault(envBootstrapCertManager, true)
	case "capsule":
		return parseBoolWithDefault(envBootstrapCapsule, true)
	case "capsule-proxy":
		return parseBoolWithDefault(envBootstrapCapsuleProxy, true)
	case "kubevela":
		return parseBoolWithDefault(envBootstrapKubeVela, true)
	case "velaux":
		return parseBoolWithDefault(envBootstrapVelaux, true)
	case "fluxcd":
		return parseBoolWithDefault(envBootstrapFluxCD, true)
	default:
		return true
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
		return "kubevela-vela-core"
	case "velaux":
		return "velaux"
	case "fluxcd":
		return "helm-controller"
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
	"capsule":       {Repo: "https://projectcapsule.github.io/charts", Chart: "capsule", Version: "0.5.3"},
	"capsule-proxy": {Repo: "https://projectcapsule.github.io/charts", Chart: "capsule-proxy", Version: "0.9.13"},
	"kubevela":      {Repo: "https://kubevela.github.io/charts", Chart: "vela-core", Version: "1.10.4"},
	"velaux":        {Repo: "oci://ghcr.io/kubevela/velaux", Chart: "velaux", Version: "v1.10.6"},
	"fluxcd":        {Repo: "https://fluxcd-community.github.io/helm-charts", Chart: "flux2", Version: "2.12.2"},
	"operator":      {Repo: "oci://ghcr.io/vaheed/kubenova/charts", Chart: "operator", Version: "0.0.1"},
}

func parseBool(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "true", "1", "yes", "on", "y":
		return true
	default:
		return false
	}
}

func parseBoolWithDefault(envKey string, def bool) bool {
	raw := os.Getenv(envKey)
	if raw == "" {
		return def
	}
	return parseBool(raw)
}

func (i *Installer) enableVelaAddon(ctx context.Context, addon string) error {
	kcfg, err := i.kubeconfigBytes()
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp("", "kubenova-kubeconfig-*.yaml")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(kcfg); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	args := []string{"addon", "enable", addon, "--yes"}
	home, err := os.MkdirTemp("", "kubenova-vela-home-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(home)
	// #nosec G204 -- command and args are controlled internally for trusted addons
	cmd := exec.CommandContext(ctx, "vela", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("KUBECONFIG=%s", tmp.Name()),
		fmt.Sprintf("VELA_HOME=%s", home),
		fmt.Sprintf("HOME=%s", home),
	)
	return cmd.Run()
}

func (i *Installer) disableVelaAddon(ctx context.Context, addon string) error {
	kcfg, err := i.kubeconfigBytes()
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp("", "kubenova-kubeconfig-*.yaml")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(kcfg); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	args := []string{"addon", "disable", addon, "--yes"}
	home, err := os.MkdirTemp("", "kubenova-vela-home-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(home)
	// #nosec G204 -- command and args are controlled internally for trusted addons
	cmd := exec.CommandContext(ctx, "vela", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("KUBECONFIG=%s", tmp.Name()),
		fmt.Sprintf("VELA_HOME=%s", home),
		fmt.Sprintf("HOME=%s", home),
	)
	return cmd.Run()
}

func (i *Installer) uninstall(ctx context.Context, component string) error {
	start := time.Now()
	meta := i.componentMeta(component)
	var err error
	if component == "velaux" || component == "fluxcd" {
		err = i.disableVelaAddon(ctx, component)
	} else {
		err = i.runHelmUninstall(ctx, component)
	}
	if err != nil {
		logging.L.Error("bootstrap_component_uninstall_failed", zap.String("component", component), zap.Error(err))
		return err
	}
	i.logSummary(component, meta, start, "uninstalled")
	return nil
}

func (i *Installer) runHelmUninstall(ctx context.Context, release string) error {
	args := []string{"uninstall", release, "--namespace", "kubenova-system"}
	cmd, cleanup, err := i.prepareHelmCommand(ctx, args...)
	if err != nil {
		return err
	}
	defer cleanup()
	return cmd.Run()
}

func (i *Installer) logSummary(component string, meta *repoMeta, start time.Time, action string) {
	elapsed := time.Since(start)
	repo := ""
	version := ""
	if meta != nil {
		repo = meta.Repo
		version = meta.Version
	}
	logging.L.Info("bootstrap_component_summary",
		zap.String("component", component),
		zap.String("action", action),
		zap.String("repo", repo),
		zap.String("version", version),
		zap.Duration("duration", elapsed),
	)
}
