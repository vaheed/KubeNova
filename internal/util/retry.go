package util

import (
	"time"
)

// Retry executes fn until it returns retry=false or the timeout elapses.
// It waits with jittered backoff between attempts and surfaces the last error.
func Retry(timeout time.Duration, fn func() (retry bool, err error)) error {
	deadline := time.Now().Add(timeout)
	backoff := 200 * time.Millisecond

	var lastErr error
	for {
		retry, err := fn()
		if !retry {
			return err
		}
		lastErr = err
		if time.Now().After(deadline) {
			if lastErr != nil {
				return lastErr
			}
			return err
		}
		time.Sleep(backoff)
		if backoff < 2*time.Second {
			backoff *= 2
		}
	}
}
