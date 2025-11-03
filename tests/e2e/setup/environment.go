package setup

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

// Environment wires together the lifecycle of the suite.
type Environment struct {
	cfg            Config
	logger         *slog.Logger
	restConfig     *rest.Config
	kubeClient     kubernetes.Interface
	httpClient     *http.Client
	kubeconfigPath string

	portForwardCmd  *exec.Cmd
	portForwardStop context.CancelFunc
	portForwardOnce sync.Once

	registerOnce sync.Once
	clusterInfo  ClusterInfo
	registerErr  error

	createdCluster bool
	tempDir        string
}

// ClusterInfo describes the registered cluster state.
type ClusterInfo struct {
	ID   int
	Name string
}

var (
	suiteEnv     *Environment
	suiteEnvOnce sync.Once
	suiteEnvErr  error
)

// ErrSuiteSkipped indicates the suite was skipped via environment toggle.
var ErrSuiteSkipped = errors.New("suite skipped")

// InitSuiteEnvironment bootstraps the shared environment.
func InitSuiteEnvironment(ctx context.Context, cfg Config) (*Environment, error) {
	logger := SuiteLogger()
	suiteEnvOnce.Do(func() {
		if cfg.SkipSuite {
			suiteEnvErr = ErrSuiteSkipped
			return
		}
		env := &Environment{cfg: cfg, logger: logger, httpClient: &http.Client{Timeout: 30 * time.Second}}
		if err := env.ensureRepoRoot(); err != nil {
			suiteEnvErr = err
			return
		}
		if err := env.ensureTempDir(); err != nil {
			suiteEnvErr = err
			return
		}
		if err := env.ensureKindCluster(ctx); err != nil {
			suiteEnvErr = err
			return
		}
		if err := env.ensureKubeClient(); err != nil {
			suiteEnvErr = err
			return
		}
		if err := env.ensureImages(ctx); err != nil {
			suiteEnvErr = err
			return
		}
		if err := env.ensureManager(ctx); err != nil {
			suiteEnvErr = err
			return
		}
		suiteEnv = env
	})
	if errors.Is(suiteEnvErr, ErrSuiteSkipped) {
		return nil, ErrSuiteSkipped
	}
	return suiteEnv, suiteEnvErr
}

// SuiteEnvironment returns the shared environment for tests.
func SuiteEnvironment() *Environment {
	return suiteEnv
}

func (e *Environment) ensureTempDir() error {
	dir, err := os.MkdirTemp("", "kubenova-e2e-")
	if err != nil {
		return err
	}
	e.tempDir = dir
	return nil
}

func (e *Environment) ensureKindCluster(ctx context.Context) error {
	e.logger.Info("ensure_kind_cluster.start", slog.String("cluster", e.cfg.ClusterName))
	if e.cfg.UseExistingCluster {
		e.logger.Info("ensure_kind_cluster.skip_existing")
		return nil
	}
	if e.clusterExists(ctx) {
		e.logger.Info("ensure_kind_cluster.reuse")
		return nil
	}
	cmd, err := e.command(ctx, e.cfg.KindBinary, "kind", "create", "cluster", "--name", e.cfg.ClusterName, "--config", "kind/kind-config.yaml")
	if err != nil {
		return err
	}
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("kind create cluster: %w", err)
	}
	e.createdCluster = true
	e.logger.Info("ensure_kind_cluster.created")
	return nil
}

func (e *Environment) clusterExists(ctx context.Context) bool {
	cmd, err := e.command(ctx, e.cfg.KindBinary, "kind", "get", "clusters")
	if err != nil {
		e.logger.Error("ensure_kind_cluster.kind_binary", slog.String("error", err.Error()))
		return false
	}
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	clusters := strings.Fields(string(out))
	for _, name := range clusters {
		if name == e.cfg.ClusterName {
			return true
		}
	}
	return false
}

func (e *Environment) ensureKubeClient() error {
	e.logger.Info("ensure_kube_client.start")
	cmd, err := e.command(context.Background(), e.cfg.KindBinary, "kind", "get", "kubeconfig", "--name", e.cfg.ClusterName)
	if err != nil {
		return err
	}
	raw, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("kind get kubeconfig: %w", err)
	}
	kubeconfig := filepath.Join(e.tempDir, "kubeconfig")
	if err := os.WriteFile(kubeconfig, raw, 0o600); err != nil {
		return err
	}
	e.kubeconfigPath = kubeconfig
	cfg, err := clientcmd.RESTConfigFromKubeConfig(raw)
	if err != nil {
		return err
	}
	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return err
	}
	e.restConfig = cfg
	e.kubeClient = clientset
	e.logger.Info("ensure_kube_client.ready")
	return nil
}

