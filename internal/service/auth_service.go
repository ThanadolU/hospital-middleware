// Package service holds the application logic between the HTTP layer and the
// repositories. It converts storage and credential failures into a small set of
// sentinel errors that handlers map onto status codes, so no driver detail or
// raw error string ever reaches a client.
package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/ThanadolU/hospital-middleware/internal/auth"
	"github.com/ThanadolU/hospital-middleware/internal/models"
	"github.com/ThanadolU/hospital-middleware/internal/repository"
)

// Sentinel errors the handler layer maps to status codes. v1 returned
// err.Error() straight to the caller and answered 500 for everything, which
// leaked Postgres constraint names and told a caller nothing useful.
var (
	// ErrDuplicateStaff — the hospital already employs that username. 409.
	ErrDuplicateStaff = errors.New("service: staff already exists")

	// ErrUnknownHospital — no hospital by that name. 400.
	ErrUnknownHospital = errors.New("service: unknown hospital")

	// ErrWeakPassword — the password fails the minimum policy. 400.
	ErrWeakPassword = errors.New("service: password is not acceptable")

	// ErrInvalidCredentials — wrong username, wrong password, or wrong
	// hospital. Deliberately one error for all three: distinguishing them
	// would let a caller enumerate which usernames exist. 401.
	ErrInvalidCredentials = errors.New("service: invalid credentials")
)

// CreateStaffInput is the brief's staff-creation input: username, password,
// and a hospital *name*.
type CreateStaffInput struct {
	Username string
	Password string
	Hospital string
}

// LoginInput mirrors CreateStaffInput; the brief specifies the same three
// fields for login, since a username is only unique within a hospital.
type LoginInput struct {
	Username string
	Password string
	Hospital string
}

// AuthService creates and authenticates staff.
type AuthService interface {
	CreateStaff(ctx context.Context, in CreateStaffInput) (*models.Staff, error)

	// Login returns a signed token on success.
	Login(ctx context.Context, in LoginInput) (string, *models.Staff, error)
}

type authService struct {
	staff     repository.StaffRepository
	hospitals repository.HospitalRepository
	tokens    *auth.TokenService
}

func NewAuthService(
	staff repository.StaffRepository,
	hospitals repository.HospitalRepository,
	tokens *auth.TokenService,
) AuthService {
	return &authService{staff: staff, hospitals: hospitals, tokens: tokens}
}

func (s *authService) CreateStaff(ctx context.Context, in CreateStaffInput) (*models.Staff, error) {
	username := strings.TrimSpace(in.Username)
	if username == "" {
		return nil, fmt.Errorf("%w: username is required", ErrInvalidCredentials)
	}

	hospital, err := s.hospitals.FindByName(ctx, in.Hospital)
	if err != nil {
		if errors.Is(err, repository.ErrHospitalNotFound) {
			return nil, fmt.Errorf("%w: %q", ErrUnknownHospital, in.Hospital)
		}
		return nil, fmt.Errorf("service: create staff: %w", err)
	}

	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		if errors.Is(err, auth.ErrPasswordTooShort) || errors.Is(err, auth.ErrPasswordTooLong) {
			// Wrapping the sentinel, not the input: the error must never carry
			// the password itself.
			return nil, fmt.Errorf("%w: %v", ErrWeakPassword, err)
		}
		return nil, fmt.Errorf("service: hash password: %w", err)
	}

	staff := &models.Staff{Username: username, Password: hash, HospitalID: hospital.ID}
	if err := s.staff.Create(ctx, staff); err != nil {
		if errors.Is(err, repository.ErrDuplicateStaff) {
			return nil, ErrDuplicateStaff
		}
		return nil, fmt.Errorf("service: create staff: %w", err)
	}

	staff.Hospital = hospital
	return staff, nil
}

func (s *authService) Login(ctx context.Context, in LoginInput) (string, *models.Staff, error) {
	hospital, err := s.hospitals.FindByName(ctx, in.Hospital)
	if err != nil {
		if errors.Is(err, repository.ErrHospitalNotFound) {
			// Not ErrUnknownHospital: at login, an unknown hospital is just
			// another failed credential, and saying which part was wrong helps
			// an attacker more than it helps a user.
			return "", nil, ErrInvalidCredentials
		}
		return "", nil, fmt.Errorf("service: login: %w", err)
	}

	staff, err := s.staff.FindByUsername(ctx, hospital.ID, in.Username)
	if err != nil {
		if errors.Is(err, repository.ErrStaffNotFound) {
			return "", nil, ErrInvalidCredentials
		}
		return "", nil, fmt.Errorf("service: login: %w", err)
	}

	if err := auth.VerifyPassword(staff.Password, in.Password); err != nil {
		if errors.Is(err, auth.ErrPasswordMismatch) {
			return "", nil, ErrInvalidCredentials
		}
		return "", nil, fmt.Errorf("service: login: %w", err)
	}

	token, err := s.tokens.Issue(*staff)
	if err != nil {
		return "", nil, fmt.Errorf("service: issue token: %w", err)
	}

	staff.Hospital = hospital
	return token, staff, nil
}
