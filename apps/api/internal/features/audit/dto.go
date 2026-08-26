package audit

import (
	"time"

	"github.com/google/uuid"
)

// LogResponse is the wire shape of one audit entry. ActorUserID is null
// for anonymous rows (password resets); ActorName is empty when the teacher
// no longer exists — the web layer decides how to render both.
type LogResponse struct {
	ID          uuid.UUID  `json:"id"`
	OccurredAt  time.Time  `json:"occurred_at"`
	ActorUserID *uuid.UUID `json:"actor_user_id"`
	ActorName   string     `json:"actor_name"`
	ActorRole   string     `json:"actor_role"`
	Action      string     `json:"action"`
	Method      string     `json:"method"`
	Path        string     `json:"path"`
	EntityType  string     `json:"entity_type"`
	EntityID    string     `json:"entity_id"`
	StatusCode  int        `json:"status_code"`
	IP          string     `json:"ip"`
	UserAgent   string     `json:"user_agent"`
	Metadata    Metadata   `json:"metadata"`
}

// ListResponse is one page of the trail. NextCursor is opaque and empty on
// the last page.
type ListResponse struct {
	Items      []LogResponse `json:"items"`
	NextCursor string        `json:"next_cursor"`
}

// FromRow maps a stored row (plus its resolved actor name) to the wire shape.
func FromRow(r *Row) LogResponse {
	return LogResponse{
		ID:          r.ID,
		OccurredAt:  r.OccurredAt,
		ActorUserID: r.ActorUserID,
		ActorName:   r.ActorName,
		ActorRole:   r.ActorRole,
		Action:      r.Action,
		Method:      r.Method,
		Path:        r.Path,
		EntityType:  r.EntityType,
		EntityID:    r.EntityID,
		StatusCode:  r.StatusCode,
		IP:          r.IP,
		UserAgent:   r.UserAgent,
		Metadata:    r.Metadata,
	}
}
