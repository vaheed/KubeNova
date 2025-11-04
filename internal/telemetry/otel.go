package telemetry

import (
	"context"
	"log"
	"os"
	"strconv"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// InitOTelProvider initializes a basic OTLP HTTP exporter if OTEL_EXPORTER_OTLP_ENDPOINT is set.
// If not set, it configures a noop tracer provider.
func InitOTelProvider() {
	if os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") == "" {
		otel.SetTracerProvider(sdktrace.NewTracerProvider())
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	exp, err := otlptracehttp.New(ctx)
	if err != nil {
		log.Printf("otel exporter error: %v", err)
		return
	}
	// sampling configuration (ratio 0..1 via OTEL_SAMPLING_RATIO)
	var sampler sdktrace.Sampler = sdktrace.ParentBased(sdktrace.TraceIDRatioBased(1.0))
	if v := os.Getenv("OTEL_SAMPLING_RATIO"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			sampler = sdktrace.ParentBased(sdktrace.TraceIDRatioBased(f))
		}
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithSampler(sampler),
		sdktrace.WithResource(resource.Empty()),
	)
	otel.SetTracerProvider(tp)
}
