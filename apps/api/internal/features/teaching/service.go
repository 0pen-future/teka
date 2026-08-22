package teaching

import (
	"context"
	"errors"
	"fmt"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"teka/apps/api/internal/database"
	"teka/apps/api/internal/features/classes"
	"teka/apps/api/internal/features/enrollments"
	"teka/apps/api/internal/features/sessions"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/id"
)

// ClassSource is the slice of the classes feature teaching needs: resolving
// one class under the caller's scope — which is also this package's
// authorization gate (non-owners cannot resolve another teacher's class).
// *classes.Service satisfies this.
type ClassSource interface {
	Get(ctx context.Context, sc authctx.Scope, classID uuid.UUID) (*classes.Class, error)
}

// SessionSource is the slice of the sessions feature teaching needs:
// resolving one session under the caller's scope — the authorization gate of
// the note/marks writes, exactly as ClassSource gates the class routes.
// *sessions.Service satisfies this.
type SessionSource interface {
	GetByID(ctx context.Context, sc authctx.Scope, sessionID uuid.UUID) (*sessions.Session, error)
}

// RosterSource is the slice of the enrollments feature teaching needs: the
// students enrolled in a class on a given date — the marks batch refuses
// students who were not on the session's roster. *enrollments.Service
// satisfies this (same contract attendance uses).
type RosterSource interface {
	ActiveOn(ctx context.Context, sc authctx.Scope, classID uuid.UUID, on time.Time) ([]enrollments.Enrollment, error)
}

// Service owns the teaching rules: who may touch a class's curriculum and
// plans, the giáo án review-loop state machine — the server-side authority
// the web's button gating merely mirrors — and the session note/marks merge
// semantics.
type Service struct {
	repo     Repository
	classes  ClassSource
	sessions SessionSource
	roster   RosterSource
	tx       database.TxManager
}

// NewService wires the teaching service to its dependencies.
func NewService(repo Repository, classSource ClassSource, sessionSource SessionSource, roster RosterSource, tx database.TxManager) *Service {
	return &Service{repo: repo, classes: classSource, sessions: sessionSource, roster: roster, tx: tx}
}

// GetCurriculum returns the class's giáo trình, or the empty default when
// none was ever saved — 200 either way, so the UI needs no null branch.
func (s *Service) GetCurriculum(ctx context.Context, sc authctx.Scope, classID uuid.UUID) (*CurriculumResponse, error) {
	if _, err := s.resolveClass(ctx, sc, classID); err != nil {
		return nil, err
	}
	cur, err := s.repo.GetCurriculum(ctx, sc, classID)
	if err != nil {
		return nil, apperror.Internal(err)
	}
	if cur == nil {
		return &CurriculumResponse{Lessons: []string{}, CurrentIndex: 0}, nil
	}
	return curriculumResponse(cur), nil
}

// PutCurriculum whole-replaces the class's lesson list (the editor modal
// always saves the entire list) and clamps the progress pointer into the new
// range. Class-teacher only — the owner's reach into another teacher's class
// is read + review actions, never content.
func (s *Service) PutCurriculum(ctx context.Context, sc authctx.Scope, classID uuid.UUID, req PutCurriculumRequest) (*CurriculumResponse, error) {
	class, err := s.resolveClass(ctx, sc, classID)
	if err != nil {
		return nil, err
	}
	if err := requireClassTeacher(class, sc); err != nil {
		return nil, err
	}
	lessons := req.Lessons
	if lessons == nil {
		lessons = []string{}
	}
	// Same clamp as the web store: removing lessons can never leave the
	// pointer past the end, and an empty list pins it to 0.
	currentIndex := min(req.CurrentIndex, len(lessons)-1)
	currentIndex = max(currentIndex, 0)

	cur := &Curriculum{
		ID:           id.New(),
		ClassID:      classID,
		TeacherID:    class.TeacherID,
		CenterID:     class.CenterID,
		Lessons:      lessons,
		CurrentIndex: currentIndex,
	}
	if err := s.repo.UpsertCurriculum(ctx, cur); err != nil {
		return nil, apperror.Internal(err)
	}
	return curriculumResponse(cur), nil
}

