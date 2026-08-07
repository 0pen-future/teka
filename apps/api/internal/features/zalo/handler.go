package zalo

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/response"
	"teka/apps/api/internal/shared/validation"
)

// Handler exposes linking a teacher's personal Zalo account, following the
// attempt, and unlinking it. No response it produces carries credential
// material: the stored session is reachable only through the service, and only
// as something to act with, never as something to read.
type Handler struct {
	svc *Service
}

// NewHandler builds the Zalo handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// teacherID resolves the authenticated tenant — the only sanctioned source of
// teacher identity; the path never carries it.
func (h *Handler) teacherID(c *gin.Context) (uuid.UUID, bool) {
	teacherID, ok := authctx.TeacherID(c)
	if !ok {
		response.Err(c, apperror.Unauthorized("authentication required"))
		return uuid.UUID{}, false
	}
	return teacherID, true
}

// status reports whether the caller has a linked Zalo account.
//
//	@Summary		Zalo link status
//	@Description	Reads zalo_accounts only — no request reaches Zalo. Having no linked account is a normal answer ({"linked":false}), not a 404. status is "linked" or "expired"; the background health check keeps it fresh, so a session that died since its last use can already read expired.
//	@Tags			zalo
//	@Produce		json
//	@Success		200	{object}	response.Envelope{data=StatusResponse}
//	@Failure		401	{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/me/zalo [get]
func (h *Handler) status(c *gin.Context) {
	teacherID, ok := h.teacherID(c)
	if !ok {
		return
	}
	acc, err := h.svc.Status(c.Request.Context(), teacherID)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, newStatusResponse(acc))
}

// startLink begins a QR link attempt.
//
//	@Summary		Start linking a Zalo account
//	@Description	Returns immediately with a link id; the QR login runs in the background and is followed with GET /me/zalo/link/status. consent_version is the consent text the teacher acknowledged — without one no attempt starts. Starting a second attempt supersedes the caller's previous one.
//	@Tags			zalo
//	@Accept			json
//	@Produce		json
//	@Param			request	body		LinkStartRequest	true	"acknowledged consent version"
//	@Success		202		{object}	response.Envelope{data=LinkStartResponse}
//	@Failure		400		{object}	response.Envelope{error=response.ErrorBody}	"consent version missing"
//	@Failure		401		{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/me/zalo/link/start [post]
func (h *Handler) startLink(c *gin.Context) {
	teacherID, ok := h.teacherID(c)
	if !ok {
		return
	}
	var req LinkStartRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Err(c, validation.BindError(err))
		return
	}
	linkID, err := h.svc.StartLink(teacherID, req.ConsentVersion)
	if err != nil {
		response.Err(c, linkError(err))
		return
	}
	response.OK(c, http.StatusAccepted, LinkStartResponse{LinkID: linkID})
}

// linkStatus reports how the caller's link attempt is going.
//
//	@Summary		Poll a link attempt
//	@Description	state is one of pending, qr_ready, scanned, confirmed, linked, expired, error. qr_png is base64 PNG, present while the code is waiting to be scanned. An id belonging to another teacher, or one that never existed, is a 404 either way.
//	@Tags			zalo
//	@Produce		json
//	@Param			id	query		string	true	"link attempt id"
//	@Success		200	{object}	response.Envelope{data=LinkStatusResponse}
//	@Failure		401	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404	{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/me/zalo/link/status [get]
func (h *Handler) linkStatus(c *gin.Context) {
	teacherID, ok := h.teacherID(c)
	if !ok {
		return
	}
	linkID, err := uuid.Parse(c.Query("id"))
	if err != nil {
		// A malformed id reads as an attempt that does not exist, which is the
		// same answer another teacher's id gets.
		response.Err(c, apperror.NotFound("link attempt"))
		return
	}
	snap, err := h.svc.LinkStatus(teacherID, linkID)
	if err != nil {
		response.Err(c, linkError(err))
		return
	}
	response.OK(c, http.StatusOK, newLinkStatusResponse(snap))
}

// friends lists the linked account's Zalo friends for the contact picker.
//
//	@Summary		List Zalo friends
//	@Description	Fetches the linked account's friend list live from Zalo for the contact-mapping picker. The chosen friend is stored via PUT /contacts/{id}/zalo-mapping; nothing is persisted here. 404 when no account is linked, 409 when the stored session no longer works.
//	@Tags			zalo
//	@Produce		json
//	@Success		200	{object}	response.Envelope{data=[]FriendResponse}
//	@Failure		401	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404	{object}	response.Envelope{error=response.ErrorBody}	"no linked account"
//	@Failure		409	{object}	response.Envelope{error=response.ErrorBody}	"session expired"
//	@Security		BearerAuth
//	@Router			/me/zalo/friends [get]
func (h *Handler) friends(c *gin.Context) {
	teacherID, ok := h.teacherID(c)
	if !ok {
		return
	}
	list, err := h.svc.ListFriends(c.Request.Context(), teacherID)
	if err != nil {
		response.Err(c, linkError(err))
		return
	}
	response.OK(c, http.StatusOK, newFriendResponses(list))
}

