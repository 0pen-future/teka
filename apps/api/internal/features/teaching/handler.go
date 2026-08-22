package teaching

import (
	"context"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/response"
	"teka/apps/api/internal/shared/validation"
)

// Handler exposes the teaching endpoints.
type Handler struct {
	svc *Service
}

// NewHandler builds the teaching handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// scope resolves the authenticated tenant's center scope — the only
// sanctioned source of tenancy; request bodies and paths never carry it.
func (h *Handler) scope(c *gin.Context) (authctx.Scope, bool) {
	sc, ok := authctx.ScopeFrom(c)
	if !ok {
		response.Err(c, apperror.Unauthorized("authentication required"))
		return authctx.Scope{}, false
	}
	return sc, true
}

// pathID parses one uuid path parameter, reading a malformed value as the
// named resource not existing.
func pathID(c *gin.Context, param, resource string) (uuid.UUID, bool) {
	parsed, err := uuid.Parse(c.Param(param))
	if err != nil {
		response.Err(c, apperror.NotFound(resource))
		return uuid.UUID{}, false
	}
	return parsed, true
}

// pathIndex parses the lesson-index path parameter; a malformed or negative
// value reads as the plan not existing.
func pathIndex(c *gin.Context) (int, bool) {
	parsed, err := strconv.Atoi(c.Param("index"))
	if err != nil || parsed < 0 {
		response.Err(c, apperror.NotFound("lesson plan"))
		return 0, false
	}
	return parsed, true
}

// getCurriculum returns a class's curriculum.
//
//	@Summary		Get class curriculum
//	@Description	Returns the class's giáo trình: ordered lesson titles and the progress pointer. A class that never saved one returns the empty default (lessons: [], current_index: 0), not a 404.
//	@Tags			teaching
//	@Produce		json
//	@Param			id	path		string	true	"class id"
//	@Success		200	{object}	response.Envelope{data=CurriculumResponse}
//	@Failure		401	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404	{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/classes/{id}/curriculum [get]
func (h *Handler) getCurriculum(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	classID, ok := pathID(c, "id", "class")
	if !ok {
		return
	}
	out, err := h.svc.GetCurriculum(c.Request.Context(), sc, classID)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, out)
}

// putCurriculum whole-replaces a class's curriculum.
//
//	@Summary		Replace class curriculum
//	@Description	Whole-list replace of the giáo trình (the editor always saves the entire list). current_index is clamped into the new list's range server-side. Class-teacher only — the owner reads but never edits another teacher's content.
//	@Tags			teaching
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string					true	"class id"
//	@Param			request	body		PutCurriculumRequest	true	"lessons and progress pointer"
//	@Success		200		{object}	response.Envelope{data=CurriculumResponse}
//	@Failure		401		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403		{object}	response.Envelope{error=response.ErrorBody}	"caller is not the class teacher"
//	@Failure		404		{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/classes/{id}/curriculum [put]
func (h *Handler) putCurriculum(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	classID, ok := pathID(c, "id", "class")
	if !ok {
		return
	}
	var req PutCurriculumRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Err(c, validation.BindError(err))
		return
	}
	out, err := h.svc.PutCurriculum(c.Request.Context(), sc, classID, req)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, out)
}

// listPlans returns all saved lesson plans of a class.
//
//	@Summary		List class lesson plans
//	@Description	Every saved giáo án of the class, ordered by lesson index, submitter names resolved. Lessons never saved have no entry — the client maps a missing index to its virtual "none" status.
//	@Tags			teaching
//	@Produce		json
//	@Param			id	path		string	true	"class id"
//	@Success		200	{object}	response.Envelope{data=[]PlanResponse}
//	@Failure		401	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404	{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/classes/{id}/lesson-plans [get]
func (h *Handler) listPlans(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	classID, ok := pathID(c, "id", "class")
	if !ok {
		return
	}
	out, err := h.svc.ListPlans(c.Request.Context(), sc, classID)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, out)
}

// savePlan saves a lesson plan's content.
//
//	@Summary		Save a lesson plan
//	@Description	Full content replace (goal, activities, homework, file metadata); status moves per the review state machine — first save creates the row as draft, saving under redo keeps redo, a plan under or after review has no legal save (409).
//	@Tags			teaching
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string			true	"class id"
//	@Param			index	path		int				true	"lesson index"
//	@Param			request	body		SavePlanRequest	true	"plan content"
//	@Success		200		{object}	response.Envelope{data=PlanResponse}
//	@Failure		401		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403		{object}	response.Envelope{error=response.ErrorBody}	"caller is not the class teacher"
//	@Failure		404		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		409		{object}	response.Envelope{error=response.ErrorBody}	"no legal save from the current status"
//	@Failure		422		{object}	response.Envelope{error=response.ErrorBody}	"lesson index outside the curriculum"
//	@Security		BearerAuth
//	@Router			/classes/{id}/lesson-plans/{index} [put]
func (h *Handler) savePlan(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	classID, ok := pathID(c, "id", "class")
	if !ok {
		return
	}
	index, ok := pathIndex(c)
	if !ok {
		return
	}
	var req SavePlanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Err(c, validation.BindError(err))
		return
	}
	out, err := h.svc.SavePlan(c.Request.Context(), sc, classID, index, req)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, out)
}

