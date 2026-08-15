package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ThanadolU/hospital-middleware/internal/models"
	"github.com/ThanadolU/hospital-middleware/internal/repository"
	"github.com/ThanadolU/hospital-middleware/internal/testsupport"
)

// Upsert is the write path the HIS sync uses. Its whole job is to be safe to
// run repeatedly against records that may arrive keyed on either identifier,
// so these cover matching, merging, and the refusals.

func upstreamPatient() *models.Patient {
	return &models.Patient{
		FirstNameTH: "สมชาย", LastNameTH: "ใจดี",
		FirstNameEN: "Somchai", LastNameEN: "Jaidee",
		DateOfBirth: time.Date(1990, 5, 17, 0, 0, 0, 0, time.UTC),
		PatientHN:   "HN-000123",
		NationalID:  "1103700123456",
		PassportID:  "AA1234567",
		PhoneNumber: "0812345678",
		Email:       "somchai@example.com",
		Gender:      "M",
	}
}

func TestUpsert_CreatesThenUpdates(t *testing.T) {
	db := testsupport.NewDB(t)
	repo := repository.NewPatientRepository(db)
	hospital := testsupport.NewHospital(t, db, "Upsert Hospital")

	created, err := repo.Upsert(context.Background(), hospital.ID, upstreamPatient())
	require.NoError(t, err)
	assert.True(t, created, "the first upsert should have created the record")

	updated := upstreamPatient()
	updated.PhoneNumber = "0899999999"

	created, err = repo.Upsert(context.Background(), hospital.ID, updated)
	require.NoError(t, err)
	assert.False(t, created, "the second upsert should have matched the existing record")

	var stored []models.Patient
	require.NoError(t, db.Where("hospital_id = ?", hospital.ID).Find(&stored).Error)
	require.Len(t, stored, 1, "the upsert duplicated the patient")
	assert.Equal(t, "0899999999", stored[0].PhoneNumber, "the update did not take")
}

// The upsert has to match on either identifier, because an upstream may return
// a record keyed on the one we did not search by.
func TestUpsert_MatchesOnEitherIdentifier(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*models.Patient)
	}{
		{"same national id, passport absent", func(p *models.Patient) { p.PassportID = "" }},
		{"same passport id, national absent", func(p *models.Patient) { p.NationalID = "" }},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			db := testsupport.NewDB(t)
			repo := repository.NewPatientRepository(db)
			hospital := testsupport.NewHospital(t, db, "Identifier Hospital")

			created, err := repo.Upsert(context.Background(), hospital.ID, upstreamPatient())
			require.NoError(t, err)
			require.True(t, created)

			second := upstreamPatient()
			second.PatientHN = "HN-CHANGED"
			tc.mutate(second)

			created, err = repo.Upsert(context.Background(), hospital.ID, second)
			require.NoError(t, err)
			assert.False(t, created, "a record sharing one identifier is the same patient")

			var count int64
			require.NoError(t, db.Model(&models.Patient{}).
				Where("hospital_id = ?", hospital.ID).Count(&count).Error)
			assert.Equal(t, int64(1), count)
		})
	}
}

// Identifiers are unique per hospital, not globally, so the same upstream
// patient synced by two hospitals yields one record each.
func TestUpsert_IsScopedPerHospital(t *testing.T) {
	db := testsupport.NewDB(t)
	repo := repository.NewPatientRepository(db)
	first := testsupport.NewHospital(t, db, "Scoped Hospital One")
	second := testsupport.NewHospital(t, db, "Scoped Hospital Two")

	createdFirst, err := repo.Upsert(context.Background(), first.ID, upstreamPatient())
	require.NoError(t, err)
	createdSecond, err := repo.Upsert(context.Background(), second.ID, upstreamPatient())
	require.NoError(t, err)

	assert.True(t, createdFirst)
	assert.True(t, createdSecond, "the second hospital must get its own record, not a match on the first's")

	for _, hospital := range []uuid.UUID{first.ID, second.ID} {
		var count int64
		require.NoError(t, db.Model(&models.Patient{}).
			Where("hospital_id = ?", hospital).Count(&count).Error)
		assert.Equal(t, int64(1), count)
	}
}

// The scope comes from the parameter. A record naming some other hospital must
// not be able to write itself into it.
func TestUpsert_IgnoresTheHospitalOnTheIncomingRecord(t *testing.T) {
	db := testsupport.NewDB(t)
	repo := repository.NewPatientRepository(db)
	hospital := testsupport.NewHospital(t, db, "Stamping Hospital")
	other := testsupport.NewHospital(t, db, "Other Hospital")

	patient := upstreamPatient()
	patient.HospitalID = other.ID

	_, err := repo.Upsert(context.Background(), hospital.ID, patient)
	require.NoError(t, err)

	var stored models.Patient
	require.NoError(t, db.Where("national_id = ?", patient.NationalID).First(&stored).Error)
	assert.Equal(t, hospital.ID, stored.HospitalID,
		"the record was written into the hospital it named rather than the caller's")
}

func TestUpsert_Refusals(t *testing.T) {
	db := testsupport.NewDB(t)
	repo := repository.NewPatientRepository(db)
	hospital := testsupport.NewHospital(t, db, "Refusal Hospital")

	t.Run("a zero hospital is rejected", func(t *testing.T) {
		_, err := repo.Upsert(context.Background(), uuid.Nil, upstreamPatient())
		assert.ErrorIs(t, err, repository.ErrHospitalScopeRequired)
	})

	t.Run("a patient with no identifier is rejected", func(t *testing.T) {
		patient := upstreamPatient()
		patient.NationalID = ""
		patient.PassportID = ""

		_, err := repo.Upsert(context.Background(), hospital.ID, patient)
		assert.ErrorIs(t, err, repository.ErrNoIdentifier)
	})

	// Whitespace is not an identifier. Without trimming, "  " would be stored
	// as a distinct key and every sync would create a new record.
	t.Run("whitespace is not an identifier", func(t *testing.T) {
		patient := upstreamPatient()
		patient.NationalID = "   "
		patient.PassportID = ""

		_, err := repo.Upsert(context.Background(), hospital.ID, patient)
		assert.ErrorIs(t, err, repository.ErrNoIdentifier)
	})

	t.Run("a nil patient is rejected", func(t *testing.T) {
		_, err := repo.Upsert(context.Background(), hospital.ID, nil)
		assert.Error(t, err)
	})
}

// An upserted record must be findable through the normal search path — storing
// it in a way search cannot see would make the sync pointless.
func TestUpsert_StoredRecordIsSearchable(t *testing.T) {
	db := testsupport.NewDB(t)
	repo := repository.NewPatientRepository(db)
	hospital := testsupport.NewHospital(t, db, "Searchable Hospital")

	_, err := repo.Upsert(context.Background(), hospital.ID, upstreamPatient())
	require.NoError(t, err)

	found, err := repo.Search(context.Background(), hospital.ID,
		models.SearchPatientRequest{FirstName: "somchai"})
	require.NoError(t, err)
	require.Len(t, found, 1)
	assert.Equal(t, "HN-000123", found[0].PatientHN)
}