// ListPlans returns every saved giáo án of the class, submitter names
// resolved. Missing indexes are simply absent — the web maps them to its
// virtual "none" status.
func (s *Service) ListPlans(ctx context.Context, sc authctx.Scope, classID uuid.UUID) ([]PlanResponse, error) {
	if _, err := s.resolveClass(ctx, sc, classID); err != nil {
		return nil, err
	}
	plans, err := s.repo.ListPlans(ctx, sc, classID)
	if err != nil {
		return nil, apperror.Internal(err)
	}
	return s.planResponses(ctx, plans)
}

// SavePlan is the teacher's content save: full replace of goal, activities,
// homework, and file metadata. Status moves per the state machine (first
// save creates the row as draft; saving under redo keeps redo so the owner's
// note stays visible); a plan under or after review has no legal save — 409.
func (s *Service) SavePlan(ctx context.Context, sc authctx.Scope, classID uuid.UUID, lessonIndex int, req SavePlanRequest) (*PlanResponse, error) {
	class, err := s.resolveClass(ctx, sc, classID)
	if err != nil {
		return nil, err
	}
	if err := requireClassTeacher(class, sc); err != nil {
		return nil, err
	}
	if err := s.validateLessonIndex(ctx, sc, classID, lessonIndex); err != nil {
		return nil, err
	}
	plan, err := s.repo.GetPlan(ctx, sc, classID, lessonIndex)
	if err != nil {
		return nil, apperror.Internal(err)
	}
	next := transition(planStatus(plan), ActionSave)
	if next == "" {
		return nil, transitionConflict(planStatus(plan), ActionSave)
	}

	if plan == nil {
		plan = &Plan{
			ID:          id.New(),
			ClassID:     classID,
			LessonIndex: lessonIndex,
			TeacherID:   class.TeacherID,
			CenterID:    class.CenterID,
		}
	}
	plan.Goal = req.Goal
	plan.Activities = cleanLines(req.Activities)
	plan.Homework = req.Homework
	plan.FileName = req.FileName
	plan.Status = next
	if err := s.writePlan(ctx, plan); err != nil {
		return nil, err
	}
	return s.planResponse(ctx, plan)
}

// SubmitPlan hands a draft (or redone) giáo án to the owner: status becomes
// pending, the redo note is consumed, and the submitter is stamped — the
// review queue shows who actually pressed submit.
func (s *Service) SubmitPlan(ctx context.Context, sc authctx.Scope, classID uuid.UUID, lessonIndex int) (*PlanResponse, error) {
	class, err := s.resolveClass(ctx, sc, classID)
	if err != nil {
		return nil, err
	}
	if err := requireClassTeacher(class, sc); err != nil {
		return nil, err
	}
	if err := s.validateLessonIndex(ctx, sc, classID, lessonIndex); err != nil {
		return nil, err
	}
	return s.applyTransition(ctx, sc, classID, lessonIndex, ActionSubmit, func(plan *Plan) {
		plan.RedoNote = nil
		teacherID := sc.TeacherID
		now := time.Now()
		plan.SubmittedBy = &teacherID
		plan.SubmittedAt = &now
	})
}

// ApprovePlan is the owner accepting a pending giáo án. The comment is
// optional; it whole-replaces owner_comment (empty clears), mirroring the
// review panel's single comment box.
func (s *Service) ApprovePlan(ctx context.Context, sc authctx.Scope, classID uuid.UUID, lessonIndex int, req ReviewRequest) (*PlanResponse, error) {
	if err := s.resolveReviewedClass(ctx, sc, classID); err != nil {
		return nil, err
	}
	return s.applyTransition(ctx, sc, classID, lessonIndex, ActionApprove, func(plan *Plan) {
		plan.OwnerComment = trimmedPtr(req.Comment)
	})
}

// RequestRedo is the owner sending a pending giáo án back. The comment is
// required — a redo without a reason gives the teacher nothing to act on
// (the UI disables the button, the server enforces it).
func (s *Service) RequestRedo(ctx context.Context, sc authctx.Scope, classID uuid.UUID, lessonIndex int, req ReviewRequest) (*PlanResponse, error) {
	if err := s.resolveReviewedClass(ctx, sc, classID); err != nil {
		return nil, err
	}
	comment := trimmedPtr(req.Comment)
	if comment == nil {
		return nil, commentRequired()
	}
	return s.applyTransition(ctx, sc, classID, lessonIndex, ActionRequestRedo, func(plan *Plan) {
		plan.RedoNote = comment
	})
}

