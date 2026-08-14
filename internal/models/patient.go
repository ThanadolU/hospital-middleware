package models

import (
	"time"

	"github.com/google/uuid"
)

// SearchPatientRequest carries the eight optional criteria a staff member may
// search on. Every field is optional; an empty request matches every patient in
// the staff member's hospital.
//
// The hospital is deliberately NOT a field here. Scope comes from the
// authenticated staff member and is passed separately into the repository, so
// no caller — and no future handler — can widen it by supplying a value.
type SearchPatientRequest struct {
	NationalID  string `form:"national_id" binding:"omitempty,max=20"`
	PassportID  string `form:"passport_id" binding:"omitempty,max=20"`
	FirstName   string `form:"first_name" binding:"omitempty,max=100"`
	MiddleName  string `form:"middle_name" binding:"omitempty,max=100"`
	LastName    string `form:"last_name" binding:"omitempty,max=100"`
	DateOfBirth string `form:"date_of_birth" binding:"omitempty,datetime=2006-01-02"`
	PhoneNumber string `form:"phone_number" binding:"omitempty,max=20"`
	Email       string `form:"email" binding:"omitempty,max=254"`
}

// Patient mirrors the field set Hospital A's HIS returns, so that a record
// fetched upstream maps onto this model one-to-one. PatientHISFields pins that
// correspondence, and patient_test.go fails if the two drift apart.
//
// Constraints and indexes live in migrations/, not in these struct tags: the
// schema this model needs cannot be expressed by GORM's AutoMigrate (partial
// unique indexes, trigram indexes), and one owner for the schema beats two.
// The tags here describe column types only.
type Patient struct {
	ID uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`

	// HospitalID is the tenant boundary: a search is always scoped to the
	// staff member's hospital, and that scoping is applied in the repository
	// rather than being left to callers.
	HospitalID uuid.UUID `gorm:"type:uuid;not null" json:"hospital_id"`
	Hospital   *Hospital `gorm:"foreignKey:HospitalID" json:"hospital,omitempty"`

	FirstNameTH  string `gorm:"not null;default:''" json:"first_name_th"`
	MiddleNameTH string `gorm:"not null;default:''" json:"middle_name_th"`
	LastNameTH   string `gorm:"not null;default:''" json:"last_name_th"`
	FirstNameEN  string `gorm:"not null;default:''" json:"first_name_en"`
	MiddleNameEN string `gorm:"not null;default:''" json:"middle_name_en"`
	LastNameEN   string `gorm:"not null;default:''" json:"last_name_en"`

	DateOfBirth time.Time `gorm:"type:date;not null" json:"date_of_birth"`

	PatientHN string `gorm:"not null;default:''" json:"patient_hn"`

	// NationalID and PassportID are empty strings rather than NULL when a
	// patient lacks one. A patient must carry at least one of the two, which
	// the schema enforces with a CHECK constraint; uniqueness is enforced per
	// hospital by partial indexes that ignore the empty value.
	NationalID string `gorm:"not null;default:''" json:"national_id"`
	PassportID string `gorm:"not null;default:''" json:"passport_id"`

	PhoneNumber string `gorm:"not null;default:''" json:"phone_number"`
	Email       string `gorm:"not null;default:''" json:"email"`

	// Gender is constrained to M or F, matching the upstream contract.
	Gender string `gorm:"not null" json:"gender"`

	CreatedAt time.Time `gorm:"type:timestamptz;default:now();not null" json:"created_at"`
	UpdatedAt time.Time `gorm:"type:timestamptz;default:now();not null" json:"updated_at"`
}

// PatientHISFields lists, in the brief's own order, the thirteen fields
// Hospital A's HIS returns. It is the machine-checkable form of "the Patient
// schema is compatible with hospital data structures" (traceability 2.1).
var PatientHISFields = []string{
	"first_name_th",
	"middle_name_th",
	"last_name_th",
	"first_name_en",
	"middle_name_en",
	"last_name_en",
	"date_of_birth",
	"patient_hn",
	"national_id",
	"passport_id",
	"phone_number",
	"email",
	"gender",
}
