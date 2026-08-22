package handoff

import "github.com/google/uuid"

// ReassignRequest names the teacher a class is handed to. TeacherID is a
// pointer so a missing field fails binding:"required" instead of binding the
// zero uuid and reading as "hand it to the nil teacher".
type ReassignRequest struct {
	TeacherID *uuid.UUID `json:"teacher_id" binding:"required"`
}

// ReassignResponse reports the outcome: the class, its new teacher, and how
// many future planned sessions moved with it. moved_planned_sessions is 0 on an
// idempotent no-op (the class was already this teacher's).
type ReassignResponse struct {
	ClassID              uuid.UUID `json:"class_id"`
	TeacherID            uuid.UUID `json:"teacher_id"`
	MovedPlannedSessions int64     `json:"moved_planned_sessions"`
}

// fromResult maps the service result onto the public response shape.
func fromResult(r *Result) ReassignResponse {
	return ReassignResponse{
		ClassID:              r.ClassID,
		TeacherID:            r.TeacherID,
		MovedPlannedSessions: r.MovedPlannedSessions,
	}
}
