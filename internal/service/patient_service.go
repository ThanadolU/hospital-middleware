package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/ThanadolU/hospital-middleware/internal/models"
	"github.com/ThanadolU/hospital-middleware/internal/repository"
)

// PatientService searches patient records.
type PatientService interface {
	// Search returns the patients in hospitalID matching req.
	//
	// hospitalID comes from the authenticated staff member's token and is a
	// parameter rather than part of req, so it cannot be supplied — or
	// widened — by anything the client sends.
	Search(ctx context.Context, hospitalID uuid.UUID, req models.SearchPatientRequest) ([]models.Patient, error)
}

type patientService struct {
	patients repository.PatientRepository
}

func NewPatientService(patients repository.PatientRepository) PatientService {
	return &patientService{patients: patients}
}

func (s *patientService) Search(
	ctx context.Context,
	hospitalID uuid.UUID,
	req models.SearchPatientRequest,
) ([]models.Patient, error) {
	patients, err := s.patients.Search(ctx, hospitalID, req)
	if err != nil {
		return nil, fmt.Errorf("service: search patients: %w", err)
	}
	return patients, nil
}
