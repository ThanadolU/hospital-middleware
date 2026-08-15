// Command api runs the hospital middleware HTTP service.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/joho/godotenv"

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
	if err := loadDotenv(log); err != nil {
		return err
	}

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

	return serve(log, server)
}

// serve runs the server until the process is asked to stop, then drains.
//
// docker compose down sends SIGTERM and waits ten seconds before SIGKILL, so
// without this an in-flight patient search is severed mid-response. Shutdown
// stops accepting new connections and lets the running handlers finish; the
// deferred pool close then runs with no queries left against it.
func serve(log *slog.Logger, server *http.Server) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	listener, err := net.Listen("tcp", server.Addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", server.Addr, err)
	}
	return serveUntil(ctx, log, server, listener)
}

// serveUntil serves on listener until ctx is cancelled, then drains.
//
// Split from serve so the drain can be tested by cancelling a context, rather
// than by sending the test process a real SIGTERM — which would take the whole
// test binary down if the signal handler were not yet registered.
func serveUntil(ctx context.Context, log *slog.Logger, server *http.Server, listener net.Listener) error {
	errs := make(chan error, 1)
	go func() {
		log.Info("listening", "addr", listener.Addr().String())
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errs <- fmt.Errorf("serve: %w", err)
			return
		}
		errs <- nil
	}()

	select {
	case err := <-errs:
		// Failed before any signal arrived — a bound port, most often.
		return err
	case <-ctx.Done():
	}

	log.Info("shutting down")
	// Bounded, and shorter than compose's ten-second grace period: a drain that
	// outlives the grace period is indistinguishable from no drain at all.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	log.Info("stopped")
	return nil
}

// loadDotenv reads .env into the environment for local development.
//
// A missing file is not an error: in production the values arrive as real
// environment variables and there is no .env to find. Any other read error is
// returned rather than swallowed, because a .env that exists but cannot be
// parsed would otherwise surface as a confusing "DATABASE_URL must be set".
//
// godotenv does not overwrite variables that are already set, so an explicit
// environment always wins over the file.
func loadDotenv(log *slog.Logger) error {
	err := godotenv.Load()
	switch {
	case err == nil:
		log.Info("loaded .env")
		return nil
	case errors.Is(err, os.ErrNotExist):
		return nil
	default:
		return fmt.Errorf("load .env: %w", err)
	}
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
