// Command seed inserts the hospitals a fresh stack needs to be usable.
//
// This is deliberately not a migration. Migrations own the schema and nothing
// else — see docs/DECISIONS.md — and they also run against every test schema,
// where a seeded "Hospital A" would collide with the rows the isolation suite
// inserts under that same name.
//
// It is safe to run repeatedly: each insert is a no-op if the hospital already
// exists, so `docker compose up` on an existing volume changes nothing.
package main

import (
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/ThanadolU/hospital-middleware/internal/database"
)

// defaultHospitals matches the two hospitals the brief names.
var defaultHospitals = []string{"Hospital A", "Hospital B"}

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	if err := run(log); err != nil {
		log.Error("seed failed", "error", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return fmt.Errorf("DATABASE_URL must be set")
	}

	db, err := database.Open(database.Config{DSN: dsn})
	if err != nil {
		return err
	}
	sqlDB, err := database.SQLDB(db)
	if err != nil {
		return err
	}
	defer func() { _ = sqlDB.Close() }()

	// Migrate first so seeding does not depend on the api container having
	// started. Both call the same idempotent migrator, so whichever wins the
	// race, the other is a no-op.
	if err := database.Migrate(sqlDB); err != nil {
		return err
	}

	for _, name := range hospitals() {
		inserted, err := insertHospital(sqlDB, name)
		if err != nil {
			return err
		}
		log.Info("seeded hospital", "name", name, "inserted", inserted)
	}
	return nil
}

// hospitals reads SEED_HOSPITALS as a comma-separated list, so a deployment can
// seed its own names without a rebuild. Unset means the default pair.
func hospitals() []string {
	raw := strings.TrimSpace(os.Getenv("SEED_HOSPITALS"))
	if raw == "" {
		return defaultHospitals
	}

	var names []string
	for _, name := range strings.Split(raw, ",") {
		if trimmed := strings.TrimSpace(name); trimmed != "" {
			names = append(names, trimmed)
		}
	}
	if len(names) == 0 {
		return defaultHospitals
	}
	return names
}

// insertHospital adds a hospital unless one already exists under that name,
// reporting whether it actually inserted.
//
// The conflict target is lower(name) because that is the expression the unique
// index is built on; naming the bare column would not match it and the
// statement would fail rather than do nothing.
func insertHospital(db *sql.DB, name string) (bool, error) {
	result, err := db.Exec(
		`INSERT INTO hospitals (name) VALUES ($1) ON CONFLICT (lower(name)) DO NOTHING`,
		name,
	)
	if err != nil {
		return false, fmt.Errorf("seed hospital %q: %w", name, err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("seed hospital %q: %w", name, err)
	}
	return affected > 0, nil
}
