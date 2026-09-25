package library

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/google/uuid"

	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/pagination"
	"teka/apps/api/internal/shared/response"
	"teka/apps/api/internal/shared/validation"
)

// listSorts whitelists the public sort keys for GET /library/templates.
var listSorts = map[string]string{
	"name":       "program_templates.name",
	"code":       "program_templates.code",
	"created_at": "program_templates.created_at",
}

// Handler exposes the program-template library endpoints.
type Handler struct {
	svc *Service
}

// NewHandler builds the library handler.
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

// listTemplates pages the center's program templates.
//
//	@Summary		List program templates
//	@Description	Live templates of the caller's center with their published and draft version numbers. q matches code or name.
//	@Tags			library
//	@Produce		json
//	@Param			q			query		string	false	"code or name fragment"
//	@Param			has_draft	query		bool	false	"only templates with an open draft"
//	@Param			page		query		int		false	"page (default 1)"
//	@Param			per_page	query		int		false	"page size (default 20, max 100)"
//	@Param			sort		query		string	false	"name | code | created_at, prefix - for descending"
//	@Success		200			{object}	response.Envelope{data=[]TemplateResponse,meta=response.Meta}
//	@Failure		401			{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403			{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/library/templates [get]
func (h *Handler) listTemplates(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	params := pagination.Parse(c, "name", listSorts)
	filter := ListFilter{Q: strings.TrimSpace(c.Query("q")), HasDraft: c.Query("has_draft") == "true"}
	rows, total, err := h.svc.ListTemplates(c.Request.Context(), sc, filter, params)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.List(c, rows, params.Meta(total))
}

// createTemplate adds a template and opens its first draft.
//
//	@Summary		Create a program template
//	@Description	Creates the template and draft version 1 in one step. The code is upper-cased and must be unique among the center's live templates (409 CODE_TAKEN).
//	@Tags			library
//	@Accept			json
//	@Produce		json
//	@Param			body	body		TemplateRequest	true	"template"
//	@Success		201		{object}	response.Envelope{data=TemplateResponse}
//	@Failure		401		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		409		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		422		{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/library/templates [post]
func (h *Handler) createTemplate(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	var req TemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Err(c, validation.BindError(err))
		return
	}
	out, err := h.svc.CreateTemplate(c.Request.Context(), sc, req)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusCreated, out)
}

// getTemplate returns one template.
//
//	@Summary		Get a program template
//	@Tags			library
//	@Produce		json
//	@Param			id	path		string	true	"template id"
//	@Success		200	{object}	response.Envelope{data=TemplateResponse}
//	@Failure		401	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404	{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/library/templates/{id} [get]
func (h *Handler) getTemplate(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	id, ok := pathID(c, "id", "program template")
	if !ok {
		return
	}
	out, err := h.svc.GetTemplate(c.Request.Context(), sc, id)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, out)
}

// updateTemplate replaces the template's own fields.
//
//	@Summary		Update a program template
//	@Description	Replaces code, name, subject, level and description. Versions and lessons are untouched. 409 CODE_TAKEN when the new code belongs to another live template.
//	@Tags			library
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string			true	"template id"
//	@Param			body	body		TemplateRequest	true	"template"
//	@Success		200		{object}	response.Envelope{data=TemplateResponse}
//	@Failure		401		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		409		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		422		{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/library/templates/{id} [put]
func (h *Handler) updateTemplate(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	id, ok := pathID(c, "id", "program template")
	if !ok {
		return
	}
	var req TemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Err(c, validation.BindError(err))
		return
	}
	out, err := h.svc.UpdateTemplate(c.Request.Context(), sc, id, req)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, out)
}

// deleteTemplate soft-deletes a template.
//
//	@Summary		Delete a program template
//	@Description	Soft delete: the template leaves the library and its code becomes reusable; versions and lessons are kept.
//	@Tags			library
//	@Produce		json
//	@Param			id	path		string	true	"template id"
//	@Success		200	{object}	response.Envelope{data=object}
//	@Failure		401	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404	{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/library/templates/{id} [delete]
func (h *Handler) deleteTemplate(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	id, ok := pathID(c, "id", "program template")
	if !ok {
		return
	}
	if err := h.svc.DeleteTemplate(c.Request.Context(), sc, id); err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, gin.H{"deleted": true})
}

