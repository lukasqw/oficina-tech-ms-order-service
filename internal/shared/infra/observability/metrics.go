package observability

import (
	"go.opentelemetry.io/otel/metric"
)

// Custom business metrics for the application.
var (
	ServiceOrderStatusTransition metric.Int64Counter
	ServiceOrderProcessingError  metric.Int64Counter
	ServiceOrderCreated          metric.Int64Counter
	// ServiceOrderStatusDuration measures how long (seconds) an order spent
	// in a given status before transitioning. Tag: status=<status_name>
	ServiceOrderStatusDuration metric.Float64Histogram

	HTTPRequestDuration metric.Float64Histogram
	HTTPRequestCount    metric.Int64Counter

	DBQueryDuration metric.Float64Histogram
)

// InitMetrics registers all custom metric instruments.
func InitMetrics(meter metric.Meter) error {
	var err error

	ServiceOrderStatusTransition, err = meter.Int64Counter(
		"service_order.status_transition",
		metric.WithDescription("Count of service order status transitions"),
	)
	if err != nil {
		return err
	}

	ServiceOrderProcessingError, err = meter.Int64Counter(
		"service_order.processing_error",
		metric.WithDescription("Count of service order processing errors"),
	)
	if err != nil {
		return err
	}

	ServiceOrderCreated, err = meter.Int64Counter(
		"service_order.created",
		metric.WithDescription("Count of service orders created"),
	)
	if err != nil {
		return err
	}

	ServiceOrderStatusDuration, err = meter.Float64Histogram(
		"service_order.status_duration",
		metric.WithDescription("Time in seconds an order spent in a status before transitioning"),
		metric.WithUnit("s"),
		// Explicit buckets covering seconds → days, since orders can stay in a status
		// from a few minutes (quick diagnose) up to several days (waiting authorization).
		// Without this, all values fall into the +Inf overflow bucket and avg/percentile
		// queries in Datadog return no data.
		metric.WithExplicitBucketBoundaries(
			60, 300, 600, 1800, 3600, 7200, 14400, 28800, 86400, 172800, 604800,
		),
	)
	if err != nil {
		return err
	}

	HTTPRequestDuration, err = meter.Float64Histogram(
		"http.server.request.duration",
		metric.WithDescription("HTTP request duration in seconds"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return err
	}

	HTTPRequestCount, err = meter.Int64Counter(
		"http.server.request.count",
		metric.WithDescription("Total HTTP requests"),
	)
	if err != nil {
		return err
	}

	DBQueryDuration, err = meter.Float64Histogram(
		"db.query.duration",
		metric.WithDescription("Database query duration in seconds"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return err
	}

	return nil
}
