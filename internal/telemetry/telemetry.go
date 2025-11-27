package telemetry

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/vaheed/kubenova/internal/logging"
	"go.uber.org/zap"
)

// Stopper cancels the global heartbeat loop.
var Stopper func()

var globalBuffer Buffer = noopBuffer{}

// Buffer is a minimal interface for async telemetry pipelines.
type Buffer interface {
	Enqueue(stream string, payload map[string]string)
	Run()
	Stop()
}

// Emit sends a telemetry event to the configured buffer.
func Emit(stream string, payload map[string]string) {
	globalBuffer.Enqueue(stream, payload)
}

// SetGlobal overrides the process-wide telemetry buffer.
func SetGlobal(buf Buffer) {
	if buf != nil {
		globalBuffer = buf
	}
}

// StartHeartbeat emits a periodic heartbeat to the manager URL.
func StartHeartbeat(ctx context.Context, managerURL string, interval time.Duration) func() {
	if ctx == nil {
		ctx = context.Background()
	}
	if interval <= 0 {
		interval = 10 * time.Second
	}
	stopCh := make(chan struct{})
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				logging.L.Info("heartbeat", zap.String("manager", managerURL))
			case <-stopCh:
				return
			case <-ctx.Done():
				return
			}
		}
	}()
	return func() {
		close(stopCh)
	}
}

// RedisBuffer is a stubbed buffer that keeps events in memory.
type RedisBuffer struct {
	events     chan map[string]string
	stop       chan struct{}
	managerURL string
	client     *http.Client
}

// NewRedisBuffer returns a buffer that stores messages in memory.
func NewRedisBuffer(managerURL string) *RedisBuffer {
	return &RedisBuffer{
		events:     make(chan map[string]string, 256),
		stop:       make(chan struct{}),
		managerURL: strings.TrimRight(managerURL, "/"),
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

// Run starts the background worker.
func (b *RedisBuffer) Run() {
	go func() {
		for {
			select {
			case ev := <-b.events:
				logging.L.Info("telemetry_event", zap.Any("payload", ev))
				b.forward(ev)
			case <-b.stop:
				return
			}
		}
	}()
}

// Stop stops the worker.
func (b *RedisBuffer) Stop() {
	close(b.stop)
}

// Enqueue adds a message to the buffer.
func (b *RedisBuffer) Enqueue(stream string, payload map[string]string) {
	if payload == nil {
		payload = map[string]string{}
	}
	payload["stream"] = stream
	select {
	case b.events <- payload:
	default:
		logging.L.Warn("telemetry buffer full, dropping event")
	}
}

func (b *RedisBuffer) forward(ev map[string]string) {
	if b.managerURL == "" {
		return
	}
	body, err := json.Marshal(ev)
	if err != nil {
		logging.L.Warn("telemetry_forward_encode_failed", zap.Error(err))
		return
	}
	url := b.managerURL + "/api/v1/telemetry/events"
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		logging.L.Warn("telemetry_forward_request_failed", zap.Error(err))
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := b.client.Do(req)
	if err != nil {
		logging.L.Warn("telemetry_forward_failed", zap.String("url", url), zap.Error(err))
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		logging.L.Warn("telemetry_forward_non2xx", zap.String("url", url), zap.Int("status", resp.StatusCode))
	}
}

type noopBuffer struct{}

func (noopBuffer) Enqueue(stream string, payload map[string]string) {}
func (noopBuffer) Run()                                             {}
func (noopBuffer) Stop()                                            {}