// listVersions returns a template's versions, newest first.
//
//	@Summary		List template versions
//	@Tags			library
//	@Produce		json
//	@Param			id	path		string	true	"template id"
//	@Success		200	{object}	response.Envelope{data=[]VersionResponse}
//	@Failure		401	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404	{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/library/templates/{id}/versions [get]
func (h *Handler) listVersions(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	id, ok := pathID(c, "id", "program template")
	if !ok {
		return
	}
	out, err := h.svc.ListVersions(c.Request.Context(), sc, id)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, out)
}

// createVersion opens a new draft version.
//
//	@Summary		Open a new draft version
//	@Description	Numbers the draft after the highest existing version and copies the lessons of the latest published version into it. 409 DRAFT_EXISTS while another draft is open.
//	@Tags			library
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string					true	"template id"
//	@Param			body	body		CreateVersionRequest	false	"optional changelog"
//	@Success		201		{object}	response.Envelope{data=VersionResponse}
//	@Failure		401		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		409		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		422		{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/library/templates/{id}/versions [post]
func (h *Handler) createVersion(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	id, ok := pathID(c, "id", "program template")
	if !ok {
		return
	}
	var req CreateVersionRequest
	if c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			response.Err(c, validation.BindError(err))
			return
		}
	}
	out, err := h.svc.CreateVersion(c.Request.Context(), sc, id, req)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusCreated, out)
}

// publishVersion locks a draft as the published version.
//
//	@Summary		Publish a draft version
//	@Description	draft → published; its lessons become read-only. 409 VERSION_NOT_DRAFT for any other status. Older published versions are left as they are.
//	@Tags			library
//	@Produce		json
//	@Param			vid	path		string	true	"version id"
//	@Success		200	{object}	response.Envelope{data=VersionResponse}
//	@Failure		401	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		409	{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/library/versions/{vid}/publish [post]
func (h *Handler) publishVersion(c *gin.Context) {
	h.versionAction(c, h.svc.Publish)
}

// archiveVersion retires a published version.
//
//	@Summary		Archive a published version
//	@Description	published → archived; lessons are kept but the version no longer counts as the template's published one. 409 VERSION_NOT_PUBLISHED for any other status.
//	@Tags			library
//	@Produce		json
//	@Param			vid	path		string	true	"version id"
//	@Success		200	{object}	response.Envelope{data=VersionResponse}
//	@Failure		401	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		409	{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/library/versions/{vid}/archive [post]
func (h *Handler) archiveVersion(c *gin.Context) {
	h.versionAction(c, h.svc.Archive)
}

func (h *Handler) versionAction(c *gin.Context, act func(ctx context.Context, sc authctx.Scope, id uuid.UUID) (*VersionResponse, error)) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	vid, ok := pathID(c, "vid", "template version")
	if !ok {
		return
	}
	out, err := act(c.Request.Context(), sc, vid)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, out)
}

// listLessons returns a version's lessons in order.
//
//	@Summary		List lessons of a version
//	@Tags			library
//	@Produce		json
//	@Param			vid	path		string	true	"version id"
//	@Success		200	{object}	response.Envelope{data=[]LessonResponse}
//	@Failure		401	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404	{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/library/versions/{vid}/lessons [get]
func (h *Handler) listLessons(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	vid, ok := pathID(c, "vid", "template version")
	if !ok {
		return
	}
	out, err := h.svc.ListLessons(c.Request.Context(), sc, vid)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, out)
}

// createLesson appends a lesson to a draft version.
//
//	@Summary		Add a lesson to a draft
//	@Description	Appends the lesson at the next position. 409 VERSION_LOCKED when the version is published or archived.
//	@Tags			library
//	@Accept			json
//	@Produce		json
//	@Param			vid		path		string			true	"version id"
//	@Param			body	body		LessonRequest	true	"lesson"
//	@Success		201		{object}	response.Envelope{data=LessonResponse}
//	@Failure		401		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		409		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		422		{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/library/versions/{vid}/lessons [post]
func (h *Handler) createLesson(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	vid, ok := pathID(c, "vid", "template version")
	if !ok {
		return
	}
	var req LessonRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Err(c, validation.BindError(err))
		return
	}
	out, err := h.svc.CreateLesson(c.Request.Context(), sc, vid, req)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusCreated, out)
}

