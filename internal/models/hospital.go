package models

import (
	"time"

	"github.com/google/uuid"
)

// Hospital is the tenant boundary of the system. Every patient and every staff
// member belongs to exactly one, and that ownership is what scopes a search.
type Hospital struct {
	ID   uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	Name string    `gorm:"not null" json:"name"`

	CreatedAt time.Time `gorm:"type:timestamptz;default:now();not null" json:"created_at"`
	UpdatedAt time.Time `gorm:"type:timestamptz;default:now();not null" json:"updated_at"`
}
