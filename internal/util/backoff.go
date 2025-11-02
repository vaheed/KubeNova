package util

import (
    "math"
    "math/rand"
    "time"
)

// Retry retries fn with exponential backoff and jitter up to max duration.
func Retry(max time.Duration, fn func() (bool, error)) error {
    start := time.Now()
    attempt := 0
    for {
        retry, err := fn()
        if !retry || time.Since(start) > max { return err }
        // exponential + jitter
        sleep := time.Duration(math.Min(float64(time.Second*1<<uint(attempt)), float64(time.Second*30)))
        jitter := time.Duration(rand.Int63n(int64(sleep/2)))
        time.Sleep(sleep/2 + jitter)
        attempt++
    }
}

