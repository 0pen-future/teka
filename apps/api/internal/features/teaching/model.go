// Package teaching owns the teaching-only data the classbook and review
// screens work on: per-class curriculum (giáo trình), lesson plans (giáo án
// with the owner review loop), and — phase 3 — session notes and per-student
// marks. This data was previously device-local in the web app's teaching
// store; the API is now the authority, including the lesson-plan state
// machine.
package teaching

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Lesson-plan statuses — mirrors lesson_plans.status CHECK. StatusNone is
// virtual: a plan never saved has no row, and the state machine treats the
// missing row as this status. It is never stored and never serialized.
const (
	StatusNone     = "none"
	StatusDraft    = "draft"
	StatusPending  = "pending"
	StatusApproved = "approved"
	StatusRedo     = "redo"
)

// Lesson-plan actions — the verbs of the review loop. Save and submit belong
// to the class teacher; approve, request-redo, and reopen to the owner.
const (
	ActionSave        = "save"
	ActionSubmit      = "submit"
	ActionApprove     = "approve"
	ActionRequestRedo = "request-redo"
	ActionReopen      = "reopen"
)

// StringList maps an ordered JSONB string array column (curriculum lesson
// titles, plan activities). The whole list is always replaced at once —
// there is no per-element addressing anywhere in the product.
type StringList []string

// Value marshals the list, writing a nil slice as [] so the JSONB columns'
// NOT NULL DEFAULT '[]' shape never sees a SQL NULL.
func (l StringList) Value() (driver.Value, error) {
	if l == nil {
		l = StringList{}
	}
	return json.Marshal(l)
}

// Scan accepts the []byte/string forms the pgx/gorm stack hands over.
func (l *StringList) Scan(value any) error {
	switch v := value.(type) {
	case nil:
		*l = nil
		return nil
	case []byte:
		return json.Unmarshal(v, l)
	case string:
		return json.Unmarshal([]byte(v), l)
	default:
		return fmt.Errorf("cannot scan %T into StringList", value)
	}
}

// Curriculum is a class's giáo trình: the ordered lesson titles plus the
// progress pointer. One row per class; plans reference lessons by index, so
// editing the list can shift which plan belongs to which title — accepted
// prototype semantics, unchanged here.
type Curriculum struct {
	ID      uuid.UUID `gorm:"primaryKey"`
	ClassID uuid.UUID
	// TeacherID/CenterID anchor the row in the class's own teacher and
	// center at creation time (guard FK to center_members); they never
	// follow a later class reassignment.
	TeacherID    uuid.UUID
	CenterID     uuid.UUID
	Lessons      StringList
	CurrentIndex int
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// TableName pins the table explicitly so a later model rename cannot
// silently break the mapping.
func (Curriculum) TableName() string { return "class_curricula" }

// Plan is one giáo án, keyed by (class, lesson index). Status never holds
// StatusNone — that state is the absence of the row.
type Plan struct {
	ID          uuid.UUID `gorm:"primaryKey"`
	ClassID     uuid.UUID
	LessonIndex int
	TeacherID   uuid.UUID
	CenterID    uuid.UUID
	Goal        string
	Activities  StringList
	Homework    string
	// FileName is attachment metadata only (decision 2026-08-14): no real
	// upload exists yet, the UI just remembers what was attached.
	FileName *string
	Status   string
	// RedoNote is the owner's "yêu cầu sửa" comment shown to the teacher;
	// OwnerComment is the optional note left when approving. Distinct
	// columns because the UI renders them in different slots.
	RedoNote     *string
	OwnerComment *string
	// SubmittedBy records who actually pressed submit — kept separate from
	// TeacherID so it stays truthful if the class changes hands.
	SubmittedBy *uuid.UUID
	SubmittedAt *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// TableName pins the table explicitly.
func (Plan) TableName() string { return "lesson_plans" }

// SessionNote is the whole-class nhận xét for one session — 1:1 with the
// session, so session_id is the primary key. An empty note is the absence of
// the row (the service deletes on empty body), never a stored "".
type SessionNote struct {
	SessionID uuid.UUID `gorm:"primaryKey"`
	// TeacherID/CenterID anchor the row like every teaching table: the
	// session's own teacher and center at write time.
	TeacherID uuid.UUID
	CenterID  uuid.UUID
	Body      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// TableName pins the table explicitly.
func (SessionNote) TableName() string { return "session_notes" }

// SessionMark is one student's score and/or personal note for one session —
// merged into a single row per (session, student) because they share the key
// and the upsert path. Both fields nullable; a row where both are NULL is
// deleted by the service, so the table never holds empty rows.
type SessionMark struct {
	ID        uuid.UUID `gorm:"primaryKey"`
	SessionID uuid.UUID
	StudentID uuid.UUID
	TeacherID uuid.UUID
	CenterID  uuid.UUID
	// Score is the 0–10 scale the UI uses (NUMERIC(4,1) in the schema).
	Score        *float64
	PersonalNote *string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// TableName pins the table explicitly.
func (SessionMark) TableName() string { return "session_marks" }

// planTransitions is the review-loop state machine — the single transition
// source, ported verbatim from the web store so the server and the UI's
// button gating can never disagree. Subtleties preserved: save from redo
// keeps redo (the owner's note stays visible until resubmission); reopen
// from redo is the owner withdrawing their own request; the teacher's path
// out of redo stays submit-only.
var planTransitions = map[string]map[string]string{
	StatusNone:     {ActionSave: StatusDraft},
	StatusDraft:    {ActionSave: StatusDraft, ActionSubmit: StatusPending},
	StatusPending:  {ActionApprove: StatusApproved, ActionRequestRedo: StatusRedo},
	StatusRedo:     {ActionSave: StatusRedo, ActionSubmit: StatusPending, ActionReopen: StatusPending},
	StatusApproved: {ActionReopen: StatusPending},
}

// transition returns the next status for a legal move and "" for an illegal
// one — callers translate "" into the 409 contract, never coerce.
func transition(status, action string) string {
	return planTransitions[status][action]
}
