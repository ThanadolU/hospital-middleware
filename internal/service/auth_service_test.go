package service_test

import (
	"errors"
	"testing"

	"github.com/ThanadolU/hospital-middleware/internal/models"
	"github.com/ThanadolU/hospital-middleware/internal/service"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

type MockStaffRepository struct {
	CreateFn                     func(staff *models.Staff) error
	FindByUsernameAndHospitalFn  func(username string, hospitalID uuid.UUID) (*models.Staff, error)
}

func (m *MockStaffRepository) Create(staff *models.Staff) error {
	return m.CreateFn(staff)
}

func (m *MockStaffRepository) FindByUsernameAndHospital(username string, hospitalID uuid.UUID) (*models.Staff, error) {
	return m.FindByUsernameAndHospitalFn(username, hospitalID)
}

func TestRegister_Success(t *testing.T) {
	mockRepo := &MockStaffRepository{
		CreateFn: func(staff *models.Staff) error {
			if staff.Username != "john" {
				t.Errorf("expected username john, got %s", staff.Username)
			}
			if staff.Password == "password" {
				t.Errorf("password should be hashed")
			}
			return nil
		},
	}

	authService := service.NewAuthService(mockRepo)

	err := authService.Register("john", "password", uuid.New())

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestRegister_RepoError(t *testing.T) {
	mockRepo := &MockStaffRepository{
		CreateFn: func(staff *models.Staff) error {
			return errors.New("db error")
		},
	}

	authService := service.NewAuthService(mockRepo)

	err := authService.Register("john", "password", uuid.New())

	if err == nil {
		t.Errorf("expected error, got nil")
	}
}

func TestLogin_Success(t *testing.T) {
	password := "password"
	hashed, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)

	mockRepo := &MockStaffRepository{
		FindByUsernameAndHospitalFn: func(username string, hospitalID uuid.UUID) (*models.Staff, error) {
			return &models.Staff{
				ID:         uuid.New(),
				Username:   "john",
				Password:   string(hashed),
				HospitalID: hospitalID,
			}, nil
		},
	}

	authService := service.NewAuthService(mockRepo)

	token, err := authService.Login("john", password, uuid.New())

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if token == "" {
		t.Errorf("expected token, got empty string")
	}
}

func TestLogin_UserNotFound(t *testing.T) {
	mockRepo := &MockStaffRepository{
		FindByUsernameAndHospitalFn: func(username string, hospitalID uuid.UUID) (*models.Staff, error) {
			return nil, errors.New("not found")
		},
	}

	authService := service.NewAuthService(mockRepo)

	_, err := authService.Login("john", "password", uuid.New())

	if err == nil {
		t.Errorf("expected error, got nil")
	}
}

func TestLogin_WrongPassword(t *testing.T) {
	hashed, _ := bcrypt.GenerateFromPassword([]byte("correct"), bcrypt.DefaultCost)

	mockRepo := &MockStaffRepository{
		FindByUsernameAndHospitalFn: func(username string, hospitalID uuid.UUID) (*models.Staff, error) {
			return &models.Staff{
				ID:         uuid.New(),
				Username:   "john",
				Password:   string(hashed),
				HospitalID: hospitalID,
			}, nil
		},
	}

	authService := service.NewAuthService(mockRepo)

	_, err := authService.Login("john", "wrongpassword", uuid.New())

	if err == nil {
		t.Errorf("expected error, got nil")
	}
}