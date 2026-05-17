package observability

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var otelInitialized bool

// OTelInitialized returns whether OTel SDK was successfully initialized.
func OTelInitialized() bool {
	return otelInitialized
}

// InitOTel initializes the OpenTelemetry SDK with OTLP gRPC exporters for traces and metrics.
// Returns a shutdown function that flushes and closes all providers.
func InitOTel(ctx context.Context) (shutdown func(context.Context) error, err error) {
	endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if endpoint == "" {
		endpoint = "localhost:4317"
	}
	endpoint = strings.TrimPrefix(strings.TrimPrefix(endpoint, "https://"), "http://")

	serviceName := os.Getenv("OTEL_SERVICE_NAME")
	if serviceName == "" {
		serviceName = "ms-order-service"
	}
	serviceVersion := os.Getenv("APP_VERSION")
	if serviceVersion == "" {
		serviceVersion = "dev"
	}
	environment := os.Getenv("APP_ENV")
	if environment == "" {
		environment = "development"
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(serviceName),
			semconv.ServiceVersion(serviceVersion),
			semconv.DeploymentEnvironment(environment),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create OTel resource: %w", err)
	}

	// gRPC connection to OTLP collector (Datadog Agent)
	conn, err := grpc.NewClient(endpoint,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create gRPC connection: %w", err)
	}

	// Trace exporter
	traceExporter, err := otlptracegrpc.New(ctx, otlptracegrpc.WithGRPCConn(conn))
	if err != nil {
		return nil, fmt.Errorf("failed to create trace exporter: %w", err)
	}

	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExporter),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tracerProvider)

	// Metric exporter — delta temporality para counters, cumulativo para histogramas.
	// O Datadog espera deltas: com cumulative o SDK reenvia o total acumulado a cada
	// intervalo mesmo sem novos eventos, fazendo o counter subir indefinidamente no dashboard.
	deltaSelector := func(kind sdkmetric.InstrumentKind) metricdata.Temporality {
		switch kind {
		case sdkmetric.InstrumentKindCounter,
			sdkmetric.InstrumentKindObservableCounter,
			sdkmetric.InstrumentKindHistogram:
			return metricdata.DeltaTemporality
		default:
			return metricdata.CumulativeTemporality
		}
	}

	metricExporter, err := otlpmetricgrpc.New(ctx,
		otlpmetricgrpc.WithGRPCConn(conn),
		otlpmetricgrpc.WithTemporalitySelector(deltaSelector),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create metric exporter: %w", err)
	}

	meterProvider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExporter, sdkmetric.WithInterval(30*time.Second))),
		sdkmetric.WithResource(res),
	)
	otel.SetMeterProvider(meterProvider)

	// Propagators
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	// Initialize custom metrics
	if err := InitMetrics(Meter()); err != nil {
		return nil, fmt.Errorf("failed to initialize metrics: %w", err)
	}

	otelInitialized = true

	// Composite shutdown
	shutdownFn := func(ctx context.Context) error {
		var errs []error
		if err := tracerProvider.Shutdown(ctx); err != nil {
			errs = append(errs, err)
		}
		if err := meterProvider.Shutdown(ctx); err != nil {
			errs = append(errs, err)
		}
		if err := conn.Close(); err != nil {
			errs = append(errs, err)
		}
		if len(errs) > 0 {
			return fmt.Errorf("OTel shutdown errors: %v", errs)
		}
		return nil
	}

	return shutdownFn, nil
}

// Tracer returns a named tracer for the application.
func Tracer() trace.Tracer {
	return otel.Tracer("oficina-tech")
}

// Meter returns a named meter for the application.
func Meter() metric.Meter {
	return otel.Meter("oficina-tech")
}

// SpanHandler starts a span for a handler method.
// Naming convention: "handler.<module>.<operation>" e.g. "handler.auth.login"
func SpanHandler(ctx context.Context, name string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	return Tracer().Start(ctx, "handler."+name,
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(attrs...),
	)
}

// SpanUseCase starts a span for a use case Execute method.
// Naming convention: "usecase.<module>.<operation>" e.g. "usecase.auth.login"
func SpanUseCase(ctx context.Context, name string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	return Tracer().Start(ctx, "usecase."+name,
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(attrs...),
	)
}
