package his

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Every test here runs against httptest.NewServer. Nothing in this package
// touches the network, so the suite passes on a machine with no DNS at all —
// which matters, because the host in the brief is fictional.

// fullResponse is a complete, valid upstream body. Tests copy it and mutate
// the one field under test.
func fullResponse() map[string]any {
	return map[string]any{
		"first_name_th":  "สมชาย",
		"middle_name_th": "กลาง",
		"last_name_th":   "ใจดี",
		"first_name_en":  "Somchai",
		"middle_name_en": "Klang",
		"last_name_en":   "Jaidee",
		"date_of_birth":  "1990-05-17",
		"patient_hn":     "HN-000123",
		"national_id":    "1103700123456",
		"passport_id":    "AA1234567",
		"phone_number":   "0812345678",
		"email":          "somchai@example.com",
		"gender":         "M",
	}
}

// recordedRequest captures what the stub HIS actually received.
type recordedRequest struct {
	method     string
	escapedURL string
	accept     string
}

// newTestClient starts a stub HIS and returns a client pointed at it, plus an
// accessor for the last request the stub saw.
func newTestClient(t *testing.T, handler http.HandlerFunc) (*HospitalA, func() recordedRequest) {
	t.Helper()

	var (
		mu   sync.Mutex
		last recordedRequest
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		last = recordedRequest{
			method:     r.Method,
			escapedURL: r.URL.EscapedPath(),
			accept:     r.Header.Get("Accept"),
		}
		mu.Unlock()
		handler(w, r)
	}))
	t.Cleanup(server.Close)

	client, err := NewHospitalA(Config{
		BaseURL:    server.URL,
		Timeout:    2 * time.Second,
		HTTPClient: server.Client(),
	})
	require.NoError(t, err)

	return client, func() recordedRequest {
		mu.Lock()
		defer mu.Unlock()
		return last
	}
}

// respondJSON is a handler that returns status and body verbatim.
func respondJSON(t *testing.T, status int, body any) http.HandlerFunc {
	t.Helper()
	encoded, err := json.Marshal(body)
	require.NoError(t, err)

	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write(encoded)
	}
}

func TestSearchPatient_MapsEveryUpstreamField(t *testing.T) {
	client, _ := newTestClient(t, respondJSON(t, http.StatusOK, fullResponse()))

	patient, err := client.SearchPatient(context.Background(), "1103700123456")
	require.NoError(t, err)
	require.NotNil(t, patient)

	// All thirteen fields of the upstream contract, asserted individually.
	assert.Equal(t, "สมชาย", patient.FirstNameTH)
	assert.Equal(t, "กลาง", patient.MiddleNameTH)
	assert.Equal(t, "ใจดี", patient.LastNameTH)
	assert.Equal(t, "Somchai", patient.FirstNameEN)
	assert.Equal(t, "Klang", patient.MiddleNameEN)
	assert.Equal(t, "Jaidee", patient.LastNameEN)
	assert.Equal(t, time.Date(1990, time.May, 17, 0, 0, 0, 0, time.UTC), patient.DateOfBirth)
	assert.Equal(t, "HN-000123", patient.PatientHN)
	assert.Equal(t, "1103700123456", patient.NationalID)
	assert.Equal(t, "AA1234567", patient.PassportID)
	assert.Equal(t, "0812345678", patient.PhoneNumber)
	assert.Equal(t, "somchai@example.com", patient.Email)
	assert.Equal(t, "M", patient.Gender)
}

// The upstream payload has no hospital identity, so the client must not invent
// one. The caller stamps HospitalID from the source it queried.
func TestSearchPatient_LeavesHospitalIDUnset(t *testing.T) {
	client, _ := newTestClient(t, respondJSON(t, http.StatusOK, fullResponse()))

	patient, err := client.SearchPatient(context.Background(), "1103700123456")
	require.NoError(t, err)
	assert.Equal(t, uuid.Nil, patient.HospitalID)
}

