// Command api runs the hospital middleware HTTP service.
package main

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/ThanadolU/hospital-middleware/internal/auth"
	"github.com/ThanadolU/hospital-middleware/internal/database"
	"github.com/ThanadolU/hospital-middleware/internal/handler"
	"github.com/ThanadolU/hospital-middleware/internal/repository"
	"github.com/ThanadolU/hospital-middleware/internal/routes"
	"github.com/ThanadolU/hospital-middleware/internal/service"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(log)

	if err := run(log); err != nil {
		log.Error("startup failed", "error", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	// Every secret and connection detail is required, with no fallback: a
	// service that boots with a default secret is worse than one that refuses
	// to boot at all.
	dsn, err := requiredEnv("DATABASE_URL")
	if err != nil {
		return err
	}
	secret, err := requiredEnv(auth.SecretEnv)
	if err != nil {
		return err
	}

	db, err := database.Open(database.Config{DSN: dsn, MaxOpenConns: 25})
	if err != nil {
		return err
	}
	sqlDB, err := database.SQLDB(db)
	if err != nil {
		return err
	}
	defer func() { _ = sqlDB.Close() }()

	// Applied at boot and checked. v1 discarded AutoMigrate's error, so a
	// failed migration left the service answering against a missing schema.
	if err := database.Migrate(sqlDB); err != nil {
		return err
	}
	log.Info("schema up to date")

	tokens, err := auth.NewTokenService(secret, tokenTTL())
	if err != nil {
		return err
	}

	authService := service.NewAuthService(
		repository.NewStaffRepository(db),
		repository.NewHospitalRepository(db),
		tokens,
	)
	patientService := service.NewPatientService(repository.NewPatientRepository(db))

	router := routes.NewRouter(routes.Dependencies{
		Staff:   handler.NewStaffHandler(authService, log),
		Patient: handler.NewPatientHandler(patientService, log),
		Tokens:  tokens,
		Health:  sqlDB.Ping,
	})

	server := &http.Server{
		Addr:              ":" + port(),
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}

	log.Info("listening", "addr", server.Addr)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve: %w", err)
	}
	return nil
}

func requiredEnv(key string) (string, error) {
	value := os.Getenv(key)
	if value == "" {
		return "", fmt.Errorf("%s must be set", key)
	}
	return value, nil
}

func port() string {
	if p := os.Getenv("PORT"); p != "" {
		return p
	}
	return "8000"
}

func tokenTTL() time.Duration {
	raw := os.Getenv("JWT_TTL_HOURS")
	if raw == "" {
		return auth.DefaultTokenTTL
	}
	hours, err := strconv.Atoi(raw)
	if err != nil || hours <= 0 {
		return auth.DefaultTokenTTL
	}
	return time.Duration(hours) * time.Hour
}
