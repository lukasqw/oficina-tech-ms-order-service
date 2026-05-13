package observability

import (
	"fmt"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
	"gorm.io/gorm"
	metricapi "go.opentelemetry.io/otel/metric"
)

const (
	gormStartKey = "otel_start_time"
	gormSpanKey  = "otel_span"
	maxSQLLen    = 1000
)

// OTelPlugin is a GORM plugin that instruments database queries with OpenTelemetry.
type OTelPlugin struct{}

// NewOTelPlugin creates a new OTelPlugin instance.
func NewOTelPlugin() *OTelPlugin {
	return &OTelPlugin{}
}

// Name returns the plugin name.
func (p *OTelPlugin) Name() string {
	return "otel:gorm"
}

// Initialize registers GORM callbacks for tracing and metrics.
func (p *OTelPlugin) Initialize(db *gorm.DB) error {
	cb := db.Callback()

	for _, reg := range []struct {
		name string
		err  error
	}{
		{"otel:before_create", cb.Create().Before("gorm:create").Register("otel:before_create", before("create"))},
		{"otel:after_create", cb.Create().After("gorm:create").Register("otel:after_create", after("create"))},
		{"otel:before_query", cb.Query().Before("gorm:query").Register("otel:before_query", before("query"))},
		{"otel:after_query", cb.Query().After("gorm:query").Register("otel:after_query", after("query"))},
		{"otel:before_update", cb.Update().Before("gorm:update").Register("otel:before_update", before("update"))},
		{"otel:after_update", cb.Update().After("gorm:update").Register("otel:after_update", after("update"))},
		{"otel:before_delete", cb.Delete().Before("gorm:delete").Register("otel:before_delete", before("delete"))},
		{"otel:after_delete", cb.Delete().After("gorm:delete").Register("otel:after_delete", after("delete"))},
		{"otel:before_row", cb.Row().Before("gorm:row").Register("otel:before_row", before("row"))},
		{"otel:after_row", cb.Row().After("gorm:row").Register("otel:after_row", after("row"))},
		{"otel:before_raw", cb.Raw().Before("gorm:raw").Register("otel:before_raw", before("raw"))},
		{"otel:after_raw", cb.Raw().After("gorm:raw").Register("otel:after_raw", after("raw"))},
	} {
		if reg.err != nil {
			return fmt.Errorf("registering gorm callback %s: %w", reg.name, reg.err)
		}
	}

	return nil
}

func before(op string) func(*gorm.DB) {
	return func(db *gorm.DB) {
		if db.Statement == nil || db.Statement.Context == nil {
			return
		}
		ctx, span := Tracer().Start(db.Statement.Context, "gorm."+op,
			trace.WithAttributes(
				semconv.DBSystemPostgreSQL,
				attribute.String("db.operation", op),
			),
			trace.WithSpanKind(trace.SpanKindClient),
		)
		db.Statement.Context = ctx
		db.InstanceSet(gormStartKey, time.Now())
		db.InstanceSet(gormSpanKey, span)
	}
}

func after(op string) func(*gorm.DB) {
	return func(db *gorm.DB) {
		if db.Statement == nil {
			return
		}

		spanVal, ok := db.InstanceGet(gormSpanKey)
		if !ok {
			return
		}
		span, ok := spanVal.(trace.Span)
		if !ok {
			return
		}
		defer span.End()

		startVal, ok := db.InstanceGet(gormStartKey)
		if !ok {
			return
		}
		start, ok := startVal.(time.Time)
		if !ok {
			return
		}
		duration := time.Since(start).Seconds()

		// Enrich span with SQL statement
		if sql := db.Statement.SQL.String(); sql != "" {
			if len(sql) > maxSQLLen {
				sql = sql[:maxSQLLen] + "..."
			}
			span.SetAttributes(attribute.String("db.statement", sql))
		}

		// Table name
		table := db.Statement.Table
		if table == "" {
			table = "unknown"
		}
		span.SetAttributes(attribute.String("db.sql.table", table))

		// Record error
		if db.Error != nil && db.Error != gorm.ErrRecordNotFound {
			span.RecordError(db.Error)
			span.SetStatus(codes.Error, db.Error.Error())
		}

		// Record duration metric
		if DBQueryDuration != nil {
			DBQueryDuration.Record(
				db.Statement.Context,
				duration,
				metricapi.WithAttributes(
					attribute.String("db.operation", op),
					attribute.String("db.sql.table", table),
				),
			)
		}
	}
}
