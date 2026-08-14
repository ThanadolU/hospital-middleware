package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ThanadolU/hospital-middleware/internal/auth"
	"github.com/ThanadolU/hospital-middleware/internal/models"
	"github.com/ThanadolU/hospital-middleware/internal/repository"
	"github.com/ThanadolU/hospital-middleware/internal/service"
)

// These use in-memory fakes rather than a database: the claim under test is
// that each storage failure maps to the right sentinel, which is logic, not
// persistence. The database-backed behaviour is covered in internal/repository
// and end to end in internal/routes.

const (
	testSecret   = "a-test-secret-that-is-long-enough-to-pass"
	testPassword = "correct-horse-battery"
	testHospital = "Hospital A"
)

type fakeHospitals struct {
	byName map[string]models.Hospital
	err    error
}

func (f *fakeHospitals) FindByName(_ context.Context, name string) (*models.Hospital, error) {
	if f.err != nil {
		return nil, f.err
	}
	hospital, ok := f.byName[name]
	if !ok {
		return nil, repository.ErrHospitalNotFound
	}
	return &hospital, nil
}

func (f *fakeHospitals) List(context.Context) ([]models.Hospital, error) { return nil, nil }

type fakeStaff struct {
	byUsername map[string]models.Staff
	createErr  error
}

func (f *fakeStaff) Create(_ context.Context, staff *models.Staff) error {
	if f.createErr != nil {
		return f.createErr
	}
	if _, exists := f.byUsername[staff.Username]; exists {
		return repository.ErrDuplicateStaff
	}
	staff.ID = uuid.New()
	f.byUsername[staff.Username] = *staff
	return nil
}

func (f *fakeStaff) FindByUsername(_ context.Context, _ uuid.UUID, username string) (*models.Staff, error) {
	staff, ok := f.byUsername[username]
	if !ok {
		return nil, repository.ErrStaffNotFound
	}
	return &staff, nil
}

func newService(t *testing.T, hospitals *fakeHospitals, staff *fakeStaff) service.AuthService {
	t.Helper()

	tokens, err := auth.NewTokenService(testSecret, time.Hour)
	require.NoError(t, err)
	return service.NewAuthService(staff, hospitals, tokens)
}

func seededHospitals() *fakeHospitals {
	return &fakeHospitals{byName: map[string]models.Hospital{
		testHospital: {ID: uuid.New(), Name: testHospital},
	}}
}

func TestCreateStaff_HappyPath(t *testing.T) {
	hospitals := seededHospitals()
	staffRepo := &fakeStaff{byUsername: map[string]models.Staff{}}

	staff, err := newService(t, hospitals, staffRepo).CreateStaff(context.Background(),
		service.CreateStaffInput{Username: "somchai", Password: testPassword, Hospital: testHospital})

	require.NoError(t, err)
	assert.Equal(t, "somchai", staff.Username)
	assert.Equal(t, hospitals.byName[testHospital].ID, staff.HospitalID)

	// The service must hash before storing; the plaintext must never persist.
	assert.NotEqual(t, testPassword, staff.Password)
	assert.NoError(t, auth.VerifyPassword(staff.Password, testPassword))
}

