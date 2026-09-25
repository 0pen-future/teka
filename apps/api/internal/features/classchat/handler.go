package classchat

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/response"
	"teka/apps/api/internal/shared/validation"
)

// Handler exposes the class chat endpoints.
type Handler struct {
	svc *Service
}

// NewHandler builds a Handler over svc.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) scope(c *gin.Context) (authctx.Scope, bool) {
	sc, ok := authctx.ScopeFrom(c)
	if !ok {
		response.Err(c, apperror.Unauthorized("authentication required"))
		return authctx.Scope{}, false
	}
	return sc, true
}

func pathID(c *gin.Context, name, resource string) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param(name))
	if err != nil {
		response.Err(c, apperror.NotFound(resource))
		return uuid.Nil, false
	}
	return id, true
}

func parseListQuery(c *gin.Context) (ListQuery, error) {
	var q ListQuery
	if raw := c.Query("before"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			return q, apperror.BadRequest("before must be a message id")
		}
		q.Before = &id
	}
	if raw := c.Query("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 {
			return q, apperror.BadRequest("limit must be a non-negative integer")
		}
		q.Limit = n
	}
	return q, nil
}

// list returns one page of the class's messages, newest first.
//
//	@Summary		List class messages
//	@Description	Internal class chat, newest first. Pass before=<id of the oldest shown message> for the next page; next_cursor is empty on the last one. Readable by the owner and teachers currently on the class.
//	@Tags			classes
//	@Produce		json
//	@Param			id		path		string	true	"class id"
//	@Param			before	query		string	false	"message id cursor: only older messages"
//	@Param			limit	query		int		false	"page size, default 20, max 50"
//	@Success		200		{object}	response.Envelope{data=ListResponse}
//	@Failure		400		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		401		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404		{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/classes/{id}/messages [get]
func (h *Handler) list(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	id, ok := pathID(c, "id", "class")
	if !ok {
		return
	}
	q, err := parseListQuery(c)
	if err != nil {
		response.Err(c, err)
		return
	}
	out, err := h.svc.List(c.Request.Context(), sc, id, q)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, out)
}

// post adds a message to the class.
//
//	@Summary		Post a class message
//	@Description	Needs class_messages.post and an active stint on the class (the owner always qualifies). Body is trimmed, max 2000 characters.
//	@Tags			classes
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string		true	"class id"
//	@Param			body	body		PostRequest	true	"message"
//	@Success		201		{object}	response.Envelope{data=MessageResponse}
//	@Failure		401		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		422		{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/classes/{id}/messages [post]
func (h *Handler) post(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	id, ok := pathID(c, "id", "class")
	if !ok {
		return
	}
	var req PostRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Err(c, validation.BindError(err))
		return
	}
	out, err := h.svc.Post(c.Request.Context(), sc, id, req)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusCreated, out)
}

// remove retracts a message.
//
//	@Summary		Retract a class message
//	@Description	Soft-deletes the message. Only its author or the center owner may retract it.
//	@Tags			classes
//	@Produce		json
//	@Param			id	path		string	true	"class id"
//	@Param			mid	path		string	true	"message id"
//	@Success		200	{object}	response.Envelope{data=object}
//	@Failure		401	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404	{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/classes/{id}/messages/{mid} [delete]
func (h *Handler) remove(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	id, ok := pathID(c, "id", "class")
	if !ok {
		return
	}
	mid, ok := pathID(c, "mid", "class message")
	if !ok {
		return
	}
	if err := h.svc.Delete(c.Request.Context(), sc, id, mid); err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, gin.H{"deleted": true})
}
