package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"oficina-tech/internal/shared/infra/observability"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// routePatternKey is the context key used to share the matched route pattern
// between WrapMux and NewObservabilityMiddleware.
type routePatternKey struct{}

// routePatternHolder is a mutable container stored in context.
// Go's http context flows inward only, so we use a pointer to let an inner
// handler write the pattern and the outer middleware read it after ServeHTTP returns.
type routePatternHolder struct {
	value string
}

// WrapMux must be used with NewObservabilityMiddleware. It intercepts the request
// before routing, calls mux.Handler to resolve the matched pattern, and writes it
// into the shared holder that the observability middleware placed in context.
//
// Why this is needed: Go 1.22's ServeMux sets r.Pattern on an internal copy of the
// request (created inside ServeHTTP) — not on the request pointer passed in from
// outside. So reading r.Pattern after mux.ServeHTTP from an outer middleware always
// returns an empty string.
func WrapMux(mux *http.ServeMux) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if holder, ok := r.Context().Value(routePatternKey{}).(*routePatternHolder); ok {
			_, pattern := mux.Handler(r)
			holder.value = pattern
		}
		mux.ServeHTTP(w, r)
	})
}

// responseRecorder wraps http.ResponseWriter to capture the status code and bytes written.
type responseRecorder struct {
	http.ResponseWriter
	statusCode int
	written    int64
}

func newResponseRecorder(w http.ResponseWriter) *responseRecorder {
	return &responseRecorder{ResponseWriter: w, statusCode: http.StatusOK}
}

func (r *responseRecorder) WriteHeader(code int) {
	r.statusCode = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	n, err := r.ResponseWriter.Write(b)
	r.written += int64(n)
	return n, err
}

// NewObservabilityMiddleware wraps an http.Handler with tracing, metrics, and structured logging.
// When wrapping a ServeMux, use WrapMux(mux) as the next handler so that route patterns
// are resolved correctly.
func NewObservabilityMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Generate request ID
		requestID := uuid.New().String()
		w.Header().Set("X-Request-ID", requestID)

		// Extract trace context from incoming HTTP headers (distributed tracing)
		ctx := otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))

		// Place a pattern holder in context so WrapMux can write the matched route
		// pattern into it after routing, while we read it below after ServeHTTP returns.
		holder := &routePatternHolder{}
		ctx = context.WithValue(ctx, routePatternKey{}, holder)

		// Start span — name is updated after routing to use the route pattern
		ctx, span := observability.Tracer().Start(ctx,
			"HTTP "+r.Method,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(
				attribute.String("http.method", r.Method),
				attribute.String("http.target", r.URL.Path),
				attribute.String("http.scheme", "http"),
				attribute.String("http.request_id", requestID),
				attribute.String("user_agent.original", r.UserAgent()),
				attribute.String("net.host.name", r.Host),
			),
		)
		defer span.End()

		// Wrap response writer to capture status code
		rec := newResponseRecorder(w)

		next.ServeHTTP(rec, r.WithContext(ctx))

		duration := time.Since(start)
		durationSec := duration.Seconds()

		// Read the route pattern written by WrapMux into the shared holder.
		// Requests with no matching route (404 from bots/scanners) would produce
		// high-cardinality spans — drop the span and skip metrics for these.
		route := holder.value
		if route == "" {
			span.End()
			return
		}

		// r.Pattern in Go 1.22+ already includes the method (e.g. "POST /service-orders"),
		// so we prepend "HTTP " to get the canonical span name without duplicating the method.
		span.SetName("HTTP " + route)

		// Strip the method prefix from the route pattern (e.g. "POST /service-orders" → "/service-orders")
		// so that http.route does not duplicate http.method in metrics/traces.
		routePath := route
		if i := strings.Index(route, " "); i >= 0 {
			routePath = route[i+1:]
		}

		// Set span attributes after response
		span.SetAttributes(
			attribute.String("http.route", routePath),
			attribute.Int("http.response.status_code", rec.statusCode),
			attribute.Int64("http.response_content_length", rec.written),
		)

		// Mark span as error for 5xx
		if rec.statusCode >= 500 {
			span.SetStatus(codes.Error, http.StatusText(rec.statusCode))
		}

		// Record metrics
		attrs := metric.WithAttributes(
			attribute.String("http.method", r.Method),
			attribute.String("http.route", routePath),
			attribute.Int("http.response.status_code", rec.statusCode),
		)
		observability.HTTPRequestDuration.Record(ctx, durationSec, attrs)
		observability.HTTPRequestCount.Add(ctx, 1, attrs)

		// Structured log
		logger := observability.LoggerFromContext(ctx)
		logAttrs := []any{
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.String("route", routePath),
			slog.Int("status", rec.statusCode),
			slog.Float64("duration_ms", float64(duration.Milliseconds())),
			slog.String("request_id", requestID),
			slog.String("remote_addr", r.RemoteAddr),
			slog.String("user_agent", r.UserAgent()),
		}

		switch {
		case rec.statusCode >= 500:
			logger.Error("HTTP request", logAttrs...)
		case rec.statusCode >= 400:
			logger.Warn("HTTP request", logAttrs...)
		default:
			logger.Info("HTTP request", logAttrs...)
		}
	})
}
