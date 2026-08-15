package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"github.com/ThanadolU/hospital-middleware/internal/models"
)

// ErrHospitalNotFound means no hospital carries that name.
var ErrHospitalNotFound = errors.New("repository: hospital not found")

// HospitalRepository resolves hospitals.
//
// Lookup is by name, not by id: the brief's staff endpoints take a `hospital`
// field, and v1 required a UUID that a caller had no way to discover.
type HospitalRepository interface {
	// FindByName resolves a hospital case-insensitively, matching the unique
	// index on lower(name).
	FindByName(ctx context.Context, name string) (*models.Hospital, error)
}

type hospitalRepository struct {
	db *gorm.DB
}

func NewHospitalRepository(db *gorm.DB) HospitalRepository {
	return &hospitalRepository{db: db}
}

func (r *hospitalRepository) FindByName(ctx context.Context, name string) (*models.Hospital, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return nil, ErrHospitalNotFound
	}

	var hospital models.Hospital
	err := r.db.WithContext(ctx).
		Where("lower(name) = lower(?)", trimmed).
		First(&hospital).Error

	switch {
	case err == nil:
		return &hospital, nil
	case errors.Is(err, gorm.ErrRecordNotFound):
		return nil, ErrHospitalNotFound
	default:
		return nil, fmt.Errorf("repository: find hospital: %w", err)
	}
}