// reorderLessons applies a complete new lesson order to a draft.
//
//	@Summary		Reorder the lessons of a draft
//	@Description	lesson_ids must list every lesson of the version exactly once, in the wanted order (422 otherwise). 409 VERSION_LOCKED when the version is not a draft.
//	@Tags			library
//	@Accept			json
//	@Produce		json
//	@Param			vid		path		string			true	"version id"
//	@Param			body	body		ReorderRequest	true	"ordered lesson ids"
//	@Success		200		{object}	response.Envelope{data=[]LessonResponse}
//	@Failure		401		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		409		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		422		{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/library/versions/{vid}/lessons/order [put]
func (h *Handler) reorderLessons(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	vid, ok := pathID(c, "vid", "template version")
	if !ok {
		return
	}
	var req ReorderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Err(c, validation.BindError(err))
		return
	}
	out, err := h.svc.ReorderLessons(c.Request.Context(), sc, vid, req)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, out)
}

// getLesson returns one lesson.
//
//	@Summary		Get a template lesson
//	@Tags			library
//	@Produce		json
//	@Param			lid	path		string	true	"lesson id"
//	@Success		200	{object}	response.Envelope{data=LessonDetailResponse}
//	@Failure		401	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404	{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/library/lessons/{lid} [get]
func (h *Handler) getLesson(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	lid, ok := pathID(c, "lid", "template lesson")
	if !ok {
		return
	}
	out, err := h.svc.GetLesson(c.Request.Context(), sc, lid)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, out)
}

// updateLesson replaces a lesson's content.
//
//	@Summary		Update a template lesson
//	@Description	Replaces title, objectives, duration and homework note. 409 VERSION_LOCKED when the lesson's version is not a draft.
//	@Tags			library
//	@Accept			json
//	@Produce		json
//	@Param			lid		path		string			true	"lesson id"
//	@Param			body	body		LessonRequest	true	"lesson"
//	@Success		200		{object}	response.Envelope{data=LessonDetailResponse}
//	@Failure		401		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		409		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		422		{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/library/lessons/{lid} [put]
func (h *Handler) updateLesson(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	lid, ok := pathID(c, "lid", "template lesson")
	if !ok {
		return
	}
	var req LessonRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Err(c, validation.BindError(err))
		return
	}
	out, err := h.svc.UpdateLesson(c.Request.Context(), sc, lid, req)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, out)
}

// deleteLesson removes a lesson from a draft and closes the position gap.
//
//	@Summary		Delete a template lesson
//	@Description	Removes the lesson and renumbers the rest. 409 VERSION_LOCKED when the lesson's version is not a draft.
//	@Tags			library
//	@Produce		json
//	@Param			lid	path		string	true	"lesson id"
//	@Success		200	{object}	response.Envelope{data=object}
//	@Failure		401	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		409	{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/library/lessons/{lid} [delete]
func (h *Handler) deleteLesson(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	lid, ok := pathID(c, "lid", "template lesson")
	if !ok {
		return
	}
	if err := h.svc.DeleteLesson(c.Request.Context(), sc, lid); err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, gin.H{"deleted": true})
}

// duplicateLesson inserts a copy of a lesson right after it.
//
//	@Summary		Duplicate a template lesson
//	@Description	Inserts a copy right after the lesson, shifting later ones up by one position. Content (mode, unit, objectives, duration, homework note, materials, exercises) is copied. 409 VERSION_LOCKED when the lesson's version is not a draft.
//	@Tags			library
//	@Produce		json
//	@Param			lid	path		string	true	"lesson id"
//	@Success		201	{object}	response.Envelope{data=DuplicateLessonResponse}
//	@Failure		401	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		409	{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/library/lessons/{lid}/duplicate [post]
func (h *Handler) duplicateLesson(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	lid, ok := pathID(c, "lid", "template lesson")
	if !ok {
		return
	}
	out, err := h.svc.DuplicateLesson(c.Request.Context(), sc, lid)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusCreated, out)
}

// clearLessons deletes every lesson of a draft version.
//
//	@Summary		Delete every lesson of a draft
//	@Description	Removes all lessons of the version in one call; their attachments cascade with them. 409 VERSION_LOCKED when the version is not a draft.
//	@Tags			library
//	@Produce		json
//	@Param			vid	path		string	true	"version id"
//	@Success		200	{object}	response.Envelope{data=object}
//	@Failure		401	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		409	{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/library/versions/{vid}/lessons [delete]
func (h *Handler) clearLessons(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	vid, ok := pathID(c, "vid", "template version")
	if !ok {
		return
	}
	if err := h.svc.ClearLessons(c.Request.Context(), sc, vid); err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, gin.H{"cleared": true})
}

