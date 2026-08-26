// Package audit records who did what: one append-only row per mutating API
// request and per auth event, consumed from the shared event bus. The
// package subscribes to events published elsewhere (middleware, auth) and
// never leaks back into them — publishers depend only on shared/events.
// Schema lives in migration 000010; the model mirrors it and is never
// auto-migrated.
package audit

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Metadata maps a flat jsonb metadata column. Values are already sanitized
// by the publisher (e.g. masked phone) — nothing here may contain raw
// credentials or request bodies.
type Metadata map[string]string

// Value marshals the map, writing nil as {} so the NOT NULL DEFAULT '{}'
// column never sees a SQL NULL.
func (m Metadata) Value() (driver.Value, error) {
	if m == nil {
		return []byte("{}"), nil
	}
	return json.Marshal(m)
}

// Scan restores the map from the jsonb column. NULL (possible through outer
// joins or aggregate projections even though the column is NOT NULL) scans to
// a nil map, matching the repo's other jsonb types.
func (m *Metadata) Scan(src any) error {
	*m = nil
	switch v := src.(type) {
	case nil:
		return nil
	case []byte:
		return json.Unmarshal(v, m)
	case string:
		return json.Unmarshal([]byte(v), m)
	default:
		return fmt.Errorf("audit: cannot scan %T into Metadata", src)
	}
}

// Log mirrors audit_logs. CenterID and ActorUserID are pointers because
// auth events have no center scope and a failed login has no known actor.
type Log struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey"`
	OccurredAt  time.Time
	CenterID    *uuid.UUID `gorm:"type:uuid"`
	ActorUserID *uuid.UUID `gorm:"type:uuid"`
	ActorRole   string
	Action      string
	Method      string
	Path        string
	EntityType  string
	EntityID    string
	StatusCode  int
	RequestID   string
	IP          string
	UserAgent   string
	Metadata    Metadata `gorm:"type:jsonb"`
}

// TableName pins the table name explicitly.
func (Log) TableName() string { return "audit_logs" }
