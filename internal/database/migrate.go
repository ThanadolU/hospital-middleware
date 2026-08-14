// Package database owns the schema and the connection to it.
package database

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"

	"github.com/ThanadolU/hospital-middleware/migrations"
)

// Migrate applies every pending migration and returns once the schema is at the
// latest version. It is safe to call on every boot: already-applied migrations
// are skipped, so `docker compose up` twice in a row is a no-op the second time.
//
// v1 called AutoMigrate and discarded its error, so a failed migration passed
// silently and the app served requests against a schema that was never created.
// Every error here is returned.
func Migrate(db *sql.DB) error {
	migrator, closeSource, err := newMigrator(db)
	if err != nil {
		return err
	}
	defer closeSource()

	if err := migrator.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("database: apply migrations: %w", err)
	}
	return nil
}

// Rollback steps the schema back by n migrations. It exists for local
// development and for the migration tests; production only ever moves forward.
func Rollback(db *sql.DB, n int) error {
	migrator, closeSource, err := newMigrator(db)
	if err != nil {
		return err
	}
	defer closeSource()

	if err := migrator.Steps(-n); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("database: roll back %d migration(s): %w", n, err)
	}
	return nil
}

// newMigrator returns a migrator plus a cleanup that releases the migration
// source only.
//
// Deliberately not migrate.Migrate.Close(): that closes the database driver as
// well, and the postgres driver built by WithInstance closes the *sql.DB it was
// handed. The connection belongs to the caller, who may still be using it —
// closing it here made a second Migrate call fail with "sql: database is
// closed", which TestMigrate_AppliesAndIsIdempotent catches.
func newMigrator(db *sql.DB) (*migrate.Migrate, func(), error) {
	if db == nil {
		return nil, nil, errors.New("database: nil connection")
	}

	source, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return nil, nil, fmt.Errorf("database: read embedded migrations: %w", err)
	}
	closeSource := func() { _ = source.Close() }

	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		closeSource()
		return nil, nil, fmt.Errorf("database: init migration driver: %w", err)
	}

	migrator, err := migrate.NewWithInstance("iofs", source, "postgres", driver)
	if err != nil {
		closeSource()
		return nil, nil, fmt.Errorf("database: init migrator: %w", err)
	}
	return migrator, closeSource, nil
}
