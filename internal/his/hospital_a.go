package his

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/ThanadolU/hospital-middleware/internal/models"
)

// BaseURLEnv names the environment variable holding Hospital A's base URL.
//
// The host given in the brief, hospital-a.api.co.th, is fictional and does not
// resolve, so the base URL is configuration rather than a constant. docker
// compose points it at the mock HIS in cmd/mock-his.
const BaseURLEnv = "HIS_HOSPITAL_A_BASE_URL"

const (
	defaultTimeout = 5 * time.Second

	// maxResponseBytes bounds how much of an upstream body we will read. A
	// single patient record is well under a kilobyte.
	maxResponseBytes = 1 << 20
)

// dateOfBirthLayouts are the formats accepted for the upstream date_of_birth.
// The brief shows a plain calendar date; RFC3339 is tolerated because HIS
// vendors commonly emit full timestamps for date fields.
var dateOfBirthLayouts = []string{"2006-01-02", time.RFC3339}

// Config configures a Hospital A client.
type Config struct {
	// BaseURL is the HIS root, without a trailing slash, e.g.
	// "https://hospital-a.api.co.th". Required.
	BaseURL string

	// Timeout bounds a single lookup, including connect and body read.
	// Defaults to defaultTimeout.
	Timeout time.Duration

	// HTTPClient is optional; supply one to share a transport or to inject a
	// test client. The per-request timeout above is applied regardless.
	HTTPClient *http.Client
}

// ConfigFromEnv reads Hospital A's configuration from the environment. It
// fails rather than falling back to a default, so a misconfigured deployment
// is caught at boot instead of on the first patient lookup.
func ConfigFromEnv() (Config, error) {
	baseURL := strings.TrimSpace(os.Getenv(BaseURLEnv))
	if baseURL == "" {
		return Config{}, fmt.Errorf("his: %s is not set", BaseURLEnv)
	}
	return Config{BaseURL: baseURL}, nil
}

// HospitalA is the Client implementation for Hospital A's HIS.
type HospitalA struct {
	baseURL string
	timeout time.Duration
	http    *http.Client
}

var _ Client = (*HospitalA)(nil)

// NewHospitalA builds a Hospital A client, validating the base URL up front.
func NewHospitalA(cfg Config) (*HospitalA, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" {
		return nil, errors.New("his: base URL is required")
	}
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("his: parse base URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("his: base URL must be http or https, got %q", cfg.BaseURL)
	}
	if parsed.Host == "" {
		return nil, fmt.Errorf("his: base URL has no host: %q", cfg.BaseURL)
	}

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}

	httpClient := cfg.HTTPClient
	if httpClient == nil {
		// Never the package-level http.DefaultClient: it has no timeout, so a
		// hung HIS would pin a request goroutine indefinitely.
		httpClient = &http.Client{Timeout: timeout}
	}

	return &HospitalA{baseURL: baseURL, timeout: timeout, http: httpClient}, nil
}

// SearchPatient implements Client against GET {base}/patient/search/{id}.
func (c *HospitalA) SearchPatient(ctx context.Context, id string) (*models.Patient, error) {
	const op = "his: hospital_a: SearchPatient"

	id = strings.TrimSpace(id)
	if id == "" {
		return nil, &Error{Op: op, Kind: ErrInvalidID, Err: errors.New("id is empty")}
	}

	// Bound every lookup, even when the caller injected its own http.Client.
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	endpoint := c.baseURL + "/patient/search/" + url.PathEscape(id)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, &Error{Op: op, Kind: ErrUpstream, Err: err}
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		// Transport failures and expired deadlines land here. The cause is
		// wrapped, so errors.Is(err, context.DeadlineExceeded) still answers.
		return nil, &Error{Op: op, Kind: ErrUpstream, Err: err}
	}
	// Discarded deliberately: the body is either fully read below or being
	// abandoned on an error path, and a close failure changes neither outcome.
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return nil, &Error{Op: op, Kind: ErrPatientNotFound, StatusCode: resp.StatusCode}
	default:
		return nil, &Error{Op: op, Kind: ErrUpstream, StatusCode: resp.StatusCode}
	}

	var dto patientResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(&dto); err != nil {
		return nil, &Error{Op: op, Kind: ErrInvalidResponse, StatusCode: resp.StatusCode, Err: err}
	}

	patient, err := dto.toDomain()
	if err != nil {
		return nil, &Error{Op: op, Kind: ErrInvalidResponse, StatusCode: resp.StatusCode, Err: err}
	}
	return patient, nil
}

// toDomain maps all thirteen upstream fields onto the domain model, rejecting
// records the middleware could not store or key on.
//
// HospitalID is left zero on purpose: see the Client doc comment.
func (r patientResponse) toDomain() (*models.Patient, error) {
	if r.NationalID == "" && r.PassportID == "" {
		return nil, errors.New("record has neither national_id nor passport_id")
	}

	dateOfBirth, err := parseDateOfBirth(r.DateOfBirth)
	if err != nil {
		return nil, err
	}
	gender, err := parseGender(r.Gender)
	if err != nil {
		return nil, err
	}

	return &models.Patient{
		FirstNameTH:  r.FirstNameTH,
		MiddleNameTH: r.MiddleNameTH,
		LastNameTH:   r.LastNameTH,
		FirstNameEN:  r.FirstNameEN,
		MiddleNameEN: r.MiddleNameEN,
		LastNameEN:   r.LastNameEN,
		DateOfBirth:  dateOfBirth,
		PatientHN:    r.PatientHN,
		NationalID:   r.NationalID,
		PassportID:   r.PassportID,
		PhoneNumber:  r.PhoneNumber,
		Email:        r.Email,
		Gender:       gender,
	}, nil
}

func parseDateOfBirth(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, errors.New("date_of_birth is empty")
	}
	for _, layout := range dateOfBirthLayouts {
		if parsed, err := time.Parse(layout, raw); err == nil {
			// Keep the calendar date as written and drop the clock. Converting
			// to UTC first would shift the date for an offset timestamp such
			// as 1990-01-01T00:00:00+07:00.
			y, m, d := parsed.Date()
			return time.Date(y, m, d, 0, 0, 0, 0, time.UTC), nil
		}
	}
	return time.Time{}, fmt.Errorf("date_of_birth %q matches no accepted layout %v", raw, dateOfBirthLayouts)
}

func parseGender(raw string) (string, error) {
	switch gender := strings.ToUpper(strings.TrimSpace(raw)); gender {
	case "M", "F":
		return gender, nil
	default:
		return "", fmt.Errorf("gender %q is not M or F", raw)
	}
}
