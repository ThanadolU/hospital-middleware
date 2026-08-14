// Package his talks to Hospital Information Systems on behalf of the
// middleware. Each upstream gets an adapter that owns its transport details
// and returns the shared domain model, so the layers above never see an
// upstream's JSON.
package his

import (
	"context"

	"github.com/ThanadolU/hospital-middleware/internal/models"
)

// Client fetches a patient record from a Hospital Information System.
//
// The returned Patient has a zero HospitalID. The upstream payload carries no
// hospital identity, so the caller stamps it from the HIS source it chose to
// query — see internal/service.PatientService.IngestFromHIS.
type Client interface {
	// SearchPatient looks up a single patient by national ID or passport ID.
	// The upstream accepts either in the same path segment, so callers pass
	// whichever identifier they hold.
	//
	// Failures are classified by the sentinels in errors.go.
	SearchPatient(ctx context.Context, id string) (*models.Patient, error)
}