func (e *Environment) ensureImages(ctx context.Context) error {
	if !e.cfg.BuildImages {
		return nil
	}
	tag := fmt.Sprintf("e2e-%d", time.Now().UnixNano())
	managerImage := fmt.Sprintf("kubenova-manager:%s", tag)
	agentImage := fmt.Sprintf("kubenova-agent:%s", tag)
	builds := []struct {
		image      string
		dockerfile string
	}{
		{image: managerImage, dockerfile: "build/Dockerfile.manager"},
		{image: agentImage, dockerfile: "build/Dockerfile.agent"},
	}
	for _, step := range builds {
		e.logger.Info("image.build", slog.String("image", step.image), slog.String("dockerfile", step.dockerfile))
		cmd, err := e.command(ctx, e.cfg.DockerBinary, "docker", "build", "-t", step.image, "-f", step.dockerfile, ".")
		if err != nil {
			return err
		}
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("docker build %s: %w", step.image, err)
		}
	}
	args := []string{"load", "docker-image", "--name", e.cfg.ClusterName, managerImage, agentImage}
	e.logger.Info("kind.load_images", slog.Any("images", []string{managerImage, agentImage}))
	cmd, err := e.command(ctx, e.cfg.KindBinary, "kind", args...)
	if err != nil {
		return err
	}
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("kind load docker-image: %w", err)
	}
	e.cfg.ManagerImage = managerImage
	e.cfg.AgentImage = agentImage
	return nil
}

func (e *Environment) ensureManager(ctx context.Context) error {
	e.logger.Info("ensure_manager.start")
	args := []string{"upgrade", "--install", e.cfg.ManagerReleaseName, e.cfg.ManagerChartPath, "-n", e.cfg.ManagerNamespace, "--create-namespace",
		"--set", fmt.Sprintf("image.repository=%s", repositoryPart(e.cfg.ManagerImage)),
		"--set", fmt.Sprintf("image.tag=%s", tagPart(e.cfg.ManagerImage)),
		"--set", "env.KUBENOVA_REQUIRE_AUTH=false",
		"--set", fmt.Sprintf("env.AGENT_IMAGE=%s", e.cfg.AgentImage),
		"--wait", "--timeout", e.cfg.WaitTimeout.String(),
	}
	cmd, err := e.command(ctx, e.cfg.HelmBinary, "helm", args...)
	if err != nil {
		return err
	}
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("helm install manager: %w", err)
	}
	e.logger.Info("ensure_manager.deployed")
	if err := e.waitForDeployment(ctx, e.cfg.ManagerNamespace, e.cfg.ManagerReleaseName); err != nil {
		return err
	}
	if err := e.startPortForward(ctx); err != nil {
		return err
	}
	if err := e.waitForManagerHealth(ctx); err != nil {
		return err
	}
	return nil
}

