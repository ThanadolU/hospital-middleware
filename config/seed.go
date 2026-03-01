package config

import (
	"time"

	"github.com/ThanadolU/hospital-middleware/internal/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func InitializeData(db *gorm.DB) {
	var count int64
	db.Model(&models.Hospital{}).Count(&count)

	if count == 0 {
		hospital := models.Hospital{
			Name: "Hospital A",
		}

		db.Create(&hospital)
		SeedPatients(db, hospital.ID)
	}
}

func SeedPatients(db *gorm.DB, hospitalID uuid.UUID) error {
	var count int64

	db.Model(&models.Patient{}).Count(&count)
	if count > 0 {
		return nil
	}

	patients := []models.Patient{
		{
			HospitalID:   hospitalID,
			FirstNameTH:  "สมชาย",
			MiddleNameTH: "",
			LastNameTH:   "ใจดี",
			FirstNameEN:  "Somchai",
			MiddleNameEN: "",
			LastNameEN:   "Jaidee",
			DateOfBirth:  time.Date(1990, 5, 12, 0, 0, 0, 0, time.UTC),
			PatientHN:    "HN0001",
			NationalID:   "1234567890123",
			PassportID:   "",
			PhoneNumber:  "0891234567",
			Email:        "somchai@example.com",
			Gender:       "M",
		},
		{
			HospitalID:   hospitalID,
			FirstNameTH:  "สุภาพร",
			MiddleNameTH: "",
			LastNameTH:   "ดีมาก",
			FirstNameEN:  "Supaporn",
			MiddleNameEN: "",
			LastNameEN:   "Deemark",
			DateOfBirth:  time.Date(1988, 11, 3, 0, 0, 0, 0, time.UTC),
			PatientHN:    "HN0002",
			NationalID:   "2345678901234",
			PassportID:   "",
			PhoneNumber:  "0812345678",
			Email:        "supaporn@example.com",
			Gender:       "F",
		},
		{
			HospitalID:   hospitalID,
			FirstNameTH:  "",
			MiddleNameTH: "",
			LastNameTH:   "",
			FirstNameEN:  "John",
			MiddleNameEN: "Michael",
			LastNameEN:   "Doe",
			DateOfBirth:  time.Date(1995, 2, 20, 0, 0, 0, 0, time.UTC),
			PatientHN:    "HN0003",
			NationalID:   "",
			PassportID:   "AA1234567",
			PhoneNumber:  "0869876543",
			Email:        "john.doe@example.com",
			Gender:       "M",
		},
		{
			HospitalID:   hospitalID,
			FirstNameTH:  "วิภา",
			MiddleNameTH: "ศรี",
			LastNameTH:   "ทอง",
			FirstNameEN:  "Wipa",
			MiddleNameEN: "Sri",
			LastNameEN:   "Thong",
			DateOfBirth:  time.Date(2000, 7, 15, 0, 0, 0, 0, time.UTC),
			PatientHN:    "HN0004",
			NationalID:   "3456789012345",
			PassportID:   "",
			PhoneNumber:  "0823456789",
			Email:        "wipa@example.com",
			Gender:       "F",
		},
	}

	return db.Create(&patients).Error
}