// submitPlan submits a lesson plan for review.
//
//	@Summary		Submit a lesson plan
//	@Description	draft/redo → pending: hands the giáo án to the owner, consumes the redo note, and stamps who submitted and when.
//	@Tags			teaching
//	@Produce		json
//	@Param			id		path		string	true	"class id"
//	@Param			index	path		int		true	"lesson index"
//	@Success		200		{object}	response.Envelope{data=PlanResponse}
//	@Failure		401		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403		{object}	response.Envelope{error=response.ErrorBody}	"caller is not the class teacher"
//	@Failure		404		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		409		{object}	response.Envelope{error=response.ErrorBody}	"no legal submit from the current status"
//	@Failure		422		{object}	response.Envelope{error=response.ErrorBody}	"lesson index outside the curriculum"
//	@Security		BearerAuth
//	@Router			/classes/{id}/lesson-plans/{index}/submit [post]
func (h *Handler) submitPlan(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	classID, ok := pathID(c, "id", "class")
	if !ok {
		return
	}
	index, ok := pathIndex(c)
	if !ok {
		return
	}
	out, err := h.svc.SubmitPlan(c.Request.Context(), sc, classID, index)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, out)
}

// approvePlan approves a pending lesson plan (owner only).
//
//	@Summary		Approve a lesson plan
//	@Description	pending → approved. The optional comment whole-replaces owner_comment (empty clears it). Owner only; owners approve their own plans too — no higher tier exists.
//	@Tags			teaching
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string			true	"class id"
//	@Param			index	path		int				true	"lesson index"
//	@Param			request	body		ReviewRequest	true	"optional comment"
//	@Success		200		{object}	response.Envelope{data=PlanResponse}
//	@Failure		401		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403		{object}	response.Envelope{error=response.ErrorBody}	"caller is not the center owner"
//	@Failure		404		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		409		{object}	response.Envelope{error=response.ErrorBody}	"plan is not pending"
//	@Security		BearerAuth
//	@Router			/classes/{id}/lesson-plans/{index}/approve [post]
func (h *Handler) approvePlan(c *gin.Context) {
	h.review(c, h.svc.ApprovePlan)
}

// requestRedo sends a pending lesson plan back for rework (owner only).
//
//	@Summary		Request lesson plan redo
//	@Description	pending → redo. The comment is required (422 otherwise) and becomes the redo note the teacher sees; the plan stays submittable and the note stays visible until resubmission.
//	@Tags			teaching
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string			true	"class id"
//	@Param			index	path		int				true	"lesson index"
//	@Param			request	body		ReviewRequest	true	"required comment"
//	@Success		200		{object}	response.Envelope{data=PlanResponse}
//	@Failure		401		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403		{object}	response.Envelope{error=response.ErrorBody}	"caller is not the center owner"
//	@Failure		404		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		409		{object}	response.Envelope{error=response.ErrorBody}	"plan is not pending"
//	@Failure		422		{object}	response.Envelope{error=response.ErrorBody}	"comment is required"
//	@Security		BearerAuth
//	@Router			/classes/{id}/lesson-plans/{index}/request-redo [post]
func (h *Handler) requestRedo(c *gin.Context) {
	h.review(c, h.svc.RequestRedo)
}

// review is the shared handler body of the two comment-carrying owner
// actions; the service decides whether the comment is optional or required.
func (h *Handler) review(c *gin.Context, action func(ctx context.Context, sc authctx.Scope, classID uuid.UUID, index int, req ReviewRequest) (*PlanResponse, error)) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	classID, ok := pathID(c, "id", "class")
	if !ok {
		return
	}
	index, ok := pathIndex(c)
	if !ok {
		return
	}
	var req ReviewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Err(c, validation.BindError(err))
		return
	}
	out, err := action(c.Request.Context(), sc, classID, index, req)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, out)
}

