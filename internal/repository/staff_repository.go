package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/ThanadolU/hospital-middleware/internal/models"
)

var (
	// ErrStaffNotFound means no staff member matches the hospital and username.
	ErrStaffNotFound = errors.New("repository: staff not found")

	// ErrDuplicateStaff means the hospital already employs that username.
	ErrDuplicateStaff = errors.New("repository: staff already exists")
)

// StaffRepository stores and retrieves hospital staff.
type StaffRepository interface {
	// Create persists a new staff member. staff.Password must already be a
	// hash; this layer never sees a plaintext credential.
	Create(ctx context.Context, staff *models.Staff) error

	// FindByUsername resolves a staff member within one hospital. The lookup is
	// scoped by hospital because usernames are unique per hospital, not
	// globally — two hospitals may each employ an "admin".
	FindByUsername(ctx context.Context, hospitalID uuid.UUID, username string) (*models.Staff, error)
}

type staffRepository struct {
	db *gorm.DB
}

func NewStaffRepository(db *gorm.DB) StaffRepository {
	return &staffRepository{db: db}
}

func (r *staffRepository) Create(ctx context.Context, staff *models.Staff) error {
	if staff == nil {
		return errors.New("repository: nil staff")
	}
	if staff.HospitalID == uuid.Nil {
		return ErrHospitalScopeRequired
	}

	err := r.db.WithContext(ctx).Create(staff).Error
	switch {
	case err == nil:
		return nil
	case errors.Is(err, gorm.ErrDuplicatedKey) || isUniqueViolation(err):
		// Surfaced as a domain error so the handler can answer 409 without
		// inspecting driver internals, and without echoing the constraint name
		// back to the caller as v1 did.
		return ErrDuplicateStaff
	default:
		return fmt.Errorf("repository: create staff: %w", err)
	}
}

func (r *staffRepository) FindByUsername(
	ctx context.Context,
	hospitalID uuid.UUID,
	username string,
) (*models.Staff, error) {
	if hospitalID == uuid.Nil {
		return nil, ErrHospitalScopeRequired
	}

	var staff models.Staff
	err := r.db.WithContext(ctx).
		Where("hospital_id = ? AND lower(username) = lower(?)", hospitalID, strings.TrimSpace(username)).
		First(&staff).Error

	switch {
	case err == nil:
		return &staff, nil
	case errors.Is(err, gorm.ErrRecordNotFound):
		return nil, ErrStaffNotFound
	default:
		return nil, fmt.Errorf("repository: find staff: %w", err)
	}
}

// isUniqueViolation reports whether err is PostgreSQL's unique_violation
// (SQLSTATE 23505). Matching on the code rather than the message keeps this
// working across driver and locale differences.
func isUniqueViolation(err error) bool {
	var pgErr interface{ SQLState() string }
	if errors.As(err, &pgErr) {
		return pgErr.SQLState() == "23505"
	}
	return false
}