// ReopenPlan is the owner pulling a plan back into review: from approved
// (re-examine) or from redo (withdrawing their own request). Both owner
// notes are cleared — the review starts fresh.
func (s *Service) ReopenPlan(ctx context.Context, sc authctx.Scope, classID uuid.UUID, lessonIndex int) (*PlanResponse, error) {
	if err := s.resolveReviewedClass(ctx, sc, classID); err != nil {
		return nil, err
	}
	return s.applyTransition(ctx, sc, classID, lessonIndex, ActionReopen, func(plan *Plan) {
		plan.RedoNote = nil
		plan.OwnerComment = nil
	})
}

// ReviewQueue lists the center's pending giáo án for the owner screen and
// the nav dot (its length), oldest submission first.
func (s *Service) ReviewQueue(ctx context.Context, sc authctx.Scope) ([]QueueItemResponse, error) {
	if !sc.IsOwner {
		return nil, ownerOnly()
	}
	rows, err := s.repo.ReviewQueue(ctx, sc)
	if err != nil {
		return nil, apperror.Internal(err)
	}
	out := make([]QueueItemResponse, len(rows))
	for i, row := range rows {
		out[i] = QueueItemResponse(row)
	}
	return out, nil
}

// GetMonthMarks is the batch read behind the classbook and records screens:
// every session note and mark row of the class's sessions in the requested
// month, in two joined queries. No session-status filter — the web pulls the
// whole month's sessions and decides render-ability itself.
func (s *Service) GetMonthMarks(ctx context.Context, sc authctx.Scope, classID uuid.UUID, month string) (*MonthMarksResponse, error) {
	from, err := time.Parse("2006-01", month)
	if err != nil {
		return nil, apperror.Invalid("validation failed", map[string]string{
			"month": "month must be YYYY-MM",
		})
	}
	to := from.AddDate(0, 1, 0)
	if _, err := s.resolveClass(ctx, sc, classID); err != nil {
		return nil, err
	}
	notes, err := s.repo.ListNotesForClassMonth(ctx, sc, classID, from, to)
	if err != nil {
		return nil, apperror.Internal(err)
	}
	marks, err := s.repo.ListMarksForClassMonth(ctx, sc, classID, from, to)
	if err != nil {
		return nil, apperror.Internal(err)
	}
	out := &MonthMarksResponse{
		SessionNotes: make([]NoteResponse, len(notes)),
		Marks:        make([]MarkResponse, len(marks)),
	}
	for i, note := range notes {
		out.SessionNotes[i] = NoteResponse{SessionID: note.SessionID, Body: note.Body}
	}
	for i, mark := range marks {
		out.Marks[i] = markResponse(mark)
	}
	return out, nil
}

// PutNote upserts the session's whole-class nhận xét; an empty body deletes
// the row — the UI has one text box and empty means "no note", so the table
// never stores empty strings. Session-teacher only.
func (s *Service) PutNote(ctx context.Context, sc authctx.Scope, sessionID uuid.UUID, req PutNoteRequest) (*NoteResponse, error) {
	session, err := s.resolveSession(ctx, sc, sessionID)
	if err != nil {
		return nil, err
	}
	if err := requireSessionTeacher(session, sc); err != nil {
		return nil, err
	}
	if trimmedPtr(req.Body) == nil {
		if err := s.repo.DeleteNote(ctx, sc, sessionID); err != nil {
			return nil, apperror.Internal(err)
		}
		return &NoteResponse{SessionID: sessionID, Body: ""}, nil
	}
	note := &SessionNote{
		SessionID: sessionID,
		TeacherID: session.TeacherID,
		CenterID:  session.CenterID,
		Body:      req.Body,
	}
	if err := s.repo.UpsertNote(ctx, note); err != nil {
		return nil, apperror.Internal(err)
	}
	return &NoteResponse{SessionID: sessionID, Body: note.Body}, nil
}

