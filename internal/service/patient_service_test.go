package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ThanadolU/hospital-middleware/internal/his"
	"github.com/ThanadolU/hospital-middleware/internal/models"
	"github.com/ThanadolU/hospital-middleware/internal/service"
)

// The sync path's job at this layer is translation: HIS failures become
// service sentinels, so the handler can map an upstream outage to a different
// status than a caller mistake. In-memory fakes, because none of that is
// persistence — the storage behaviour is covered in internal/repository.

type fakeHISClient struct {
	patient *models.Patient
	err     error
	calls   int
	lastID  string
}

func (f *fakeHISClient) SearchPatient(_ context.Context, id string) (*models.Patient, error) {
	f.calls++
	f.lastID = id
	if f.err != nil {
		return nil, f.err
	}
	return f.patient, nil
}

type fakePatients struct {
	created  bool
	err      error
	calls    int
	gotScope uuid.UUID
}

func (f *fakePatients) Search(context.Context, uuid.UUID, models.SearchPatientRequest) ([]models.Patient, error) {
	return nil, nil
}

func (f *fakePatients) Upsert(_ context.Context, hospitalID uuid.UUID, _ *models.Patient) (bool, error) {
	f.calls++
	f.gotScope = hospitalID
	return f.created, f.err
}

func TestSyncFromHIS_StoresUnderTheCallersHospital(t *testing.T) {
	scope := uuid.New()
	hisClient := &fakeHISClient{patient: &models.Patient{NationalID: "1103700123456"}}
	patients := &fakePatients{created: true}
	svc := service.NewPatientService(patients, hisClient)

	patient, created, err := svc.SyncFromHIS(context.Background(), scope, "1103700123456")

	require.NoError(t, err)
	assert.True(t, created)
	require.NotNil(t, patient)
	assert.Equal(t, "1103700123456", hisClient.lastID, "the identifier must reach the HIS unchanged")
	assert.Equal(t, scope, patients.gotScope, "the record was stored under the wrong hospital")
}

// Each upstream failure must map to its own sentinel. Collapsing them would
// leave the handler unable to tell a missing patient from an outage.
func TestSyncFromHIS_TranslatesUpstreamFailures(t *testing.T) {
	tests := []struct {
		name   string
		hisErr error
		want   error
	}{
		{"patient absent", his.ErrPatientNotFound, service.ErrPatientNotFoundUpstream},
		{"upstream unreachable", his.ErrUpstream, service.ErrUpstreamUnavailable},
		{"unusable body", his.ErrInvalidResponse, service.ErrUpstreamUnavailable},
		{"rejected identifier", his.ErrInvalidID, service.ErrInvalidPatientID},
		{"unclassified failure", errors.New("something else"), service.ErrUpstreamUnavailable},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			patients := &fakePatients{}
			svc := service.NewPatientService(patients, &fakeHISClient{err: tc.hisErr})

			_, _, err := svc.SyncFromHIS(context.Background(), uuid.New(), "123")

			assert.ErrorIs(t, err, tc.want)
			assert.Zero(t, patients.calls, "nothing must be stored when the fetch failed")
		})
	}
}

// A nil client is the unconfigured deployment. It must be reported, not
// panicked on, so the rest of the API keeps working.
func TestSyncFromHIS_WithoutAClientIsReportedNotPanicked(t *testing.T) {
	patients := &fakePatients{}
	svc := service.NewPatientService(patients, nil)

	require.NotPanics(t, func() {
		_, _, err := svc.SyncFromHIS(context.Background(), uuid.New(), "123")
		assert.ErrorIs(t, err, service.ErrHISNotConfigured)
	})
	assert.Zero(t, patients.calls)
}

func TestSyncFromHIS_RejectsAnEmptyIdentifierWithoutCallingUpstream(t *testing.T) {
	for _, id := range []string{"", "   "} {
		hisClient := &fakeHISClient{patient: &models.Patient{}}
		svc := service.NewPatientService(&fakePatients{}, hisClient)

		_, _, err := svc.SyncFromHIS(context.Background(), uuid.New(), id)

		assert.ErrorIs(t, err, service.ErrInvalidPatientID)
		assert.Zero(t, hisClient.calls, "an empty identifier must not reach the upstream")
	}
}

// A storage failure is not an upstream failure, and must not be reported as
// one — otherwise a full disk looks like the HIS being down.
func TestSyncFromHIS_StorageFailureIsNotAnUpstreamFailure(t *testing.T) {
	svc := service.NewPatientService(
		&fakePatients{err: errors.New("connection refused")},
		&fakeHISClient{patient: &models.Patient{NationalID: "1"}},
	)

	_, _, err := svc.SyncFromHIS(context.Background(), uuid.New(), "1")

	require.Error(t, err)
	assert.NotErrorIs(t, err, service.ErrUpstreamUnavailable)
	assert.NotErrorIs(t, err, service.ErrPatientNotFoundUpstream)
}