// listExerciseGroups lists a version's exercise groups.
//
//	@Summary		List the exercise groups of a version
//	@Description	Groups by position, each with how many lesson-exercise links point at it.
//	@Tags			library
//	@Produce		json
//	@Param			vid	path		string	true	"version id"
//	@Success		200	{object}	response.Envelope{data=[]ExerciseGroupResponse}
//	@Failure		401	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404	{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/library/versions/{vid}/exercise-groups [get]
func (h *Handler) listExerciseGroups(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	vid, ok := pathID(c, "vid", "template version")
	if !ok {
		return
	}
	out, err := h.svc.ListExerciseGroups(c.Request.Context(), sc, vid)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, out)
}

// createExerciseGroup adds an exercise group to a draft version.
//
//	@Summary		Add an exercise group to a draft
//	@Description	Appends the group at the next position. 409 VERSION_LOCKED when the version is not a draft.
//	@Tags			library
//	@Accept			json
//	@Produce		json
//	@Param			vid		path		string					true	"version id"
//	@Param			body	body		ExerciseGroupRequest	true	"exercise group"
//	@Success		201		{object}	response.Envelope{data=ExerciseGroupResponse}
//	@Failure		401		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		409		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		422		{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/library/versions/{vid}/exercise-groups [post]
func (h *Handler) createExerciseGroup(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	vid, ok := pathID(c, "vid", "template version")
	if !ok {
		return
	}
	var req ExerciseGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Err(c, validation.BindError(err))
		return
	}
	out, err := h.svc.CreateExerciseGroup(c.Request.Context(), sc, vid, req)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusCreated, out)
}

// deleteExerciseGroup removes an exercise group from a draft version.
//
//	@Summary		Delete an exercise group from a draft
//	@Description	The lesson-exercise links that pointed at it lose their group_id instead of being deleted. 409 VERSION_LOCKED when the version is not a draft.
//	@Tags			library
//	@Produce		json
//	@Param			vid	path		string	true	"version id"
//	@Param			gid	path		string	true	"exercise group id"
//	@Success		200	{object}	response.Envelope{data=object}
//	@Failure		401	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		409	{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/library/versions/{vid}/exercise-groups/{gid} [delete]
func (h *Handler) deleteExerciseGroup(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	vid, ok := pathID(c, "vid", "template version")
	if !ok {
		return
	}
	gid, ok := pathID(c, "gid", "exercise group")
	if !ok {
		return
	}
	if err := h.svc.DeleteExerciseGroup(c.Request.Context(), sc, vid, gid); err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, gin.H{"deleted": true})
}

// bindArray decodes a bare JSON array body. Element validation failures are
// reported as 422 keyed by "<index>.<field>", matching the keys the service
// uses for its own per-row checks; anything else is a 400.
func bindArray[T any](c *gin.Context) ([]T, bool) {
	var items []T
	if err := c.ShouldBindJSON(&items); err != nil {
		var elementErrs binding.SliceValidationError
		if appErr := validation.Elements(items); errors.As(err, &elementErrs) && appErr != nil {
			response.Err(c, appErr)
		} else {
			response.Err(c, validation.BindError(err))
		}
		return nil, false
	}
	return items, true
}

// itemSorts whitelists the public sort keys for the material and exercise
// lists; both tables share the column names.
var itemSorts = map[string]string{
	"title":      "title",
	"created_at": "created_at",
}

// activeFilter parses the optional ?active= query flag; absent or anything
// other than "true"/"false" leaves the filter unset so the list is not
// narrowed by it.
func activeFilter(c *gin.Context) *bool {
	switch c.Query("active") {
	case "true":
		v := true
		return &v
	case "false":
		v := false
		return &v
	default:
		return nil
	}
}

// getVersion returns a version with everything a class would inherit.
//
//	@Summary		Get a template version
//	@Description	The version with its score set, log fields and lessons, each lesson with its attached materials and exercises.
//	@Tags			library
//	@Produce		json
//	@Param			vid	path		string	true	"version id"
//	@Success		200	{object}	response.Envelope{data=VersionDetailResponse}
//	@Failure		401	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404	{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/library/versions/{vid} [get]
func (h *Handler) getVersion(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	vid, ok := pathID(c, "vid", "template version")
	if !ok {
		return
	}
	out, err := h.svc.GetVersion(c.Request.Context(), sc, vid)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, out)
}

