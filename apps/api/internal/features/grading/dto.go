package grading

import (
	"github.com/google/uuid"
)

// ClassComponentResponse is one snapshot component of a class. It carries its
// own id because student scores reference
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
