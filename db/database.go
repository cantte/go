package db

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/cantte/go/assert"
	"github.com/cantte/go/fault"
	"github.com/cantte/go/logger"
	"github.com/cantte/go/retry"
	_ "github.com/jackc/pgx/v5"
	_ "github.com/jackc/pgx/v5/stdlib"
)

// Config defines the parameters needed to establish database connections.
// It supports separate connections for read and write operations to allow
// for primary/replica setups.
type Config struct {
	// The primary DSN for your database. This must support both reads and writes.
	PrimaryDSN string

	// The readonly replica will be used for most read queries.
	// If omitted, the primary is used.
	ReadOnlyDSN string

	// Namespace prefixes all Prometheus metrics emitted by the database
	// replicas. For example, "appointments" produces metrics such as
	// "appointments_database_operations_total". The same namespace is used
	// for both primary and read-only replicas. Leave it empty to emit metrics
	// without an application prefix, such as "database_operations_total".
	Namespace string
}

// database implements the Database interface, providing access to database replicas
// and handling connection lifecycle.
type database struct {
	writeReplica *Replica // Primary database connection used for write operations
	readReplica  *Replica // Connection used for read operations (may be same as primary)
}

func open(dsn string) (db *sql.DB, err error) {
	// sql.Open only validates the DSN, it doesn't actually connect.
	// We need to call Ping() to verify connectivity.
	db, err = sql.Open("pgx", dsn)
	if err != nil {
		return nil, fault.Wrap(err, fault.Internal("failed to open database"))
	}

	// Configure connection pool for better performance
	// These settings prevent cold-start latency by maintaining warm connections
	db.SetMaxOpenConns(25)                 // Max concurrent connections
	db.SetMaxIdleConns(10)                 // Keep 10 idle connections ready
	db.SetConnMaxLifetime(5 * time.Minute) // Refresh connections every 5 min (PlanetScale recommendation)
	db.SetConnMaxIdleTime(1 * time.Minute) // Close idle connections after 1 min of inactivity

	// Verify connectivity at startup with retries - this establishes at least one connection
	// so the first request doesn't pay the connection establishment cost
	err = retry.New(
		retry.Attempts(5),
		retry.Backoff(func(n int) time.Duration {
			return time.Duration(n) * time.Second
		}),
	).Do(func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		pingErr := db.PingContext(ctx)
		if pingErr != nil {
			logger.Info("postgres not ready yet, retrying...", "error", pingErr.Error())
		}
		return pingErr
	})
	if err != nil {
		_ = db.Close()
		return nil, fault.Wrap(err, fault.Internal("failed to ping database after retries"))
	}

	logger.Info("database connection pool initialized successfully")
	return db, nil
}

// New creates a new database instance with the provided configuration.
// It establishes connections to the primary database and optionally to a read-only replica.
// Returns an error if connections cannot be established or if DSNs are misconfigured.
func New(config Config) (Database, error) {
	return newDatabase(config, open)
}

func newDatabase(config Config, opener func(string) (*sql.DB, error)) (Database, error) {
	err := assert.All(
		assert.NotEmpty(config.PrimaryDSN),
	)
	if err != nil {
		return nil, fault.Wrap(err, fault.Internal("invalid configuration"))
	}

	write, err := opener(config.PrimaryDSN)
	if err != nil {
		return nil, fault.Wrap(err, fault.Internal("cannot open primary replica"))
	}

	// Initialize primary replica
	writeReplica := NewReplica(write, "rw", WithMetricsNamespace(config.Namespace))

	// Share the primary wrapper when no dedicated read pool is configured.
	// Besides avoiding an unnecessary allocation, this makes ownership clear
	// and prevents the same sql.DB from being closed twice.
	readReplica := writeReplica

	// If a separate read-only DSN is provided, establish that connection
	if config.ReadOnlyDSN != "" {
		read, err := opener(config.ReadOnlyDSN)
		if err != nil {
			_ = write.Close()
			return nil, fault.Wrap(err, fault.Internal("cannot open read replica"))
		}

		readReplica = NewReplica(read, "ro", WithMetricsNamespace(config.Namespace))
		logger.Info("database configured with separate read replica")
	} else {
		logger.Info("database configured without separate read replica, using primary for reads")
	}

	return &database{
		writeReplica: writeReplica,
		readReplica:  readReplica,
	}, nil
}

// RW returns the write replica for performing database write operations.
func (d *database) RW() *Replica {
	return d.writeReplica
}

// RO returns the read replica for performing database read operations.
// If no dedicated read replica is configured, it returns the write replica.
func (d *database) RO() *Replica {
	return d.readReplica
}

// Close properly closes all database connections.
// This should be called when the application is shutting down.
func (d *database) Close() error {
	var closeErrors []error
	if d.readReplica != d.writeReplica {
		if err := d.readReplica.db.Close(); err != nil {
			closeErrors = append(closeErrors, err)
		}
	}
	if err := d.writeReplica.db.Close(); err != nil {
		closeErrors = append(closeErrors, err)
	}

	if err := errors.Join(closeErrors...); err != nil {
		return fault.Wrap(err, fault.Internal("failed to close database connections"))
	}
	return nil
}
