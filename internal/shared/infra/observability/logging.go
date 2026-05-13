package observability

import (
	"context"
	"encoding/binary"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"go.opentelemetry.io/otel/trace"
)

// NewLogger creates a JSON structured logger with Datadog-compatible field names
// and sets it as the default. All logs will include service, env and version tags.
func NewLogger() *slog.Logger {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			switch a.Key {
			case slog.MessageKey:
				a.Key = "message"
			case slog.LevelKey:
				a.Key = "status"
				if level, ok := a.Value.Any().(slog.Level); ok {
					a.Value = slog.StringValue(strings.ToLower(level.String()))
				}
			case slog.TimeKey:
				a.Key = "timestamp"
			}
			return a
		},
	})).With(
		slog.String("service", "oficina-tech"),
		slog.String("env", os.Getenv("APP_ENV")),
		slog.String("version", os.Getenv("APP_VERSION")),
	)
	slog.SetDefault(logger)
	return logger
}

// LoggerFromContext returns a logger enriched with trace correlation fields
// extracted from the OTel span in the context.
// Includes dd.trace_id and dd.span_id for Datadog log-trace correlation.
func LoggerFromContext(ctx context.Context) *slog.Logger {
	span := trace.SpanFromContext(ctx)
	if !span.SpanContext().IsValid() {
		return slog.Default()
	}

	sc := span.SpanContext()
	traceID := sc.TraceID().String()
	spanID := sc.SpanID().String()

	// Datadog expects trace_id as lower 64 bits of the 128-bit trace ID in decimal format
	traceIDBytes := sc.TraceID()
	ddTraceID := binary.BigEndian.Uint64(traceIDBytes[8:])

	spanIDBytes := sc.SpanID()
	ddSpanID := binary.BigEndian.Uint64(spanIDBytes[:])

	return slog.Default().With(
		slog.String("dd.trace_id", fmt.Sprintf("%d", ddTraceID)),
		slog.String("dd.span_id", fmt.Sprintf("%d", ddSpanID)),
		slog.String("trace_id", traceID),
		slog.String("span_id", spanID),
	)
}
