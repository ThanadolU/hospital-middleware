package models

import (
	"time"

	"github.com/google/uuid"
)

// Patient mirrors the field set Hospital A's HIS returns, so that a record
// fetched upstream maps onto this model one-to-one.
//
// Indexes and constraints are deliberately absent from the struct tags: the
// schema is owned by the migrations, not by AutoMigrate.
type Patient struct {
	ID         uuid.UUID `gorm:"type:uuid;default:uuid_generate_v4();primaryKey" json:"id"`
	HospitalID uuid.UUID `gorm:"type:uuid;not null" json:"hospital_id"`

	FirstNameTH  string `json:"first_name_th"`
	MiddleNameTH string `json:"middle_name_th"`
	LastNameTH   string `json:"last_name_th"`
	FirstNameEN  string `json:"first_name_en"`
	MiddleNameEN string `json:"middle_name_en"`
	LastNameEN   string `json:"last_name_en"`

	DateOfBirth time.Time `gorm:"type:date" json:"date_of_birth"`

	PatientHN  string `json:"patient_hn"`
	NationalID string `json:"national_id"`
	PassportID string `json:"passport_id"`

	PhoneNumber string `json:"phone_number"`
	Email       string `json:"email"`
	Gender      string `json:"gender"`

	CreatedAt time.Time `gorm:"type:timestamp;default:now();not null" json:"created_at"`
	UpdatedAt time.Time `gorm:"type:timestamp;default:now();not null" json:"updated_at"`
}
