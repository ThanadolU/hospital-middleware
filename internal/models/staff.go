package models

import (
	"time"

	"github.com/google/uuid"
)

// Staff is a hospital employee who can search that hospital's patients — and
// only that hospital's. The HospitalID here is the sole source of the scope
// applied to every patient search; it is read from the authenticated token and
// never from caller-supplied input.
type Staff struct {
	ID       uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	Username string    `gorm:"not null" json:"username"`

	// Password holds a bcrypt hash, never a plaintext password. The json:"-"
	// tag keeps it out of every response body: a struct that cannot be
	// serialised cannot be leaked by a handler that forgets to strip it.
	Password string `gorm:"not null" json:"-"`

	HospitalID uuid.UUID `gorm:"type:uuid;not null" json:"hospital_id"`
	Hospital   *Hospital `gorm:"foreignKey:HospitalID" json:"hospital,omitempty"`

	CreatedAt time.Time `gorm:"type:timestamptz;default:now();not null" json:"created_at"`
	UpdatedAt time.Time `gorm:"type:timestamptz;default:now();not null" json:"updated_at"`
}
