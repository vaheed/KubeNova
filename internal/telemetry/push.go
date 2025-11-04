package telemetry

import (
	"log"
	"net/http"
	"os"
	"time"
)

// Stopper holds the current stopper function for background routines.
var Stopper = func() {}

// Very small heartbeat/batch pusher used by the Agent. It intentionally
// remains simple for CI smoke purposes; production would push real data.

func StartHeartbeat(client *http.Client, managerURL string, interval time.Duration) func() {
	if client == nil {
		client = http.DefaultClient
	}
	stop := make(chan struct{})
	if managerURL == "" {
		managerURL = os.Getenv("MANAGER_URL")
	}
	if interval == 0 {
		interval = 15 * time.Second
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				req, _ := http.NewRequest(http.MethodPost, managerURL+"/sync/metrics", nil)
				resp, err := client.Do(req)
				if err != nil {
					log.Printf("heartbeat error: %v", err)
					continue
				}
				_ = resp.Body.Close()
			case <-stop:
				return
			}
		}
	}()
	return func() { close(stop) }
}
