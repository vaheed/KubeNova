package logging

import (
	"sync"

	"go.uber.org/zap"
)

var (
	// L is the shared structured logger used across the project.
	L    *zap.Logger
	once sync.Once
)

func init() {
	Init()
}

// Init builds the global logger if it has not been constructed yet.
// It uses zap's production configuration for consistent structured output.
func Init() {
	once.Do(func() {
		cfg := zap.NewProductionConfig()
		cfg.Level = zap.NewAtomicLevelAt(zap.DebugLevel)
		cfg.Sampling = nil
		logger, err := cfg.Build()
		if err != nil {
			panic(err)
		}
		L = logger
	})
}
