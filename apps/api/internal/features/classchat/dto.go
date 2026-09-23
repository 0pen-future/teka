package classchat

import (
	"time"

	"github.com/google/uuid"
)

// Page size bounds for the message list.
const (
	DefaultLimit = 20
	MaxLimit     = 50
)

// PostRequest is a new message. The body is trimmed; the 2000-character cap
// matches the table's CHECK constraint.
type PostRequest struct {
	Body string `json:"body" binding:"required,max=2000"`
}

// ListQuery pages the class's messages newest first. Before is the id of
// the oldest message already shown: the page holds only messages older than
// it. Limit is clamped to [1, MaxLimit] with DefaultLimit for 0.
type ListQuery struct {
	Before *uuid.UUID
	Limit  int
}

// MessageResponse is one message with its author's display name.
type MessageResponse struct {
	ID         uuid.UUID `json:"id"`
	ClassID    uuid.UUID `json:"class_id"`
	AuthorID   uuid.UUID `json:"author_id"`
	AuthorName string    `json:"author_name"`
	Body       string    `json:"body"`
	CreatedAt  time.Time `json:"created_at"`
}

// ListResponse is one page, newest first. next_cursor is the id to pass as
// before for the next (older) page, empty when this is the last one.
type ListResponse struct {
	Items      []MessageResponse `json:"items"`
	NextCursor string            `json:"next_cursor"`
}

func messageResponse(row *MessageRow) MessageResponse {
	return MessageResponse{
		ID:         row.ID,
		ClassID:    row.ClassID,
		AuthorID:   row.AuthorID,
		AuthorName: row.AuthorName,
		Body:       row.Body,
		CreatedAt:  row.CreatedAt,
	}
}
