package db

import (
	"context"
	"database/sql"
	"time"

	"github.com/cantte/go/otel/tracing"
	"github.com/cantte/go/pg/metrics"

	"go.opentelemetry.io/otel/attribute"
)

// Replica wraps a standard SQL database connection and implements the gen.DBTX interface
// to enable interaction with the generated database code.
type Replica struct {
	mode    string
	db      *sql.DB
	metrics *metrics.Metrics
}

// ReplicaOption configures a Replica.
type ReplicaOption func(*replicaConfig)

type replicaConfig struct {
	metricsEnabled   bool
	metricsNamespace string
}

// WithMetricsNamespace sets the Prometheus namespace used by database
// metrics. An empty namespace means that metric names have no namespace
// prefix.
func WithMetricsNamespace(namespace string) ReplicaOption {
	return func(config *replicaConfig) {
		config.metricsNamespace = namespace
	}
}

// WithMetrics enables or disables Prometheus database metrics.
func WithMetrics(enabled bool) ReplicaOption {
	return func(config *replicaConfig) {
		config.metricsEnabled = enabled
	}
}

// NewReplica wraps db with tracing and Prometheus instrumentation.
// Metrics are enabled by default without a namespace prefix.
func NewReplica(db *sql.DB, mode string, options ...ReplicaOption) *Replica {
	config := replicaConfig{metricsEnabled: true}
	for _, option := range options {
		if option != nil {
			option(&config)
		}
	}

	replica := &Replica{db: db, mode: mode}
	if config.metricsEnabled {
		replica.metrics = metrics.New(config.metricsNamespace)
	}

	return replica
}

// Ensure Replica implements the gen.DBTX interface
var _ DBTX = (*Replica)(nil)

// ExecContext executes a SQL statement and returns a result summary.
// It's used for INSERT, UPDATE, DELETE statements that don't return rows.
func (r *Replica) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	ctx, span := tracing.Start(ctx, "ExecContext")
	defer span.End()
	span.SetAttributes(
		attribute.String("query", query),
	)

	// Track metrics
	start := time.Now()
	result, err := r.db.ExecContext(ctx, query, args...)

	// Record latency and operation count
	duration := time.Since(start).Seconds()
	status := statusSuccess
	if err != nil {
		status = statusError
	}

	r.recordMetric("exec", status, duration)

	tracing.RecordErrorUnless(span, err, sql.ErrNoRows)

	return result, err
}

// PrepareContext prepares a SQL statement for later execution.
func (r *Replica) PrepareContext(ctx context.Context, query string) (*sql.Stmt, error) {
	ctx, span := tracing.Start(ctx, "PrepareContext")
	defer span.End()
	span.SetAttributes(
		attribute.String("query", query),
	)

	// Track metrics
	start := time.Now()
	stmt, err := r.db.PrepareContext(ctx, query)

	// Record latency and operation count
	duration := time.Since(start).Seconds()
	status := statusSuccess
	if err != nil {
		status = statusError
	}

	r.recordMetric("prepare", status, duration)

	tracing.RecordErrorUnless(span, err, sql.ErrNoRows)

	return stmt, err
}

// QueryContext executes a SQL query that returns rows.
func (r *Replica) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	ctx, span := tracing.Start(ctx, "QueryContext")
	defer span.End()
	span.SetAttributes(
		attribute.String("query", query),
	)

	// Track metrics
	start := time.Now()
	rows, err := r.db.QueryContext(ctx, query, args...)

	// Record latency and operation count
	duration := time.Since(start).Seconds()
	status := statusSuccess
	if err != nil {
		status = statusError
	}

	r.recordMetric("query", status, duration)

	tracing.RecordErrorUnless(span, err, sql.ErrNoRows)

	return rows, err
}

// QueryRowContext executes a SQL query that returns a single row.
func (r *Replica) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	ctx, span := tracing.Start(ctx, "QueryRowContext")
	defer span.End()
	span.SetAttributes(
		attribute.String("query", query),
	)

	// Track metrics
	start := time.Now()
	row := r.db.QueryRowContext(ctx, query, args...)

	// Record latency and operation count
	duration := time.Since(start).Seconds()
	// QueryRowContext doesn't return an error, but we can still track timing
	status := statusSuccess

	r.recordMetric("query_row", status, duration)

	return row
}

// Begin starts a transaction and returns it.
// This method provides a way to use the Replica in transaction-based operations.
func (r *Replica) Begin(ctx context.Context) (DBTx, error) {
	ctx, span := tracing.Start(ctx, "Begin")
	defer span.End()

	// Track metrics
	start := time.Now()
	tx, err := r.db.BeginTx(ctx, nil)

	// Record latency and operation count
	duration := time.Since(start).Seconds()
	status := statusSuccess
	if err != nil {
		status = statusError
	}

	r.recordMetric("begin", status, duration)

	tracing.RecordErrorUnless(span, err, sql.ErrNoRows)

	if err != nil {
		return nil, err
	}

	// Wrap the transaction with tracing
	return wrapTxWithMetrics(tx, r.mode+"_tx", ctx, r.metrics), nil
}

func (r *Replica) recordMetric(operation, status string, duration float64) {
	r.metrics.Observe(r.mode, operation, status, duration)
}
