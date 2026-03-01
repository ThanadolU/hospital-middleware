package handler_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"github.com/ThanadolU/hospital-middleware/internal/handler"
	"github.com/ThanadolU/hospital-middleware/internal/middleware"
	"github.com/ThanadolU/hospital-middleware/internal/models"
)

type MockPatientService struct{}

func (m *MockPatientService) SearchPatients(req models.SearchPatientRequest, hospitalID uuid.UUID) ([]models.Patient, error) {

	if req.NationalID == "1234567890123" {
		return []models.Patient{
			{
				FirstNameEN: "John",
				LastNameEN:  "Doe",
			},
		}, nil
	}

	return []models.Patient{}, nil
}

func MockAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("auth", middleware.AuthContext{
			HospitalID: uuid.New(),
		})
		c.Next()
	}
}

func TestSearchPatients_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)

	mockService := &MockPatientService{}
	h := handler.NewPatientHandler(mockService)

	router := gin.Default()
	router.Use(MockAuthMiddleware())
	router.GET("/patient/search", h.SearchPatients)

	req := httptest.NewRequest(http.MethodGet, "/patient/search?national_id=1234567890123", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "John")
}

func TestSearchPatients_Unauthorized(t *testing.T) {
	gin.SetMode(gin.TestMode)

	mockService := &MockPatientService{}
	h := handler.NewPatientHandler(mockService)

	router := gin.Default()
	router.GET("/patient/search", h.SearchPatients)

	req := httptest.NewRequest(http.MethodGet, "/patient/search?national_id=1234567890123", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestSearchPatients_InvalidQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)

	mockService := &MockPatientService{}
	h := handler.NewPatientHandler(mockService)

	router := gin.Default()
	router.Use(MockAuthMiddleware())
	router.GET("/patient/search", h.SearchPatients)

	req := httptest.NewRequest(http.MethodGet, "/patient/search?date_of_birth=invalid-date", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}