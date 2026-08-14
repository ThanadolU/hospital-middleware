package testsupport

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ThanadolU/hospital-middleware/internal/models"
)

// Patient field options, so a test can state only what it is actually testing.

func WithNationalID(id string) PatientOption {
	return func(p *models.Patient) { p.NationalID = id }
}

func WithPassportID(id string) PatientOption {
	return func(p *models.Patient) { p.PassportID = id }
}

// WithPassportOnly clears the national id, which is the case v1's schema could
// not represent more than once per hospital.
func WithPassportOnly(passportID string) PatientOption {
	return func(p *models.Patient) {
		p.NationalID = ""
		p.PassportID = passportID
	}
}

func WithNamesEN(first, middle, last string) PatientOption {
	return func(p *models.Patient) {
		p.FirstNameEN, p.MiddleNameEN, p.LastNameEN = first, middle, last
	}
}

func WithNamesTH(first, middle, last string) PatientOption {
	return func(p *models.Patient) {
		p.FirstNameTH, p.MiddleNameTH, p.LastNameTH = first, middle, last
	}
}

func WithDateOfBirth(t *testing.T, iso string) PatientOption {
	return func(p *models.Patient) { p.DateOfBirth = mustDate(t, iso) }
}

func WithPhone(phone string) PatientOption {
	return func(p *models.Patient) { p.PhoneNumber = phone }
}

func WithEmail(email string) PatientOption {
	return func(p *models.Patient) { p.Email = email }
}

func WithGender(gender string) PatientOption {
	return func(p *models.Patient) { p.Gender = gender }
}

func mustDate(t *testing.T, iso string) time.Time {
	t.Helper()

	parsed, err := time.Parse("2006-01-02", iso)
	require.NoError(t, err)
	return parsed.UTC()
}
