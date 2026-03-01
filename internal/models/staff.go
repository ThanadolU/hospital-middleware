package models

import (
	"time"

	"github.com/google/uuid"
)

type Staff struct {
	ID 			uuid.UUID `gorm:"type:uuid;default:uuid_generate_v4();primaryKey" json:"id"`
	Username    string    `gorm:"not null;uniqueIndex:idx_username_hospital" json:"username"`
	Password    string    `gorm:"not null" json:"-"`
	HospitalID  uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_username_hospital" json:"hospital_id"`
	Hospital    Hospital  `gorm:"foreignKey:HospitalID" json:"hospital"`

	CreatedAt   time.Time `gorm:"type:timestamp;default:now();not null" json:"created_at"`
	UpdatedAt   time.Time `gorm:"type:timestamp;default:now();not null" json:"updated_at"`
}