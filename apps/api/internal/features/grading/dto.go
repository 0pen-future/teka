package grading

import (
	"strings"

	"github.com/google/uuid"
)

// ScoreSetResponse is one template set with its component names in position
// order. The component list is a plain ordered []string — position is the
// index, and the whole list is always replaced at once (like a curriculum's
// lessons), so the web never addresses a single component.
type ScoreSetResponse struct {
	ID         uuid.UUID `json:"id"`
	Name       string    `json:"name"`
	Components []string  `json:"components"`
}

// ScoreSetRequest is the create/update body: a name and the ordered component
// names. Components binds 1..10 non-blank names; the service trims, rejects
// case-insensitive duplicates, and derives each position from the index.
type ScoreSetRequest struct {
	Name       string   `json:"name" binding:"required,max=100"`
	Components []string `json:"components" binding:"required,min=1,max=10,dive,required,max=50"`
}

// AssignScoreSetRequest carries the set to snapshot onto a class.
type AssignScoreSetRequest struct {
	SetID uuid.UUID `json:"set_id" binding:"required"`
}

// ClassComponentResponse is one snapshot component of a class. Unlike a
// template component it carries its own id, because student scores reference
// component_id — the web needs it to build the grid columns and address cells.
type ClassComponentResponse struct {
	ID       uuid.UUID `json:"id"`
	Name     string    `json:"name"`
	Position int16     `json:"position"`
}

// ClassComponentsResponse is a class's whole snapshot: the ordered component
// columns the score grid renders. An empty list means the class uses the plain
// general-score UI, not the component grid.
type ClassComponentsResponse struct {
	ClassID    uuid.UUID                `json:"class_id"`
	Components []ClassComponentResponse `json:"components"`
}

// ScoreResponse is one student's score for one component in one session.
type ScoreResponse struct {
	StudentID   uuid.UUID `json:"student_id"`
	ComponentID uuid.UUID `json:"component_id"`
	Score       float64   `json:"score"`
}

// SessionScoresResponse is the one round-trip the score grid needs: the
// class's component columns plus every recorded cell for the session.
type SessionScoresResponse struct {
	Components []ClassComponentResponse `json:"components"`
	Scores     []ScoreResponse          `json:"scores"`
}

// ScoreEntryRequest is one cell of the score batch. score is nullable: a value
// upserts the cell, null deletes it (the table never holds empty cells, like
// session_marks). Not tri-state like teaching's marks — a cell is a single
// value, so null unambiguously means "clear this cell".
type ScoreEntryRequest struct {
	StudentID   uuid.UUID `json:"student_id" binding:"required"`
	ComponentID uuid.UUID `json:"component_id" binding:"required"`
	Score       *float64  `json:"score"`
}

// componentKey identifies a score cell within one session.
type componentKey struct {
	componentID uuid.UUID
	studentID   uuid.UUID
}

// normalizeComponentNames trims each component name and rejects blanks and
// case-insensitive duplicates within the set. It returns the cleaned names in
// input order — the caller uses the index as position.
func normalizeComponentNames(names []string) ([]string, string) {
	out := make([]string, 0, len(names))
	seen := make(map[string]bool, len(names))
	for _, raw := range names {
		name := strings.TrimSpace(raw)
		if name == "" {
			return nil, "component names cannot be blank"
		}
		key := strings.ToLower(name)
		if seen[key] {
			return nil, "component names must be unique within the set"
		}
		seen[key] = true
		out = append(out, name)
	}
	return out, ""
}