// The brief's id path segment accepts either identifier; the client sends
// whichever it is given, unchanged.
func TestSearchPatient_AcceptsNationalIDOrPassportID(t *testing.T) {
	tests := []struct {
		name     string
		id       string
		wantPath string
	}{
		{"national id", "1103700123456", "/patient/search/1103700123456"},
		{"passport id", "AA1234567", "/patient/search/AA1234567"},
		{"id needing escaping", "AA/123 456", "/patient/search/AA%2F123%20456"},
		{"id with surrounding space", "  AA1234567  ", "/patient/search/AA1234567"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			client, lastRequest := newTestClient(t, respondJSON(t, http.StatusOK, fullResponse()))

			_, err := client.SearchPatient(context.Background(), tc.id)
			require.NoError(t, err)

			got := lastRequest()
			assert.Equal(t, http.MethodGet, got.method)
			assert.Equal(t, tc.wantPath, got.escapedURL)
			assert.Equal(t, "application/json", got.accept)
		})
	}
}

func TestSearchPatient_EmptyIDIsRejectedWithoutCallingUpstream(t *testing.T) {
	called := false
	client, _ := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	for _, id := range []string{"", "   "} {
		_, err := client.SearchPatient(context.Background(), id)
		assert.ErrorIs(t, err, ErrInvalidID)
	}
	assert.False(t, called, "no request should reach the HIS for an empty id")
}

func TestSearchPatient_NotFound(t *testing.T) {
	client, _ := newTestClient(t, respondJSON(t, http.StatusNotFound, map[string]string{
		"message": "patient not found",
	}))

	patient, err := client.SearchPatient(context.Background(), "0000000000000")

	assert.Nil(t, patient)
	assert.ErrorIs(t, err, ErrPatientNotFound)
	assert.NotErrorIs(t, err, ErrUpstream, "a clean 404 is an answer, not an outage")

	var hisErr *Error
	require.ErrorAs(t, err, &hisErr)
	assert.Equal(t, http.StatusNotFound, hisErr.StatusCode)
}

func TestSearchPatient_UpstreamStatusCodes(t *testing.T) {
	for _, status := range []int{
		http.StatusBadRequest,
		http.StatusUnauthorized,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
	} {
		t.Run(fmt.Sprintf("status %d", status), func(t *testing.T) {
			client, _ := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(status)
			})

			patient, err := client.SearchPatient(context.Background(), "1103700123456")

			assert.Nil(t, patient)
			assert.ErrorIs(t, err, ErrUpstream)

			var hisErr *Error
			require.ErrorAs(t, err, &hisErr)
			assert.Equal(t, status, hisErr.StatusCode)
		})
	}
}

func TestSearchPatient_TimesOutOnSlowUpstream(t *testing.T) {
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(server.Close)

	client, err := NewHospitalA(Config{
		BaseURL:    server.URL,
		Timeout:    50 * time.Millisecond,
		HTTPClient: server.Client(),
	})
	require.NoError(t, err)

	start := time.Now()
	patient, err := client.SearchPatient(context.Background(), "1103700123456")

	assert.Nil(t, patient)
	assert.ErrorIs(t, err, ErrUpstream)
	assert.ErrorIs(t, err, context.DeadlineExceeded, "the underlying cause stays inspectable")
	assert.Less(t, time.Since(start), 2*time.Second, "the configured timeout must bound the call")
}

