package util

import (
    crand "crypto/rand"
    "math/big"
    "time"
)

// Retry retries fn with exponential backoff and jitter up to max duration.
// Backoff doubles from 1s and is capped at 30s to avoid overflow.
func Retry(max time.Duration, fn func() (bool, error)) error {
    start := time.Now()
    attempt := 0
    steps := []time.Duration{1 * time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second, 30 * time.Second}
    for {
        retry, err := fn()
        if !retry || time.Since(start) > max { return err }
        // exponential + jitter (crypto/rand), via bounded steps slice (no bit shifts)
        if attempt >= len(steps) { attempt = len(steps) - 1 }
        sleep := steps[attempt]
        // jitter in [0, sleep/2)
        half := sleep / 2
        var jitter time.Duration
        if half > 0 {
            if n, err := crand.Int(crand.Reader, big.NewInt(int64(half))); err == nil {
                jitter = time.Duration(n.Int64())
            }
        }
        time.Sleep(half + jitter)
        attempt++
    }
}
