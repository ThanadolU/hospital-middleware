package models

import (
	"time"

	"github.com/google/uuid"
)

type Hospital struct {
	ID   	  uuid.UUID `gorm:"type:uuid;default:uuid_generate_v4();primaryKey" json:"id"`
	Name 	  string    `gorm:"unique:not null" json:"name"`
	CreatedAt time.Time `gorm:"type:timestamp;default:now();not null" json:"created_at"`
	UpdatedAt time.Time `gorm:"type:timestamp;default:now();not null" json:"updated_at"`

	Staffs    []Staff   `json:"staffs,omitempty"`
	Patients  []Patient `json:"patients,omitempty"`
}