// A caller's deadline must win even when it is tighter than the client's.
func TestSearchPatient_HonoursCallerContext(t *testing.T) {
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(server.Close)

	client, err := NewHospitalA(Config{
		BaseURL:    server.URL,
		Timeout:    30 * time.Second,
		HTTPClient: server.Client(),
	})
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err = client.SearchPatient(ctx, "1103700123456")
	assert.ErrorIs(t, err, ErrUpstream)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestSearchPatient_MalformedJSON(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"first_name_en": "Somchai"`)) // truncated
	})

	patient, err := client.SearchPatient(context.Background(), "1103700123456")

	assert.Nil(t, patient)
	assert.ErrorIs(t, err, ErrInvalidResponse)
}

func TestSearchPatient_RejectsUnusableRecords(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(map[string]any)
	}{
		{"no identifier at all", func(b map[string]any) {
			b["national_id"] = ""
			b["passport_id"] = ""
		}},
		{"empty date of birth", func(b map[string]any) { b["date_of_birth"] = "" }},
		{"unparseable date of birth", func(b map[string]any) { b["date_of_birth"] = "17/05/1990" }},
		{"gender outside M and F", func(b map[string]any) { b["gender"] = "X" }},
		{"empty gender", func(b map[string]any) { b["gender"] = "" }},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			body := fullResponse()
			tc.mutate(body)

			client, _ := newTestClient(t, respondJSON(t, http.StatusOK, body))

			patient, err := client.SearchPatient(context.Background(), "1103700123456")
			assert.Nil(t, patient)
			assert.ErrorIs(t, err, ErrInvalidResponse)
		})
	}
}

func TestSearchPatient_AcceptsRecordWithOnlyOneIdentifier(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(map[string]any)
	}{
		{"passport only", func(b map[string]any) { b["national_id"] = "" }},
		{"national id only", func(b map[string]any) { b["passport_id"] = "" }},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			body := fullResponse()
			tc.mutate(body)

			client, _ := newTestClient(t, respondJSON(t, http.StatusOK, body))

			patient, err := client.SearchPatient(context.Background(), "AA1234567")
			require.NoError(t, err)
			assert.NotNil(t, patient)
		})
	}
}

func TestSearchPatient_NormalisesFieldValues(t *testing.T) {
	tests := []struct {
		name       string
		mutate     func(map[string]any)
		wantDOB    time.Time
		wantGender string
	}{
		{
			name:       "plain calendar date",
			mutate:     func(map[string]any) {},
			wantDOB:    time.Date(1990, time.May, 17, 0, 0, 0, 0, time.UTC),
			wantGender: "M",
		},
		{
			name:       "rfc3339 keeps the written calendar date",
			mutate:     func(b map[string]any) { b["date_of_birth"] = "1990-05-17T00:00:00+07:00" },
			wantDOB:    time.Date(1990, time.May, 17, 0, 0, 0, 0, time.UTC),
			wantGender: "M",
		},
		{
			name:       "lowercase gender is uppercased",
			mutate:     func(b map[string]any) { b["gender"] = "f" },
			wantDOB:    time.Date(1990, time.May, 17, 0, 0, 0, 0, time.UTC),
			wantGender: "F",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			body := fullResponse()
			tc.mutate(body)

			client, _ := newTestClient(t, respondJSON(t, http.StatusOK, body))

			patient, err := client.SearchPatient(context.Background(), "1103700123456")
			require.NoError(t, err)
			assert.Equal(t, tc.wantDOB, patient.DateOfBirth)
			assert.Equal(t, tc.wantGender, patient.Gender)
		})
	}
}

func TestNewHospitalA_ValidatesConfig(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
	}{
		{"empty", ""},
		{"whitespace only", "   "},
		{"no scheme", "hospital-a.api.co.th"},
		{"unsupported scheme", "ftp://hospital-a.api.co.th"},
		{"no host", "https://"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			client, err := NewHospitalA(Config{BaseURL: tc.baseURL})
			assert.Nil(t, client)
			assert.Error(t, err)
		})
	}
}

func TestNewHospitalA_AppliesDefaults(t *testing.T) {
	client, err := NewHospitalA(Config{BaseURL: "https://hospital-a.api.co.th/"})
	require.NoError(t, err)

	assert.Equal(t, "https://hospital-a.api.co.th", client.baseURL, "trailing slash is trimmed")
	assert.Equal(t, defaultTimeout, client.timeout)
	require.NotNil(t, client.http)
	assert.NotZero(t, client.http.Timeout, "the constructed client must never be timeout-free")
}

func TestConfigFromEnv(t *testing.T) {
	t.Run("reads the base URL", func(t *testing.T) {
		t.Setenv(BaseURLEnv, "https://hospital-a.api.co.th")

		cfg, err := ConfigFromEnv()
		require.NoError(t, err)
		assert.Equal(t, "https://hospital-a.api.co.th", cfg.BaseURL)
	})

	t.Run("fails fast when unset", func(t *testing.T) {
		t.Setenv(BaseURLEnv, "")

		_, err := ConfigFromEnv()
		assert.Error(t, err, "a missing base URL must surface at boot, not at first lookup")
	})
}
