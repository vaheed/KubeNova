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
    for {
        retry, err := fn()
        if !retry || time.Since(start) > max { return err }
        // exponential + jitter (crypto/rand), cap at 30s, and cap shift to avoid overflow
        capShift := 5 // 1s << 5 = 32s (~ cap of 30s)
        if attempt > capShift { attempt = capShift }
        sleep := time.Second << uint(attempt)
        if sleep > 30*time.Second { sleep = 30 * time.Second }
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
