package main

import (
	"context"
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
	"sigs.k8s.io/controller-runtime/pkg/manager"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/vaheed/kubenova/internal/logging"
	"github.com/vaheed/kubenova/internal/observability"
	"github.com/vaheed/kubenova/internal/reconcile"
	"github.com/vaheed/kubenova/internal/telemetry"
	"go.uber.org/zap"

	capsulebackend "github.com/vaheed/kubenova/internal/backends/capsule"
	velabackend "github.com/vaheed/kubenova/internal/backends/vela"
	v1alpha1 "github.com/vaheed/kubenova/pkg/api/v1alpha1"
)

func main() {
	shutdownTrace := func(context.Context) error { return nil }
	if closer, err := observability.SetupOTel(context.Background(), observability.Config{
		ServiceName:    "kubenova-operator",
		ServiceVersion: os.Getenv("KUBENOVA_VERSION"),
		Environment:    os.Getenv("KUBENOVA_ENV"),
	}); err != nil {
		logging.L.Warn("otel_setup_failed", zap.Error(err))
	} else {
		shutdownTrace = closer
		defer func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = shutdownTrace(ctx)
		}()
	}
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(corev1.AddToScheme(scheme))
	capsulebackend.AddToScheme(scheme)
	velabackend.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

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
	buf := telemetry.NewRedisBuffer(os.Getenv("MANAGER_URL"))
	buf.Run()
	defer buf.Stop()
	telemetry.SetGlobal(buf)
	buf.Enqueue("events", map[string]string{"event": "operator_started"})

	if err := mgr.Add(manager.RunnableFunc(func(ctx context.Context) error {
		if !mgr.GetCache().WaitForCacheSync(ctx) {
			return fmt.Errorf("manager cache failed to sync")
		}
		if err := reconcile.BootstrapHelmJob(ctx, mgr.GetClient(), mgr.GetAPIReader(), mgr.GetScheme()); err != nil {
			logging.L.Error("bootstrap error", zap.Error(err))
		}
		interval := time.Duration(getEnvInt("COMPONENT_RECONCILE_SECONDS", 300)) * time.Second
		go func() {
			if err := reconcile.PeriodicComponentReconciler(ctx, mgr.GetClient(), mgr.GetAPIReader(), mgr.GetScheme(), interval); err != nil && ctx.Err() == nil {
				logging.L.Error("component_reconcile_loop_failed", zap.Error(err))
			}
		}()
		return nil
	})); err != nil {
		logging.L.Fatal("bootstrap runnable", zap.Error(err))
	}

	// Single shared context for shutdown
	ctx := ctrl.SetupSignalHandler()

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
