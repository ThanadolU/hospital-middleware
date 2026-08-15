package testsupport_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ThanadolU/hospital-middleware/internal/models"
	"github.com/ThanadolU/hospital-middleware/internal/testsupport"
)

// These test the test helper, which sounds circular but is not: every
// database-backed suite in the project trusts NewDB to hand out an isolated
// schema. When that isolation broke, packages truncated each other's fixtures
// and tests passed alone while failing together — results were being corrupted
// silently. A helper this much is resting on deserves its own assertions.

// Each call must land in its own schema, or parallel packages share tables.
func TestNewDB_GivesEachTestItsOwnSchema(t *testing.T) {
	first := testsupport.NewDB(t)
	second := testsupport.NewDB(t)

	var firstSchema, secondSchema string
	require.NoError(t, first.Raw("SELECT current_schema()").Scan(&firstSchema).Error)
	require.NoError(t, second.Raw("SELECT current_schema()").Scan(&secondSchema).Error)

	assert.NotEmpty(t, firstSchema)
	assert.NotEqual(t, firstSchema, secondSchema,
		"two handles shared a schema; parallel tests would corrupt each other")
	assert.NotEqual(t, "public", firstSchema,
		"tests must not run in public, or they touch whatever else lives there")
}

// The isolation has to hold for data, not just for the schema name.
func TestNewDB_DataDoesNotLeakBetweenSchemas(t *testing.T) {
	first := testsupport.NewDB(t)
	second := testsupport.NewDB(t)

	testsupport.NewHospital(t, first, "Only In The First Schema")

	var count int64
	require.NoError(t, second.Model(&models.Hospital{}).Count(&count).Error)
	assert.Zero(t, count, "the second schema can see the first schema's rows")
}

// Migrations must have been applied, or every suite would have to run them.
func TestNewDB_AppliesMigrations(t *testing.T) {
	db := testsupport.NewDB(t)

	for _, table := range []string{"hospitals", "patients", "staffs"} {
		var exists bool
		require.NoError(t, db.Raw(
			`SELECT EXISTS (
				SELECT 1 FROM information_schema.tables
				WHERE table_name = ? AND table_schema = current_schema()
			)`, table).Scan(&exists).Error)
		assert.True(t, exists, "%q was not created in this test's schema", table)
	}
}

// pg_trgm is installed into public deliberately, so it survives a transient
// schema being dropped. If it ever lands in the per-test schema, the first
// test to finish takes the extension away from every later one.
func TestNewDB_TrigramExtensionLivesInPublic(t *testing.T) {
	db := testsupport.NewDB(t)

	var schema string
	require.NoError(t, db.Raw(`
		SELECT n.nspname FROM pg_extension e
		JOIN pg_namespace n ON n.oid = e.extnamespace
		WHERE e.extname = 'pg_trgm'`).Scan(&schema).Error)

	assert.Equal(t, "public", schema,
		"pg_trgm must live in public, or it disappears when a test schema is dropped")
}

func TestNewHospital_InsertsAndReturnsTheRecord(t *testing.T) {
	db := testsupport.NewDB(t)

	hospital := testsupport.NewHospital(t, db, "Fixture Hospital")

	assert.NotEqual(t, "", hospital.ID.String())
	assert.Equal(t, "Fixture Hospital", hospital.Name)

	var stored models.Hospital
	require.NoError(t, db.First(&stored, "id = ?", hospital.ID).Error)
	assert.Equal(t, hospital.Name, stored.Name)
}

// The staff fixture must store a usable bcrypt hash, never the plaintext —
// otherwise tests asserting "the password is not stored in plaintext" would
// pass against a fixture that never hashed anything.
func TestNewStaff_StoresAHashNotThePassword(t *testing.T) {
	db := testsupport.NewDB(t)
	hospital := testsupport.NewHospital(t, db, "Staff Fixture Hospital")

	const password = "correct-horse-battery"
	staff := testsupport.NewStaff(t, db, hospital.ID, "somchai", password)

	var stored models.Staff
	require.NoError(t, db.First(&stored, "id = ?", staff.ID).Error)

	assert.NotEqual(t, password, stored.Password)
	assert.Contains(t, stored.Password, "$2", "the stored value is not a bcrypt hash")
	assert.Equal(t, hospital.ID, stored.HospitalID)
}

func TestNewPatient_IsScopedToItsHospital(t *testing.T) {
	db := testsupport.NewDB(t)
	hospital := testsupport.NewHospital(t, db, "Patient Fixture Hospital")

	patient := testsupport.NewPatient(t, db, hospital.ID)

	var stored models.Patient
	require.NoError(t, db.First(&stored, "id = ?", patient.ID).Error)
	assert.Equal(t, hospital.ID, stored.HospitalID)
	// The schema rejects a patient with neither identifier, so the default
	// fixture must supply at least one or every caller has to pass options.
	assert.True(t, stored.NationalID != "" || stored.PassportID != "",
		"the default patient fixture has no identifier")
}
