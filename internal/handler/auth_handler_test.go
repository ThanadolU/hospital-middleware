package handler_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ThanadolU/hospital-middleware/internal/handler"
	"github.com/ThanadolU/hospital-middleware/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

// 🔹 Mock that implements service.AuthService
type MockAuthService struct {
	RegisterFunc func(username string, password string, hospitalID uuid.UUID) error
	LoginFunc    func(username string, password string, hospitalID uuid.UUID) (string, error)
}

func (m *MockAuthService) Register(username string, password string, hospitalID uuid.UUID) error {
	if m.RegisterFunc != nil {
		return m.RegisterFunc(username, password, hospitalID)
	}
	return nil
}

func (m *MockAuthService) Login(username string, password string, hospitalID uuid.UUID) (string, error) {
	if m.LoginFunc != nil {
		return m.LoginFunc(username, password, hospitalID)
	}
	return "", nil
}

func setupRouter(authService service.AuthService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.Default()

	authHandler := handler.NewAuthHandler(authService)

	r.POST("/staff/register", authHandler.Register)
	r.POST("/staff/login", authHandler.Login)

	return r
}

func TestRegister_Success(t *testing.T) {
	hospitalID := uuid.New()

	mockService := &MockAuthService{
		RegisterFunc: func(username string, password string, hospitalID uuid.UUID) error {
			return nil
		},
	}

	router := setupRouter(mockService)

	body := map[string]interface{}{
		"username":    "john",
		"password":    "password123",
		"hospital_id": hospitalID,
	}

	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest(http.MethodPost, "/staff/register", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
	assert.Contains(t, w.Body.String(), "staff registered")
}

func TestRegister_Error(t *testing.T) {
	hospitalID := uuid.New()

	mockService := &MockAuthService{
		RegisterFunc: func(username string, password string, hospitalID uuid.UUID) error {
			return errors.New("duplicate user")
		},
	}

	router := setupRouter(mockService)

	body := map[string]interface{}{
		"username":    "john",
		"password":    "password123",
		"hospital_id": hospitalID,
	}

	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest(http.MethodPost, "/staff/register", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestLogin_Success(t *testing.T) {
	hospitalID := uuid.New()

	mockService := &MockAuthService{
		LoginFunc: func(username string, password string, hospitalID uuid.UUID) (string, error) {
			return "mock-jwt-token", nil
		},
	}

	router := setupRouter(mockService)

	body := map[string]interface{}{
		"username":    "john",
		"password":    "password123",
		"hospital_id": hospitalID,
	}

	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest(http.MethodPost, "/staff/login", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code) // change to 200 if you update handler
	assert.Contains(t, w.Body.String(), "mock-jwt-token")
}

func TestLogin_InvalidCredentials(t *testing.T) {
	hospitalID := uuid.New()

	mockService := &MockAuthService{
		LoginFunc: func(username string, password string, hospitalID uuid.UUID) (string, error) {
			return "", errors.New("invalid credentials")
		},
	}

	router := setupRouter(mockService)

	body := map[string]interface{}{
		"username":    "john",
		"password":    "wrongpass",
		"hospital_id": hospitalID,
	}

	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest(http.MethodPost, "/staff/login", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestLogin_InvalidJSON(t *testing.T) {
	mockService := &MockAuthService{}
	router := setupRouter(mockService)

	req, _ := http.NewRequest(http.MethodPost, "/staff/login", bytes.NewBuffer([]byte("invalid-json")))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}