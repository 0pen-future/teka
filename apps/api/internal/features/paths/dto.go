package paths

import (
	"time"

	"github.com/google/uuid"
)

// PathRequest creates or fully replaces a learning path's own fields. A
// blank status keeps the stored one (draft on create).
type PathRequest struct {
	Code        string  `json:"code" binding:"required,min=2,max=20"`
	Name        string  `json:"name" binding:"required,min=1,max=200"`
	Description *string `json:"description" binding:"omitempty,max=2000"`
	Status      string  `json:"status" binding:"omitempty,oneof=draft active archived"`
}

// StageRequest creates or fully replaces a stage's own fields; the position
// is owned by the path (append on create, reorder endpoint to move).
type StageRequest struct {
	Name string  `json:"name" binding:"required,min=1,max=200"`
	Goal *string `json:"goal" binding:"omitempty,max=2000"`
}

// ReorderRequest lists every stage of the path in its new order.
type ReorderRequest struct {
	StageIDs []uuid.UUID `json:"stage_ids" binding:"required,min=1"`
}

// StageCoursesRequest replaces a stage's course list wholesale; [] clears
// it. The tag caps the list at maxStageCourses.
type StageCoursesRequest struct {
	CourseIDs []uuid.UUID `json:"course_ids" binding:"required,max=20"`
}

// StageCourseResponse is a course as recommended inside a stage.
type StageCourseResponse struct {
	ID       uuid.UUID `json:"id"`
	Code     string    `json:"code"`
	Name     string    `json:"name"`
	Status   string    `json:"status"`
	Position int       `json:"position"`
}

// StageResponse is one stage with its courses in position order.
type StageResponse struct {
	ID       uuid.UUID             `json:"id"`
	Position int                   `json:"position"`
	Name     string                `json:"name"`
	Goal     *string               `json:"goal"`
	Courses  []StageCourseResponse `json:"courses"`
}

// PathResponse is the public learning path shape, stages embedded in
// position order. CourseCount counts distinct courses across stages.
type PathResponse struct {
	ID          uuid.UUID       `json:"id"`
	Code        string          `json:"code"`
	Name        string          `json:"name"`
	Description *string         `json:"description"`
	Status      string          `json:"status"`
	StageCount  int             `json:"stage_count"`
	CourseCount int             `json:"course_count"`
	Stages      []StageResponse `json:"stages"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

// pathResponse assembles one path from its rows; stages arrive in position
// order and courses grouped by stage in position order.
func pathResponse(p *LearningPath, stages []Stage, courses map[uuid.UUID][]StageCourseRow) PathResponse {
	out := PathResponse{
		ID: p.ID, Code: p.Code, Name: p.Name, Description: p.Description, Status: p.Status,
		Stages:    make([]StageResponse, 0, len(stages)),
		CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt,
	}
	distinct := map[uuid.UUID]struct{}{}
	for i := range stages {
		st := &stages[i]
		rows := courses[st.ID]
		resp := StageResponse{
			ID: st.ID, Position: st.Position, Name: st.Name, Goal: st.Goal,
			Courses: make([]StageCourseResponse, 0, len(rows)),
		}
		for _, r := range rows {
			distinct[r.CourseID] = struct{}{}
			resp.Courses = append(resp.Courses, StageCourseResponse{
				ID: r.CourseID, Code: r.Code, Name: r.Name, Status: r.Status, Position: r.Position,
			})
		}
		out.Stages = append(out.Stages, resp)
	}
	out.StageCount = len(out.Stages)
	out.CourseCount = len(distinct)
	return out
}
