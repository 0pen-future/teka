package grading

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/response"
	"teka/apps/api/internal/shared/validation"
)

// Handler exposes the grading endpoints.
type Handler struct {
	svc *Service
}

// NewHandler builds the grading handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// scope resolves the authenticated tenant's center scope — the only sanctioned
// source of tenancy; request bodies and paths never carry it.
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

// listSets lists the center's score sets (owner only).
//
//	@Summary		List score sets
//	@Description	Every live score set (bộ điểm) in the center with its component names in position order. Owner only — this is a center-configuration surface.
//	@Tags			grading
//	@Produce		json
//	@Success		200	{object}	response.Envelope{data=[]ScoreSetResponse}
//	@Failure		401	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403	{object}	response.Envelope{error=response.ErrorBody}	"caller is not the center owner"
//	@Security		BearerAuth
//	@Router			/score-sets [get]
func (h *Handler) listSets(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	out, err := h.svc.ListSets(c.Request.Context(), sc)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, out)
}

// createSet creates a score set (owner only).
//
//	@Summary		Create a score set
//	@Description	Creates a named set with its ordered component names (1..10, unique case-insensitively within the set). Owner only. A duplicate live name in the center is a 409.
//	@Tags			grading
//	@Accept			json
//	@Produce		json
//	@Param			request	body		ScoreSetRequest	true	"set name and components"
//	@Success		201		{object}	response.Envelope{data=ScoreSetResponse}
//	@Failure		401		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403		{object}	response.Envelope{error=response.ErrorBody}	"caller is not the center owner"
//	@Failure		409		{object}	response.Envelope{error=response.ErrorBody}	"a set with this name already exists"
//	@Failure		422		{object}	response.Envelope{error=response.ErrorBody}	"invalid or duplicate component names"
//	@Security		BearerAuth
//	@Router			/score-sets [post]
func (h *Handler) createSet(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	var req ScoreSetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Err(c, validation.BindError(err))
		return
	}
	out, err := h.svc.CreateSet(c.Request.Context(), sc, req)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusCreated, out)
}

// updateSet renames a set and replaces its components (owner only).
//
//	@Summary		Update a score set
//	@Description	Renames the set and whole-replaces its component list. Owner only. Per-class snapshots taken from this set earlier are untouched — that is the point of the snapshot design.
//	@Tags			grading
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string			true	"score set id"
//	@Param			request	body		ScoreSetRequest	true	"set name and components"
//	@Success		200		{object}	response.Envelope{data=ScoreSetResponse}
//	@Failure		401		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403		{object}	response.Envelope{error=response.ErrorBody}	"caller is not the center owner"
//	@Failure		404		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		409		{object}	response.Envelope{error=response.ErrorBody}	"a set with this name already exists"
//	@Failure		422		{object}	response.Envelope{error=response.ErrorBody}	"invalid or duplicate component names"
//	@Security		BearerAuth
//	@Router			/score-sets/{id} [put]
func (h *Handler) updateSet(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	setID, ok := pathID(c, "id", "score set")
	if !ok {
		return
	}
	var req ScoreSetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Err(c, validation.BindError(err))
		return
	}
	out, err := h.svc.UpdateSet(c.Request.Context(), sc, setID, req)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, out)
}

