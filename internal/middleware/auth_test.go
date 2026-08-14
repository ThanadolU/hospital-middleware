package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ThanadolU/hospital-middleware/internal/auth"
	"github.com/ThanadolU/hospital-middleware/internal/models"
)

const testSecret = "a-test-secret-that-is-long-enough-to-pass"

func init() { gin.SetMode(gin.TestMode) }

func newTokens(t *testing.T, ttl time.Duration) *auth.TokenService {
	t.Helper()

	tokens, err := auth.NewTokenService(testSecret, ttl)
	require.NoError(t, err)
	return tokens
}

// guardedRouter mounts a probe behind RequireAuth that echoes the hospital the
// middleware resolved, so tests can assert both access and scope.
func guardedRouter(t *testing.T, tokens *auth.TokenService) *gin.Engine {
	t.Helper()

	r := gin.New()
	r.GET("/probe", RequireAuth(tokens), func(c *gin.Context) {
		hospitalID, err := HospitalIDFrom(c)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "no scope"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"hospital_id": hospitalID.String()})
	})
	return r
}

func get(r *gin.Engine, header string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/probe", nil)
	if header != "" {
		req.Header.Set("Authorization", header)
	}
	recorder := httptest.NewRecorder()
	r.ServeHTTP(recorder, req)
	return recorder
}

func TestRequireAuth_AcceptsAValidTokenAndExposesTheHospital(t *testing.T) {
	tokens := newTokens(t, time.Hour)
	staff := models.Staff{ID: uuid.New(), HospitalID: uuid.New(), Username: "somchai"}

	token, err := tokens.Issue(staff)
	require.NoError(t, err)

	recorder := get(guardedRouter(t, tokens), "Bearer "+token)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), staff.HospitalID.String(),
		"the middleware must expose the token's hospital as the search scope")
}

// The scheme is case-insensitive per RFC 7235, so a client sending "bearer"
// must not be rejected.
func TestRequireAuth_AcceptsSchemeInAnyCase(t *testing.T) {
	tokens := newTokens(t, time.Hour)
	token, err := tokens.Issue(models.Staff{ID: uuid.New(), HospitalID: uuid.New()})
	require.NoError(t, err)

	for _, scheme := range []string{"Bearer", "bearer", "BEARER", "BeArEr"} {
		recorder := get(guardedRouter(t, tokens), scheme+" "+token)
		assert.Equal(t, http.StatusOK, recorder.Code, "scheme %q was rejected", scheme)
	}
}

func TestRequireAuth_RejectsBadHeaders(t *testing.T) {
	tokens := newTokens(t, time.Hour)
	token, err := tokens.Issue(models.Staff{ID: uuid.New(), HospitalID: uuid.New()})
	require.NoError(t, err)

	tests := []struct {
		name   string
		header string
	}{
		{"absent", ""},
		{"whitespace only", "   "},
		{"no scheme", token},
		{"wrong scheme", "Basic " + token},
		{"scheme with no credential", "Bearer"},
		{"scheme with empty credential", "Bearer "},
		{"garbage credential", "Bearer not-a-token"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			recorder := get(guardedRouter(t, tokens), tc.header)
			assert.Equal(t, http.StatusUnauthorized, recorder.Code)
		})
	}
}

// The algorithm-confusion case, asserted at the middleware boundary as well as
// in the token service: an unsigned token must never reach a handler.
func TestRequireAuth_RejectsUnsignedToken(t *testing.T) {
	tokens := newTokens(t, time.Hour)

	forged := "eyJhbGciOiJub25lIiwidHlwIjoiSldUIn0." +
		"eyJzdWIiOiIwMDAwMDAwMC0wMDAwLTAwMDAtMDAwMC0wMDAwMDAwMDAwMDEifQ."

	recorder := get(guardedRouter(t, tokens), "Bearer "+forged)
	assert.Equal(t, http.StatusUnauthorized, recorder.Code)
}

func TestRequireAuth_RejectsExpiredToken(t *testing.T) {
	tokens := newTokens(t, time.Nanosecond)
	token, err := tokens.Issue(models.Staff{ID: uuid.New(), HospitalID: uuid.New()})
	require.NoError(t, err)
	time.Sleep(2 * time.Millisecond)

	recorder := get(guardedRouter(t, tokens), "Bearer "+token)
	require.Equal(t, http.StatusUnauthorized, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "expired",
		"an expired token should tell the caller to log in again")
}

// A token signed with a different secret must not be accepted.
func TestRequireAuth_RejectsForeignSignature(t *testing.T) {
	foreign, err := auth.NewTokenService("a-completely-different-secret-value-here", time.Hour)
	require.NoError(t, err)
	token, err := foreign.Issue(models.Staff{ID: uuid.New(), HospitalID: uuid.New()})
	require.NoError(t, err)

	recorder := get(guardedRouter(t, newTokens(t, time.Hour)), "Bearer "+token)
	assert.Equal(t, http.StatusUnauthorized, recorder.Code)
}

// A guarded handler must never see a request the middleware rejected.
func TestRequireAuth_DoesNotRunTheHandlerOnFailure(t *testing.T) {
	tokens := newTokens(t, time.Hour)

	var reached bool
	r := gin.New()
	r.GET("/probe", RequireAuth(tokens), func(c *gin.Context) {
		reached = true
		c.Status(http.StatusOK)
	})

	get(r, "Bearer garbage")
	assert.False(t, reached, "the handler ran despite authentication failing")
}

// HospitalIDFrom must fail closed when a handler is mounted outside the
// middleware, rather than returning a zero uuid that would run unscoped.
func TestHospitalIDFrom_FailsClosed(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*gin.Context)
	}{
		{"nothing set", func(*gin.Context) {}},
		{"wrong type", func(c *gin.Context) { c.Set(ContextHospitalID, "not-a-uuid") }},
		{"zero uuid", func(c *gin.Context) { c.Set(ContextHospitalID, uuid.Nil) }},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			tc.setup(c)

			hospitalID, err := HospitalIDFrom(c)
			assert.Error(t, err)
			assert.Equal(t, uuid.Nil, hospitalID)
		})
	}
}

func TestHospitalIDFrom_ReturnsTheScope(t *testing.T) {
	expected := uuid.New()

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set(ContextHospitalID, expected)

	hospitalID, err := HospitalIDFrom(c)
	require.NoError(t, err)
	assert.Equal(t, expected, hospitalID)
}