// PutMarks merges a batch of per-student entries into the session's mark
// rows. Per entry, each field is tri-state (omitted = untouched, null =
// clear, value = set); a row whose resulting fields are both NULL is deleted
// — no separate DELETE endpoint, no empty rows. A NEW row requires the
// student to have been on the session's roster; a student's EXISTING row
// stays correctable and clearable after their enrollment ends, so a wrong
// score never becomes immutable history. Returns the session's full mark set
// after the write so the client can reconcile its cache.
func (s *Service) PutMarks(ctx context.Context, sc authctx.Scope, sessionID uuid.UUID, entries []MarkEntryRequest) ([]MarkResponse, error) {
	session, err := s.resolveSession(ctx, sc, sessionID)
	if err != nil {
		return nil, err
	}
	if err := requireSessionTeacher(session, sc); err != nil {
		return nil, err
	}
	if err := validateMarkEntries(entries); err != nil {
		return nil, err
	}
	existing, err := s.repo.ListMarksBySession(ctx, sc, sessionID)
	if err != nil {
		return nil, apperror.Internal(err)
	}
	byStudent := make(map[uuid.UUID]SessionMark, len(existing))
	for _, mark := range existing {
		byStudent[mark.StudentID] = mark
	}

	roster, err := s.roster.ActiveOn(ctx, sc, session.ClassID, session.SessionDate)
	if err != nil {
		return nil, apperror.Internal(err)
	}
	enrolled := make(map[uuid.UUID]bool, len(roster))
	for _, enrollment := range roster {
		enrolled[enrollment.StudentID] = true
	}
	for _, entry := range entries {
		if _, hasRow := byStudent[entry.StudentID]; !hasRow && !enrolled[entry.StudentID] {
			return nil, apperror.Invalid("validation failed", map[string]string{
				"marks": fmt.Sprintf("student %s was not on the session's roster", entry.StudentID),
			})
		}
	}

	var upserts []SessionMark
	var deletes []uuid.UUID
	for _, entry := range entries {
		mark, existed := byStudent[entry.StudentID]
		if !existed {
			mark = SessionMark{
				ID:        id.New(),
				SessionID: sessionID,
				StudentID: entry.StudentID,
				TeacherID: session.TeacherID,
				CenterID:  session.CenterID,
			}
		}
		if entry.Score.Set {
			mark.Score = entry.Score.Value
		}
		if entry.PersonalNote.Set {
			mark.PersonalNote = nil
			if entry.PersonalNote.Value != nil {
				mark.PersonalNote = trimmedPtr(*entry.PersonalNote.Value)
			}
		}
		if mark.Score == nil && mark.PersonalNote == nil {
			if existed {
				deletes = append(deletes, entry.StudentID)
			}
			continue
		}
		upserts = append(upserts, mark)
	}

	// One transaction so the upsert and the empty-row cleanup land (or fail)
	// together — safe under the web's debounce retries.
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.repo.UpsertMarks(ctx, upserts); err != nil {
			return err
		}
		return s.repo.DeleteMarks(ctx, sc, sessionID, deletes)
	})
	if err != nil {
		return nil, apperror.Internal(err)
	}

	current, err := s.repo.ListMarksBySession(ctx, sc, sessionID)
	if err != nil {
		return nil, apperror.Internal(err)
	}
	out := make([]MarkResponse, len(current))
	for i, mark := range current {
		out[i] = markResponse(mark)
	}
	return out, nil
}

// validateMarkEntries bounds the batch shape: at most one full roster's worth
// of entries, no duplicate students (two entries would race each other inside
// one statement), scores on the UI's 0–10 scale, and personal notes within the
// same size bound the plain-string note columns get from their binding tags
// (Optional's custom unmarshalling bypasses gin's validator).
func validateMarkEntries(entries []MarkEntryRequest) error {
	if len(entries) > 100 {
		return apperror.Invalid("validation failed", map[string]string{
			"marks": fmt.Sprintf("batch of %d entries exceeds the 100-entry limit", len(entries)),
		})
	}
	seen := make(map[uuid.UUID]bool, len(entries))
	for _, entry := range entries {
		if seen[entry.StudentID] {
			return apperror.Invalid("validation failed", map[string]string{
				"marks": fmt.Sprintf("student %s appears more than once", entry.StudentID),
			})
		}
		seen[entry.StudentID] = true
		if entry.Score.Set && entry.Score.Value != nil {
			if score := *entry.Score.Value; score < 0 || score > 10 {
				return apperror.Invalid("validation failed", map[string]string{
					"marks": fmt.Sprintf("score %.1f is outside the 0–10 scale", score),
				})
			}
		}
		if entry.PersonalNote.Set && entry.PersonalNote.Value != nil {
			if utf8.RuneCountInString(*entry.PersonalNote.Value) > 1000 {
				return apperror.Invalid("validation failed", map[string]string{
					"marks": fmt.Sprintf("personal note for student %s exceeds 1000 characters", entry.StudentID),
				})
			}
		}
	}
	return nil
}

