package setup

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"time"
)

// Config captures environment options for the E2E suite.
type Config struct {
	ClusterName        string
	KindBinary         string
	HelmBinary         string
	KubectlBinary      string
	DockerBinary       string
	RepositoryRoot     string
	ManagerChartPath   string
	ManagerImage       string
	AgentImage         string
	ManagerNamespace   string
	ManagerReleaseName string
	PortForwardPort    int
	UseExistingCluster bool
	SkipCleanup        bool
	SkipSuite          bool
	BuildImages        bool
	WaitTimeout        time.Duration
}

func LoadConfig() Config {
	repoRoot := defaultRepoRoot()
	runSuite, runSet := lookupEnvBool("E2E_RUN")
	skipSuite := true
	if runSet {
		skipSuite = !runSuite
	}
	if skip, ok := lookupEnvBool("E2E_SKIP"); ok {
		skipSuite = skip
	}
	cfg := Config{
		ClusterName:        getenvDefault("E2E_KIND_CLUSTER", "kubenova-e2e"),
		KindBinary:         getenvDefault("E2E_KIND_BIN", "kind"),
		HelmBinary:         getenvDefault("E2E_HELM_BIN", "helm"),
		KubectlBinary:      getenvDefault("E2E_KUBECTL_BIN", "kubectl"),
		DockerBinary:       getenvDefault("E2E_DOCKER_BIN", "docker"),
		RepositoryRoot:     getenvDefault("E2E_REPO_ROOT", repoRoot),
		ManagerChartPath:   getenvDefault("E2E_MANAGER_CHART", "deploy/helm/manager"),
		ManagerImage:       getenvDefault("E2E_MANAGER_IMAGE", "ghcr.io/vaheed/kubenova/manager:dev"),
		AgentImage:         getenvDefault("E2E_AGENT_IMAGE", "ghcr.io/vaheed/kubenova/agent:dev"),
		ManagerNamespace:   getenvDefault("E2E_MANAGER_NAMESPACE", "kubenova-system"),
		ManagerReleaseName: getenvDefault("E2E_MANAGER_RELEASE", "kubenova-manager"),
		PortForwardPort:    getenvInt("E2E_MANAGER_PORT", 18080),
		UseExistingCluster: getenvBool("E2E_USE_EXISTING_CLUSTER"),
		SkipCleanup:        getenvBool("E2E_SKIP_CLEANUP"),
		SkipSuite:          skipSuite,
		BuildImages:        getenvBool("E2E_BUILD_IMAGES"),
		WaitTimeout:        getenvDuration("E2E_WAIT_TIMEOUT", 20*time.Minute),
	}
	return cfg
}

func getenvDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getenvBool(key string) bool {
	b, _ := lookupEnvBool(key)
	return b
}

func lookupEnvBool(key string) (bool, bool) {
	if v, ok := os.LookupEnv(key); ok {
		if v == "" {
			return false, false
		}
		b, err := strconv.ParseBool(v)
		if err == nil {
			return b, true
		}
		return false, true
	}
	return false, false
}

func getenvInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func getenvDuration(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		d, err := time.ParseDuration(v)
		if err == nil {
			return d
		}
	}
	return def
}

func defaultRepoRoot() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "."
	}
	root := filepath.Join(filepath.Dir(file), "..", "..", "..")
	abs, err := filepath.Abs(root)
	if err != nil {
		return root
	}
	return abs
}

func (c Config) ManagerService() string {
	return fmt.Sprintf("svc/%s", c.ManagerReleaseName)
}
