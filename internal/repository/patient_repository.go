package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

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

	// Upsert stores a patient under hospitalID, updating the existing record
	// when one already carries the same national ID or passport ID within that
	// hospital. It reports whether a new record was created.
	//
	// patient.HospitalID is ignored: the scope comes from the parameter, so a
	// record fetched from an upstream that named some other hospital cannot
	// write itself into it.
	Upsert(ctx context.Context, hospitalID uuid.UUID, patient *models.Patient) (created bool, err error)
}

// ErrNoIdentifier is returned when a patient has neither identifier, so there
// is no key to match an existing record on. The schema enforces the same rule;
// this catches it before the round trip and with a clearer message.
var ErrNoIdentifier = errors.New("repository: patient has neither a national ID nor a passport ID")

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

// Upsert matches on either identifier within the hospital, then updates or
// inserts accordingly.
//
// A plain ON CONFLICT will not do here. There are two partial unique indexes —
// one per identifier — and PostgreSQL infers a single arbiter per statement, so
// a record carrying both a national ID and a passport ID could conflict on the
// index that was not named and fail. Matching first inside a transaction
// handles both keys, at the cost of a round trip that a sync path can afford.
func (r *patientRepository) Upsert(
	ctx context.Context,
	hospitalID uuid.UUID,
	patient *models.Patient,
) (bool, error) {
	if hospitalID == uuid.Nil {
		return false, ErrHospitalScopeRequired
	}
	if patient == nil {
		return false, errors.New("repository: patient is required")
	}

	nationalID := strings.TrimSpace(patient.NationalID)
	passportID := strings.TrimSpace(patient.PassportID)
	if nationalID == "" && passportID == "" {
		return false, ErrNoIdentifier
	}

	// The scope is stamped from the parameter, never from the incoming record.
	patient.HospitalID = hospitalID

	created := false
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Locked so two concurrent syncs of the same patient cannot both miss
		// the existing row and then race to insert it.
		query := tx.Model(&models.Patient{}).
			Where("hospital_id = ?", hospitalID).
			Clauses(clause.Locking{Strength: "UPDATE"})

		switch {
		case nationalID != "" && passportID != "":
			query = query.Where("national_id = ? OR passport_id = ?", nationalID, passportID)
		case nationalID != "":
			query = query.Where("national_id = ?", nationalID)
		default:
			query = query.Where("passport_id = ?", passportID)
		}

		var existing models.Patient
		switch err := query.First(&existing).Error; {
		case errors.Is(err, gorm.ErrRecordNotFound):
			created = true
			if err := tx.Create(patient).Error; err != nil {
				return fmt.Errorf("repository: create patient: %w", err)
			}
			return nil
		case err != nil:
			return fmt.Errorf("repository: find patient: %w", err)
		}

		// Keep the identity of the row we matched; everything else comes from
		// upstream. Assigning the id explicitly stops GORM treating this as an
		// insert of a record that happens to carry a primary key.
		patient.ID = existing.ID
		patient.CreatedAt = existing.CreatedAt
		if err := tx.Model(&models.Patient{}).
			Where("id = ?", existing.ID).
			Select("*").
			Omit("id", "created_at").
			Updates(patient).Error; err != nil {
			return fmt.Errorf("repository: update patient: %w", err)
		}
		return nil
	})
	if err != nil {
		return false, err
	}
	return created, nil
}
