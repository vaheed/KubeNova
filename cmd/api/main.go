package main

import (
    "context"
    "net/http"
    "os"
    "time"

    "github.com/vaheed/kubenova/internal/api"
    "github.com/vaheed/kubenova/internal/logging"
    "github.com/vaheed/kubenova/internal/store"
    "github.com/vaheed/kubenova/internal/util"
    "go.uber.org/zap"
)

func main() {
    var (
        st store.Store
        closeFn func(context.Context) error
        err error
    )
    // Prefer Postgres if DATABASE_URL set; retry briefly so compose DB can come up.
    if os.Getenv("DATABASE_URL") != "" {
        var pst store.Store
        var pclose func(context.Context) error
        err := util.Retry(60*time.Second, func() (bool, error) {
            s, c, e := store.EnvOrMemory()
            if e != nil {
                return true, e
            }
            // If we got memory while DATABASE_URL was set, retry
            // but EnvOrMemory returns Postgres when DATABASE_URL is set; keep guard anyway
            pst, pclose = s, c
            return false, nil
        })
        if err != nil { logging.L.Fatal("postgres connect", zap.Error(err)) }
        st, closeFn = pst, pclose
    } else {
        st, closeFn, err = store.EnvOrMemory()
        if err != nil { logging.L.Fatal("store init", zap.Error(err)) }
    }
    defer closeFn(context.Background())

    srv := api.NewServer(st)
    s := &http.Server{
        Addr:              ":8080",
        Handler:           srv.Router(),
        ReadHeaderTimeout: 5 * time.Second,
        ReadTimeout:       30 * time.Second,
        WriteTimeout:      30 * time.Second,
        IdleTimeout:       60 * time.Second,
    }
    logging.L.Info("KubeNova API listening", zap.String("addr", s.Addr))
    if err := api.StartHTTP(context.Background(), s); err != nil && err != http.ErrServerClosed {
        logging.L.Error("server error", zap.Error(err))
        os.Exit(1)
    }
    time.Sleep(100 * time.Millisecond)
}