// setLogFields replaces a version's session-log fields.
//
//	@Summary		Replace the log fields of a version
//	@Description	Wholesale replace, body order is the position. A select field needs at least one option. 409 VERSION_LOCKED unless the version is a draft.
//	@Tags			library
//	@Accept			json
//	@Produce		json
//	@Param			vid		path		string			true	"version id"
//	@Param			body	body		[]LogFieldInput	true	"log fields; [] clears"
//	@Success		200		{object}	response.Envelope{data=[]LogFieldResponse}
//	@Failure		401		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		409		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		422		{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/library/versions/{vid}/log-fields [put]
func (h *Handler) setLogFields(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	vid, ok := pathID(c, "vid", "template version")
	if !ok {
		return
	}
	items, ok := bindArray[LogFieldInput](c)
	if !ok {
		return
	}
	out, err := h.svc.SetLogFields(c.Request.Context(), sc, vid, items)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, out)
}

// setScoreSet replaces a version's score set.
//
//	@Summary		Replace the score set of a version
//	@Description	Wholesale replace. Group and component keys are lowercase identifiers; a component key is unique within its own group only. max must be positive. 409 VERSION_LOCKED unless the version is a draft.
//	@Tags			library
//	@Accept			json
//	@Produce		json
//	@Param			vid		path		string					true	"version id"
//	@Param			body	body		[]ScoreSetGroupInput	true	"score set groups; [] clears"
//	@Success		200		{object}	response.Envelope{data=[]ScoreSetGroup}
//	@Failure		401		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		409		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		422		{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/library/versions/{vid}/score-set [put]
func (h *Handler) setScoreSet(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	vid, ok := pathID(c, "vid", "template version")
	if !ok {
		return
	}
	items, ok := bindArray[ScoreSetGroupInput](c)
	if !ok {
		return
	}
	out, err := h.svc.SetScoreSet(c.Request.Context(), sc, vid, items)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, out)
}

// setLessonMaterials replaces the materials attached to a lesson.
//
//	@Summary		Replace the materials of a template lesson
//	@Description	Wholesale replace, body order is the display order. Every id must be a live material of the center. 409 VERSION_LOCKED unless the lesson's version is a draft.
//	@Tags			library
//	@Accept			json
//	@Produce		json
//	@Param			lid		path		string					true	"lesson id"
//	@Param			body	body		[]LessonMaterialInput	true	"materials; [] clears"
//	@Success		200		{object}	response.Envelope{data=[]LessonMaterialResponse}
//	@Failure		401		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		409		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		422		{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/library/lessons/{lid}/materials [put]
func (h *Handler) setLessonMaterials(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	lid, ok := pathID(c, "lid", "template lesson")
	if !ok {
		return
	}
	items, ok := bindArray[LessonMaterialInput](c)
	if !ok {
		return
	}
	out, err := h.svc.SetLessonMaterials(c.Request.Context(), sc, lid, items)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, out)
}

// setLessonExercises replaces the exercises attached to a lesson.
//
//	@Summary		Replace the exercises of a template lesson
//	@Description	Wholesale replace, body order is the display order. Every id must be a live exercise of the center. 409 VERSION_LOCKED unless the lesson's version is a draft.
//	@Tags			library
//	@Accept			json
//	@Produce		json
//	@Param			lid		path		string					true	"lesson id"
//	@Param			body	body		[]LessonExerciseInput	true	"exercises; [] clears"
//	@Success		200		{object}	response.Envelope{data=[]LessonExerciseResponse}
//	@Failure		401		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		409		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		422		{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/library/lessons/{lid}/exercises [put]
func (h *Handler) setLessonExercises(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	lid, ok := pathID(c, "lid", "template lesson")
	if !ok {
		return
	}
	items, ok := bindArray[LessonExerciseInput](c)
	if !ok {
		return
	}
	out, err := h.svc.SetLessonExercises(c.Request.Context(), sc, lid, items)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, out)
}

