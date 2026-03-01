package repository

import (
	"github.com/ThanadolU/hospital-middleware/internal/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type StaffRepository interface {
	FindByUsernameAndHospital(username string, hospitalID uuid.UUID) (*models.Staff, error)
	Create(staff *models.Staff) error
}

type staffRepository struct {
	db *gorm.DB
}

func NewStaffRepository(db *gorm.DB) StaffRepository {
	return &staffRepository{db: db}
}

func (r *staffRepository) FindByUsernameAndHospital(username string, hospitalID uuid.UUID) (*models.Staff, error) {
	var staff models.Staff
	err := r.db.Where("username = ? AND hospital_id = ?", username, hospitalID).First(&staff).Error
	return &staff, err
}

func (r *staffRepository) Create(staff *models.Staff) error {
	return r.db.Create(staff).Error
}