func markResponse(mark SessionMark) MarkResponse {
	return MarkResponse{
		SessionID:    mark.SessionID,
		StudentID:    mark.StudentID,
		Score:        mark.Score,
		PersonalNote: mark.PersonalNote,
	}
}

// resolveSession is resolveClass's session counterpart: normalise the
// SessionSource error shape into this package's 404 contract.
func (s *Service) resolveSession(ctx context.Context, sc authctx.Scope, sessionID uuid.UUID) (*sessions.Session, error) {
	session, err := s.sessions.GetByID(ctx, sc, sessionID)
	if err != nil {
		var appErr *apperror.AppError
		if errors.As(err, &appErr) {
			return nil, appErr
		}
		if errors.Is(err, sessions.ErrNotFound) {
			return nil, sessionNotFound()
		}
		return nil, apperror.Internal(err)
	}
	return session, nil
}

// applyTransition runs one state-machine action on an existing plan: load,
// check legality (a missing row is StatusNone, so actions on it fall out as
// 409 through the same table), mutate, write back.
func (s *Service) applyTransition(ctx context.Context, sc authctx.Scope, classID uuid.UUID, lessonIndex int, action string, mutate func(*Plan)) (*PlanResponse, error) {
	plan, err := s.repo.GetPlan(ctx, sc, classID, lessonIndex)
	if err != nil {
		return nil, apperror.Internal(err)
	}
	status := planStatus(plan)
	next := transition(status, action)
	if next == "" {
		return nil, transitionConflict(status, action)
	}
	plan.Status = next
	mutate(plan)
	if err := s.repo.UpdatePlan(ctx, plan); err != nil {
		return nil, apperror.Internal(err)
	}
	return s.planResponse(ctx, plan)
}

// resolveReviewedClass gates the three owner review actions: owner-only
// first (a member gets 403 regardless of the class), then class resolution
// under the owner's center-wide scope.
func (s *Service) resolveReviewedClass(ctx context.Context, sc authctx.Scope, classID uuid.UUID) error {
	if !sc.IsOwner {
		return ownerOnly()
	}
	_, err := s.resolveClass(ctx, sc, classID)
	return err
}

// validateLessonIndex bounds plan writes to the curriculum the class
// actually has — a plan for lesson 7 of a 6-lesson curriculum is a stale
// client, not a new row.
func (s *Service) validateLessonIndex(ctx context.Context, sc authctx.Scope, classID uuid.UUID, lessonIndex int) error {
	cur, err := s.repo.GetCurriculum(ctx, sc, classID)
	if err != nil {
		return apperror.Internal(err)
	}
	length := 0
	if cur != nil {
		length = len(cur.Lessons)
	}
	if lessonIndex >= length {
		return indexOutOfRange(lessonIndex, length)
	}
	return nil
}

// writePlan routes a first save to insert and a re-save to a whole-row
// update (the loaded row is the source, so cleared nullable columns persist).
func (s *Service) writePlan(ctx context.Context, plan *Plan) error {
	if plan.CreatedAt.IsZero() {
		if err := s.repo.CreatePlan(ctx, plan); err != nil {
			// Losing a concurrent first-save race means the plan now exists
			// with someone else's content — the same stale-view situation as
			// an illegal transition, so the client's 409 reload path applies.
			if errors.Is(err, gorm.ErrDuplicatedKey) {
				return apperror.Conflict("lesson plan was saved concurrently — reload and retry")
			}
			return apperror.Internal(err)
		}
		return nil
	}
	if err := s.repo.UpdatePlan(ctx, plan); err != nil {
		return apperror.Internal(err)
	}
	return nil
}

// resolveClass fetches the class through the ClassSource contract and
// normalises whatever error shape it returns (a pre-translated *AppError
// from the real classes.Service, or a raw classes.ErrNotFound from a test
// fake) into this package's 404 contract.
func (s *Service) resolveClass(ctx context.Context, sc authctx.Scope, classID uuid.UUID) (*classes.Class, error) {
	class, err := s.classes.Get(ctx, sc, classID)
	if err != nil {
		var appErr *apperror.AppError
		if errors.As(err, &appErr) {
			return nil, appErr
		}
		if errors.Is(err, classes.ErrNotFound) {
			return nil, classNotFound()
		}
		return nil, apperror.Internal(err)
	}
	return class, nil
}

