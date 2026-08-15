package handler_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ThanadolU/hospital-middleware/internal/handler"
	"github.com/ThanadolU/hospital-middleware/internal/middleware"
	"github.com/ThanadolU/hospital-middleware/internal/models"
)

// Handler-level tests for what the search handler alone decides: that it
// refuses to run without an authenticated hospital, that it rejects
// unparseable criteria, and that a service failure becomes a 500 carrying no
// detail. Searching itself is covered against a real database in
// internal/repository, and end to end through the real router in
// internal/routes.

// stubPatients records the scope it was called with, so a test can prove the
// handler passed the token's hospital rather than anything from the query.
type stubPatients struct {
	patients   []models.Patient
	err        error
	calls      int
	gotScope   uuid.UUID
	gotRequest models.SearchPatientRequest
}

func (s *stubPatients) Search(_ context.Context, hospitalID uuid.UUID, req models.SearchPatientRequest) ([]models.Patient, error) {
	s.calls++
	s.gotScope = hospitalID
	s.gotRequest = req
	return s.patients, s.err
}

// searchWithScope drives the handler with an authenticated hospital already in
// the context, which is what RequireAuth would have put there.
func searchWithScope(t *testing.T, h *handler.PatientHandler, scope uuid.UUID, query string) *httptest.ResponseRecorder {
	t.Helper()

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/patient/search"+query, nil)
	c.Set(middleware.ContextHospitalID, scope)

	h.Search(c)
	return recorder
}

// The handler must fail closed. If it is ever reached without authentication —
// a route registered outside the guarded group, say — the failure mode has to
// be 401, never an unscoped search across every hospital in the system.
func TestSearch_WithoutAnAuthenticatedHospitalIs401(t *testing.T) {
	patients := &stubPatients{}
	h := handler.NewPatientHandler(patients, discardLogger())

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/patient/search", nil)
	// Deliberately no hospital in the context.

	h.Search(c)

	assert.Equal(t, http.StatusUnauthorized, recorder.Code)
	assert.Zero(t, patients.calls, "the search must not run without a hospital scope")
}

func TestSearch_RejectsUnparseableCriteria(t *testing.T) {
	// Every field is optional, so these fail on their own constraints rather
	// than on being absent.
	tests := []struct {
		name  string
		query string
	}{
		{"date of birth in the wrong format", "?date_of_birth=17-05-1990"},
		{"date of birth that is not a date", "?date_of_birth=yesterday"},
		{"national id beyond its maximum", "?national_id=" + longString(21)},
		{"first name beyond its maximum", "?first_name=" + longString(101)},
		{"email beyond its maximum", "?email=" + longString(255)},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			patients := &stubPatients{}
			h := handler.NewPatientHandler(patients, discardLogger())

			recorder := searchWithScope(t, h, uuid.New(), tc.query)

			assert.Equal(t, http.StatusBadRequest, recorder.Code)
			assert.Contains(t, recorder.Body.String(), "invalid search parameters")
			assert.Zero(t, patients.calls, "invalid criteria must not reach the database")
		})
	}
}

// The mirror of the above: a valid date must be accepted, so the test proves
// the format constraint rather than that dates are rejected generally.
func TestSearch_AcceptsAWellFormedDateOfBirth(t *testing.T) {
	patients := &stubPatients{}
	h := handler.NewPatientHandler(patients, discardLogger())

	recorder := searchWithScope(t, h, uuid.New(), "?date_of_birth=1990-05-17")

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, 1, patients.calls)
	assert.Equal(t, "1990-05-17", patients.gotRequest.DateOfBirth)
}

// The scope reaching the service must be the one from the context, whatever the
// query string claims. internal/routes proves this end to end against a real
// database; this pins it at the boundary the handler itself owns.
func TestSearch_PassesTheContextScopeNotTheQueryString(t *testing.T) {
	scope := uuid.New()
	other := uuid.New()
	patients := &stubPatients{}
	h := handler.NewPatientHandler(patients, discardLogger())

	recorder := searchWithScope(t, h, scope, "?hospital_id="+other.String())

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, scope, patients.gotScope, "the query string widened the hospital scope")
}

func TestSearch_ServiceFailureIs500AndLeaksNothing(t *testing.T) {
	leaky := errors.New(`pq: relation "patients" does not exist`)
	patients := &stubPatients{err: leaky}
	h := handler.NewPatientHandler(patients, discardLogger())

	recorder := searchWithScope(t, h, uuid.New(), "")

	require.Equal(t, http.StatusInternalServerError, recorder.Code)
	assert.NotContains(t, recorder.Body.String(), leaky.Error())
	assert.NotContains(t, recorder.Body.String(), "patients")
	assert.Contains(t, recorder.Body.String(), "internal server error")
}

// No match is an empty result, not an error, and the envelope stays the same
// shape so a client never has to special-case it.
func TestSearch_NoMatchesIsAnEmptyListNotAnError(t *testing.T) {
	patients := &stubPatients{patients: []models.Patient{}}
	h := handler.NewPatientHandler(patients, discardLogger())

	recorder := searchWithScope(t, h, uuid.New(), "?first_name=Nobody")

	require.Equal(t, http.StatusOK, recorder.Code)
	body := recorder.Body.String()
	assert.Contains(t, body, `"data":[]`)
	assert.Contains(t, body, `"total":0`)
}

func longString(n int) string {
	out := make([]byte, n)
	for i := range out {
		out[i] = 'a'
	}
	return string(out)
}
