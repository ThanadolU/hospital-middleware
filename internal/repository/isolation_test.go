package repository_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ThanadolU/hospital-middleware/internal/models"
	"github.com/ThanadolU/hospital-middleware/internal/repository"
	"github.com/ThanadolU/hospital-middleware/internal/testsupport"
)

// CROSS-HOSPITAL ISOLATION
//
// This is the security property the entire Staff model exists to enforce: a
// staff member at Hospital A must never see Hospital B's patients. It is the
// highest-priority test in the project, and it is deliberately the first file
// a reviewer opening internal/repository will meet.
//
// v1 enforced this but never tested it, so nothing would have caught a
// regression — and the one layer with no tests was where its real bugs lived.

// twoHospitals seeds two hospitals whose patients are identical in every
// searchable field, differing only in which hospital they belong to. If scoping
// were dropped, every assertion below would return both rows.
type twoHospitals struct {
	a, b       models.Hospital
	patientA   models.Patient
	patientB   models.Patient
	repository repository.PatientRepository
}

func seedTwoHospitals(t *testing.T) twoHospitals {
	t.Helper()

	db := testsupport.NewDB(t)

	hospitalA := testsupport.NewHospital(t, db, "Hospital A")
	hospitalB := testsupport.NewHospital(t, db, "Hospital B")

	// Same person, recorded at both hospitals: identical name, DOB, phone,
	// email and national id. Only hospital_id differs.
	const (
		nationalID = "1103700123456"
		email      = "somchai@example.com"
		phone      = "0812345678"
	)
	shared := []testsupport.PatientOption{
		testsupport.WithNamesEN("Somchai", "Klang", "Jaidee"),
		testsupport.WithNamesTH("สมชาย", "กลาง", "ใจดี"),
		testsupport.WithNationalID(nationalID),
		testsupport.WithEmail(email),
		testsupport.WithPhone(phone),
		testsupport.WithDateOfBirth(t, "1990-05-17"),
	}

	return twoHospitals{
		a:          hospitalA,
		b:          hospitalB,
		patientA:   testsupport.NewPatient(t, db, hospitalA.ID, shared...),
		patientB:   testsupport.NewPatient(t, db, hospitalB.ID, shared...),
		repository: repository.NewPatientRepository(db),
	}
}

// The headline case: an unfiltered search returns only the caller's hospital.
func TestSearch_NeverReturnsAnotherHospitalsPatients(t *testing.T) {
	seed := seedTwoHospitals(t)

	found, err := seed.repository.Search(context.Background(), seed.a.ID, models.SearchPatientRequest{})
	require.NoError(t, err)

	require.Len(t, found, 1, "Hospital A's staff must see exactly Hospital A's patients")
	assert.Equal(t, seed.patientA.ID, found[0].ID)
	assert.Equal(t, seed.a.ID, found[0].HospitalID)

	for _, patient := range found {
		assert.NotEqual(t, seed.patientB.ID, patient.ID, "Hospital B's patient leaked into Hospital A's results")
		assert.NotEqual(t, seed.b.ID, patient.HospitalID)
	}
}

// Isolation must hold on every search field individually. A criterion that
// matches the other hospital's patient exactly must still return nothing.
func TestSearch_IsolationHoldsForEverySearchField(t *testing.T) {
	seed := seedTwoHospitals(t)

	tests := []struct {
		field string
		req   models.SearchPatientRequest
	}{
		{"national_id", models.SearchPatientRequest{NationalID: "1103700123456"}},
		{"first_name", models.SearchPatientRequest{FirstName: "Somchai"}},
		{"middle_name", models.SearchPatientRequest{MiddleName: "Klang"}},
		{"last_name", models.SearchPatientRequest{LastName: "Jaidee"}},
		{"first_name th", models.SearchPatientRequest{FirstName: "สมชาย"}},
		{"date_of_birth", models.SearchPatientRequest{DateOfBirth: "1990-05-17"}},
		{"phone_number", models.SearchPatientRequest{PhoneNumber: "0812345678"}},
		{"email", models.SearchPatientRequest{Email: "somchai@example.com"}},
	}

	for _, tc := range tests {
		t.Run(tc.field, func(t *testing.T) {
			// Searching as Hospital B's staff must find only B's record, even
			// though A holds a patient matching the same criterion exactly.
			found, err := seed.repository.Search(context.Background(), seed.b.ID, tc.req)
			require.NoError(t, err)

			require.Len(t, found, 1, "searching by %s crossed the hospital boundary", tc.field)
			assert.Equal(t, seed.patientB.ID, found[0].ID)
			assert.Equal(t, seed.b.ID, found[0].HospitalID)
		})
	}
}

// A hospital with no patients must return an empty result, not everyone else's.
func TestSearch_EmptyHospitalSeesNobody(t *testing.T) {
	db := testsupport.NewDB(t)

	populated := testsupport.NewHospital(t, db, "Populated Hospital")
	empty := testsupport.NewHospital(t, db, "Empty Hospital")
	testsupport.NewPatient(t, db, populated.ID)

	found, err := repository.NewPatientRepository(db).
		Search(context.Background(), empty.ID, models.SearchPatientRequest{})
	require.NoError(t, err)
	assert.Empty(t, found)
}

// A zero hospital id is a programming error — a scope that was never set. It
// must fail rather than fall through to an unscoped query returning every
// patient in the system.
func TestSearch_RefusesToRunWithoutAHospitalScope(t *testing.T) {
	seed := seedTwoHospitals(t)

	found, err := seed.repository.Search(context.Background(), uuid.Nil, models.SearchPatientRequest{})

	assert.ErrorIs(t, err, repository.ErrHospitalScopeRequired)
	assert.Empty(t, found, "an unscoped search must return nothing, never everything")
}

// LIKE wildcards in user input must not widen the search beyond the hospital,
// nor turn a narrowing field into a table dump.
func TestSearch_WildcardInputCannotWidenTheScope(t *testing.T) {
	seed := seedTwoHospitals(t)

	for _, wildcard := range []string{"%", "_", "%%", `\`} {
		t.Run("name is "+wildcard, func(t *testing.T) {
			found, err := seed.repository.Search(
				context.Background(), seed.a.ID,
				models.SearchPatientRequest{FirstName: wildcard},
			)
			require.NoError(t, err)
			assert.Empty(t, found,
				"%q was treated as a pattern rather than a literal name", wildcard)
		})
	}
}

// An unknown hospital id must return nothing rather than everything.
func TestSearch_UnknownHospitalReturnsNothing(t *testing.T) {
	seed := seedTwoHospitals(t)

	found, err := seed.repository.Search(context.Background(), uuid.New(), models.SearchPatientRequest{})
	require.NoError(t, err)
	assert.Empty(t, found)
}