// listMaterials pages the center's materials.
//
//	@Summary		List library materials
//	@Description	Live materials of the caller's center. q matches the title.
//	@Tags			library
//	@Produce		json
//	@Param			q			query		string	false	"title fragment"
//	@Param			active		query		bool	false	"only materials with this active value"
//	@Param			page		query		int		false	"page (default 1)"
//	@Param			per_page	query		int		false	"page size (default 20, max 100)"
//	@Param			sort		query		string	false	"title | created_at, prefix - for descending"
//	@Success		200			{object}	response.Envelope{data=[]MaterialResponse,meta=response.Meta}
//	@Failure		401			{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403			{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/library/materials [get]
func (h *Handler) listMaterials(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	params := pagination.Parse(c, "title", itemSorts)
	filter := ListFilter{Q: strings.TrimSpace(c.Query("q")), Active: activeFilter(c)}
	rows, total, err := h.svc.ListMaterials(c.Request.Context(), sc, filter, params)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.List(c, rows, params.Meta(total))
}

// createMaterial adds a material.
//
//	@Summary		Create a library material
//	@Tags			library
//	@Accept			json
//	@Produce		json
//	@Param			body	body		MaterialRequest	true	"material"
//	@Success		201		{object}	response.Envelope{data=MaterialResponse}
//	@Failure		401		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		422		{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/library/materials [post]
func (h *Handler) createMaterial(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	var req MaterialRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Err(c, validation.BindError(err))
		return
	}
	out, err := h.svc.CreateMaterial(c.Request.Context(), sc, req)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusCreated, out)
}

// getMaterial returns one material.
//
//	@Summary		Get a library material
//	@Tags			library
//	@Produce		json
//	@Param			id	path		string	true	"material id"
//	@Success		200	{object}	response.Envelope{data=MaterialResponse}
//	@Failure		401	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404	{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/library/materials/{id} [get]
func (h *Handler) getMaterial(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	id, ok := pathID(c, "id", "library material")
	if !ok {
		return
	}
	out, err := h.svc.GetMaterial(c.Request.Context(), sc, id)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, out)
}

// updateMaterial replaces a material's fields.
//
//	@Summary		Update a library material
//	@Description	Replaces every field. Lessons linking the material see the change at once.
//	@Tags			library
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string			true	"material id"
//	@Param			body	body		MaterialRequest	true	"material"
//	@Success		200		{object}	response.Envelope{data=MaterialResponse}
//	@Failure		401		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		422		{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/library/materials/{id} [put]
func (h *Handler) updateMaterial(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	id, ok := pathID(c, "id", "library material")
	if !ok {
		return
	}
	var req MaterialRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Err(c, validation.BindError(err))
		return
	}
	out, err := h.svc.UpdateMaterial(c.Request.Context(), sc, id, req)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, out)
}

// deleteMaterial soft-deletes an unlinked material.
//
//	@Summary		Delete a library material
//	@Description	409 MATERIAL_IN_USE while any template lesson still links it.
//	@Tags			library
//	@Produce		json
//	@Param			id	path		string	true	"material id"
//	@Success		200	{object}	response.Envelope{data=object}
//	@Failure		401	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		409	{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/library/materials/{id} [delete]
func (h *Handler) deleteMaterial(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	id, ok := pathID(c, "id", "library material")
	if !ok {
		return
	}
	if err := h.svc.DeleteMaterial(c.Request.Context(), sc, id); err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, gin.H{"deleted": true})
}

// setMaterialStatus toggles a material's active flag.
//
//	@Summary		Set a library material's active status
//	@Description	An inactive material stays visible to a lesson that already links it and stays in the bank list by default; SetLessonMaterials refuses to attach it to another lesson.
//	@Tags			library
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string				true	"material id"
//	@Param			body	body		ItemStatusRequest	true	"status"
//	@Success		200		{object}	response.Envelope{data=MaterialResponse}
//	@Failure		401		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		422		{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/library/materials/{id}/status [patch]
func (h *Handler) setMaterialStatus(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	id, ok := pathID(c, "id", "library material")
	if !ok {
		return
	}
	var req ItemStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Err(c, validation.BindError(err))
		return
	}
	out, err := h.svc.SetMaterialStatus(c.Request.Context(), sc, id, *req.Active)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, out)
}