func (e *Environment) waitForDeployment(ctx context.Context, namespace, name string) error {
	e.logger.Info("deployment_wait", slog.String("namespace", namespace), slog.String("name", name))
	return wait.PollUntilContextTimeout(ctx, 2*time.Second, e.cfg.WaitTimeout, true, func(ctx context.Context) (bool, error) {
		dep, err := e.kubeClient.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
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
}

func (e *Environment) startPortForward(ctx context.Context) error {
	var startErr error
	e.portForwardOnce.Do(func() {
		local := fmt.Sprintf("%d:8080", e.cfg.PortForwardPort)
		args := []string{"port-forward", "-n", e.cfg.ManagerNamespace, e.cfg.ManagerService(), local}
		pfCtx, cancel := context.WithCancel(context.Background())
		cmd, err := e.command(pfCtx, e.cfg.KubectlBinary, "kubectl", args...)
		if err != nil {
			cancel()
			startErr = err
			return
		}
		stdout, _ := cmd.StdoutPipe()
		stderr, _ := cmd.StderrPipe()
		if err := cmd.Start(); err != nil {
			cancel()
			startErr = fmt.Errorf("kubectl port-forward start: %w", err)
			return
		}
		go func() {
			_, _ = io.Copy(os.Stdout, stdout)
		}()
		go func() {
			_, _ = io.Copy(os.Stderr, stderr)
		}()
		readyCtx, readyCancel := context.WithTimeout(ctx, 30*time.Second)
		defer readyCancel()
		for {
			select {
			case <-readyCtx.Done():
				cancel()
				startErr = fmt.Errorf("port-forward readiness timeout: %w", readyCtx.Err())
				return
			default:
			}
			conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", e.cfg.PortForwardPort), 1*time.Second)
			if err == nil {
				_ = conn.Close()
				e.portForwardCmd = cmd
				e.portForwardStop = cancel
				e.logger.Info("port_forward.ready", slog.Int("port", e.cfg.PortForwardPort))
				return
			}
			time.Sleep(500 * time.Millisecond)
		}
	})
	return startErr
}

func (e *Environment) waitForManagerHealth(ctx context.Context) error {
	url := fmt.Sprintf("http://127.0.0.1:%d/wait?timeout=60", e.cfg.PortForwardPort)
	e.logger.Info("manager_health.wait", slog.String("url", url))
	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		resp, err := e.httpClient.Do(req)
		if err == nil && resp.StatusCode == http.StatusOK {
			_ = resp.Body.Close()
			e.logger.Info("manager_health.ready")
			return nil
		}
		if resp != nil {
			_ = resp.Body.Close()
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("manager health check failed at %s", url)
}

// EnsureClusterRegistered registers the suite cluster once and returns metadata.
func (e *Environment) EnsureClusterRegistered(ctx context.Context, name string) (ClusterInfo, error) {
	e.registerOnce.Do(func() {
		payload, err := e.buildRegistrationPayload(ctx, name)
		if err != nil {
			e.registerErr = err
			return
		}
		id, err := e.registerCluster(ctx, payload)
		if err != nil {
			e.registerErr = err
			return
		}
		e.clusterInfo = ClusterInfo{ID: id, Name: name}
	})
	return e.clusterInfo, e.registerErr
}

type registrationPayload struct {
	Name          string `json:"name"`
	KubeconfigB64 string `json:"kubeconfig"`
}

func (e *Environment) buildRegistrationPayload(ctx context.Context, name string) (registrationPayload, error) {
	caData, server, err := e.clusterAccessDetails()
	if err != nil {
		return registrationPayload{}, err
	}
	token, err := e.ensureServiceAccountToken(ctx)
	if err != nil {
		return registrationPayload{}, err
	}
	cfg := &clientcmdapi.Config{
		Kind:       "Config",
		APIVersion: "v1",
		Clusters: map[string]*clientcmdapi.Cluster{
			"kubenova": {
				Server:                   server,
				CertificateAuthorityData: caData,
			},
		},
		AuthInfos: map[string]*clientcmdapi.AuthInfo{
			"kubenova": {
				Token: token,
			},
		},
		Contexts: map[string]*clientcmdapi.Context{
			"kubenova": {
				Cluster:  "kubenova",
				AuthInfo: "kubenova",
			},
		},
		CurrentContext: "kubenova",
	}
	b, err := clientcmd.Write(*cfg)
	if err != nil {
		return registrationPayload{}, err
	}
	return registrationPayload{Name: name, KubeconfigB64: base64.StdEncoding.EncodeToString(b)}, nil
}

func (e *Environment) clusterAccessDetails() ([]byte, string, error) {
	raw, err := os.ReadFile(e.kubeconfigPath)
	if err != nil {
		return nil, "", err
	}
	cfg, err := clientcmd.Load(raw)
	if err != nil {
		return nil, "", err
	}
	for _, cluster := range cfg.Clusters {
		if len(cluster.CertificateAuthorityData) > 0 {
			return cluster.CertificateAuthorityData, "https://kubernetes.default.svc", nil
		}
	}
	return nil, "", fmt.Errorf("certificate authority data not found")
}

func (e *Environment) ensureServiceAccountToken(ctx context.Context) (string, error) {
	name := "kubenova-e2e-manager"
	namespace := "kube-system"
	e.logger.Info("serviceaccount.ensure", slog.String("name", name))
	_, err := e.kubeClient.CoreV1().ServiceAccounts(namespace).Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = e.kubeClient.CoreV1().ServiceAccounts(namespace).Create(ctx, &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: name}}, metav1.CreateOptions{})
		if err != nil && !apierrors.IsAlreadyExists(err) {
			return "", err
		}
	} else if err != nil {
		return "", err
	}
	bindingName := "kubenova-e2e-manager"
	_, err = e.kubeClient.RbacV1().ClusterRoleBindings().Get(ctx, bindingName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = e.kubeClient.RbacV1().ClusterRoleBindings().Create(ctx, &rbacv1.ClusterRoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: bindingName},
			Subjects:   []rbacv1.Subject{{Kind: "ServiceAccount", Name: name, Namespace: namespace}},
			RoleRef:    rbacv1.RoleRef{APIGroup: "rbac.authorization.k8s.io", Kind: "ClusterRole", Name: "cluster-admin"},
		}, metav1.CreateOptions{})
		if err != nil && !apierrors.IsAlreadyExists(err) {
			return "", err
		}
	} else if err != nil {
		return "", err
	}
	tok, err := e.kubeClient.CoreV1().ServiceAccounts(namespace).CreateToken(ctx, name, &authenticationv1.TokenRequest{Spec: authenticationv1.TokenRequestSpec{Audiences: []string{"kubernetes.default.svc"}}}, metav1.CreateOptions{})
	if err != nil {
		return "", err
	}
	if tok.Status.Token == "" {
		return "", fmt.Errorf("received empty token")
	}
	return tok.Status.Token, nil
}

