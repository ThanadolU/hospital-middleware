package service_test

import (
	"errors"
	"testing"

	"github.com/ThanadolU/hospital-middleware/internal/models"
	"github.com/ThanadolU/hospital-middleware/internal/service"
	"github.com/google/uuid"
)

type MockPatientRepository struct {
	SearchFn func(req models.SearchPatientRequest, hospitalID uuid.UUID) ([]models.Patient, error)
}

func (m *MockPatientRepository) Search(req models.SearchPatientRequest, hospitalID uuid.UUID) ([]models.Patient, error) {
	return m.SearchFn(req, hospitalID)
}

func TestSearchPatients_Success(t *testing.T) {
	hospitalID := uuid.New()

	expectedPatients := []models.Patient{
		{
			ID:        uuid.New(),
			FirstNameEN: "John",
			LastNameEN:  "Doe",
		},
	}

	mockRepo := &MockPatientRepository{
		SearchFn: func(req models.SearchPatientRequest, hID uuid.UUID) ([]models.Patient, error) {
			// verify hospitalID forwarded correctly
			if hID != hospitalID {
				t.Errorf("expected hospitalID %v, got %v", hospitalID, hID)
			}

			// verify request forwarded correctly
			if req.NationalID != "1234567890123" {
				t.Errorf("expected nationalID 1234567890123, got %s", req.NationalID)
			}

			return expectedPatients, nil
		},
	}

	patientService := service.NewPatientService(mockRepo)

	req := models.SearchPatientRequest{
		NationalID: "1234567890123",
	}

	result, err := patientService.SearchPatients(req, hospitalID)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if len(result) != 1 {
		t.Errorf("expected 1 patient, got %d", len(result))
	}
}

func TestSearchPatients_RepoError(t *testing.T) {
	hospitalID := uuid.New()

	mockRepo := &MockPatientRepository{
		SearchFn: func(req models.SearchPatientRequest, hID uuid.UUID) ([]models.Patient, error) {
			return nil, errors.New("database error")
		},
	}

	patientService := service.NewPatientService(mockRepo)

	req := models.SearchPatientRequest{
		NationalID: "1234567890123",
	}

	result, err := patientService.SearchPatients(req, hospitalID)

	if err == nil {
		t.Errorf("expected error, got nil")
	}

	if result != nil {
		t.Errorf("expected nil result, got %v", result)
	}
}