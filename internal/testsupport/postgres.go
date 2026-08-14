// Package testsupport provides shared fixtures for tests that need a real
// PostgreSQL. It is test-only code that lives in a normal package so several
// test packages can share it.
package testsupport

import (
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"github.com/ThanadolU/hospital-middleware/internal/database"
	"github.com/ThanadolU/hospital-middleware/internal/models"
)

// DatabaseURLEnv names the connection string used by tests that need a real
// database. When it is unset those tests skip loudly rather than passing
// quietly, so an unverified layer is never mistaken for a verified one.
const DatabaseURLEnv = "TEST_DATABASE_URL"

// NewDB returns a migrated, empty database that belongs to this test alone.
//
// Each test gets a private PostgreSQL schema. `go test ./...` runs packages in
// parallel, so a shared schema means one package's setup truncates another
// package's fixtures mid-run — which showed up as tests that passed alone and
// failed together. A schema per test also means no ordering dependence, which
// matters most for the isolation tests, where a leftover row could mask a leak.
func NewDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := os.Getenv(DatabaseURLEnv)
	if dsn == "" {
		t.Skipf("%s is not set; skipping tests that need a real PostgreSQL", DatabaseURLEnv)
	}

	schema := uniqueSchemaName()

	// public stays on the path so shared extensions (pg_trgm) and their
	// operator classes resolve; the private schema leads, so every table the
	// migrations create lands there.
	scopedDSN, err := withSearchPath(dsn, schema+",public")
	require.NoError(t, err)

	db, err := database.Open(database.Config{DSN: scopedDSN})
	require.NoError(t, err, "cannot reach the database named by %s", DatabaseURLEnv)

	require.NoError(t, db.Exec(`CREATE SCHEMA IF NOT EXISTS `+schema).Error)

	sqlDB, err := database.SQLDB(db)
	require.NoError(t, err)
	require.NoError(t, database.Migrate(sqlDB))

	t.Cleanup(func() {
		// Best effort: a leaked schema in a throwaway test database is
		// harmless, and failing cleanup would mask the real test failure.
		_ = db.Exec(`DROP SCHEMA IF EXISTS ` + schema + ` CASCADE`).Error
		_ = sqlDB.Close()
	})
	return db
}

// uniqueSchemaName returns an identifier that is unique per test and safe to
// interpolate: hex only, so it needs no quoting.
func uniqueSchemaName() string {
	return "test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
}

// withSearchPath returns dsn with the search_path parameter set.
func withSearchPath(dsn, searchPath string) (string, error) {
	parsed, err := url.Parse(dsn)
	if err != nil {
		return "", fmt.Errorf("testsupport: parse %s: %w", DatabaseURLEnv, err)
	}
	query := parsed.Query()
	query.Set("search_path", searchPath)
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

// NewHospital inserts a hospital and returns it.
func NewHospital(t *testing.T, db *gorm.DB, name string) models.Hospital {
	t.Helper()

	hospital := models.Hospital{Name: name}
	require.NoError(t, db.Create(&hospital).Error)
	require.NotEqual(t, uuid.Nil, hospital.ID)
	return hospital
}

// NewStaff inserts a staff member whose password is hashed at bcrypt's minimum
// cost.
//
// Fixtures deliberately do not use auth.HashPassword: at the production cost
// every fixture costs ~150ms, which added minutes to the suite. The cost itself
// is asserted directly in the auth package, so nothing is lost here — and the
// stored value is still a real bcrypt hash, so the schema's hashed-password
// constraint and VerifyPassword both still exercise the real thing.
func NewStaff(t *testing.T, db *gorm.DB, hospitalID uuid.UUID, username, password string) models.Staff {
	t.Helper()

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	require.NoError(t, err)

	staff := models.Staff{Username: username, Password: string(hash), HospitalID: hospitalID}
	require.NoError(t, db.Create(&staff).Error)
	return staff
}

// PatientOption customises a patient built by NewPatient.
type PatientOption func(*models.Patient)

// NewPatient inserts a patient belonging to hospitalID. It fills in valid
// defaults for every required column, so a test only states the fields it
// actually cares about.
func NewPatient(t *testing.T, db *gorm.DB, hospitalID uuid.UUID, opts ...PatientOption) models.Patient {
	t.Helper()

	patient := models.Patient{
		HospitalID:  hospitalID,
		FirstNameEN: "Somchai",
		LastNameEN:  "Jaidee",
		FirstNameTH: "สมชาย",
		LastNameTH:  "ใจดี",
		DateOfBirth: mustDate(t, "1990-05-17"),
		NationalID:  uuid.NewString()[:13],
		Gender:      "M",
	}
	for _, opt := range opts {
		opt(&patient)
	}

	require.NoError(t, db.Create(&patient).Error)
	return patient
}
