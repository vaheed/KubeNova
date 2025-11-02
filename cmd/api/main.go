package main

import (
    "context"
    "net/http"
    "os"
    "time"

    "github.com/vaheed/kubenova/internal/api"
    "github.com/vaheed/kubenova/internal/logging"
    "github.com/vaheed/kubenova/internal/store"
    "go.uber.org/zap"
)

func main() {
    var (
        st store.Store
        closeFn func(context.Context) error
        err error
    )
    st, closeFn, err = store.EnvOrMemory()
    if err != nil { logging.L.Fatal("store init", zap.Error(err)) }
    defer closeFn(context.Background())

    srv := api.NewServer(st)
    s := &http.Server{ Addr: ":8080", Handler: srv.Router() }
    logging.L.Info("KubeNova API listening", zap.String("addr", s.Addr))
    if err := api.StartHTTP(context.Background(), s); err != nil && err != http.ErrServerClosed {
        logging.L.Error("server error", zap.Error(err))
        os.Exit(1)
    }
    time.Sleep(100 * time.Millisecond)
}
