package library

import (
	"time"

	"github.com/google/uuid"
)

// TemplateRequest creates or fully replaces a template's own fields. Code
// follows the class-code shape (uppercase letters, digits, dashes) and is
// unique among the center's live templates.
type TemplateRequest struct {
	Code        string  `json:"code" binding:"required,min=2,max=20"`
	Name        string  `json:"name" binding:"required,min=1,max=200"`
	Subject     *string `json:"subject" binding:"omitempty,max=100"`
	Level       *string `json:"level" binding:"omitempty,max=100"`
	Description *string `json:"description" binding:"omitempty,max=2000"`
	// LessonCount, on create only, seeds that many empty lessons
	// ("Buổi 1".."Buổi N") into the first draft so preparation can start
	// before the content is written. Ignored on update.
	LessonCount *int `json:"lesson_count" binding:"omitempty,min=1,max=100"`
}

// PrepSummaryResponse summarises the preparation of a template's open draft:
// how many lessons it has, how many are done and who is assigned.
type PrepSummaryResponse struct {
	LessonCount int      `json:"lesson_count"`
	DoneCount   int      `json:"done_count"`
	Assignees   []string `json:"assignees"`
}

// VersionRefResponse is one entry of a template's version history: enough to
// list every version without paging a second endpoint.
type VersionRefResponse struct {
	ID        uuid.UUID `json:"id"`
	VersionNo int       `json:"version_no"`
	Status    string    `json:"status"`
}

// TemplateResponse is the wire form of a template with its version summary.
// Prep is nil when the template has no draft. ClassCount counts classes bound
// to any version; LessonCount is the released version's lesson count when one
// exists, else the open draft's.
type TemplateResponse struct {
	ID                 uuid.UUID            `json:"id"`
	Code               string               `json:"code"`
	Name               string               `json:"name"`
	Subject            *string              `json:"subject"`
	Level              *string              `json:"level"`
	Description        *string              `json:"description"`
	CreatedBy          *uuid.UUID           `json:"created_by"`
	PublishedVersionNo *int                 `json:"published_version_no"`
	DraftVersionNo     *int                 `json:"draft_version_no"`
	DraftVersionID     *uuid.UUID           `json:"draft_version_id"`
	VersionCount       int                  `json:"version_count"`
	ClassCount         int                  `json:"class_count"`
	LessonCount        int                  `json:"lesson_count"`
	Versions           []VersionRefResponse `json:"versions"`
	Prep               *PrepSummaryResponse `json:"prep"`
	CreatedAt          time.Time            `json:"created_at"`
	UpdatedAt          time.Time            `json:"updated_at"`
}

