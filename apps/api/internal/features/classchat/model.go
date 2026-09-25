package classchat

import (
	"time"

	"github.com/google/uuid"
)

// Message is one class_messages row: an internal note between the owner
// and the class's staff. deleted_at soft-deletes it so the author or the
// owner can retract a message while the row stays for the record.
type Message struct {
	ID        uuid.UUID `gorm:"primaryKey"`
	CenterID  uuid.UUID
	ClassID   uuid.UUID
	AuthorID  uuid.UUID
	Body      string
	CreatedAt time.Time
	DeletedAt *time.Time
}

// TableName maps the model onto class_messages.
func (Message) TableName() string { return "class_messages" }

// MessageRow is a Message joined with its author's display name.
type MessageRow struct {
	Message
	AuthorName string
}
