package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/ThanadolU/hospital-middleware/internal/models"
)

// ErrHospitalScopeRequired is returned when a search is attempted without a
// hospital. It exists so that a missing scope fails loudly instead of quietly
// returning every patient in the system.
var ErrHospitalScopeRequired = errors.New("repository: hospital scope is required")

// PatientRepository reads and writes patient records.
//
// Every method takes hospitalID as its own parameter rather than reading it
// from a request struct. That is the whole isolation design: the scope cannot
// be omitted without failing to compile, and cannot be widened by anything a
// client sends.
type PatientRepository interface {
	// Search returns every patient in hospitalID matching req. With no criteria
	// set it returns all of that hospital's patients — the brief asks for all
	// matches, so there is no implicit page limit to truncate them.
	Search(ctx context.Context, hospitalID uuid.UUID, req models.SearchPatientRequest) ([]models.Patient, error)
}

type patientRepository struct {
	db *gorm.DB
}

func NewPatientRepository(db *gorm.DB) PatientRepository {
	return &patientRepository{db: db}
}

func (r *patientRepository) Search(
	ctx context.Context,
	hospitalID uuid.UUID,
	req models.SearchPatientRequest,
) ([]models.Patient, error) {
	if hospitalID == uuid.Nil {
		return nil, ErrHospitalScopeRequired
	}

	// The scope is applied first and unconditionally. Nothing below can remove
	// it; every other clause only narrows the result further.
	query := r.db.WithContext(ctx).
		Model(&models.Patient{}).
		Where("hospital_id = ?", hospitalID)

	if v := strings.TrimSpace(req.NationalID); v != "" {
		query = query.Where("national_id = ?", v)
	}
	if v := strings.TrimSpace(req.PassportID); v != "" {
		query = query.Where("passport_id = ?", v)
	}

	// A name may be recorded in Thai or English, so each name criterion matches
	// against both columns.
	if v := strings.TrimSpace(req.FirstName); v != "" {
		query = whereEitherScript(query, "first_name_th", "first_name_en", v)
	}
	if v := strings.TrimSpace(req.MiddleName); v != "" {
		// v1 filtered this branch on req.FirstName against the first_name
		// columns, so middle_name silently returned wrong results despite being
		// one of the eight documented search fields.
		query = whereEitherScript(query, "middle_name_th", "middle_name_en", v)
	}
	if v := strings.TrimSpace(req.LastName); v != "" {
		query = whereEitherScript(query, "last_name_th", "last_name_en", v)
	}

	if v := strings.TrimSpace(req.DateOfBirth); v != "" {
		query = query.Where("date_of_birth = ?", v)
	}
	if v := strings.TrimSpace(req.PhoneNumber); v != "" {
		query = query.Where("phone_number = ?", v)
	}
	if v := strings.TrimSpace(req.Email); v != "" {
		// Must be lower(email): the supporting index is on lower(email), and a
		// plain `email = ?` cannot use it. See docs/DECISIONS.md.
		query = query.Where("lower(email) = lower(?)", v)
	}

	// A stable order keeps results reproducible for the caller and for tests.
	patients := []models.Patient{}
	if err := query.Order("created_at ASC, id ASC").Find(&patients).Error; err != nil {
		return nil, fmt.Errorf("repository: search patients: %w", err)
	}
	return patients, nil
}

// whereEitherScript matches term against the Thai or English form of a name,
// case-insensitively and as a substring.
func whereEitherScript(query *gorm.DB, thColumn, enColumn, term string) *gorm.DB {
	pattern := "%" + escapeLike(term) + "%"
	return query.Where(
		fmt.Sprintf(`(%s ILIKE @pattern ESCAPE '\' OR %s ILIKE @pattern ESCAPE '\')`, thColumn, enColumn),
		map[string]any{"pattern": pattern},
	)
}

// escapeLike neutralises the LIKE wildcards in user input.
//
// Without this, searching for "%" matches every patient in the hospital: not an
// injection (the value is still parameterised) but a silent way to dump the
// whole table through a field meant to narrow results.
func escapeLike(s string) string {
	replacer := strings.NewReplacer(
		`\`, `\\`,
		`%`, `\%`,
		`_`, `\_`,
	)
	return replacer.Replace(s)
}
