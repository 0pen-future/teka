package teaching

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
)

// CurriculumResponse is the class's giáo trình. A class that never saved one
// reads as the empty default (lessons: [], current_index: 0) — the web maps
// that straight onto its store shape without a null branch.
type CurriculumResponse struct {
	Lessons      []string `json:"lessons"`
	CurrentIndex int      `json:"current_index"`
}

// PutCurriculumRequest whole-replaces the lesson list (matches the editor
// modal, which always saves the entire list). current_index is clamped
// server-side into the new list's range, so a shrinking edit can never leave
// the pointer past the end.
//
// Lessons deliberately has no `required` tag: an empty list is a legitimate
// replace and must bind the same as an explicit [] the client sent.
type PutCurriculumRequest struct {
	Lessons      []string `json:"lessons" binding:"max=100,dive,max=200"`
	CurrentIndex int      `json:"current_index" binding:"omitempty,min=0"`
}

// PlanResponse is one giáo án. submitted_by_name is resolved for display
// ("Soạn trực tiếp bởi …" and the review queue's teacher column) — the web
// never needs the raw id, but it travels along for completeness.
type PlanResponse struct {
	ClassID         uuid.UUID  `json:"class_id"`
	LessonIndex     int        `json:"lesson_index"`
	Goal            string     `json:"goal"`
	Activities      []string   `json:"activities"`
	Homework        string     `json:"homework"`
	FileName        *string    `json:"file_name"`
	Status          string     `json:"status"`
	RedoNote        *string    `json:"redo_note"`
	OwnerComment    *string    `json:"owner_comment"`
	SubmittedBy     *uuid.UUID `json:"submitted_by"`
	SubmittedByName *string    `json:"submitted_by_name"`
	SubmittedAt     *time.Time `json:"submitted_at"`
}

// SavePlanRequest is the teacher's save: full content replace, status left
// to the state machine. file_name is attachment metadata only; null clears
// it.
type SavePlanRequest struct {
	Goal       string   `json:"goal" binding:"max=2000"`
	Activities []string `json:"activities" binding:"max=50,dive,max=500"`
	Homework   string   `json:"homework" binding:"max=2000"`
	FileName   *string  `json:"file_name" binding:"omitempty,max=255"`
}

// ReviewRequest carries the owner's comment for approve (optional — becomes
// owner_comment) and request-redo (required — becomes redo_note; enforced in
// the service so whitespace-only fails too).
type ReviewRequest struct {
	Comment string `json:"comment" binding:"max=1000"`
}

// QueueItemResponse is one pending giáo án in the owner's review queue.
// lesson_title and teacher_name are null-safe (curriculum shrank or the
// submitter is unknown); the web falls back to its own placeholders.
type QueueItemResponse struct {
	PlanID      uuid.UUID  `json:"plan_id"`
	ClassID     uuid.UUID  `json:"class_id"`
	ClassName   string     `json:"class_name"`
	LessonIndex int        `json:"lesson_index"`
	LessonTitle *string    `json:"lesson_title"`
	TeacherName *string    `json:"teacher_name"`
	SubmittedAt *time.Time `json:"submitted_at"`
}

// Optional distinguishes a JSON field that was omitted (leave the stored
// value untouched) from one explicitly sent as null (clear it) — the marks
// batch needs this because the classbook writes scores without touching
// personal notes and the student record writes notes without touching scores.
type Optional[T any] struct {
	// Set is true when the field appeared in the request body at all.
	Set   bool
	Value *T
}

// UnmarshalJSON marks the field present; a JSON null leaves Value nil.
func (o *Optional[T]) UnmarshalJSON(data []byte) error {
	o.Set = true
	if string(data) == "null" {
		o.Value = nil
		return nil
	}
	return json.Unmarshal(data, &o.Value)
}

// NoteResponse is a session's whole-class nhận xét. body is "" when no note
// exists (or the write just deleted it) — the web treats both the same.
type NoteResponse struct {
	SessionID uuid.UUID `json:"session_id"`
	Body      string    `json:"body"`
}

// PutNoteRequest upserts the session note; an empty (or whitespace-only)
// body deletes the row instead of storing an empty string.
type PutNoteRequest struct {
	Body string `json:"body" binding:"max=2000"`
}

// MarkResponse is one student's score and/or personal note for one session.
type MarkResponse struct {
	SessionID    uuid.UUID `json:"session_id"`
	StudentID    uuid.UUID `json:"student_id"`
	Score        *float64  `json:"score"`
	PersonalNote *string   `json:"personal_note"`
}

// MarkEntryRequest is one row of the marks batch. score and personal_note
// are tri-state: omitted = leave unchanged, null = clear, value = set. A row
// whose resulting fields are both NULL is deleted.
type MarkEntryRequest struct {
	StudentID    uuid.UUID         `json:"student_id" binding:"required"`
	Score        Optional[float64] `json:"score" swaggertype:"number"`
	PersonalNote Optional[string]  `json:"personal_note" swaggertype:"string"`
}

// MonthMarksResponse is the batch read the classbook and records screens
// rebuild their slices from: every session note and mark row of the class's
// sessions in the requested month.
type MonthMarksResponse struct {
	SessionNotes []NoteResponse `json:"session_notes"`
	Marks        []MarkResponse `json:"marks"`
}

// trimmedPtr collapses a possibly-blank optional string into the pointer
// form the nullable columns carry, whitespace-only reading as NULL.
func trimmedPtr(s string) *string {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

// cleanLines mirrors the web editor's activity handling: trim each line and
// drop the empty ones, preserving order.
func cleanLines(lines []string) []string {
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