// planResponse resolves one plan's submitter name and maps it to the wire
// shape.
func (s *Service) planResponse(ctx context.Context, plan *Plan) (*PlanResponse, error) {
	responses, err := s.planResponses(ctx, []Plan{*plan})
	if err != nil {
		return nil, err
	}
	return &responses[0], nil
}

// planResponses maps plans to the wire shape with one batched submitter-name
// lookup.
func (s *Service) planResponses(ctx context.Context, plans []Plan) ([]PlanResponse, error) {
	idSet := make(map[uuid.UUID]bool, len(plans))
	var ids []uuid.UUID
	for _, plan := range plans {
		if plan.SubmittedBy != nil && !idSet[*plan.SubmittedBy] {
			idSet[*plan.SubmittedBy] = true
			ids = append(ids, *plan.SubmittedBy)
		}
	}
	names, err := s.repo.TeacherNames(ctx, ids)
	if err != nil {
		return nil, apperror.Internal(err)
	}
	out := make([]PlanResponse, len(plans))
	for i, plan := range plans {
		resp := PlanResponse{
			ClassID:      plan.ClassID,
			LessonIndex:  plan.LessonIndex,
			Goal:         plan.Goal,
			Activities:   plan.Activities,
			Homework:     plan.Homework,
			FileName:     plan.FileName,
			Status:       plan.Status,
			RedoNote:     plan.RedoNote,
			OwnerComment: plan.OwnerComment,
			SubmittedBy:  plan.SubmittedBy,
			SubmittedAt:  plan.SubmittedAt,
		}
		if resp.Activities == nil {
			resp.Activities = []string{}
		}
		if plan.SubmittedBy != nil {
			if name, ok := names[*plan.SubmittedBy]; ok {
				resp.SubmittedByName = &name
			}
		}
		out[i] = resp
	}
	return out, nil
}

// planStatus reads a loaded row's status, a missing row being the virtual
// StatusNone the state machine starts from.
func planStatus(plan *Plan) string {
	if plan == nil {
		return StatusNone
	}
	return plan.Status
}

func curriculumResponse(cur *Curriculum) *CurriculumResponse {
	lessons := []string(cur.Lessons)
	if lessons == nil {
		lessons = []string{}
	}
	return &CurriculumResponse{Lessons: lessons, CurrentIndex: cur.CurrentIndex}
}

func classNotFound() error {
	appErr := apperror.NotFound("class")
	appErr.Err = ErrClassNotFound
	return appErr
}

func transitionConflict(status, action string) error {
	appErr := apperror.Conflict(fmt.Sprintf("lesson plan cannot %s from status %q", action, status))
	appErr.Err = ErrIllegalTransition
	return appErr
}

func notClassTeacher() error {
	appErr := apperror.Forbidden("only the class teacher can edit teaching content")
	appErr.Err = ErrNotClassTeacher
	return appErr
}

func ownerOnly() error {
	appErr := apperror.Forbidden("only the center owner can review lesson plans")
	appErr.Err = ErrOwnerOnly
	return appErr
}

func commentRequired() error {
	return apperror.Invalid("validation failed", map[string]string{
		"comment": "a redo request needs a comment the teacher can act on",
	})
}

func indexOutOfRange(index, length int) error {
	return apperror.Invalid("validation failed", map[string]string{
		"lesson_index": fmt.Sprintf("index %d is outside the curriculum's %d lessons", index, length),
	})
}

// requireClassTeacher bounds content writes (curriculum, plan save/submit)
// to the class's own teacher. The owner deliberately included: their reach
// into another teacher's class is read + the three review actions.
func requireClassTeacher(class *classes.Class, sc authctx.Scope) error {
	if class.TeacherID != sc.TeacherID {
		return notClassTeacher()
	}
	return nil
}

func sessionNotFound() error {
	appErr := apperror.NotFound("session")
	appErr.Err = ErrSessionNotFound
	return appErr
}

func notSessionTeacher() error {
	appErr := apperror.Forbidden("only the session's teacher can record notes and marks")
	appErr.Err = ErrNotSessionTeacher
	return appErr
}

// requireSessionTeacher bounds note/marks writes to the session's own
// teacher — the owner reads center-wide but never records another teacher's
// scores or notes.
func requireSessionTeacher(session *sessions.Session, sc authctx.Scope) error {
	if session.TeacherID != sc.TeacherID {
		return notSessionTeacher()
	}
	return nil
}