// reopenPlan pulls a reviewed lesson plan back into review (owner only).
//
//	@Summary		Reopen a lesson plan
//	@Description	approved/redo → pending: re-examine an approved plan, or withdraw the owner's own redo request. Clears both owner notes — the review starts fresh.
//	@Tags			teaching
//	@Produce		json
//	@Param			id		path		string	true	"class id"
//	@Param			index	path		int		true	"lesson index"
//	@Success		200		{object}	response.Envelope{data=PlanResponse}
//	@Failure		401		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403		{object}	response.Envelope{error=response.ErrorBody}	"caller is not the center owner"
//	@Failure		404		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		409		{object}	response.Envelope{error=response.ErrorBody}	"plan is neither approved nor redo"
//	@Security		BearerAuth
//	@Router			/classes/{id}/lesson-plans/{index}/reopen [post]
func (h *Handler) reopenPlan(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	classID, ok := pathID(c, "id", "class")
	if !ok {
		return
	}
	index, ok := pathIndex(c)
	if !ok {
		return
	}
	out, err := h.svc.ReopenPlan(c.Request.Context(), sc, classID, index)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, out)
}

// getMonthMarks returns a class's session notes and marks for one month.
//
//	@Summary		Month batch read of session notes and marks
//	@Description	Every session note (nhận xét buổi) and per-student mark (điểm + ghi chú riêng) of the class's sessions in the requested month, in one response. No session-status filter — the client already holds the month's session list and correlates by session_id.
//	@Tags			teaching
//	@Produce		json
//	@Param			id		path		string	true	"class id"
//	@Param			month	query		string	true	"month, YYYY-MM"
//	@Success		200		{object}	response.Envelope{data=MonthMarksResponse}
//	@Failure		401		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		422		{object}	response.Envelope{error=response.ErrorBody}	"month is not YYYY-MM"
//	@Security		BearerAuth
//	@Router			/classes/{id}/marks [get]
func (h *Handler) getMonthMarks(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	classID, ok := pathID(c, "id", "class")
	if !ok {
		return
	}
	out, err := h.svc.GetMonthMarks(c.Request.Context(), sc, classID, c.Query("month"))
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, out)
}

// putNote upserts a session's whole-class note.
//
//	@Summary		Save a session note
//	@Description	Upserts the session's whole-class nhận xét; an empty (or whitespace-only) body deletes the note instead of storing an empty string. Session-teacher only.
//	@Tags			teaching
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string			true	"session id"
//	@Param			request	body		PutNoteRequest	true	"note body; empty deletes"
//	@Success		200		{object}	response.Envelope{data=NoteResponse}
//	@Failure		401		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403		{object}	response.Envelope{error=response.ErrorBody}	"caller is not the session's teacher"
//	@Failure		404		{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/sessions/{id}/note [put]
func (h *Handler) putNote(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	sessionID, ok := pathID(c, "id", "session")
	if !ok {
		return
	}
	var req PutNoteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Err(c, validation.BindError(err))
		return
	}
	out, err := h.svc.PutNote(c.Request.Context(), sc, sessionID, req)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, out)
}

// putMarks batch-upserts a session's per-student marks.
//
//	@Summary		Save session marks
//	@Description	Batch merge of per-student marks. Per entry, score and personal_note are tri-state: omitted leaves the stored value, null clears it, a value sets it; a row whose resulting fields are both NULL is deleted. A new row requires the student to have been on the session's roster; a student's existing row stays editable and clearable after their enrollment ends. Returns the session's full mark set after the write. Session-teacher only.
//	@Tags			teaching
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string				true	"session id"
//	@Param			request	body		[]MarkEntryRequest	true	"mark entries"
//	@Success		200		{object}	response.Envelope{data=[]MarkResponse}
//	@Failure		401		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403		{object}	response.Envelope{error=response.ErrorBody}	"caller is not the session's teacher"
//	@Failure		404		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		422		{object}	response.Envelope{error=response.ErrorBody}	"duplicate student, score outside 0–10, or student not on the roster"
//	@Security		BearerAuth
//	@Router			/sessions/{id}/marks [put]
func (h *Handler) putMarks(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	sessionID, ok := pathID(c, "id", "session")
	if !ok {
		return
	}
	var req []MarkEntryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Err(c, validation.BindError(err))
		return
	}
	out, err := h.svc.PutMarks(c.Request.Context(), sc, sessionID, req)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, out)
}

// reviewQueue lists the center's pending lesson plans (owner only).
//
//	@Summary		Owner review queue
//	@Description	Every pending giáo án in the center with class name, submitter, lesson title (null-safe against a shrunken curriculum), oldest submission first. Its length is the nav-dot count.
//	@Tags			teaching
//	@Produce		json
//	@Success		200	{object}	response.Envelope{data=[]QueueItemResponse}
//	@Failure		401	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403	{object}	response.Envelope{error=response.ErrorBody}	"caller is not the center owner"
//	@Security		BearerAuth
//	@Router			/teaching/review-queue [get]
func (h *Handler) reviewQueue(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	out, err := h.svc.ReviewQueue(c.Request.Context(), sc)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, out)
}