func TestCreateStaff_MapsFailuresToSentinels(t *testing.T) {
	tests := []struct {
		name  string
		input service.CreateStaffInput
		setup func(*fakeHospitals, *fakeStaff)
		want  error
	}{
		{
			name:  "unknown hospital",
			input: service.CreateStaffInput{Username: "a", Password: testPassword, Hospital: "Nowhere"},
			want:  service.ErrUnknownHospital,
		},
		{
			name:  "duplicate username",
			input: service.CreateStaffInput{Username: "taken", Password: testPassword, Hospital: testHospital},
			setup: func(_ *fakeHospitals, s *fakeStaff) { s.byUsername["taken"] = models.Staff{} },
			want:  service.ErrDuplicateStaff,
		},
		{
			name: "password too short",
			// Not the literal word "short": the error text says "too short",
			// which would make the no-leak assertion below pass or fail by
			// coincidence rather than by behaviour.
			input: service.CreateStaffInput{Username: "a", Password: "pw12", Hospital: testHospital},
			want:  service.ErrWeakPassword,
		},
		{
			name:  "empty username",
			input: service.CreateStaffInput{Username: "  ", Password: testPassword, Hospital: testHospital},
			want:  service.ErrInvalidCredentials,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			hospitals := seededHospitals()
			staffRepo := &fakeStaff{byUsername: map[string]models.Staff{}}
			if tc.setup != nil {
				tc.setup(hospitals, staffRepo)
			}

			staff, err := newService(t, hospitals, staffRepo).CreateStaff(context.Background(), tc.input)

			assert.Nil(t, staff)
			assert.ErrorIs(t, err, tc.want)
			assert.NotContains(t, err.Error(), tc.input.Password,
				"an error must never carry the password")
		})
	}
}

// An unexpected storage failure must not be mistaken for a domain error, or a
// database outage would be reported to the caller as a duplicate username.
func TestCreateStaff_UnexpectedFailureIsNotASentinel(t *testing.T) {
	hospitals := seededHospitals()
	staffRepo := &fakeStaff{byUsername: map[string]models.Staff{}, createErr: errors.New("connection refused")}

	_, err := newService(t, hospitals, staffRepo).CreateStaff(context.Background(),
		service.CreateStaffInput{Username: "somchai", Password: testPassword, Hospital: testHospital})

	require.Error(t, err)
	assert.NotErrorIs(t, err, service.ErrDuplicateStaff)
	assert.NotErrorIs(t, err, service.ErrUnknownHospital)
}

func TestLogin_HappyPathIssuesAScopedToken(t *testing.T) {
	hospitals := seededHospitals()
	staffRepo := &fakeStaff{byUsername: map[string]models.Staff{}}
	authService := newService(t, hospitals, staffRepo)

	_, err := authService.CreateStaff(context.Background(),
		service.CreateStaffInput{Username: "somchai", Password: testPassword, Hospital: testHospital})
	require.NoError(t, err)

	token, staff, err := authService.Login(context.Background(),
		service.LoginInput{Username: "somchai", Password: testPassword, Hospital: testHospital})
	require.NoError(t, err)
	require.NotEmpty(t, token)

	tokens, err := auth.NewTokenService(testSecret, time.Hour)
	require.NoError(t, err)
	claims, err := tokens.Verify(token)
	require.NoError(t, err)
	assert.Equal(t, staff.HospitalID, claims.HospitalID)
}

// Every credential failure returns the same sentinel, so the API cannot be used
// to discover which usernames or hospitals exist.
func TestLogin_AllCredentialFailuresAreIndistinguishable(t *testing.T) {
	tests := []struct {
		name  string
		input service.LoginInput
	}{
		{"wrong password", service.LoginInput{Username: "somchai", Password: "wrong-password-here", Hospital: testHospital}},
		{"unknown username", service.LoginInput{Username: "nobody", Password: testPassword, Hospital: testHospital}},
		{"unknown hospital", service.LoginInput{Username: "somchai", Password: testPassword, Hospital: "Nowhere"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			hospitals := seededHospitals()
			staffRepo := &fakeStaff{byUsername: map[string]models.Staff{}}
			authService := newService(t, hospitals, staffRepo)

			_, err := authService.CreateStaff(context.Background(),
				service.CreateStaffInput{Username: "somchai", Password: testPassword, Hospital: testHospital})
			require.NoError(t, err)

			token, staff, err := authService.Login(context.Background(), tc.input)

			assert.Empty(t, token)
			assert.Nil(t, staff)
			assert.ErrorIs(t, err, service.ErrInvalidCredentials)
		})
	}
}
