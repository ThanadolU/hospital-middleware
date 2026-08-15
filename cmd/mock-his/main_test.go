package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ThanadolU/hospital-middleware/internal/his"
)

// These tests drive the real Hospital A client against the mock, so the two
// cannot drift apart: if the mock stops honouring the contract the client
// expects, this fails rather than the demo.

func newMockHIS(t *testing.T) *his.HospitalA {
	t.Helper()

	server := httptest.NewServer(newMux(byID(fixtures)))
	t.Cleanup(server.Close)

	client, err := his.NewHospitalA(his.Config{
		BaseURL:    server.URL,
		Timeout:    2 * time.Second,
		HTTPClient: server.Client(),
	})
	require.NoError(t, err)
	return client
}

func TestMockHIS_ServesEveryFixtureUnderBothIdentifiers(t *testing.T) {
	client := newMockHIS(t)

	for _, fixture := range fixtures {
		for _, id := range []string{fixture.NationalID, fixture.PassportID} {
			if id == "" {
				continue // this fixture is keyed by the other identifier only
			}

			t.Run(fixture.PatientHN+"/"+id, func(t *testing.T) {
				patient, err := client.SearchPatient(context.Background(), id)
				require.NoError(t, err)

				assert.Equal(t, fixture.PatientHN, patient.PatientHN)
				assert.Equal(t, fixture.NationalID, patient.NationalID)
				assert.Equal(t, fixture.PassportID, patient.PassportID)
				assert.Equal(t, fixture.FirstNameEN, patient.FirstNameEN)
				assert.Equal(t, fixture.Gender, patient.Gender)
				assert.Equal(t, fixture.DateOfBirth, patient.DateOfBirth.Format("2006-01-02"))
			})
		}
	}
}

func TestMockHIS_UnknownIDIsNotFound(t *testing.T) {
	client := newMockHIS(t)

	patient, err := client.SearchPatient(context.Background(), "0000000000000")

	assert.Nil(t, patient)
	assert.ErrorIs(t, err, his.ErrPatientNotFound)
}

func TestMockHIS_Health(t *testing.T) {
	server := httptest.NewServer(newMux(byID(fixtures)))
	t.Cleanup(server.Close)

	resp, err := server.Client().Get(server.URL + "/health")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestByID_IndexesBothIdentifiersAndSkipsBlanks(t *testing.T) {
	index := byID([]patient{
		{PatientHN: "HN-1", NationalID: "111", PassportID: "AAA"},
		{PatientHN: "HN-2", NationalID: "222"},
		{PatientHN: "HN-3", PassportID: "CCC"},
	})

	require.Len(t, index, 4, "two keys for the dual-identifier patient, one each for the others")
	assert.Equal(t, "HN-1", index["111"].PatientHN)
	assert.Equal(t, "HN-1", index["AAA"].PatientHN)
	assert.Equal(t, "HN-2", index["222"].PatientHN)
	assert.Equal(t, "HN-3", index["CCC"].PatientHN)

	_, blankIndexed := index[""]
	assert.False(t, blankIndexed, "a missing identifier must not become a lookup key")
}
