package database

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Config describes a PostgreSQL connection.
type Config struct {
	// DSN is the full connection string. Required.
	DSN string

	// MaxOpenConns bounds the pool. Zero means the driver default.
	MaxOpenConns int

	// LogLevel controls GORM's SQL logging. Defaults to logger.Warn, which
	// keeps slow queries and errors visible without printing every statement —
	// and this application queries patient data, so full statement logging
	// would put PII in the logs.
	LogLevel logger.LogLevel
}

// Open connects to PostgreSQL and verifies the connection before returning it.
// gorm.Open alone is lazy: it can hand back a handle for a database that is not
// reachable, so a failure would surface on the first query rather than at boot.
func Open(cfg Config) (*gorm.DB, error) {
	if cfg.DSN == "" {
		return nil, errors.New("database: DSN is required")
	}

	logLevel := cfg.LogLevel
	if logLevel == 0 {
		logLevel = logger.Warn
	}

	db, err := gorm.Open(postgres.Open(cfg.DSN), &gorm.Config{
		Logger: logger.Default.LogMode(logLevel),
	})
	if err != nil {
		return nil, fmt.Errorf("database: connect: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("database: unwrap pool: %w", err)
	}
	if cfg.MaxOpenConns > 0 {
		sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	}
	sqlDB.SetConnMaxLifetime(time.Hour)

	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("database: ping: %w", err)
	}
	return db, nil
}

// SQLDB unwraps the standard-library pool, which the migrator needs.
func SQLDB(db *gorm.DB) (*sql.DB, error) {
	if db == nil {
		return nil, errors.New("database: nil handle")
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("database: unwrap pool: %w", err)
	}
	return sqlDB, nil
}
