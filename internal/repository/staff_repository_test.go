package repository_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ThanadolU/hospital-middleware/internal/auth"
	"github.com/ThanadolU/hospital-middleware/internal/models"
	"github.com/ThanadolU/hospital-middleware/internal/repository"
	"github.com/ThanadolU/hospital-middleware/internal/testsupport"
)

const staffPassword = "correct-horse-battery"

func TestStaffRepository_CreateAndFind(t *testing.T) {
	db := testsupport.NewDB(t)
	hospital := testsupport.NewHospital(t, db, "Find Hospital")
	repo := repository.NewStaffRepository(db)

	hash, err := auth.HashPassword(staffPassword)
	require.NoError(t, err)

	staff := models.Staff{Username: "somchai", Password: hash, HospitalID: hospital.ID}
	require.NoError(t, repo.Create(context.Background(), &staff))
	assert.NotEqual(t, uuid.Nil, staff.ID, "the database should have assigned an id")

	found, err := repo.FindByUsername(context.Background(), hospital.ID, "somchai")
	require.NoError(t, err)
	assert.Equal(t, staff.ID, found.ID)
	assert.Equal(t, hospital.ID, found.HospitalID)
}

// Traceability 3.3: the stored credential is a hash, never the plaintext.
func TestStaffRepository_NeverStoresThePasswordInPlaintext(t *testing.T) {
	db := testsupport.NewDB(t)
	hospital := testsupport.NewHospital(t, db, "Hash Hospital")

	testsupport.NewStaff(t, db, hospital.ID, "somchai", staffPassword)

	var stored string
	require.NoError(t, db.Raw(`SELECT password FROM staffs WHERE lower(username) = 'somchai'`).Scan(&stored).Error)

	assert.NotEqual(t, staffPassword, stored)
	assert.NotContains(t, stored, staffPassword)
	assert.NoError(t, auth.VerifyPassword(stored, staffPassword), "the stored hash must still verify")
}

// The schema refuses a raw password even if a caller bypasses the service layer.
func TestStaffRepository_SchemaRejectsAnUnhashedPassword(t *testing.T) {
	db := testsupport.NewDB(t)
	hospital := testsupport.NewHospital(t, db, "Raw Password Hospital")

	err := db.Exec(
		`INSERT INTO staffs (hospital_id, username, password) VALUES (?, ?, ?)`,
		hospital.ID, "sneaky", "plaintext",
	).Error
	assert.Error(t, err, "a password too short to be a bcrypt hash must be rejected")
}

func TestStaffRepository_DuplicateUsernameInSameHospital(t *testing.T) {
	db := testsupport.NewDB(t)
	hospital := testsupport.NewHospital(t, db, "Duplicate Hospital")
	repo := repository.NewStaffRepository(db)

	hash, err := auth.HashPassword(staffPassword)
	require.NoError(t, err)

	first := models.Staff{Username: "somchai", Password: hash, HospitalID: hospital.ID}
	require.NoError(t, repo.Create(context.Background(), &first))

	second := models.Staff{Username: "somchai", Password: hash, HospitalID: hospital.ID}
	assert.ErrorIs(t, repo.Create(context.Background(), &second), repository.ErrDuplicateStaff)

	// Case must not be a way around it.
	third := models.Staff{Username: "SomChai", Password: hash, HospitalID: hospital.ID}
	assert.ErrorIs(t, repo.Create(context.Background(), &third), repository.ErrDuplicateStaff)
}

// Usernames are unique per hospital, not globally: two hospitals may each
// employ an "admin", and one hospital must not be able to squat a username
// across the whole system.
func TestStaffRepository_SameUsernameAllowedAtDifferentHospitals(t *testing.T) {
	db := testsupport.NewDB(t)
	first := testsupport.NewHospital(t, db, "Shared Name Hospital One")
	second := testsupport.NewHospital(t, db, "Shared Name Hospital Two")
	repo := repository.NewStaffRepository(db)

	hash, err := auth.HashPassword(staffPassword)
	require.NoError(t, err)

	require.NoError(t, repo.Create(context.Background(), &models.Staff{
		Username: "admin", Password: hash, HospitalID: first.ID,
	}))
	require.NoError(t, repo.Create(context.Background(), &models.Staff{
		Username: "admin", Password: hash, HospitalID: second.ID,
	}))
}

// A staff member is only findable within their own hospital. This is the login
// half of the isolation property: authenticating against Hospital B must not
// match Hospital A's account of the same name.
func TestStaffRepository_LookupIsScopedToOneHospital(t *testing.T) {
	db := testsupport.NewDB(t)
	first := testsupport.NewHospital(t, db, "Lookup Hospital One")
	second := testsupport.NewHospital(t, db, "Lookup Hospital Two")
	repo := repository.NewStaffRepository(db)

	testsupport.NewStaff(t, db, first.ID, "admin", staffPassword)

	found, err := repo.FindByUsername(context.Background(), second.ID, "admin")
	assert.ErrorIs(t, err, repository.ErrStaffNotFound)
	assert.Nil(t, found, "Hospital One's account must not be visible to Hospital Two")
}

func TestStaffRepository_FindByUsername_NotFound(t *testing.T) {
	db := testsupport.NewDB(t)
	hospital := testsupport.NewHospital(t, db, "Missing Staff Hospital")
	repo := repository.NewStaffRepository(db)

	found, err := repo.FindByUsername(context.Background(), hospital.ID, "nobody")
	assert.ErrorIs(t, err, repository.ErrStaffNotFound)
	assert.Nil(t, found)
}

// Username lookup is case-insensitive, matching the unique index.
func TestStaffRepository_FindByUsername_IsCaseInsensitive(t *testing.T) {
	db := testsupport.NewDB(t)
	hospital := testsupport.NewHospital(t, db, "Case Lookup Hospital")
	repo := repository.NewStaffRepository(db)

	testsupport.NewStaff(t, db, hospital.ID, "Somchai", staffPassword)

	found, err := repo.FindByUsername(context.Background(), hospital.ID, "SOMCHAI")
	require.NoError(t, err)
	assert.Equal(t, hospital.ID, found.HospitalID)
}

// A missing hospital scope must fail rather than search across all hospitals.
func TestStaffRepository_RequiresAHospitalScope(t *testing.T) {
	db := testsupport.NewDB(t)
	repo := repository.NewStaffRepository(db)

	_, err := repo.FindByUsername(context.Background(), uuid.Nil, "somchai")
	assert.ErrorIs(t, err, repository.ErrHospitalScopeRequired)

	err = repo.Create(context.Background(), &models.Staff{Username: "somchai", Password: "x"})
	assert.ErrorIs(t, err, repository.ErrHospitalScopeRequired)
}

// Traceability 3.3, response half: the credential must never reach a response
// body, whatever a handler forgets to strip. Asserted on the round trip a
// handler actually performs.
func TestStaff_PasswordIsNeverSerialised(t *testing.T) {
	db := testsupport.NewDB(t)
	hospital := testsupport.NewHospital(t, db, "Serialisation Hospital")

	staff := testsupport.NewStaff(t, db, hospital.ID, "somchai", staffPassword)
	require.NotEmpty(t, staff.Password, "the in-memory struct does hold the hash")

	encoded, err := json.Marshal(staff)
	require.NoError(t, err)

	body := string(encoded)
	assert.NotContains(t, body, staffPassword, "the plaintext must never be serialised")
	assert.NotContains(t, body, staff.Password, "the hash must never be serialised either")
	assert.NotContains(t, body, "password")
	assert.Contains(t, body, "somchai", "the rest of the record still serialises")
}
