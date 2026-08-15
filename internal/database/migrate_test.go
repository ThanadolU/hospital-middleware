package database

import (
	"database/sql"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests apply the real migrations to a real PostgreSQL and then try to
// break the constraints. A schema test that does not touch a database proves
// nothing: partial unique indexes, CHECK constraints and trigram indexes are
// exactly the things a mock cannot model.
//
// Set TEST_DATABASE_URL to run them, e.g.
//
//	docker run --rm -d -p 5433:5432 -e POSTGRES_PASSWORD=postgres postgres:15
//	TEST_DATABASE_URL='postgres://postgres:postgres@localhost:5433/postgres?sslmode=disable' go test ./internal/database/...
//
// Without it they skip loudly rather than passing quietly, so an unverified
// schema is never mistaken for a verified one.
const testDatabaseURLEnv = "TEST_DATABASE_URL"

// newTestDB opens the configured database on a private schema.
//
// The schema per test is not cosmetic: `go test ./...` runs packages in
// parallel against one database, so without it these migrations race other
// packages' fixtures. It also lets each test roll the schema back and forth
// without disturbing anything else.
func newTestDB(t *testing.T) *sql.DB {
	t.Helper()

	dsn := os.Getenv(testDatabaseURLEnv)
	if dsn == "" {
		t.Skipf("%s is not set; skipping schema verification against a real PostgreSQL", testDatabaseURLEnv)
	}

	// public stays on the path so pg_trgm and its operator classes resolve.
	schema := "test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	parsed, err := url.Parse(dsn)
	require.NoError(t, err)
	query := parsed.Query()
	query.Set("search_path", schema+",public")
	parsed.RawQuery = query.Encode()

	db, err := sql.Open("postgres", parsed.String())
	require.NoError(t, err)
	require.NoError(t, db.Ping(), "cannot reach the database named by %s", testDatabaseURLEnv)

	_, err = db.Exec(`CREATE SCHEMA IF NOT EXISTS ` + schema)
	require.NoError(t, err)

	t.Cleanup(func() {
		_, _ = db.Exec(`DROP SCHEMA IF EXISTS ` + schema + ` CASCADE`)
		_ = db.Close()
	})
	return db
}

func TestMigrate_AppliesAndIsIdempotent(t *testing.T) {
	db := newTestDB(t)

	require.NoError(t, Migrate(db), "first apply")
	require.NoError(t, Migrate(db), "second apply must be a no-op, not an error")

	for _, table := range []string{"hospitals", "patients"} {
		assert.True(t, tableExists(t, db, table), "migration should have created %q", table)
	}
}

// A down migration that has never been run is a down migration that does not
// work. This rolls back the most recent migration and reapplies it, which stays
// correct as further migrations are added rather than hardcoding how many exist.
func TestRollback_UndoesTheLatestMigrationAndCanReapply(t *testing.T) {
	db := newTestDB(t)
	require.NoError(t, Migrate(db))

	require.NoError(t, Rollback(db, 1))
	assert.False(t, tableExists(t, db, "staffs"), "rollback should have dropped the latest migration's table")
	assert.True(t, tableExists(t, db, "patients"), "rolling back one step must not undo earlier migrations")

	require.NoError(t, Migrate(db), "the schema must be reapplicable after a rollback")
	assert.True(t, tableExists(t, db, "staffs"))
}

// tableExists reports whether the table exists **in this test's own schema**.
//
// The table_schema filter is what makes the answer meaningful. information_schema
// spans every schema the connection can see, and each test runs in a private one,
// so an unfiltered query answers "does any package have a staffs table right now"
// — which is intermittently true for reasons that have nothing to do with this
// test, and would just as happily report a rollback succeeded when it had not.
func tableExists(t *testing.T, db *sql.DB, name string) bool {
	t.Helper()

	var exists bool
	err := db.QueryRow(
		`SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_name = $1 AND table_schema = current_schema()
		)`, name,
	).Scan(&exists)
	require.NoError(t, err)
	return exists
}

func TestSchema_AllowsManyPassportOnlyPatientsInOneHospital(t *testing.T) {
	db := newTestDB(t)
	require.NoError(t, Migrate(db))

	hospitalID := insertHospital(t, db, "Passport Only Hospital")

	// The exact case v1's plain composite unique on (hospital_id, national_id)
	// would have rejected: both rows carry national_id = ''.
	require.NoError(t, insertPatient(t, db, hospitalID, "", "AA1111111"))
	require.NoError(t, insertPatient(t, db, hospitalID, "", "BB2222222"),
		"a second passport-only patient must be allowed in the same hospital")
}

func TestSchema_RejectsDuplicateIdentifierWithinAHospital(t *testing.T) {
	db := newTestDB(t)
	require.NoError(t, Migrate(db))

	hospitalID := insertHospital(t, db, "Duplicate Guard Hospital")

	require.NoError(t, insertPatient(t, db, hospitalID, "1103700123456", ""))
	assert.Error(t, insertPatient(t, db, hospitalID, "1103700123456", ""),
		"the same national id must not appear twice in one hospital")

	require.NoError(t, insertPatient(t, db, hospitalID, "", "AA1234567"))
	assert.Error(t, insertPatient(t, db, hospitalID, "", "AA1234567"),
		"the same passport must not appear twice in one hospital")
}

// Uniqueness is per hospital, not global: two hospitals may each hold a record
// for the same person, and neither should block the other.
func TestSchema_AllowsSameIdentifierAcrossHospitals(t *testing.T) {
	db := newTestDB(t)
	require.NoError(t, Migrate(db))

	first := insertHospital(t, db, "Cross Hospital One")
	second := insertHospital(t, db, "Cross Hospital Two")

	require.NoError(t, insertPatient(t, db, first, "1103700999999", ""))
	require.NoError(t, insertPatient(t, db, second, "1103700999999", ""),
		"the same person may be a patient at two hospitals")
}

func TestSchema_RejectsUnusableRecords(t *testing.T) {
	db := newTestDB(t)
	require.NoError(t, Migrate(db))

	hospitalID := insertHospital(t, db, "Constraint Hospital")

	t.Run("no identifier at all", func(t *testing.T) {
		assert.Error(t, insertPatient(t, db, hospitalID, "", ""),
			"a patient with neither identifier cannot be matched or upserted")
	})

	t.Run("gender outside M and F", func(t *testing.T) {
		_, err := db.Exec(
			`INSERT INTO patients (hospital_id, date_of_birth, national_id, gender)
			 VALUES ($1, '1990-01-01', '1103700111111', 'X')`,
			hospitalID,
		)
		assert.Error(t, err)
	})

	t.Run("unknown hospital", func(t *testing.T) {
		assert.Error(t, insertPatient(t, db, "00000000-0000-0000-0000-000000000000", "1103700222222", ""),
			"the foreign key must reject a patient with no real hospital")
	})
}

func TestSchema_HospitalNameIsUniqueCaseInsensitively(t *testing.T) {
	db := newTestDB(t)
	require.NoError(t, Migrate(db))

	insertHospital(t, db, "Case Test Hospital")

	_, err := db.Exec(`INSERT INTO hospitals (name) VALUES ($1)`, "case test hospital")
	assert.Error(t, err, "staff supply a hospital by name, so names must not be ambiguous")
}

// An index that exists is not an index that gets used. This asserts each one is
// *usable* for the query shape the repository will write, which is a different
// claim from "the CREATE INDEX succeeded" — and it is the claim that matters.
//
// enable_seqscan is disabled so the assertion is about usability rather than
// planner preference: on a small table a sequential scan is genuinely cheaper,
// and that would make the test depend on row counts.
//
// This test exists because the email index was originally written as a partial
// index over non-empty emails, which the planner could not prove applied to a
// `lower(email) = $1` lookup, so it was silently skipped in favour of a
// sequential scan. Nothing but a plan inspection would have caught it.
func TestSchema_IndexesServeTheSearchQueryShapes(t *testing.T) {
	db := newTestDB(t)
	require.NoError(t, Migrate(db))

	hospitalID := insertHospital(t, db, "Index Plan Hospital")
	for i := range 200 {
		_, err := db.Exec(
			`INSERT INTO patients (hospital_id, date_of_birth, national_id, passport_id,
			                       first_name_en, middle_name_en, last_name_en,
			                       first_name_th, middle_name_th, last_name_th,
			                       phone_number, email, gender)
			 VALUES ($1, '1990-01-01', $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, 'M')`,
			hospitalID,
			"nat"+itoa(i), "pass"+itoa(i),
			"Somchai"+itoa(i), "Klang"+itoa(i), "Jaidee"+itoa(i),
			"สมชาย"+itoa(i), "กลาง"+itoa(i), "ใจดี"+itoa(i),
			"08"+itoa(i), "user"+itoa(i)+"@example.com",
		)
		require.NoError(t, err)
	}
	_, err := db.Exec(`ANALYZE patients`)
	require.NoError(t, err)

	tests := []struct {
		field     string
		where     string
		arg       any
		wantIndex string
	}{
		{"national_id", `hospital_id = $1 AND national_id = 'nat7'`, hospitalID, "patients_hospital_national_id_key"},
		{"passport_id", `hospital_id = $1 AND passport_id = 'pass7'`, hospitalID, "patients_hospital_passport_id_key"},
		{"date_of_birth", `hospital_id = $1 AND date_of_birth = '1990-01-01'`, hospitalID, "patients_hospital_dob_idx"},
		{"phone_number", `hospital_id = $1 AND phone_number = '087'`, hospitalID, "patients_hospital_phone_idx"},
		{"email", `hospital_id = $1 AND lower(email) = lower('User7@Example.com')`, hospitalID, "patients_hospital_email_lower_idx"},

		// The six name columns are matched with a leading wildcard, which only
		// a trigram index can serve. Asserted on the name predicate alone: with
		// hospital_id also present the planner may reasonably prefer to filter.
		{"first_name_en", `first_name_en ILIKE '%omchai7%'`, nil, "patients_first_name_en_trgm_idx"},
		{"middle_name_en", `middle_name_en ILIKE '%lang7%'`, nil, "patients_middle_name_en_trgm_idx"},
		{"last_name_en", `last_name_en ILIKE '%aidee7%'`, nil, "patients_last_name_en_trgm_idx"},
		{"first_name_th", `first_name_th ILIKE '%สมชาย7%'`, nil, "patients_first_name_th_trgm_idx"},
		{"middle_name_th", `middle_name_th ILIKE '%กลาง7%'`, nil, "patients_middle_name_th_trgm_idx"},
		{"last_name_th", `last_name_th ILIKE '%ใจดี7%'`, nil, "patients_last_name_th_trgm_idx"},
	}

	for _, tc := range tests {
		t.Run(tc.field, func(t *testing.T) {
			plan := explain(t, db, `SELECT * FROM patients WHERE `+tc.where, tc.arg)
			assert.Contains(t, plan, tc.wantIndex,
				"searching by %s cannot use %s; plan was:\n%s", tc.field, tc.wantIndex, plan)
		})
	}
}

// explain returns the query plan as text, with sequential scans disabled.
func explain(t *testing.T, db *sql.DB, query string, arg any) string {
	t.Helper()

	conn, err := db.Conn(t.Context())
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()

	// Must be the same session as the EXPLAIN, hence the pinned connection.
	_, err = conn.ExecContext(t.Context(), `SET enable_seqscan = off`)
	require.NoError(t, err)

	var rows *sql.Rows
	if arg == nil {
		rows, err = conn.QueryContext(t.Context(), `EXPLAIN (COSTS OFF) `+query)
	} else {
		rows, err = conn.QueryContext(t.Context(), `EXPLAIN (COSTS OFF) `+query, arg)
	}
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()

	var plan strings.Builder
	for rows.Next() {
		var line string
		require.NoError(t, rows.Scan(&line))
		plan.WriteString(line)
		plan.WriteString("\n")
	}
	require.NoError(t, rows.Err())
	return plan.String()
}

func itoa(i int) string { return strconv.Itoa(i) }

func insertHospital(t *testing.T, db *sql.DB, name string) string {
	t.Helper()

	var id string
	err := db.QueryRow(`INSERT INTO hospitals (name) VALUES ($1) RETURNING id`, name).Scan(&id)
	require.NoError(t, err)
	return id
}

func insertPatient(t *testing.T, db *sql.DB, hospitalID, nationalID, passportID string) error {
	t.Helper()

	_, err := db.Exec(
		`INSERT INTO patients (hospital_id, date_of_birth, national_id, passport_id, gender)
		 VALUES ($1, '1990-01-01', $2, $3, 'M')`,
		hospitalID, nationalID, passportID,
	)
	return err
}
