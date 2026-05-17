package observability

import (
	"context"
	"os"
	"testing"

	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func TestMain(m *testing.M) {
	// Use real SDK (not noop) so spans have valid IDs — required for ExtractSpanLinkFromSQS.
	tp := sdktrace.NewTracerProvider()
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{},
	))
	if err := InitMetrics(otel.GetMeterProvider().Meter("test")); err != nil {
		panic("init metrics: " + err.Error())
	}
	os.Exit(m.Run())
}

func TestInitMetrics_NoError(t *testing.T) {
	err := InitMetrics(otel.GetMeterProvider().Meter("test-reinit"))
	if err != nil {
		t.Fatalf("InitMetrics() error: %v", err)
	}
}

func TestOTelInitialized_DoesNotPanic(t *testing.T) {
	_ = OTelInitialized()
}

func TestInjectTraceToSQS_WithActiveSpan(t *testing.T) {
	tracer := otel.Tracer("test")
	ctx, span := tracer.Start(context.Background(), "producer")
	defer span.End()

	attrs := InjectTraceToSQS(ctx)
	if len(attrs) == 0 {
		t.Fatal("expected non-empty SQS attributes from active span")
	}
}

func TestInjectTraceToSQS_NoSpan_DoesNotPanic(t *testing.T) {
	attrs := InjectTraceToSQS(context.Background())
	_ = attrs
}

func TestExtractSpanLinkFromSQS_ValidAttributes(t *testing.T) {
	tracer := otel.Tracer("test")
	ctx, span := tracer.Start(context.Background(), "producer")
	defer span.End()

	attrs := InjectTraceToSQS(ctx)
	msg := sqstypes.Message{MessageAttributes: attrs}

	link, ok := ExtractSpanLinkFromSQS(msg)
	if !ok {
		t.Fatal("expected valid span link from injected trace context")
	}
	if !link.SpanContext.IsValid() {
		t.Error("expected valid SpanContext in link")
	}
}

func TestExtractSpanLinkFromSQS_EmptyMessage_ReturnsFalse(t *testing.T) {
	_, ok := ExtractSpanLinkFromSQS(sqstypes.Message{})
	if ok {
		t.Fatal("expected false for message with no trace attributes")
	}
}

func TestExtractSpanLinkFromSQS_NilAttributes(t *testing.T) {
	msg := sqstypes.Message{MessageAttributes: nil}
	_, ok := ExtractSpanLinkFromSQS(msg)
	if ok {
		t.Fatal("expected false for nil attributes")
	}
}

func TestLoggerFromContext_WithSpan(t *testing.T) {
	tracer := otel.Tracer("test")
	ctx, span := tracer.Start(context.Background(), "log-test")
	defer span.End()

	logger := LoggerFromContext(ctx)
	if logger == nil {
		t.Fatal("expected non-nil logger")
	}
}

func TestLoggerFromContext_NoSpan_ReturnsDefault(t *testing.T) {
	logger := LoggerFromContext(context.Background())
	if logger == nil {
		t.Fatal("expected non-nil default logger")
	}
}

func TestNewOTelPlugin_NotNil(t *testing.T) {
	p := NewOTelPlugin()
	if p == nil {
		t.Fatal("expected non-nil OTel GORM plugin")
	}
}

func TestOTelPlugin_Name(t *testing.T) {
	p := NewOTelPlugin()
	if p.Name() != "otel:gorm" {
		t.Errorf("expected 'otel:gorm', got %q", p.Name())
	}
}

func TestMetricGlobals_Initialized(t *testing.T) {
	if ServiceOrderStatusTransition == nil {
		t.Error("ServiceOrderStatusTransition should be initialized")
	}
	if ServiceOrderCreated == nil {
		t.Error("ServiceOrderCreated should be initialized")
	}
	if HTTPRequestDuration == nil {
		t.Error("HTTPRequestDuration should be initialized")
	}
	if HTTPRequestCount == nil {
		t.Error("HTTPRequestCount should be initialized")
	}
}

// --- NewLogger ---

func TestNewLogger_NotNil(t *testing.T) {
	logger := NewLogger()
	if logger == nil {
		t.Fatal("expected non-nil logger")
	}
}

// --- Tracer / Meter ---

func TestTracer_CanStartSpan(t *testing.T) {
	tracer := Tracer()
	ctx, span := tracer.Start(context.Background(), "test-span")
	defer span.End()
	if ctx == nil {
		t.Fatal("expected non-nil context from Tracer().Start")
	}
}

func TestMeter_CanCreateCounter(t *testing.T) {
	meter := Meter()
	_, err := meter.Int64Counter("test_counter_meter")
	if err != nil {
		t.Fatalf("Meter().Int64Counter() failed: %v", err)
	}
}

// --- SpanHandler / SpanUseCase ---

func TestSpanHandler_ReturnsContextAndSpan(t *testing.T) {
	ctx, span := SpanHandler(context.Background(), "order.create")
	defer span.End()
	if ctx == nil {
		t.Fatal("expected non-nil context from SpanHandler")
	}
}

func TestSpanUseCase_ReturnsContextAndSpan(t *testing.T) {
	ctx, span := SpanUseCase(context.Background(), "order.create")
	defer span.End()
	if ctx == nil {
		t.Fatal("expected non-nil context from SpanUseCase")
	}
}
