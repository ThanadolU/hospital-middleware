package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ThanadolU/hospital-middleware/internal/handler"
	"github.com/ThanadolU/hospital-middleware/internal/models"
	"github.com/ThanadolU/hospital-middleware/internal/service"
)

// Handler-level tests for the translation the handler owns: service sentinel
// to status code, and error text to something safe to send a client. The
// end-to-end behaviour is covered through the real router in internal/routes.

func init() { gin.SetMode(gin.TestMode) }

// stubAuth returns whatever it is configured to return.
type stubAuth struct {
	staff *models.Staff
	token string
	err   error
}

func (s stubAuth) CreateStaff(context.Context, service.CreateStaffInput) (*models.Staff, error) {
	return s.staff, s.err
}

func (s stubAuth) Login(context.Context, service.LoginInput) (string, *models.Staff, error) {
	return s.token, s.staff, s.err
}

func post(t *testing.T, h gin.HandlerFunc, body any) *httptest.ResponseRecorder {
	t.Helper()

	encoded, err := json.Marshal(body)
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(encoded))
	c.Request.Header.Set("Content-Type", "application/json")

	h(c)
	return recorder
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func validCreateBody() gin.H {
	return gin.H{"username": "somchai", "password": "correct-horse-battery", "hospital": "Hospital A"}
}

func TestCreate_MapsServiceErrorsToStatusCodes(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{"duplicate staff", service.ErrDuplicateStaff, http.StatusConflict},
		{"unknown hospital", service.ErrUnknownHospital, http.StatusBadRequest},
		{"weak password", service.ErrWeakPassword, http.StatusBadRequest},
		{"unexpected failure", assert.AnError, http.StatusInternalServerError},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := handler.NewStaffHandler(stubAuth{err: tc.err}, discardLogger())

			recorder := post(t, h.Create, validCreateBody())
			assert.Equal(t, tc.want, recorder.Code)
		})
	}
}

// v1 returned err.Error() straight to the client, surfacing Postgres constraint
// names. Whatever the service fails with, the response must not echo it.
func TestCreate_NeverLeaksTheUnderlyingError(t *testing.T) {
	leaky := assert.AnError
	h := handler.NewStaffHandler(stubAuth{err: leaky}, discardLogger())

	recorder := post(t, h.Create, validCreateBody())

	require.Equal(t, http.StatusInternalServerError, recorder.Code)
	assert.NotContains(t, recorder.Body.String(), leaky.Error())
	assert.Contains(t, recorder.Body.String(), "internal server error")
}

func TestCreate_ReturnsCreatedWithoutTheCredential(t *testing.T) {
	staff := &models.Staff{
		ID:       uuid.New(),
		Username: "somchai",
		Password: "$2a$11$a-hash-that-must-never-be-serialised",
		Hospital: &models.Hospital{Name: "Hospital A"},
	}
	h := handler.NewStaffHandler(stubAuth{staff: staff}, discardLogger())

	recorder := post(t, h.Create, validCreateBody())

	require.Equal(t, http.StatusCreated, recorder.Code)
	body := recorder.Body.String()
	assert.Contains(t, body, "somchai")
	assert.Contains(t, body, "Hospital A")
	assert.NotContains(t, body, staff.Password)
	assert.NotContains(t, body, "password")
}

func TestLogin_ReturnsOKNotCreated(t *testing.T) {
	staff := &models.Staff{ID: uuid.New(), Username: "somchai", Hospital: &models.Hospital{Name: "Hospital A"}}
	h := handler.NewStaffHandler(stubAuth{staff: staff, token: "a.b.c"}, discardLogger())

	recorder := post(t, h.Login, validCreateBody())

	// v1 answered 201 here and left a test comment admitting it was wrong.
	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.NotEqual(t, http.StatusCreated, recorder.Code)
}

func TestLogin_InvalidCredentialsIs401(t *testing.T) {
	h := handler.NewStaffHandler(stubAuth{err: service.ErrInvalidCredentials}, discardLogger())

	recorder := post(t, h.Login, validCreateBody())
	assert.Equal(t, http.StatusUnauthorized, recorder.Code)
}

func TestCreate_RejectsMalformedBody(t *testing.T) {
	h := handler.NewStaffHandler(stubAuth{}, discardLogger())

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte(`{"username":`)))
	c.Request.Header.Set("Content-Type", "application/json")

	h.Create(c)
	assert.Equal(t, http.StatusBadRequest, recorder.Code)
}
