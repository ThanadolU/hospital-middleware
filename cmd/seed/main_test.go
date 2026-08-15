package main

import (
	"database/sql"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ThanadolU/hospital-middleware/internal/database"
	"github.com/ThanadolU/hospital-middleware/internal/testsupport"
)

func TestHospitals_ReadsTheEnvironmentOrFallsBack(t *testing.T) {
	tests := []struct {
		name string
		env  string
		want []string
	}{
		{"unset falls back to the brief's pair", "", defaultHospitals},
		{"single name", "Hospital Z", []string{"Hospital Z"}},
		{"comma separated", "Hospital X,Hospital Y", []string{"Hospital X", "Hospital Y"}},
		{"surrounding whitespace is trimmed", "  Hospital X ,  Hospital Y  ", []string{"Hospital X", "Hospital Y"}},
		{"empty entries are dropped", "Hospital X,,  ,Hospital Y", []string{"Hospital X", "Hospital Y"}},
		// A variable set to nothing but separators is a misconfiguration, not a
		// request to seed nothing: falling back beats booting a stack whose
		// hospitals table is empty and whose staff creation therefore 400s.
		{"only separators falls back", " , , ", defaultHospitals},
		{"only whitespace falls back", "   ", defaultHospitals},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("SEED_HOSPITALS", tc.env)
			assert.Equal(t, tc.want, hospitals())
		})
	}
}

// The seed runs on every `docker compose up`, including against a volume that
// already holds data, so a second run has to be a no-op rather than a
// unique-constraint failure.
func TestInsertHospital_IsIdempotent(t *testing.T) {
	db := newTestDB(t)

	inserted, err := insertHospital(db, "Hospital A")
	require.NoError(t, err)
	assert.True(t, inserted, "the first insert should have created the row")

	inserted, err = insertHospital(db, "Hospital A")
	require.NoError(t, err, "re-running the seed must not fail")
	assert.False(t, inserted, "the second insert should have been a no-op")

	assert.Equal(t, 1, countHospitals(t, db, "Hospital A"))
}

// Uniqueness is on lower(name), so a differently-cased duplicate is the same
// hospital. If the conflict target ever stopped matching that index, the
// statement would error rather than silently insert — this pins both.
func TestInsertHospital_TreatsNamesCaseInsensitively(t *testing.T) {
	db := newTestDB(t)

	_, err := insertHospital(db, "Hospital A")
	require.NoError(t, err)

	inserted, err := insertHospital(db, "hospital a")
	require.NoError(t, err)
	assert.False(t, inserted, "a differently-cased name is the same hospital")

	assert.Equal(t, 1, countHospitals(t, db, "Hospital A"))
}

func TestInsertHospital_AddsEachDistinctName(t *testing.T) {
	db := newTestDB(t)

	for _, name := range defaultHospitals {
		inserted, err := insertHospital(db, name)
		require.NoError(t, err)
		assert.True(t, inserted, "%q should have been created", name)
	}

	for _, name := range defaultHospitals {
		assert.Equal(t, 1, countHospitals(t, db, name))
	}
}

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()

	sqlDB, err := database.SQLDB(testsupport.NewDB(t))
	require.NoError(t, err)
	return sqlDB
}

func countHospitals(t *testing.T, db *sql.DB, name string) int {
	t.Helper()

	var count int
	require.NoError(t, db.QueryRow(
		`SELECT count(*) FROM hospitals WHERE lower(name) = lower($1)`, name,
	).Scan(&count))
	return count
}
