package service

import (
	"github.com/ThanadolU/hospital-middleware/internal/models"
	"github.com/ThanadolU/hospital-middleware/internal/repository"
	"github.com/google/uuid"
)

type PatientService interface {
	SearchPatients(req models.SearchPatientRequest, hospitalID uuid.UUID) ([]models.Patient, error)
}

type patientService struct{
	repo repository.PatientRepository
}

func NewPatientService(repo repository.PatientRepository) PatientService {
	return &patientService{repo: repo}
}

func (p *patientService) SearchPatients(req models.SearchPatientRequest, hospitalID uuid.UUID) ([]models.Patient, error) {
	return p.repo.Search(req, hospitalID)
}