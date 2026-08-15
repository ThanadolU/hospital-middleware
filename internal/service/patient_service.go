package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/ThanadolU/hospital-middleware/internal/his"
	"github.com/ThanadolU/hospital-middleware/internal/models"
	"github.com/ThanadolU/hospital-middleware/internal/repository"
)

// Sentinel errors for the sync path, so the handler can map an upstream
// problem to a different status than a caller mistake.
var (
	// ErrHISNotConfigured means no HIS client was wired in. Returned rather
	// than panicking on a nil client, so a misconfigured deployment answers
	// 503 instead of crashing the request.
	ErrHISNotConfigured = errors.New("service: no HIS client is configured")

	// ErrPatientNotFoundUpstream means the HIS answered, and answered that it
	// has no such patient. Distinct from ErrUpstreamUnavailable: an absence is
	// not an outage.
	ErrPatientNotFoundUpstream = errors.New("service: patient not found in the HIS")

	// ErrUpstreamUnavailable means we could not get an answer, or got one we
	// could not use.
	ErrUpstreamUnavailable = errors.New("service: the HIS is unavailable")

	// ErrInvalidPatientID means the caller supplied no usable identifier.
	ErrInvalidPatientID = errors.New("service: a patient identifier is required")
)

// PatientService searches patient records and ingests them from a HIS.
type PatientService interface {
	// Search returns the patients in hospitalID matching req.
	//
	// hospitalID comes from the authenticated staff member's token and is a
	// parameter rather than part of req, so it cannot be supplied — or
	// widened — by anything the client sends.
	Search(ctx context.Context, hospitalID uuid.UUID, req models.SearchPatientRequest) ([]models.Patient, error)

	// SyncFromHIS fetches one patient from the Hospital Information System and
	// stores it under hospitalID, reporting whether the record was new.
	//
	// The upstream payload carries no hospital identity, so the scope is
	// stamped here from the authenticated caller rather than taken from the
	// response — see internal/his.Client.
	SyncFromHIS(ctx context.Context, hospitalID uuid.UUID, id string) (patient *models.Patient, created bool, err error)
}

type patientService struct {
	patients repository.PatientRepository
	his      his.Client
}

// NewPatientService builds the service. hisClient may be nil, in which case
// the sync path returns ErrHISNotConfigured and search still works — a missing
// upstream must not take the whole service down.
func NewPatientService(patients repository.PatientRepository, hisClient his.Client) PatientService {
	return &patientService{patients: patients, his: hisClient}
}

func (s *patientService) Search(
	ctx context.Context,
	hospitalID uuid.UUID,
	req models.SearchPatientRequest,
) ([]models.Patient, error) {
	patients, err := s.patients.Search(ctx, hospitalID, req)
	if err != nil {
		return nil, fmt.Errorf("service: search patients: %w", err)
	}
	return patients, nil
}

func (s *patientService) SyncFromHIS(
	ctx context.Context,
	hospitalID uuid.UUID,
	id string,
) (*models.Patient, bool, error) {
	if s.his == nil {
		return nil, false, ErrHISNotConfigured
	}
	if strings.TrimSpace(id) == "" {
		return nil, false, ErrInvalidPatientID
	}

	patient, err := s.his.SearchPatient(ctx, id)
	if err != nil {
		// The HIS package classifies its own failures; this translates that
		// classification into the service's vocabulary rather than leaking
		// transport detail upwards.
		switch {
		case errors.Is(err, his.ErrPatientNotFound):
			return nil, false, fmt.Errorf("%w: %q", ErrPatientNotFoundUpstream, id)
		case errors.Is(err, his.ErrInvalidID):
			return nil, false, ErrInvalidPatientID
		default:
			return nil, false, fmt.Errorf("%w: %v", ErrUpstreamUnavailable, err)
		}
	}

	created, err := s.patients.Upsert(ctx, hospitalID, patient)
	if err != nil {
		return nil, false, fmt.Errorf("service: store synced patient: %w", err)
	}
	return patient, created, nil
}
