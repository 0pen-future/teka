package statements

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/logger"
	"teka/apps/api/internal/shared/response"
)

// PublicHandler exposes the one unauthenticated route in the product: the
// link a parent opens to view their family's statement, plus its QR image.
// Every handler here resolves teacher/contact/period scope from the token
// itself (via Service.LookupPublic), never from anything else the request
// carries.
type PublicHandler struct {
	svc *Service
}

// NewPublicHandler builds the public statement handler.
func NewPublicHandler(svc *Service) *PublicHandler {
	return &PublicHandler{svc: svc}
}

// securityHeaders sets the three headers every response from this route must
// carry, on both success and the neutral 404. Applied as route-group
// middleware, rather than repeated per-handler, so a failure branch can never
// forget them; the qr.png handler overrides Cache-Control afterward, which is
// safe because Gin buffers headers until the first write.
func securityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Robots-Tag", "noindex, nofollow, noarchive")
		c.Header("Cache-Control", "no-store, no-cache, must-revalidate, private")
		c.Header("Referrer-Policy", "no-referrer")
		c.Next()
	}
}

// writeNeutralNotFound is the one place that ever writes this route's 404:
// every failure branch — unknown token, malformed token, revoked, expired,
// soft-deleted, or already fully paid — calls this and only this, so the
// response body can never drift between causes. Distinguishing them would
// already confirm a token once existed.
func writeNeutralNotFound(c *gin.Context) {
	response.Err(c, apperror.NotFound("statement"))
}

// lookup resolves the path token to its statement, writing the neutral 404
// itself on any failure. ok is false exactly when the caller must return
// without doing anything further.
func (h *PublicHandler) lookup(c *gin.Context) (*Statement, bool) {
	stmt, err := h.svc.LookupPublic(c.Request.Context(), c.Param("token"))
	if err != nil {
		writeNeutralNotFound(c)
		return nil, false
	}
	return stmt, true
}

// get renders the parent-facing statement payload.
//
//	@Summary		Get a parent's statement
//	@Description	No authentication: the token itself is the credential. Always returns the same neutral 404 for an unknown, malformed, revoked, expired, soft-deleted, or already fully-paid token.
//	@Tags			public
//	@Produce		json
//	@Param			token	path		string	true	"statement token"
//	@Success		200		{object}	response.Envelope{data=PublicStatement}
//	@Failure		404		{object}	response.Envelope{error=response.ErrorBody}
//	@Router			/public/statements/{token} [get]
func (h *PublicHandler) get(c *gin.Context) {
	stmt, ok := h.lookup(c)
	if !ok {
		return
	}
	payload, _, err := h.svc.RenderPublic(c.Request.Context(), stmt)
	if err != nil {
		writeNeutralNotFound(c)
		return
	}
	response.OK(c, http.StatusOK, payload)
	h.touchView(c, stmt)
}

// qrImage renders the same statement's payment QR as a PNG. Does not itself
// count as an "open" for view tracking — get already counts the page load
// that fetches this image alongside the JSON payload — so it does not call
// touchView.
//
//	@Summary		Get a parent's statement payment QR code
//	@Description	Same token, same neutral 404. Returns the neutral 404, not an empty image, when the teacher's bank details are not configured.
//	@Tags			public
//	@Produce		image/png
//	@Param			token	path	string	true	"statement token"
//	@Success		200
//	@Failure		404	{object}	response.Envelope{error=response.ErrorBody}
//	@Router			/public/statements/{token}/qr.png [get]
func (h *PublicHandler) qrImage(c *gin.Context) {
	stmt, ok := h.lookup(c)
	if !ok {
		return
	}
	_, qrPayload, err := h.svc.RenderPublic(c.Request.Context(), stmt)
	if err != nil {
		writeNeutralNotFound(c)
		return
	}
	if qrPayload == "" {
		// No bank config: no placeholder image, the same neutral 404 as an
		// invalid token.
		writeNeutralNotFound(c)
		return
	}
	png, err := h.svc.RenderQR(qrPayload)
	if err != nil {
		response.Err(c, apperror.Internal(err))
		return
	}
	// The image is derived from data the token holder can already see, so a
	// short private cache is safe here even though the route group default is
	// no-store.
	c.Header("Cache-Control", "private, max-age=300")
	c.Data(http.StatusOK, "image/png", png)
}

// touchView fires after the response has already been written. A view
// counter must never cost a parent their page, so a failure here is logged
// and nothing else.
func (h *PublicHandler) touchView(c *gin.Context, stmt *Statement) {
	if err := h.svc.TouchView(c.Request.Context(), stmt.TeacherID, stmt.ID); err != nil {
		logger.FromContext(c.Request.Context()).Error("statement view tracking failed",
			"statement_id", stmt.ID, "error", err)
	}
}
