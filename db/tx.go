// Package db provides database transaction utilities for the platform.
// It offers transaction lifecycle management with automatic rollback on errors
// and proper error wrapping for consistent fault handling across services.
//
// The package is shared across all services.
package db

import (
	"context"
	"database/sql"
	"errors"

	"github.com/cantte/go/codes"
	"github.com/cantte/go/fault"
)

// TxWithResult executes fn within a database transaction and returns the result.
// It begins a transaction on db, executes fn with the transaction context,
// and commits on success or rolls back on failure.
//
// The function automatically handles the complete transaction lifecycle:
// begin, execute, and commit/rollback.
//
// TxWithResult is generic and preserves type safety for return values.
// The ctx parameter provides cancellation and timeout control for the
// entire transaction. The db parameter must be a valid [Replica] instance,
// typically from [Database.Primary] for write operations.
//
// The fn parameter receives the transaction context and a [DBTX] interface
// for database operations. It should perform all required operations and
// return the result with any error.
//
// TxWithResult returns the function result on successful commit, or an error
// if any step fails. Transaction begin errors return ServiceUnavailable.
// Rollback errors during error handling also return ServiceUnavailable,
// except for sql.ErrTxDone which indicates the transaction was already
// completed. Commit errors return ServiceUnavailable.
//
// Context cancellation triggers automatic rollback. The function is safe
// for concurrent use but callers must avoid operations that could deadlock
// with other concurrent transactions.
//
// Common usage scenarios include:
//   - Creating tenants with associated information atomically
//   - Batch operations that must succeed or fail as a unit
//   - Complex queries requiring consistency guarantees
//
// Edge cases and limitations:
//   - If fn returns an error, rollback is attempted even if the transaction
//     is already in a failed state, which may produce additional errors
//   - Database connection issues during commit may leave the transaction
//     in an undefined state on the server side
//   - Context cancellation after fn execution causes rollback instead of commit
//
// Anti-patterns to avoid:
//   - Long-running operations within fn that could timeout
//   - Nesting calls to TxWithResult (creates nested transactions)
//   - Ignoring the returned error from fn
//   - Accessing the DBTX parameter outside of the fn callback
//
// Use context.WithTimeout to prevent indefinite blocking. For operations
// that may conflict, implement retry logic with exponential backoff at
// the caller level.
//
// See [Replica.Begin] for transaction initiation and [DBTX] for available
// operations within transactions. For read-only operations that don't
// require transactions, use query methods directly on [Database.RO].
func TxWithResult[T any](ctx context.Context, db *Replica, fn func(context.Context, DBTX) (T, error)) (T, error) {
	var t T

	tx, err := db.Begin(ctx)
	if err != nil {
		return t, fault.Wrap(err,
			fault.Code(codes.AppErrorsInternalServiceUnavailable),
			fault.Internal("database failed to create transaction"), fault.Public("Unable to start database transaction."),
		)
	}

	t, err = fn(ctx, tx)
	if err != nil {
		rollbackErr := tx.Rollback()

		if rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
			return t, fault.Wrap(rollbackErr,
				fault.Code(codes.AppErrorsInternalServiceUnavailable),
				fault.Internal("database failed to rollback transaction"), fault.Public("Unable to rollback database transaction."),
			)
		}

		return t, err
	}

	err = tx.Commit()
	if err != nil {
		return t, fault.Wrap(err,
			fault.Code(codes.AppErrorsInternalServiceUnavailable),
			fault.Internal("database failed to commit transaction"), fault.Public("Unable to commit database transaction."),
		)
	}

	return t, nil
}

// Tx executes fn within a database transaction without returning a result.
// It is a convenience wrapper around [TxWithResult] for operations that
// only need error handling.
//
// Tx begins a transaction on db, executes fn with the transaction context,
// and commits on success or rolls back on failure. All database errors
// are wrapped with ServiceUnavailable fault codes.
//
// The ctx parameter provides cancellation and timeout control. The db
// parameter must be a valid [Replica] instance. The fn parameter receives
// the transaction context and a [DBTX] interface for database operations.
//
// Tx returns nil on successful commit, or an error if any step fails.
// Error handling follows the same patterns as [TxWithResult].
//
// Use Tx for operations that don't need to return values, such as:
//   - Deleting records with audit logging
//   - Updating configuration settings
//   - Batch cleanup operations
//   - State changes that only need success/failure indication
//
// See [TxWithResult] for detailed transaction behavior and [DBTX] for
// available database operations.
func Tx(ctx context.Context, db *Replica, fn func(context.Context, DBTX) error) error {
	_, err := TxWithResult(ctx, db, func(inner context.Context, tx DBTX) (any, error) {
		return nil, fn(inner, tx)
	})
	return err
}
