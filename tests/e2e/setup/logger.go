package setup

import (
	"io"
	"log/slog"
	"os"
	"sync"
)

var (
	suiteLogger     *slog.Logger
	suiteLoggerOnce sync.Once
)

// loggerWriter routes slog output to stdout and tests simultaneously.
type loggerWriter struct {
	writers []io.Writer
}

func (w loggerWriter) Write(p []byte) (int, error) {
	for _, dst := range w.writers {
		if dst == nil {
			continue
		}
		if _, err := dst.Write(p); err != nil {
			return 0, err
		}
	}
	return len(p), nil
}

// initSuiteLogger builds a singleton logger for the suite.
func initSuiteLogger() *slog.Logger {
	suiteLoggerOnce.Do(func() {
		handler := slog.NewTextHandler(loggerWriter{writers: []io.Writer{os.Stdout}}, &slog.HandlerOptions{Level: slog.LevelInfo})
		suiteLogger = slog.New(handler)
	})
	return suiteLogger
}

// SuiteLogger returns the shared logger instance.
func SuiteLogger() *slog.Logger {
	return initSuiteLogger()
}