func (e *Environment) registerCluster(ctx context.Context, payload registrationPayload) (int, error) {
	url := fmt.Sprintf("http://127.0.0.1:%d/api/v1/clusters", e.cfg.PortForwardPort)
	e.logger.Info("cluster_register.start", slog.String("url", url))
	body, err := json.Marshal(payload)
	if err != nil {
		return 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(string(body)))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := e.httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("cluster registration failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	var result struct {
		ID int `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, err
	}
	e.logger.Info("cluster_register.success", slog.Int("id", result.ID))
	return result.ID, nil
}

// Teardown releases resources created during setup.
func (e *Environment) Teardown(ctx context.Context) {
	if e == nil {
		return
	}
	e.logger.Info("suite.teardown.start")
	if e.portForwardStop != nil {
		e.logger.Info("suite.teardown.port_forward")
		e.portForwardStop()
		if e.portForwardCmd != nil {
			_ = e.portForwardCmd.Wait()
		}
	}
	if !e.cfg.SkipCleanup {
		if cmd, err := e.command(ctx, e.cfg.HelmBinary, "helm", "uninstall", e.cfg.ManagerReleaseName, "-n", e.cfg.ManagerNamespace); err == nil {
			_ = cmd.Run()
		} else {
			e.logger.Error("suite.teardown.helm_binary", slog.String("error", err.Error()))
		}
		if e.createdCluster {
			if cmd, err := e.command(ctx, e.cfg.KindBinary, "kind", "delete", "cluster", "--name", e.cfg.ClusterName); err == nil {
				_ = cmd.Run()
			} else {
				e.logger.Error("suite.teardown.kind_binary", slog.String("error", err.Error()))
			}
		}
	}
	if e.tempDir != "" {
		_ = os.RemoveAll(e.tempDir)
	}
	e.logger.Info("suite.teardown.complete")
}

func repositoryPart(image string) string {
	parts := strings.Split(image, ":")
	if len(parts) <= 1 {
		return image
	}
	return strings.Join(parts[:len(parts)-1], ":")
}

func tagPart(image string) string {
	parts := strings.Split(image, ":")
	if len(parts) == 0 {
		return ""
	}
	return parts[len(parts)-1]
}

func (e *Environment) ManagerBaseURL() string {
	return fmt.Sprintf("http://127.0.0.1:%d", e.cfg.PortForwardPort)
}

func (e *Environment) HTTPClient() *http.Client {
	return e.httpClient
}

func (e *Environment) KubeClient() kubernetes.Interface {
	return e.kubeClient
}

func (e *Environment) Config() Config {
	return e.cfg
}

func (e *Environment) Logger() *slog.Logger {
	return e.logger
}

func (e *Environment) command(ctx context.Context, binary, allowed string, args ...string) (*exec.Cmd, error) {
	sanitized, err := sanitizeBinary(binary, allowed)
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, sanitized, args...) // #nosec G204 -- binary name validated by sanitizeBinary
	if e.cfg.RepositoryRoot != "" {
		cmd.Dir = e.cfg.RepositoryRoot
	}
	return cmd, nil
}

func (e *Environment) ensureRepoRoot() error {
	if e.cfg.RepositoryRoot == "" {
		return fmt.Errorf("repository root is empty")
	}
	info, err := os.Stat(e.cfg.RepositoryRoot)
	if err != nil {
		return fmt.Errorf("repository root: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("repository root is not a directory: %s", e.cfg.RepositoryRoot)
	}
	return nil
}

func sanitizeBinary(path, allowed string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("binary path is empty")
	}
	cleaned := filepath.Clean(path)
	if filepath.Base(cleaned) != allowed {
		return "", fmt.Errorf("unsupported binary %q (expected %s)", cleaned, allowed)
	}
	return cleaned, nil
}
