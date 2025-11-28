package observability

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/vaheed/kubenova/internal/logging"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	"go.uber.org/zap"
	"google.golang.org/grpc/credentials"
)

// Config represents OTLP exporter settings for a single service.
type Config struct {
	ServiceName    string
	ServiceVersion string
	Environment    string
}

// SetupOTel configures OpenTelemetry tracing with an OTLP/gRPC exporter (SigNoz friendly).
// If no endpoint is provided, a no-op shutdown function is returned and tracing stays disabled.
func SetupOTel(ctx context.Context, cfg Config) (func(context.Context) error, error) {
	endpoint := strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"))
	if endpoint == "" {
		return noopShutdown, nil
	}
	service := cfg.ServiceName
	if service == "" {
		service = "kubenova"
	}
	version := strings.TrimSpace(cfg.ServiceVersion)
	if version == "" {
		version = "dev"
	}
	env := strings.TrimSpace(cfg.Environment)

	hostPort, insecure, err := normalizeEndpoint(endpoint)
	if err != nil {
		return noopShutdown, fmt.Errorf("otel endpoint: %w", err)
	}
	if parseBool(os.Getenv("OTEL_EXPORTER_OTLP_INSECURE")) {
		insecure = true
	}

	clientOpts := []otlptracegrpc.Option{
		otlptracegrpc.WithEndpoint(hostPort),
	}
	if insecure {
		clientOpts = append(clientOpts, otlptracegrpc.WithInsecure())
	} else {
		clientOpts = append(clientOpts, otlptracegrpc.WithTLSCredentials(credentials.NewClientTLSFromCert(nil, "")))
	}

	setupCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	traceExp, err := otlptracegrpc.New(setupCtx, clientOpts...)
	if err != nil {
		return noopShutdown, fmt.Errorf("otlp trace exporter: %w", err)
	}

	attrs := []attribute.KeyValue{
		semconv.ServiceNameKey.String(service),
		semconv.ServiceVersionKey.String(version),
	}
	if env != "" {
		attrs = append(attrs, attribute.String("deployment.environment", env))
	}
	res, err := resource.New(setupCtx,
		resource.WithFromEnv(),
		resource.WithProcess(),
		resource.WithTelemetrySDK(),
		resource.WithAttributes(attrs...),
	)
	if err != nil {
		return noopShutdown, fmt.Errorf("otel resource: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExp),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	logging.L.Info("otel_configured",
		zap.String("endpoint", hostPort),
		zap.Bool("insecure", insecure),
		zap.String("service", service),
		zap.String("version", version),
		zap.String("environment", env),
	)

	return tp.Shutdown, nil
}

func normalizeEndpoint(raw string) (string, bool, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false, nil
	}
	if strings.Contains(raw, "://") {
		u, err := url.Parse(raw)
		if err != nil {
			return "", false, err
		}
		return u.Host, u.Scheme == "http", nil
	}
	return raw, false, nil
}

func parseBool(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "t", "true", "y", "yes", "on":
		return true
	default:
		return false
	}
}

func noopShutdown(context.Context) error { return nil }
