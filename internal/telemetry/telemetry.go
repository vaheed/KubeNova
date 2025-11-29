package telemetry

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
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

// SpoolBuffer persists events to disk so they can be replayed when the manager returns.
type SpoolBuffer struct {
	dir           string
	managerURL    string
	client        *http.Client
	stop          chan struct{}
	flushInterval time.Duration
	seq           uint64
}

// NewSpoolBuffer returns a buffer that stores messages on disk and flushes them when the manager is reachable.
func NewSpoolBuffer(managerURL, dir string) *SpoolBuffer {
	if dir == "" {
		dir = filepath.Join(os.TempDir(), "kubenova", "telemetry")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		logging.L.Warn("telemetry_spool_mkdir_failed", zap.Error(err), zap.String("dir", dir))
	}
	return &SpoolBuffer{
		dir:           dir,
		managerURL:    strings.TrimRight(managerURL, "/"),
		client:        &http.Client{Timeout: 5 * time.Second},
		stop:          make(chan struct{}),
		flushInterval: 5 * time.Second,
	}
}

// Run starts the background flush loop.
func (b *SpoolBuffer) Run() {
	go b.flushLoop()
}

// Stop stops the spool loop and flushes remaining events.
func (b *SpoolBuffer) Stop() {
	close(b.stop)
	b.flush()
}

// Enqueue stores the event on disk for later delivery.
func (b *SpoolBuffer) Enqueue(stream string, payload map[string]string) {
	if payload == nil {
		payload = map[string]string{}
	}
	payload["stream"] = stream
	if err := b.storeEvent(payload); err != nil {
		logging.L.Warn("telemetry_spool_store_failed", zap.Error(err))
	}
}

func (b *SpoolBuffer) flushLoop() {
	if b.flushInterval <= 0 {
		b.flushInterval = 5 * time.Second
	}
	ticker := time.NewTicker(b.flushInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			b.flush()
		case <-b.stop:
			b.flush()
			return
		}
	}
}

func (b *SpoolBuffer) storeEvent(payload map[string]string) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}
	tmp, err := os.CreateTemp(b.dir, "event-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return fmt.Errorf("write payload: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return fmt.Errorf("close payload: %w", err)
	}
	name := fmt.Sprintf("%020d-%d.json", time.Now().UnixNano(), atomic.AddUint64(&b.seq, 1))
	final := filepath.Join(b.dir, name)
	if err := os.Rename(tmp.Name(), final); err != nil {
		os.Remove(tmp.Name())
		return fmt.Errorf("rename payload: %w", err)
	}
	return nil
}

func (b *SpoolBuffer) flush() {
	if b.managerURL == "" {
		return
	}
	entries, err := os.ReadDir(b.dir)
	if err != nil {
		logging.L.Warn("telemetry_spool_read_failed", zap.Error(err))
		return
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		full := filepath.Join(b.dir, entry.Name())
		data, err := os.ReadFile(full)
		if err != nil {
			logging.L.Warn("telemetry_spool_read_file_failed", zap.Error(err), zap.String("path", full))
			continue
		}
		req, err := http.NewRequest(http.MethodPost, b.managerURL+"/api/v1/telemetry/events", bytes.NewReader(data))
		if err != nil {
			logging.L.Warn("telemetry_spool_request_failed", zap.Error(err), zap.String("path", full))
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := b.client.Do(req)
		if err != nil {
			logging.L.Warn("telemetry_spool_forward_failed", zap.String("url", req.URL.String()), zap.Error(err))
			return
		}
		resp.Body.Close()
		if resp.StatusCode >= 300 {
			logging.L.Warn("telemetry_spool_non2xx", zap.String("url", req.URL.String()), zap.Int("status", resp.StatusCode))
			return
		}
		_ = os.Remove(full)
	}
}

type noopBuffer struct{}

func (noopBuffer) Enqueue(stream string, payload map[string]string) {}
func (noopBuffer) Run()                                             {}
func (noopBuffer) Stop()                                            {}
