package models

import (
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This is the field-by-field diff against the brief that traceability row 2.1
// asks for, expressed as a test so it cannot go stale. It needs no database.

// jsonFieldNames returns the JSON tag of every field on a struct, skipping
// fields excluded from serialisation.
func jsonFieldNames(t *testing.T, v any) []string {
	t.Helper()

	typ := reflect.TypeOf(v)
	require.Equal(t, reflect.Struct, typ.Kind())

	names := make([]string, 0, typ.NumField())
	for i := range typ.NumField() {
		tag := typ.Field(i).Tag.Get("json")
		name, _, _ := strings.Cut(tag, ",")
		if name == "" || name == "-" {
			continue
		}
		names = append(names, name)
	}
	return names
}

func TestPatient_CoversEveryHISField(t *testing.T) {
	require.Len(t, PatientHISFields, 13, "the brief defines thirteen upstream fields")

	present := make(map[string]bool)
	for _, name := range jsonFieldNames(t, Patient{}) {
		present[name] = true
	}

	for _, field := range PatientHISFields {
		assert.True(t, present[field], "Patient has no field for the upstream %q", field)
	}
}

// The model may carry fields the HIS does not send — an id, the hospital it
// belongs to, timestamps — but nothing beyond that set, or the schema has
// drifted from the contract without anyone deciding to.
func TestPatient_CarriesNoUnexpectedFields(t *testing.T) {
	allowedBeyondHIS := map[string]bool{
		"id":          true,
		"hospital_id": true,
		"hospital":    true,
		"created_at":  true,
		"updated_at":  true,
	}
	for _, field := range PatientHISFields {
		allowedBeyondHIS[field] = true
	}

	for _, name := range jsonFieldNames(t, Patient{}) {
		assert.True(t, allowedBeyondHIS[name],
			"Patient field %q is neither a HIS field nor a known local field", name)
	}
}

// Traceability 2.2: a patient belongs to a hospital.
func TestPatient_BelongsToAHospital(t *testing.T) {
	field, ok := reflect.TypeOf(Patient{}).FieldByName("HospitalID")
	require.True(t, ok, "Patient must carry the hospital it belongs to")
	assert.Contains(t, field.Tag.Get("gorm"), "not null",
		"hospital_id must be mandatory: an unscoped patient could leak across hospitals")
}
