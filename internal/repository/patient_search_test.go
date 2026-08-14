package repository_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ThanadolU/hospital-middleware/internal/models"
	"github.com/ThanadolU/hospital-middleware/internal/repository"
	"github.com/ThanadolU/hospital-middleware/internal/testsupport"
)

// Traceability 4.4: all eight search inputs, one test each.
//
// The isolation suite proves each field does not cross hospitals. This proves
// each field actually *matches* — a different claim, and the one v1 got wrong:
// its middle_name branch filtered on req.FirstName against the first_name
// columns, so a documented search parameter silently returned wrong results in
// the single layer that had no tests.

// searchFixture seeds one target patient and two decoys that differ from the
// target in exactly the field under test, so a criterion that is ignored
// entirely returns three rows instead of one.
type searchFixture struct {
	repo   repository.PatientRepository
	target models.Patient
	scope  models.Hospital
}

func newSearchFixture(t *testing.T) searchFixture {
	t.Helper()

	db := testsupport.NewDB(t)
	hospital := testsupport.NewHospital(t, db, "Search Hospital")

	target := testsupport.NewPatient(t, db, hospital.ID,
		testsupport.WithNamesEN("Somchai", "Klang", "Jaidee"),
		testsupport.WithNamesTH("สมชาย", "กลาง", "ใจดี"),
		testsupport.WithNationalID("1103700123456"),
		testsupport.WithPassportID("AA1234567"),
		testsupport.WithDateOfBirth(t, "1990-05-17"),
		testsupport.WithPhone("0812345678"),
		testsupport.WithEmail("somchai@example.com"),
	)

	// Decoys: same hospital, entirely different values in every field.
	testsupport.NewPatient(t, db, hospital.ID,
		testsupport.WithNamesEN("Wichai", "Noi", "Rakdee"),
		testsupport.WithNamesTH("วิชัย", "น้อย", "รักดี"),
		testsupport.WithNationalID("2203700999999"),
		testsupport.WithPassportID("ZZ9999999"),
		testsupport.WithDateOfBirth(t, "1975-01-02"),
		testsupport.WithPhone("0899999999"),
		testsupport.WithEmail("wichai@example.com"),
	)
	testsupport.NewPatient(t, db, hospital.ID,
		testsupport.WithNamesEN("Malee", "Sri", "Suksai"),
		testsupport.WithNamesTH("มาลี", "ศรี", "สุขใส"),
		testsupport.WithNationalID("3303700888888"),
		testsupport.WithPassportID("YY8888888"),
		testsupport.WithDateOfBirth(t, "2001-12-31"),
		testsupport.WithPhone("0888888888"),
		testsupport.WithEmail("malee@example.com"),
	)

	return searchFixture{
		repo:   repository.NewPatientRepository(db),
		target: target,
		scope:  hospital,
	}
}

func TestSearch_EachFieldMatchesTheRightPatient(t *testing.T) {
	tests := []struct {
		field string
		req   models.SearchPatientRequest
	}{
		{"national_id", models.SearchPatientRequest{NationalID: "1103700123456"}},
		{"passport_id", models.SearchPatientRequest{PassportID: "AA1234567"}},
		{"first_name", models.SearchPatientRequest{FirstName: "Somchai"}},

		// THE v1 BUG. This subtest fails against v1's implementation.
		{"middle_name", models.SearchPatientRequest{MiddleName: "Klang"}},

		{"last_name", models.SearchPatientRequest{LastName: "Jaidee"}},
		{"date_of_birth", models.SearchPatientRequest{DateOfBirth: "1990-05-17"}},
		{"phone_number", models.SearchPatientRequest{PhoneNumber: "0812345678"}},
		{"email", models.SearchPatientRequest{Email: "somchai@example.com"}},
	}

	for _, tc := range tests {
		t.Run(tc.field, func(t *testing.T) {
			fixture := newSearchFixture(t)

			found, err := fixture.repo.Search(context.Background(), fixture.scope.ID, tc.req)
			require.NoError(t, err)

			require.Len(t, found, 1, "searching by %s did not narrow to one patient", tc.field)
			assert.Equal(t, fixture.target.ID, found[0].ID, "searching by %s found the wrong patient", tc.field)
		})
	}
}

