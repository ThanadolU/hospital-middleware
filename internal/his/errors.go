package his

import (
	"errors"
	"fmt"
)

// Sentinels a caller can branch on with errors.Is. The service layer maps
// these onto HTTP status codes: not-found to 404, the rest to 502.
var (
	// ErrInvalidID means the caller supplied an id the upstream cannot accept.
	// No request is made.
	ErrInvalidID = errors.New("his: invalid patient id")

	// ErrPatientNotFound means the HIS answered, and has no such patient.
	ErrPatientNotFound = errors.New("his: patient not found")

	// ErrUpstream covers every failure to get an answer at all: transport
	// errors, timeouts, and any non-200 status other than 404.
	ErrUpstream = errors.New("his: upstream failure")

	// ErrInvalidResponse means the HIS answered 200 with a body we cannot
	// turn into a Patient — malformed JSON, an unparseable date, an unknown
	// gender, or a record with no usable identifier.
	ErrInvalidResponse = errors.New("his: invalid upstream response")
)

// Error carries the sentinel alongside the detail needed to debug a failure.
// Callers classify with errors.Is(err, his.ErrUpstream); the underlying cause
// stays reachable too, so errors.Is(err, context.DeadlineExceeded) also works.
type Error struct {
	Op         string // operation that failed, e.g. "his: hospital_a: SearchPatient"
	Kind       error  // one of the sentinels above
	StatusCode int    // upstream status, or 0 if the request never completed
	Err        error  // underlying cause, may be nil
}

func (e *Error) Error() string {
	msg := fmt.Sprintf("%s: %v", e.Op, e.Kind)
	if e.StatusCode != 0 {
		msg = fmt.Sprintf("%s (status %d)", msg, e.StatusCode)
	}
	if e.Err != nil {
		msg = fmt.Sprintf("%s: %v", msg, e.Err)
	}
	return msg
}

// Unwrap exposes both the sentinel and the cause to errors.Is and errors.As.
func (e *Error) Unwrap() []error {
	if e.Err == nil {
		return []error{e.Kind}
	}
	return []error{e.Kind, e.Err}
}
