package telemetry

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"

	redis "github.com/redis/go-redis/v9"
)

// RedisBuffer batches events/metrics/logs in Redis lists and pushes to manager.
type RedisBuffer struct {
	rdb  *redis.Client
	http *http.Client
	base string
	max  int
	tick time.Duration
	stop chan struct{}
	noop bool
}

// global buffer instance for convenience publishing from subpackages
var global *RedisBuffer

// SetGlobal sets the global buffer used by helpers below.
func SetGlobal(b *RedisBuffer) { global = b }

// PublishEvent enqueues an event payload to the manager if a global buffer exists.
func PublishEvent(fields map[string]any) {
	if global == nil {
		return
	}
	global.Enqueue("events", fields)
}

// PublishStage is a convenience to publish bootstrap stages with status and message.
func PublishStage(component, stage, status, message string) {
	PublishEvent(map[string]any{
		"ts":        time.Now().UTC().Format(time.RFC3339Nano),
		"component": component,
		"stage":     stage,
		"status":    status,
		"message":   message,
	})
}

func NewRedisBuffer() *RedisBuffer {
	// If REDIS_ADDR is not set, operate in no-op mode to avoid DNS errors
	// on clusters where Redis is not deployed with the Agent.
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		return &RedisBuffer{noop: true, http: http.DefaultClient, base: getenv("MANAGER_URL", "http://kubenova-manager.kubenova.svc.cluster.local:8080"), max: getenvInt("BATCH_MAX_ITEMS", 100), tick: time.Duration(getenvInt("BATCH_INTERVAL_SECONDS", 10)) * time.Second, stop: make(chan struct{})}
	}
	rdb := redis.NewClient(&redis.Options{Addr: addr})
	base := getenv("MANAGER_URL", "http://kubenova-manager.kubenova.svc.cluster.local:8080")
	max := getenvInt("BATCH_MAX_ITEMS", 100)
	iv := time.Duration(getenvInt("BATCH_INTERVAL_SECONDS", 10)) * time.Second
	return &RedisBuffer{rdb: rdb, http: http.DefaultClient, base: base, max: max, tick: iv, stop: make(chan struct{})}
}

func getenv(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
func getenvInt(k string, d int) int {
	if v := os.Getenv(k); v != "" {
		var n int
		_, _ = fmtSscanf(v, &n)
		if n > 0 {
			return n
		}
	}
	return d
}

// small wrapper to avoid importing fmt; faster compile in limited environments
func fmtSscanf(s string, n *int) (int, error) {
	var x int
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			break
		}
		x = x*10 + int(c-'0')
	}
	*n = x
	return 1, nil
}

func (b *RedisBuffer) Enqueue(kind string, payload any) {
	if b.noop {
		return
	}
	raw, _ := json.Marshal(payload)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := b.rdb.RPush(ctx, "kubenova:"+kind, raw).Err(); err != nil {
		log.Printf("redis push %s: %v", kind, err)
	}
}

func (b *RedisBuffer) Run() {
	if b.noop {
		return
	}
	go b.loop("events")
	go b.loop("metrics")
	go b.loop("logs")
}

func (b *RedisBuffer) Stop() { close(b.stop) }

func (b *RedisBuffer) loop(kind string) {
	t := time.NewTicker(b.tick)
	defer t.Stop()
	for {
		select {
		case <-b.stop:
			return
		case <-t.C:
			b.flush(kind)
		}
	}
}

func (b *RedisBuffer) flush(kind string) {
	if b.noop {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	key := "kubenova:" + kind
	for i := 0; i < b.max; i++ {
		raw, err := b.rdb.LPop(ctx, key).Bytes()
		if err != nil {
			break
		}
		req, _ := http.NewRequest(http.MethodPost, b.base+"/sync/"+kind, bytes.NewReader(raw))
		resp, err := b.http.Do(req)
		if err != nil {
			log.Printf("push %s error: %v", kind, err)
			continue
		}
		_ = resp.Body.Close()
	}
}
