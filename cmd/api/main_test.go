package main

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ThanadolU/hospital-middleware/internal/auth"
)

// The wiring in run() needs a database and is covered end to end through
// internal/routes. What is testable here in isolation is the configuration
// reading around it — where the defaults and the failure modes live, and where
// a mistake means the service boots wrong rather than not at all.

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestRequiredEnv(t *testing.T) {
	t.Run("returns the value when set", func(t *testing.T) {
		t.Setenv("SOME_REQUIRED_VALUE", "present")

		value, err := requiredEnv("SOME_REQUIRED_VALUE")
		require.NoError(t, err)
		assert.Equal(t, "present", value)
	})

	// A missing secret must stop the boot. v1 shipped a hardcoded fallback, so
	// a deployment that forgot to set one ran on a key that was public.
	t.Run("errors when unset", func(t *testing.T) {
		t.Setenv("SOME_REQUIRED_VALUE", "")

		_, err := requiredEnv("SOME_REQUIRED_VALUE")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "SOME_REQUIRED_VALUE")
	})
}

func TestPort(t *testing.T) {
	tests := []struct {
		name string
		env  string
		want string
	}{
		{"defaults when unset", "", "8000"},
		{"honours the environment", "9999", "9999"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("PORT", tc.env)
			assert.Equal(t, tc.want, port())
		})
	}
}

// The Dockerfile's EXPOSE is written against this default. If the default
// changes and EXPOSE does not, `docker run -P` publishes the wrong port —
// which is exactly the mismatch v1 shipped.
func TestPort_DefaultMatchesTheDockerfileExposeDirective(t *testing.T) {
	t.Setenv("PORT", "")

	dockerfile, err := os.ReadFile(filepath.Join("..", "..", "Dockerfile"))
	require.NoError(t, err)

	assert.Contains(t, string(dockerfile), "EXPOSE "+port(),
		"the Dockerfile exposes a different port than cmd/api binds by default")
}

func TestTokenTTL(t *testing.T) {
	tests := []struct {
		name string
		env  string
		want time.Duration
	}{
		{"defaults when unset", "", auth.DefaultTokenTTL},
		{"reads whole hours", "3", 3 * time.Hour},
		// Anything unusable falls back rather than producing a token that
		// expires immediately, or one that never expires.
		{"falls back on non-numeric", "soon", auth.DefaultTokenTTL},
		{"falls back on zero", "0", auth.DefaultTokenTTL},
		{"falls back on negative", "-5", auth.DefaultTokenTTL},
		{"falls back on fractional", "1.5", auth.DefaultTokenTTL},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("JWT_TTL_HOURS", tc.env)
			assert.Equal(t, tc.want, tokenTTL())
		})
	}
}

func TestLoadDotenv(t *testing.T) {
	// A missing .env is the normal production case: the values arrive as real
	// environment variables and there is no file to read.
	t.Run("a missing file is not an error", func(t *testing.T) {
		t.Chdir(t.TempDir())

		assert.NoError(t, loadDotenv(discardLogger()))
	})

	t.Run("reads values from the file", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, ".env"),
			[]byte("LOADED_FROM_DOTENV=yes\n"), 0o600))
		t.Chdir(dir)

		require.NoError(t, loadDotenv(discardLogger()))
		assert.Equal(t, "yes", os.Getenv("LOADED_FROM_DOTENV"))
	})

	// An explicit environment variable must win, so a stale .env cannot
	// override what a deployment set deliberately.
	t.Run("does not overwrite an existing variable", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, ".env"),
			[]byte("ALREADY_SET=from-file\n"), 0o600))
		t.Chdir(dir)
		t.Setenv("ALREADY_SET", "from-environment")

		require.NoError(t, loadDotenv(discardLogger()))
		assert.Equal(t, "from-environment", os.Getenv("ALREADY_SET"))
	})
}

// `docker compose down` sends SIGTERM. Without a drain, a patient search in
// flight is severed mid-response. The signal wiring itself is one line in
// serve(); what matters, and what is tested here, is that an in-flight request
// is allowed to finish once shutdown begins.
func TestServeUntil_DrainsRequestsInFlight(t *testing.T) {
	released := make(chan struct{})
	mux := http.NewServeMux()
	mux.HandleFunc("/slow", func(w http.ResponseWriter, _ *http.Request) {
		<-released
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("finished"))
	})

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	server := &http.Server{Handler: mux}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- serveUntil(ctx, discardLogger(), server, listener) }()

	response := make(chan *http.Response, 1)
	go func() {
		resp, err := http.Get("http://" + listener.Addr().String() + "/slow")
		if err == nil {
			response <- resp
		}
	}()

	// Let the request reach the handler and block there, then begin shutdown
	// while it is still in flight.
	time.Sleep(200 * time.Millisecond)
	cancel()
	time.Sleep(200 * time.Millisecond)
	close(released)

	select {
	case resp := <-response:
		defer func() { _ = resp.Body.Close() }()
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "finished", string(body),
			"the in-flight request was cut off instead of drained")
	case <-time.After(5 * time.Second):
		t.Fatal("the in-flight request never completed")
	}

	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("serveUntil did not return after the context was cancelled")
	}
}

// A port already in use must be reported, not swallowed into a server that
// silently serves nothing.
func TestServe_ReportsABoundPort(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = listener.Close() }()

	err = serve(discardLogger(), &http.Server{Addr: listener.Addr().String()})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "listen")
}