// deleteSet soft-deletes a score set (owner only).
//
//	@Summary		Delete a score set
//	@Description	Soft-deletes the set. Owner only. Classes already using a snapshot of it keep their components.
//	@Tags			grading
//	@Produce		json
//	@Param			id	path	string	true	"score set id"
//	@Success		204	"deleted"
//	@Failure		401	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403	{object}	response.Envelope{error=response.ErrorBody}	"caller is not the center owner"
//	@Failure		404	{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/score-sets/{id} [delete]
func (h *Handler) deleteSet(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	setID, ok := pathID(c, "id", "score set")
	if !ok {
		return
	}
	if err := h.svc.DeleteSet(c.Request.Context(), sc, setID); err != nil {
		response.Err(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// assignScoreSet snapshots a set onto a class (owner only).
//
//	@Summary		Assign a score set to a class
//	@Description	Snapshots the set's components into the class (class_score_components), replacing whatever it had. Owner only. A class that already carries any score refuses with 409 — replacing the components would cascade-delete recorded grades.
//	@Tags			grading
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string					true	"class id"
//	@Param			request	body		AssignScoreSetRequest	true	"set to assign"
//	@Success		200		{object}	response.Envelope{data=ClassComponentsResponse}
//	@Failure		401		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403		{object}	response.Envelope{error=response.ErrorBody}	"caller is not the center owner"
//	@Failure		404		{object}	response.Envelope{error=response.ErrorBody}	"class or set not found"
//	@Failure		409		{object}	response.Envelope{error=response.ErrorBody}	"class already has recorded scores"
//	@Security		BearerAuth
//	@Router			/classes/{id}/score-set [post]
func (h *Handler) assignScoreSet(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	classID, ok := pathID(c, "id", "class")
	if !ok {
		return
	}
	var req AssignScoreSetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Err(c, validation.BindError(err))
		return
	}
	out, err := h.svc.AssignScoreSet(c.Request.Context(), sc, classID, req.SetID)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, out)
}

// clearScoreSet removes a class's snapshot (owner only).
//
//	@Summary		Clear a class's score set
//	@Description	Removes the class's snapshot components — the fix for a wrong assignment. Owner only, and refused with 409 if the class already has recorded scores.
//	@Tags			grading
//	@Produce		json
//	@Param			id	path	string	true	"class id"
//	@Success		204	"cleared"
//	@Failure		401	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403	{object}	response.Envelope{error=response.ErrorBody}	"caller is not the center owner"
//	@Failure		404	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		409	{object}	response.Envelope{error=response.ErrorBody}	"class already has recorded scores"
//	@Security		BearerAuth
//	@Router			/classes/{id}/score-set [delete]
func (h *Handler) clearScoreSet(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	classID, ok := pathID(c, "id", "class")
	if !ok {
		return
	}
	if err := h.svc.ClearScoreSet(c.Request.Context(), sc, classID); err != nil {
		response.Err(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// getClassComponents returns a class's snapshot components.
//
//	@Summary		Get a class's score components
//	@Description	The class's snapshot components (score grid columns), position order. An empty list means the class uses the plain general-score UI. Readable by the class's teacher and any center-wide reader.
//	@Tags			grading
//	@Produce		json
//	@Param			id	path		string	true	"class id"
//	@Success		200	{object}	response.Envelope{data=ClassComponentsResponse}
//	@Failure		401	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404	{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/classes/{id}/score-components [get]
func (h *Handler) getClassComponents(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	classID, ok := pathID(c, "id", "class")
	if !ok {
		return
	}
	out, err := h.svc.GetClassComponents(c.Request.Context(), sc, classID)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, out)
}

// getSessionScores returns a session's component columns and recorded cells.
//
//	@Summary		Get session component scores
//	@Description	The class's component columns plus every recorded cell for the session — one round-trip the score grid rebuilds from. Read gate is session resolution (the session's teacher and any center-wide reader).
//	@Tags			grading
//	@Produce		json
//	@Param			id	path		string	true	"session id"
//	@Success		200	{object}	response.Envelope{data=SessionScoresResponse}
//	@Failure		401	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404	{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/sessions/{id}/scores [get]
func (h *Handler) getSessionScores(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	sessionID, ok := pathID(c, "id", "session")
	if !ok {
		return
	}
	out, err := h.svc.GetSessionScores(c.Request.Context(), sc, sessionID)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, out)
}

// putSessionScores batch-upserts a session's component scores.
//
//	@Summary		Save session component scores
//	@Description	Batch merge of per-cell scores. Per entry a value upserts the cell and null deletes it; the table never holds empty cells. A new cell requires the student to have been on the session's roster; an already-scored student stays editable after their enrollment ends. Returns the session's full score set after the write. Writable by the session's teacher OR the center owner (deliberate divergence from marks, which are teacher-only).
//	@Tags			grading
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string				true	"session id"
//	@Param			request	body		[]ScoreEntryRequest	true	"score entries"
//	@Success		200		{object}	response.Envelope{data=[]ScoreResponse}
//	@Failure		401		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403		{object}	response.Envelope{error=response.ErrorBody}	"caller is neither the session's teacher nor the owner"
//	@Failure		404		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		422		{object}	response.Envelope{error=response.ErrorBody}	"duplicate cell, score outside 0–10, component not in the class, or student not on the roster"
//	@Security		BearerAuth
//	@Router			/sessions/{id}/scores [put]
func (h *Handler) putSessionScores(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	sessionID, ok := pathID(c, "id", "session")
	if !ok {
		return
	}
	var req []ScoreEntryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Err(c, validation.BindError(err))
		return
	}
	out, err := h.svc.PutSessionScores(c.Request.Context(), sc, sessionID, req)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, out)
}
