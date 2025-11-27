package main

import (
	"context"
	"net/http"
	"os"
	"time"

	"github.com/vaheed/kubenova/internal/logging"
	mngr "github.com/vaheed/kubenova/internal/manager"
	"github.com/vaheed/kubenova/internal/observability"
	"github.com/vaheed/kubenova/internal/store"
	"github.com/vaheed/kubenova/internal/util"
	"go.uber.org/zap"
)

func main() {
	var (
		st      store.Store
		closeFn func(context.Context) error
		err     error
	)
	shutdownTrace := func(context.Context) error { return nil }
	if closer, err := observability.SetupOTel(context.Background(), observability.Config{
		ServiceName:    "kubenova-manager",
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
	// Validate required env
	if v := os.Getenv("KUBENOVA_REQUIRE_AUTH"); v == "true" || v == "1" || v == "t" || v == "on" || v == "yes" || v == "y" {
		if os.Getenv("JWT_SIGNING_KEY") == "" {
			logging.L.Fatal("missing required env for auth", zap.String("env", "JWT_SIGNING_KEY"))
		}
	}
	// Require DATABASE_URL to be set; no in-memory fallback
	if os.Getenv("DATABASE_URL") == "" {
		logging.L.Fatal("missing required env", zap.String("env", "DATABASE_URL"))
	}
	var pst store.Store
	var pclose func(context.Context) error
	err = util.Retry(60*time.Second, func() (bool, error) {
		s, c, e := store.EnvOrMemory() // EnvOrMemory returns Postgres when DATABASE_URL is set
		if e != nil {
			return true, e
		}
		pst, pclose = s, c
		return false, nil
	})
	if err != nil {
		logging.L.Fatal("postgres connect", zap.Error(err))
	}
	st, closeFn = pst, pclose
	defer closeFn(context.Background())
	if err := st.Health(context.Background()); err != nil {
		logging.L.Fatal("store health check", zap.Error(err))
	}

	srv := mngr.NewServer(st)
	s := &http.Server{
		Addr:              ":8080",
		Handler:           srv.Router(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	logging.L.Info("KubeNova Manager listening", zap.String("addr", s.Addr))
	if err := mngr.StartHTTP(context.Background(), s); err != nil && err != http.ErrServerClosed {
		logging.L.Error("server error", zap.Error(err))
		os.Exit(1)
	}
	time.Sleep(100 * time.Millisecond)
}
