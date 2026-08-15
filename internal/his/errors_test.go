package his

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Error.Error() is what lands in a log line when an upstream call fails, so it
// has to carry enough to debug with: which operation, which class of failure,
// the status when there was one, and the underlying cause. Nothing else
// asserts that, and a message that silently loses the cause is a bad hour for
// whoever is on call.

func TestError_MessageCarriesTheDetailNeededToDebug(t *testing.T) {
	tests := []struct {
		name     string
		err      *Error
		contains []string
		absent   []string
	}{
		{
			name:     "sentinel and operation only",
			err:      &Error{Op: "his: hospital_a: SearchPatient", Kind: ErrPatientNotFound},
			contains: []string{"his: hospital_a: SearchPatient", ErrPatientNotFound.Error()},
			// No status was involved, so none should be invented.
			absent: []string{"status"},
		},
		{
			name: "status is included when the upstream answered",
			err: &Error{
				Op:         "his: hospital_a: SearchPatient",
				Kind:       ErrUpstream,
				StatusCode: 503,
			},
			contains: []string{ErrUpstream.Error(), "status 503"},
		},
		{
			name: "underlying cause is not swallowed",
			err: &Error{
				Op:   "his: hospital_a: SearchPatient",
				Kind: ErrUpstream,
				Err:  context.DeadlineExceeded,
			},
			contains: []string{ErrUpstream.Error(), context.DeadlineExceeded.Error()},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			message := tc.err.Error()

			for _, want := range tc.contains {
				assert.Contains(t, message, want)
			}
			for _, unwanted := range tc.absent {
				assert.NotContains(t, message, unwanted)
			}
		})
	}
}

// The message is one thing; classification is another. Both must survive on
// the same value, which is the whole point of Unwrap returning two errors.
func TestError_RemainsClassifiableAlongsideItsCause(t *testing.T) {
	err := &Error{
		Op:   "his: hospital_a: SearchPatient",
		Kind: ErrUpstream,
		Err:  context.DeadlineExceeded,
	}

	assert.ErrorIs(t, err, ErrUpstream, "the sentinel must stay reachable")
	assert.ErrorIs(t, err, context.DeadlineExceeded, "the cause must stay reachable")
	assert.NotErrorIs(t, err, ErrPatientNotFound, "an unrelated sentinel must not match")
}

func TestError_WithoutACauseUnwrapsToTheSentinelAlone(t *testing.T) {
	err := &Error{Op: "his: hospital_a: SearchPatient", Kind: ErrInvalidResponse}

	unwrapped := err.Unwrap()
	assert.Len(t, unwrapped, 1)
	assert.True(t, errors.Is(err, ErrInvalidResponse))
}
