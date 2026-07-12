package db

import (
	"database/sql"
	"errors"
	"testing"
)

func TestNewDatabaseUsesPrimaryForReadsByDefault(t *testing.T) {
	t.Parallel()

	primary := newUnopenedDB(t)
	database, err := newDatabase(Config{PrimaryDSN: "primary", Namespace: "project"}, func(dsn string) (*sql.DB, error) {
		if dsn != "primary" {
			t.Fatalf("unexpected DSN: %q", dsn)
		}
		return primary, nil
	})
	if err != nil {
		t.Fatalf("newDatabase returned an error: %v", err)
	}

	if database.RO() != database.RW() {
		t.Fatal("read and write replicas should be identical without a read-only DSN")
	}
	if database.RW().metrics == nil {
		t.Fatal("database should configure replica metrics")
	}
	if err := database.Close(); err != nil {
		t.Fatalf("Close returned an error: %v", err)
	}
}

func TestNewDatabaseCreatesDedicatedReadReplica(t *testing.T) {
	t.Parallel()

	primary := newUnopenedDB(t)
	readOnly := newUnopenedDB(t)
	database, err := newDatabase(Config{
		PrimaryDSN:  "primary",
		ReadOnlyDSN: "read-only",
		Namespace:   "project",
	}, func(dsn string) (*sql.DB, error) {
		switch dsn {
		case "primary":
			return primary, nil
		case "read-only":
			return readOnly, nil
		default:
			t.Fatalf("unexpected DSN: %q", dsn)
			return nil, nil
		}
	})
	if err != nil {
		t.Fatalf("newDatabase returned an error: %v", err)
	}

	if database.RO() == database.RW() {
		t.Fatal("read and write replicas should differ with a read-only DSN")
	}
	if database.RO().mode != "ro" || database.RW().mode != "rw" {
		t.Fatalf("unexpected replica modes: read=%q write=%q", database.RO().mode, database.RW().mode)
	}
	if database.RO().metrics != database.RW().metrics {
		t.Fatal("replicas with the same namespace should share metrics")
	}
	if err := database.Close(); err != nil {
		t.Fatalf("Close returned an error: %v", err)
	}
}

func TestNewDatabaseClosesPrimaryWhenReadReplicaFails(t *testing.T) {
	t.Parallel()

	primary := newUnopenedDB(t)
	readError := errors.New("read replica unavailable")
	_, err := newDatabase(Config{PrimaryDSN: "primary", ReadOnlyDSN: "read-only"}, func(dsn string) (*sql.DB, error) {
		if dsn == "primary" {
			return primary, nil
		}
		return nil, readError
	})
	if err == nil {
		t.Fatal("newDatabase should return the read replica error")
	}
	if pingErr := primary.Ping(); pingErr == nil {
		t.Fatal("primary database should be closed after read replica failure")
	}
}

func TestNewDatabaseRejectsEmptyPrimaryDSN(t *testing.T) {
	t.Parallel()

	openerCalled := false
	_, err := newDatabase(Config{}, func(string) (*sql.DB, error) {
		openerCalled = true
		return nil, nil
	})
	if err == nil {
		t.Fatal("newDatabase should reject an empty primary DSN")
	}
	if openerCalled {
		t.Fatal("opener should not be called for invalid configuration")
	}
}

func newUnopenedDB(t *testing.T) *sql.DB {
	t.Helper()

	database, err := sql.Open("pgx", "postgres://unused")
	if err != nil {
		t.Fatalf("sql.Open returned an error: %v", err)
	}
	return database
}