func templateResponse(row *TemplateRow) TemplateResponse {
	versions := make([]VersionRefResponse, 0, len(row.Versions))
	for _, v := range row.Versions {
		versions = append(versions, VersionRefResponse(v))
	}
	out := TemplateResponse{
		ID: row.ID, Code: row.Code, Name: row.Name,
		Subject: row.Subject, Level: row.Level, Description: row.Description,
		CreatedBy:          row.CreatedBy,
		PublishedVersionNo: row.PublishedVersionNo,
		DraftVersionNo:     row.DraftVersionNo,
		DraftVersionID:     row.DraftVersionID,
		VersionCount:       row.VersionCount,
		ClassCount:         row.ClassCount,
		LessonCount:        row.LessonCount,
		Versions:           versions,
		CreatedAt:          row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
	if row.DraftVersionNo != nil {
		assignees := []string(row.DraftAssignees)
		if assignees == nil {
			assignees = []string{}
		}
		out.Prep = &PrepSummaryResponse{LessonCount: row.DraftLessonCount, DoneCount: row.DraftDoneCount, Assignees: assignees}
	}
	return out
}

// CreateVersionRequest opens a new draft; the changelog is what changed
// against the previous published version.
type CreateVersionRequest struct {
	Changelog *string `json:"changelog" binding:"omitempty,max=2000"`
}

// VersionClassRefResponse is one class bound to a version, as listed on
// VersionResponse.Classes.
type VersionClassRefResponse struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

// VersionResponse is the wire form of a version. ClassCount is the true
// count of classes bound to it; Classes is capped (see ListVersionClasses)
// and ordered by name.
type VersionResponse struct {
	ID          uuid.UUID                 `json:"id"`
	TemplateID  uuid.UUID                 `json:"template_id"`
	VersionNo   int                       `json:"version_no"`
	Status      string                    `json:"status"`
	Changelog   *string                   `json:"changelog"`
	PublishedAt *time.Time                `json:"published_at"`
	CreatedBy   *uuid.UUID                `json:"created_by"`
	LessonCount int                       `json:"lesson_count"`
	ClassCount  int                       `json:"class_count"`
	Classes     []VersionClassRefResponse `json:"classes"`
	CreatedAt   time.Time                 `json:"created_at"`
	UpdatedAt   time.Time                 `json:"updated_at"`
}

func versionResponse(row *VersionRow) VersionResponse {
	return VersionResponse{
		ID: row.ID, TemplateID: row.TemplateID, VersionNo: row.VersionNo,
		Status: row.Status, Changelog: row.Changelog, PublishedAt: row.PublishedAt,
		CreatedBy: row.CreatedBy, LessonCount: row.LessonCount,
		ClassCount: row.ClassCount, Classes: []VersionClassRefResponse{},
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
}

// versionResponseWithClasses overlays the version's bound classes onto the
// base response; used by GetVersion where the class list is fetched.
func versionResponseWithClasses(row *VersionRow, classes []VersionClassRef) VersionResponse {
	out := versionResponse(row)
	refs := make([]VersionClassRefResponse, 0, len(classes))
	for _, c := range classes {
		refs = append(refs, VersionClassRefResponse(c))
	}
	out.Classes = refs
	return out
}

// LessonRequest creates or fully replaces a template lesson's content.
// Position is never in the body: create appends, order changes go through
// the reorder endpoint.
type LessonRequest struct {
	Title        string  `json:"title" binding:"required,min=1,max=200"`
	Mode         string  `json:"mode" binding:"omitempty,oneof=scheduled self_study"`
	Unit         *string `json:"unit" binding:"omitempty,max=100"`
	Objectives   *string `json:"objectives" binding:"omitempty,max=4000"`
	DurationMin  *int    `json:"duration_min" binding:"omitempty,min=1,max=1440"`
	HomeworkNote *string `json:"homework_note" binding:"omitempty,max=4000"`
}

// mode defaults a blank request mode to scheduled: the field is optional on
// the wire, but the stored row always has one of the two known values.
func (r LessonRequest) mode() string {
	if r.Mode == "" {
		return LessonModeScheduled
	}
	return r.Mode
}

// PrepRequest changes the preparation state of a lesson. Both fields are
// optional; an omitted field keeps its current value and a present
// checklist replaces the stored one wholesale.
type PrepRequest struct {
	PrepStatus *string          `json:"prep_status" binding:"omitempty,oneof=todo doing review done"`
	Checklist  *[]ChecklistItem `json:"checklist" binding:"omitempty,max=50,dive"`
}

// AssignmentRequest replaces the assignee and due date of a lesson as one
// block, unlike PrepRequest: there is no "keep the current value" reading of
// an omitted field here, so a caller changing only the due date must still
// resend the current assignee_id, or that assignee is cleared along with it.
// The assignee must be a live member of the center; the due date is a
// calendar day (YYYY-MM-DD).
type AssignmentRequest struct {
	AssigneeID *uuid.UUID `json:"assignee_id"`
	DueDate    *string    `json:"due_date" binding:"omitempty,datetime=2006-01-02"`
}

// dueDateLayout is the wire form of a due date: a calendar day without time.
const dueDateLayout = "2006-01-02"

// AssigneeResponse is one live member the caller may assign a lesson to.
// It carries only what the assign picker needs, not the phone/email a
// members.list directory would — prep.assign does not imply members.list.
type AssigneeResponse struct {
	ID       uuid.UUID `json:"id"`
	FullName string    `json:"full_name"`
}

// LessonResponse is the wire form of a template lesson. MaterialCount and
// ExerciseCount are always 0 when the response was built from a bare Lesson
// (e.g. a fresh row just after CreateLesson); list/detail responses built
// from a LessonRow fill them in via lessonRowResponse.
type LessonResponse struct {
	ID            uuid.UUID       `json:"id"`
	VersionID     uuid.UUID       `json:"version_id"`
	Position      int             `json:"position"`
	Title         string          `json:"title"`
	Mode          string          `json:"mode"`
	Unit          *string         `json:"unit"`
	Objectives    *string         `json:"objectives"`
	DurationMin   *int            `json:"duration_min"`
	HomeworkNote  *string         `json:"homework_note"`
	PrepStatus    string          `json:"prep_status"`
	AssigneeID    *uuid.UUID      `json:"assignee_id"`
	DueDate       *string         `json:"due_date"`
	Checklist     []ChecklistItem `json:"checklist"`
	MaterialCount int             `json:"material_count"`
	ExerciseCount int             `json:"exercise_count"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
}

// DuplicateLessonResponse is the wire form of POST .../duplicate: the same
// shape as any other single-lesson response.
type DuplicateLessonResponse = LessonResponse

func lessonResponse(l *Lesson) LessonResponse {
	checklist := []ChecklistItem(l.Checklist)
	if checklist == nil {
		checklist = []ChecklistItem{}
	}
	return LessonResponse{
		ID: l.ID, VersionID: l.VersionID, Position: l.Position, Title: l.Title,
		Mode: l.Mode, Unit: l.Unit,
		Objectives: l.Objectives, DurationMin: l.DurationMin, HomeworkNote: l.HomeworkNote,
		PrepStatus: l.PrepStatus, AssigneeID: l.AssigneeID, DueDate: dueDateString(l.DueDate), Checklist: checklist,
		CreatedAt: l.CreatedAt, UpdatedAt: l.UpdatedAt,
	}
}

// lessonRowResponse overlays the material/exercise counts onto the base
// response; used by list/detail responses built from a LessonRow.
func lessonRowResponse(row *LessonRow) LessonResponse {
	out := lessonResponse(&row.Lesson)
	out.MaterialCount = row.MaterialCount
	out.ExerciseCount = row.ExerciseCount
	return out
}

func dueDateString(d *time.Time) *string {
	if d == nil {
		return nil
	}
	s := d.Format(dueDateLayout)
	return &s
}

// BoardResponse is the preparation board of one version: the four fixed
// status columns, each holding its lessons by position.
type BoardResponse struct {
	Template TemplateResponse      `json:"template"`
	Version  VersionResponse       `json:"version"`
	Columns  []BoardColumnResponse `json:"columns"`
}

// BoardColumnResponse is one status column of the board.
type BoardColumnResponse struct {
	Status  string              `json:"status"`
	Lessons []BoardCardResponse `json:"lessons"`
}

// BoardCardResponse is the card of one lesson on the board.
type BoardCardResponse struct {
	ID             uuid.UUID  `json:"id"`
	Position       int        `json:"position"`
	Title          string     `json:"title"`
	PrepStatus     string     `json:"prep_status"`
	AssigneeID     *uuid.UUID `json:"assignee_id"`
	AssigneeName   *string    `json:"assignee_name"`
	DueDate        *string    `json:"due_date"`
	ChecklistDone  int        `json:"checklist_done"`
	ChecklistTotal int        `json:"checklist_total"`
}

func boardResponse(tpl TemplateResponse, version VersionResponse, cards []BoardCard) *BoardResponse {
	columns := make([]BoardColumnResponse, 0, len(PrepStatuses))
	index := map[string]int{}
	for i, status := range PrepStatuses {
		index[status] = i
		columns = append(columns, BoardColumnResponse{Status: status, Lessons: []BoardCardResponse{}})
	}
	for i := range cards {
		card := &cards[i]
		done := 0
		for _, item := range card.Checklist {
			if item.Done {
				done++
			}
		}
		col := index[card.PrepStatus]
		columns[col].Lessons = append(columns[col].Lessons, BoardCardResponse{
			ID: card.ID, Position: card.Position, Title: card.Title, PrepStatus: card.PrepStatus,
			AssigneeID: card.AssigneeID, AssigneeName: card.AssigneeName, DueDate: dueDateString(card.DueDate),
			ChecklistDone: done, ChecklistTotal: len(card.Checklist),
		})
	}
	return &BoardResponse{Template: tpl, Version: version, Columns: columns}
}

func lessonResponses(rows []LessonRow) []LessonResponse {
	out := make([]LessonResponse, 0, len(rows))
	for i := range rows {
		out = append(out, lessonRowResponse(&rows[i]))
	}
	return out
}

// ReorderRequest is the complete new order of a version's lessons: every
// lesson id exactly once.
type ReorderRequest struct {
	LessonIDs []uuid.UUID `json:"lesson_ids" binding:"required,min=1"`
}

// MaterialRequest creates or fully replaces a library material. A material
// is a link (or a named reference when url is empty), never an upload. Kind
// "other" binds successfully here (a legacy "other" material must remain
// editable) but the service rejects it on create, and accepts it on update
// only when the stored material is already kind "other" — it is never
// offered as a choice for new content.
type MaterialRequest struct {
	Title       string   `json:"title" binding:"required,min=1,max=200"`
	Kind        string   `json:"kind" binding:"required,oneof=video audio image doc note live link other"`
	URL         *string  `json:"url" binding:"omitempty,max=2000"`
	Description *string  `json:"description" binding:"omitempty,max=4000"`
	Tags        []string `json:"tags" binding:"omitempty,max=20,dive,max=50"`
}

// MaterialResponse is the wire form of a material. LessonCount and
// TemplateCount are always 0 when the response was built from a bare
// Material (e.g. as attached to a lesson); the bank's own Get/List/Create/
// Update/status responses fill them in via materialBankResponse.
type MaterialResponse struct {
	ID            uuid.UUID `json:"id"`
	Title         string    `json:"title"`
	Kind          string    `json:"kind"`
	URL           *string   `json:"url"`
	Description   *string   `json:"description"`
	Tags          []string  `json:"tags"`
	Active        bool      `json:"active"`
	LessonCount   int       `json:"lesson_count"`
	TemplateCount int       `json:"template_count"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func materialResponse(m *Material) MaterialResponse {
	return MaterialResponse{
		ID: m.ID, Title: m.Title, Kind: m.Kind, URL: m.URL, Description: m.Description,
		Tags: tags(m.Tags), Active: m.Active, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt,
	}
}

// materialBankResponse overlays the "in use" counts onto the base response;
// used by the bank's own Get/List/Create/Update/status-toggle responses.
func materialBankResponse(row *MaterialRow) MaterialResponse {
	out := materialResponse(&row.Material)
	out.LessonCount = row.LessonCount
	out.TemplateCount = row.TemplateCount
	return out
}

// ExerciseRequest creates or fully replaces a library exercise. Code, when
// given, is uppercased and checked against ^[A-Za-z0-9-]+$; when omitted the
// service assigns the next BT-0001-style code of the center.
type ExerciseRequest struct {
	Title       string   `json:"title" binding:"required,min=1,max=200"`
	Description *string  `json:"description" binding:"omitempty,max=4000"`
	Difficulty  *int     `json:"difficulty" binding:"omitempty,min=1,max=5"`
	Code        *string  `json:"code" binding:"omitempty,max=20"`
	Skill       *string  `json:"skill" binding:"omitempty,max=50"`
	Level       *string  `json:"level" binding:"omitempty,max=50"`
	Tags        []string `json:"tags" binding:"omitempty,max=20,dive,max=50"`
}

// ExerciseResponse is the wire form of an exercise. LessonCount and
// TemplateCount follow the same bank-only convention as MaterialResponse.
type ExerciseResponse struct {
	ID            uuid.UUID `json:"id"`
	Title         string    `json:"title"`
	Description   *string   `json:"description"`
	Difficulty    *int      `json:"difficulty"`
	Code          string    `json:"code"`
	Skill         *string   `json:"skill"`
	Level         *string   `json:"level"`
	Tags          []string  `json:"tags"`
	Active        bool      `json:"active"`
	LessonCount   int       `json:"lesson_count"`
	TemplateCount int       `json:"template_count"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func exerciseResponse(e *Exercise) ExerciseResponse {
	return ExerciseResponse{
		ID: e.ID, Title: e.Title, Description: e.Description, Difficulty: e.Difficulty,
		Code: e.Code, Skill: e.Skill, Level: e.Level,
		Tags: tags(e.Tags), Active: e.Active, CreatedAt: e.CreatedAt, UpdatedAt: e.UpdatedAt,
	}
}

// exerciseBankResponse overlays the "in use" counts onto the base response;
// used by the bank's own Get/List/Create/Update/status-toggle responses.
func exerciseBankResponse(row *ExerciseRow) ExerciseResponse {
	out := exerciseResponse(&row.Exercise)
	out.LessonCount = row.LessonCount
	out.TemplateCount = row.TemplateCount
	return out
}

// ItemStatusRequest toggles a material or exercise's Active flag.
type ItemStatusRequest struct {
	Active *bool `json:"active" binding:"required"`
}

// tags returns a non-nil slice so the wire form is always a JSON array.
func tags(l []string) []string {
	if l == nil {
		return []string{}
	}
	return l
}

// LessonMaterialInput is one entry of the wholesale-replace body of
// PUT /library/lessons/{lid}/materials; the body order is the display order.
type LessonMaterialInput struct {
	MaterialID         uuid.UUID `json:"material_id" binding:"required"`
	SharedWithStudents bool      `json:"shared_with_students"`
}

// LessonMaterialResponse is a material as attached to one lesson.
type LessonMaterialResponse struct {
	MaterialResponse
	SharedWithStudents bool `json:"shared_with_students"`
	Position           int  `json:"position"`
}

func lessonMaterialResponses(rows []LessonMaterialRow) []LessonMaterialResponse {
	out := make([]LessonMaterialResponse, 0, len(rows))
	for i := range rows {
		out = append(out, LessonMaterialResponse{
			MaterialResponse:   materialResponse(&rows[i].Material),
			SharedWithStudents: rows[i].SharedWithStudents,
			Position:           rows[i].Position,
		})
	}
	return out
}

// LessonExerciseInput is one entry of the wholesale-replace body of
// PUT /library/lessons/{lid}/exercises. GroupID, when given, must name an
// exercise group of the lesson's own version (422 otherwise).
type LessonExerciseInput struct {
	ExerciseID uuid.UUID  `json:"exercise_id" binding:"required"`
	GroupID    *uuid.UUID `json:"group_id"`
}

// LessonExerciseResponse is an exercise as attached to one lesson.
type LessonExerciseResponse struct {
	ExerciseResponse
	GroupID  *uuid.UUID `json:"group_id"`
	Position int        `json:"position"`
}

func lessonExerciseResponses(rows []LessonExerciseRow) []LessonExerciseResponse {
	out := make([]LessonExerciseResponse, 0, len(rows))
	for i := range rows {
		out = append(out, LessonExerciseResponse{
			ExerciseResponse: exerciseResponse(&rows[i].Exercise),
			GroupID:          rows[i].GroupID,
			Position:         rows[i].Position,
		})
	}
	return out
}

// ExerciseGroupRequest creates an exercise group of a draft version.
type ExerciseGroupRequest struct {
	Name string `json:"name" binding:"required,max=100"`
}

// ExerciseGroupResponse is the wire form of an exercise group.
type ExerciseGroupResponse struct {
	ID            uuid.UUID `json:"id"`
	VersionID     uuid.UUID `json:"version_id"`
	Name          string    `json:"name"`
	Position      int       `json:"position"`
	ExerciseCount int       `json:"exercise_count"`
}

func exerciseGroupResponse(row *ExerciseGroupRow) ExerciseGroupResponse {
	return ExerciseGroupResponse{
		ID: row.ID, VersionID: row.VersionID, Name: row.Name,
		Position: row.Position, ExerciseCount: row.ExerciseCount,
	}
}

func exerciseGroupResponses(rows []ExerciseGroupRow) []ExerciseGroupResponse {
	out := make([]ExerciseGroupResponse, 0, len(rows))
	for i := range rows {
		out = append(out, exerciseGroupResponse(&rows[i]))
	}
	return out
}

// LessonDetailResponse is a lesson with everything attached to it.
type LessonDetailResponse struct {
	LessonResponse
	Materials []LessonMaterialResponse `json:"materials"`
	Exercises []LessonExerciseResponse `json:"exercises"`
}

// LogFieldInput is one entry of the wholesale-replace body of
// PUT /library/versions/{vid}/log-fields; the body order is the position.
// Options are only meaningful for kind select, where at least one is
// required; other kinds store an empty list.
type LogFieldInput struct {
	Label    string   `json:"label" binding:"required,min=1,max=100"`
	Kind     string   `json:"kind" binding:"required,oneof=text long_text checkbox student number select"`
	Options  []string `json:"options" binding:"omitempty,max=20,dive,max=100"`
	Required bool     `json:"required"`
}

// LogFieldResponse is the wire form of a log field.
type LogFieldResponse struct {
	ID       uuid.UUID `json:"id"`
	Position int       `json:"position"`
	Label    string    `json:"label"`
	Kind     string    `json:"kind"`
	Options  []string  `json:"options"`
	Required bool      `json:"required"`
}

func logFieldResponses(rows []LogField) []LogFieldResponse {
	out := make([]LogFieldResponse, 0, len(rows))
	for i := range rows {
		f := &rows[i]
		out = append(out, LogFieldResponse{
			ID: f.ID, Position: f.Position, Label: f.Label, Kind: f.Kind,
			Options: tags(f.Options), Required: f.Required,
		})
	}
	return out
}

// ScoreComponentInput is one component of a score set group. Key is the
// stable identifier a class's grading refers to: lowercase letters, digits
// and underscores, unique within its group.
type ScoreComponentInput struct {
	Key    string  `json:"key" binding:"required,min=1,max=30"`
	Label  string  `json:"label" binding:"required,min=1,max=100"`
	Max    float64 `json:"max" binding:"required,gt=0"`
	Weight float64 `json:"weight" binding:"gte=0"`
}

// ScoreSetGroupInput is one entry of the wholesale-replace body of
// PUT /library/versions/{vid}/score-set. Key is the group's own stable
// identifier, unique within the set.
type ScoreSetGroupInput struct {
	Key        string                `json:"key" binding:"required,min=1,max=30"`
	Title      string                `json:"title" binding:"required,min=1,max=100"`
	Components []ScoreComponentInput `json:"components" binding:"omitempty,dive"`
}

// VersionDetailResponse is a version with its score set, log fields and
// every lesson including attachments: what a class binding to the version
// will inherit.
type VersionDetailResponse struct {
	VersionResponse
	ScoreSet  []ScoreSetGroup        `json:"score_set"`
	LogFields []LogFieldResponse     `json:"log_fields"`
	Lessons   []LessonDetailResponse `json:"lessons"`
}