// Names may be recorded in Thai or English, and either must be searchable.
func TestSearch_NameFieldsMatchBothScripts(t *testing.T) {
	tests := []struct {
		field string
		req   models.SearchPatientRequest
	}{
		{"first_name th", models.SearchPatientRequest{FirstName: "สมชาย"}},
		{"middle_name th", models.SearchPatientRequest{MiddleName: "กลาง"}},
		{"last_name th", models.SearchPatientRequest{LastName: "ใจดี"}},
	}

	for _, tc := range tests {
		t.Run(tc.field, func(t *testing.T) {
			fixture := newSearchFixture(t)

			found, err := fixture.repo.Search(context.Background(), fixture.scope.ID, tc.req)
			require.NoError(t, err)
			require.Len(t, found, 1)
			assert.Equal(t, fixture.target.ID, found[0].ID)
		})
	}
}

// Name search is a case-insensitive substring match, so partial input works.
func TestSearch_NamesMatchPartiallyAndIgnoreCase(t *testing.T) {
	for _, term := range []string{"somchai", "SOMCHAI", "omcha", "Som"} {
		t.Run(term, func(t *testing.T) {
			fixture := newSearchFixture(t)

			found, err := fixture.repo.Search(context.Background(), fixture.scope.ID,
				models.SearchPatientRequest{FirstName: term})
			require.NoError(t, err)
			require.Len(t, found, 1)
			assert.Equal(t, fixture.target.ID, found[0].ID)
		})
	}
}

// Email matching ignores case, which is what the lower(email) index supports.
func TestSearch_EmailIgnoresCase(t *testing.T) {
	fixture := newSearchFixture(t)

	found, err := fixture.repo.Search(context.Background(), fixture.scope.ID,
		models.SearchPatientRequest{Email: "SomChai@Example.COM"})
	require.NoError(t, err)
	require.Len(t, found, 1)
	assert.Equal(t, fixture.target.ID, found[0].ID)
}

// Criteria combine: every supplied field must match the same patient.
func TestSearch_MultipleCriteriaAreCombined(t *testing.T) {
	fixture := newSearchFixture(t)

	t.Run("all matching the same patient", func(t *testing.T) {
		found, err := fixture.repo.Search(context.Background(), fixture.scope.ID,
			models.SearchPatientRequest{FirstName: "Somchai", LastName: "Jaidee", DateOfBirth: "1990-05-17"})
		require.NoError(t, err)
		require.Len(t, found, 1)
		assert.Equal(t, fixture.target.ID, found[0].ID)
	})

	t.Run("criteria drawn from different patients match nobody", func(t *testing.T) {
		found, err := fixture.repo.Search(context.Background(), fixture.scope.ID,
			models.SearchPatientRequest{FirstName: "Somchai", LastName: "Rakdee"})
		require.NoError(t, err)
		assert.Empty(t, found, "criteria must AND together, not OR")
	})
}

// The brief asks for all matching patients, so an empty request returns every
// patient in the hospital — not a silently truncated first page.
func TestSearch_NoCriteriaReturnsEveryPatientInTheHospital(t *testing.T) {
	fixture := newSearchFixture(t)

	found, err := fixture.repo.Search(context.Background(), fixture.scope.ID, models.SearchPatientRequest{})
	require.NoError(t, err)
	assert.Len(t, found, 3, "an unfiltered search must return all matches, with no implicit page limit")
}

func TestSearch_NoMatchReturnsEmptyNotError(t *testing.T) {
	fixture := newSearchFixture(t)

	found, err := fixture.repo.Search(context.Background(), fixture.scope.ID,
		models.SearchPatientRequest{NationalID: "0000000000000"})
	require.NoError(t, err)
	assert.Empty(t, found)
}

// Whitespace-only criteria are treated as absent rather than as a literal
// search for spaces, which would match nothing and look like data loss.
func TestSearch_WhitespaceCriteriaAreIgnored(t *testing.T) {
	fixture := newSearchFixture(t)

	found, err := fixture.repo.Search(context.Background(), fixture.scope.ID,
		models.SearchPatientRequest{FirstName: "   ", Email: "  "})
	require.NoError(t, err)
	assert.Len(t, found, 3)
}
