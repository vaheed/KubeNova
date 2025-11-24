package main

import (
	"fmt"
	"os"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	crzap "sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/vaheed/kubenova/internal/logging"
	"github.com/vaheed/kubenova/internal/reconcile"
	"github.com/vaheed/kubenova/internal/telemetry"
	"go.uber.org/zap"
)

func main() {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(corev1.AddToScheme(scheme))

	ctrl.SetLogger(crzap.New(crzap.UseDevMode(true)))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsserver.Options{BindAddress: ":8081"},
		HealthProbeBindAddress: ":8082",
		LeaderElection:         true,
		LeaderElectionID:       "kubenova-operator-leader",
	})
	if err != nil {
		logging.L.Fatal("manager", zap.Error(err))
	}

	if err := (&reconcile.ProjectReconciler{Client: mgr.GetClient()}).SetupWithManager(mgr); err != nil {
		logging.L.Fatal("project reconciler", zap.Error(err))
	}
	// Placeholders for future controllers
	_ = (&reconcile.TenantReconciler{Client: mgr.GetClient()}).SetupWithManager(mgr)
	_ = (&reconcile.AppReconciler{Client: mgr.GetClient()}).SetupWithManager(mgr)

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		logging.L.Fatal("healthz", zap.Error(err))
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		logging.L.Fatal("readyz", zap.Error(err))
	}

	// Heartbeat to manager for smoke observability
	telemetry.Stopper = telemetry.StartHeartbeat(nil, os.Getenv("MANAGER_URL"), time.Duration(getEnvInt("BATCH_INTERVAL_SECONDS", 10))*time.Second)
	// Start redis buffer if available
	buf := telemetry.NewRedisBuffer()
	buf.Run()
	defer buf.Stop()
	telemetry.SetGlobal(buf)
	buf.Enqueue("events", map[string]string{"event": "operator_started"})

	// Single shared context for shutdown
	ctx := ctrl.SetupSignalHandler()

	// Bootstrap addons via a Helm job if not present; then verify readiness
	go func() {
		if err := reconcile.BootstrapHelmJob(ctx); err != nil {
			logging.L.Error("bootstrap error", zap.Error(err))
		}
	}()

	logging.L.Info("KubeNova Operator starting manager")
	if err := mgr.Start(ctx); err != nil {
		logging.L.Fatal("manager stopped", zap.Error(err))
	}
}

func getEnvInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	var n int
	_, err := fmt.Sscanf(v, "%d", &n)
	if err != nil {
		return def
	}
	return n
}
