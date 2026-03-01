package models

import (
	"time"

	"github.com/google/uuid"
)

type SearchPatientRequest struct {
	NationalID  string `form:"national_id" binding:"omitempty,len=13,numeric"`
	PassportID  string `form:"passport_id" binding:"omitempty,min=6,max=20"`
	FirstName   string `form:"first_name" binding:"omitempty,min=2,max=100"`
	MiddleName  string `form:"middle_name" binding:"omitempty,min=1,max=100"`
	LastName    string `form:"last_name" binding:"omitempty,min=2,max=100"`
	DateOfBirth string `form:"date_of_birth" binding:"omitempty,datetime=2006-01-02"`
	PhoneNumber string `form:"phone_number" binding:"omitempty,min=9,max=15"`
	Email       string `form:"email" binding:"omitempty,email"`
}

type Patient struct {
	ID           uuid.UUID `gorm:"type:uuid;default:uuid_generate_v4();primaryKey" json:"id"`

	HospitalID   uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_hospital_national" json:"hospital_id"`
	Hospital     Hospital  `gorm:"foreignKey:HospitalID" json:"hospital,omitzero"`

	FirstNameTH  string    `json:"first_name_th"`
	MiddleNameTH string    `json:"middle_name_th"`
	LastNameTH   string    `json:"last_name_th"`
	FirstNameEN  string    `json:"first_name_en"`
	MiddleNameEN string    `json:"middle_name_en"`
	LastNameEN   string    `json:"last_name_en"`

	DateOfBirth  time.Time `gorm:"type:date" json:"date_of_birth"`

	PatientHN    string    `json:"patient_hn"`
	NationalID   string    `gorm:"uniqueIndex:idx_hospital_national" json:"national_id"`
	PassportID   string    `gorm:"index" json:"passport_id"`

	PhoneNumber  string    `json:"phone_number"`
	Email		 string    `json:"email"`
	Gender       string    `json:"gender"`
	
	CreatedAt    time.Time `gorm:"type:timestamp;default:now();not null" json:"created_at"`
	UpdatedAt    time.Time `gorm:"type:timestamp;default:now();not null" json:"updated_at"`
}