// listExercises pages the center's exercises.
//
//	@Summary		List library exercises
//	@Description	Live exercises of the caller's center. q matches the title or code.
//	@Tags			library
//	@Produce		json
//	@Param			q			query		string	false	"title or code fragment"
//	@Param			active		query		bool	false	"only exercises with this active value"
//	@Param			page		query		int		false	"page (default 1)"
//	@Param			per_page	query		int		false	"page size (default 20, max 100)"
//	@Param			sort		query		string	false	"title | created_at, prefix - for descending"
//	@Success		200			{object}	response.Envelope{data=[]ExerciseResponse,meta=response.Meta}
//	@Failure		401			{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403			{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/library/exercises [get]
func (h *Handler) listExercises(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	params := pagination.Parse(c, "title", itemSorts)
	filter := ListFilter{Q: strings.TrimSpace(c.Query("q")), Active: activeFilter(c)}
	rows, total, err := h.svc.ListExercises(c.Request.Context(), sc, filter, params)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.List(c, rows, params.Meta(total))
}

// createExercise adds an exercise.
//
//	@Summary		Create a library exercise
//	@Description	Code, when given, is upper-cased and must match ^[A-Za-z0-9-]+$ (409 EXERCISE_CODE_TAKEN on a clash); omitted, the next BT-0001-style code of the center is assigned.
//	@Tags			library
//	@Accept			json
//	@Produce		json
//	@Param			body	body		ExerciseRequest	true	"exercise"
//	@Success		201		{object}	response.Envelope{data=ExerciseResponse}
//	@Failure		401		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		422		{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/library/exercises [post]
func (h *Handler) createExercise(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	var req ExerciseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Err(c, validation.BindError(err))
		return
	}
	out, err := h.svc.CreateExercise(c.Request.Context(), sc, req)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusCreated, out)
}

// getExercise returns one exercise.
//
//	@Summary		Get a library exercise
//	@Tags			library
//	@Produce		json
//	@Param			id	path		string	true	"exercise id"
//	@Success		200	{object}	response.Envelope{data=ExerciseResponse}
//	@Failure		401	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404	{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/library/exercises/{id} [get]
func (h *Handler) getExercise(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	id, ok := pathID(c, "id", "library exercise")
	if !ok {
		return
	}
	out, err := h.svc.GetExercise(c.Request.Context(), sc, id)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, out)
}

// updateExercise replaces an exercise's fields.
//
//	@Summary		Update a library exercise
//	@Description	Replaces every field. Lessons linking the exercise see the change at once. Code, when given, follows the create rule (409 EXERCISE_CODE_TAKEN on a clash); omitted, the exercise keeps its current code.
//	@Tags			library
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string			true	"exercise id"
//	@Param			body	body		ExerciseRequest	true	"exercise"
//	@Success		200		{object}	response.Envelope{data=ExerciseResponse}
//	@Failure		401		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		422		{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/library/exercises/{id} [put]
func (h *Handler) updateExercise(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	id, ok := pathID(c, "id", "library exercise")
	if !ok {
		return
	}
	var req ExerciseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Err(c, validation.BindError(err))
		return
	}
	out, err := h.svc.UpdateExercise(c.Request.Context(), sc, id, req)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, out)
}

// deleteExercise soft-deletes an unlinked exercise.
//
//	@Summary		Delete a library exercise
//	@Description	409 EXERCISE_IN_USE while any template lesson still links it.
//	@Tags			library
//	@Produce		json
//	@Param			id	path		string	true	"exercise id"
//	@Success		200	{object}	response.Envelope{data=object}
//	@Failure		401	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		409	{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/library/exercises/{id} [delete]
func (h *Handler) deleteExercise(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	id, ok := pathID(c, "id", "library exercise")
	if !ok {
		return
	}
	if err := h.svc.DeleteExercise(c.Request.Context(), sc, id); err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, gin.H{"deleted": true})
}

// setExerciseStatus toggles an exercise's active flag.
//
//	@Summary		Set a library exercise's active status
//	@Description	An inactive exercise stays visible to a lesson that already links it and stays in the bank list by default; SetLessonExercises refuses to attach it to another lesson.
//	@Tags			library
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string				true	"exercise id"
//	@Param			body	body		ItemStatusRequest	true	"status"
//	@Success		200		{object}	response.Envelope{data=ExerciseResponse}
//	@Failure		401		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		422		{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/library/exercises/{id}/status [patch]
func (h *Handler) setExerciseStatus(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	id, ok := pathID(c, "id", "library exercise")
	if !ok {
		return
	}
	var req ItemStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Err(c, validation.BindError(err))
		return
	}
	out, err := h.svc.SetExerciseStatus(c.Request.Context(), sc, id, *req.Active)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, out)
}
