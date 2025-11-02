package logging

import (
    "context"
    "os"
    "go.uber.org/zap"
    "go.uber.org/zap/zapcore"
    "go.opentelemetry.io/otel/trace"
)

var L *zap.Logger

func init() {
    encCfg := zapcore.EncoderConfig{
        TimeKey:        "ts",
        LevelKey:       "level",
        NameKey:        "logger",
        MessageKey:     "msg",
        CallerKey:      "caller",
        StacktraceKey:  "stack",
        EncodeTime:     zapcore.ISO8601TimeEncoder,
        EncodeLevel:    zapcore.LowercaseLevelEncoder,
        EncodeCaller:   zapcore.ShortCallerEncoder,
    }
    core := zapcore.NewCore(zapcore.NewJSONEncoder(encCfg), zapcore.AddSync(os.Stdout), zapcore.InfoLevel)
    L = zap.New(core)
}

type ctxKey int
const reqIDKey ctxKey = 1
const corrIDKey ctxKey = 2

func WithRequestID(ctx context.Context, id string) context.Context { return context.WithValue(ctx, reqIDKey, id) }
func WithCorrelationID(ctx context.Context, id string) context.Context { return context.WithValue(ctx, corrIDKey, id) }
func FromContext(ctx context.Context) *zap.Logger {
    l := L
    if v := ctx.Value(reqIDKey); v != nil {
        l = l.With(zap.String("request_id", v.(string)))
    }
    if v := ctx.Value(corrIDKey); v != nil {
        l = l.With(zap.String("correlation_id", v.(string)))
    }
    return l
}

// WithTrace returns a logger enriched with trace/span ids if present in ctx.
func WithTrace(ctx context.Context, l *zap.Logger) *zap.Logger {
    sc := trace.SpanFromContext(ctx).SpanContext()
    if sc.IsValid() {
        l = l.With(zap.String("trace_id", sc.TraceID().String()), zap.String("span_id", sc.SpanID().String()))
    }
    return l
}
