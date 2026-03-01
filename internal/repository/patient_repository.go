package repository

import (
	"github.com/ThanadolU/hospital-middleware/internal/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type PatientRepository interface {
	Search(req models.SearchPatientRequest, hospitalID uuid.UUID) ([]models.Patient, error)
}

type patientRepository struct {
	db *gorm.DB
}

func NewPatientRepository(db *gorm.DB) PatientRepository {
	return &patientRepository{db: db}
}

func (r *patientRepository) Search(req models.SearchPatientRequest, hospitalID uuid.UUID) ([]models.Patient, error)  {
	var patients []models.Patient

	query := r.db.Where("hospital_id = ?", hospitalID)

	if req.NationalID != "" {
		query = query.Where("national_id = ?", req.NationalID)
	}

	if req.PassportID != "" {
		query = query.Where("passport_id = ?", req.PassportID)
	}

	if req.FirstName != "" {
		query = query.Where(
			"(first_name_th LIKE ? OR first_name_en LIKE ?)",
			"%"+req.FirstName+"%",
			"%"+req.FirstName+"%",
		)
	}

	if req.MiddleName != "" {
		query = query.Where(
			"(first_name_th LIKE ? OR first_name_en LIKE ?)",
			"%"+req.FirstName+"%",
			"%"+req.FirstName+"%",
		)
	}

	if req.LastName != "" {
		query = query.Where(
			"(last_name_th LIKE ? OR last_name_en LIKE ?)",
			"%"+req.LastName+"%",
			"%"+req.LastName+"%",
		)
	}

	if req.DateOfBirth != "" {
		query = query.Where("date_of_birth = ?", req.DateOfBirth)
	}

	if req.PhoneNumber != "" {
		query = query.Where("phone_number = ?", req.PhoneNumber)
	}

	if req.Email != "" {
		query = query.Where("email = ?", req.Email)
	}

	if err := query.Preload("Hospital").Find(&patients).Error; err != nil {
		return nil, err
	}

	return patients, nil
}