// matchFriends resolves phone numbers against Zalo and the friend list.
//
//	@Summary		Match phones against Zalo accounts
//	@Description	Looks up 1–200 phone numbers against Zalo in paced chunks, intersects the hits with the linked account's friend list, and returns one row per phone in request order. Rows echo the phone exactly as sent; unresolved phones come back matched=false. Nothing is persisted — confirming a suggestion goes through PUT /contacts/{id}/zalo-mapping. 404 when no account is linked, 409 when the stored session no longer works.
//	@Tags			zalo
//	@Accept			json
//	@Produce		json
//	@Param			request	body		MatchFriendsRequest	true	"phones to look up"
//	@Success		200		{object}	response.Envelope{data=[]FriendMatchResponse}
//	@Failure		400		{object}	response.Envelope{error=response.ErrorBody}	"empty or oversized phone list"
//	@Failure		401		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404		{object}	response.Envelope{error=response.ErrorBody}	"no linked account"
//	@Failure		409		{object}	response.Envelope{error=response.ErrorBody}	"session expired"
//	@Security		BearerAuth
//	@Router			/me/zalo/friends/match [post]
func (h *Handler) matchFriends(c *gin.Context) {
	teacherID, ok := h.teacherID(c)
	if !ok {
		return
	}
	var req MatchFriendsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Err(c, validation.BindError(err))
		return
	}
	if len(req.Phones) == 0 {
		response.Err(c, apperror.BadRequest("phones is required"))
		return
	}
	if len(req.Phones) > MaxMatchPhones {
		response.Err(c, apperror.BadRequest(fmt.Sprintf("at most %d phones per request", MaxMatchPhones)))
		return
	}
	rows, err := h.svc.MatchFriends(c.Request.Context(), teacherID, req.Phones)
	if err != nil {
		response.Err(c, linkError(err))
		return
	}
	response.OK(c, http.StatusOK, newFriendMatchResponses(rows))
}

// friendRequest sends one friend request as the teacher.
//
//	@Summary		Send one Zalo friend request
//	@Description	Sends exactly one friend request per call to the given Zalo user — no batch variant exists. message is optional; a blank one falls back to a short Vietnamese greeting. 404 when no account is linked, 409 when the stored session no longer works.
//	@Tags			zalo
//	@Accept			json
//	@Param			request	body	FriendRequestRequest	true	"target user and optional message"
//	@Success		204
//	@Failure		400	{object}	response.Envelope{error=response.ErrorBody}	"user_id missing"
//	@Failure		401	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404	{object}	response.Envelope{error=response.ErrorBody}	"no linked account"
//	@Failure		409	{object}	response.Envelope{error=response.ErrorBody}	"session expired"
//	@Security		BearerAuth
//	@Router			/me/zalo/friends/request [post]
func (h *Handler) friendRequest(c *gin.Context) {
	teacherID, ok := h.teacherID(c)
	if !ok {
		return
	}
	var req FriendRequestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Err(c, validation.BindError(err))
		return
	}
	if req.UserID == "" {
		response.Err(c, apperror.BadRequest("user_id is required"))
		return
	}
	if err := h.svc.SendRequest(c.Request.Context(), teacherID, req.UserID, req.Message); err != nil {
		response.Err(c, linkError(err))
		return
	}
	c.Status(http.StatusNoContent)
}

// unlink detaches the caller's Zalo account.
//
//	@Summary		Unlink the Zalo account
//	@Description	Cancels any attempt in flight, drops the live session, and removes the stored credentials. Idempotent: answers 204 whether or not an account was linked.
//	@Tags			zalo
//	@Success		204
//	@Failure		401	{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/me/zalo [delete]
func (h *Handler) unlink(c *gin.Context) {
	teacherID, ok := h.teacherID(c)
	if !ok {
		return
	}
	if err := h.svc.Unlink(c.Request.Context(), teacherID); err != nil {
		response.Err(c, linkError(err))
		return
	}
	c.Status(http.StatusNoContent)
}

// linkError translates the package's sentinels into HTTP answers. A link id
// that is not the caller's is reported as missing rather than forbidden: the
// caller has no way to know one exists, and telling them is itself the leak.
func linkError(err error) error {
	switch {
	case errors.Is(err, ErrConsentVersionRequired):
		return apperror.BadRequest("consent version is required")
	case errors.Is(err, ErrLinkNotFound):
		return apperror.NotFound("link attempt")
	case errors.Is(err, ErrNotLinked):
		return apperror.NotFound("zalo account")
	case errors.Is(err, ErrLinkExpired):
		return apperror.Conflict("the linked Zalo session expired; link the account again")
	default:
		return err
	}
}
