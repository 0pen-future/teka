package statements

import "github.com/gin-gonic/gin"

// RegisterRoutes mounts the statement endpoints under /billing-periods and
// /statements, all behind authentication and center-scope resolution.
func RegisterRoutes(rg *gin.RouterGroup, h *Handler, auth ...gin.HandlerFunc) {
	periods := rg.Group("/billing-periods", auth...)
	periods.POST("/:id/statements/generate", h.generate)
	periods.GET("/:id/statements", h.list)

	g := rg.Group("/statements", auth...)
	g.GET("/:id", h.get)
	g.POST("/:id/revoke", h.revoke)
}

// RegisterPublicRoutes mounts the one unauthenticated route in the product on
// r's root engine — deliberately outside /api/v1 and outside requireAuth, so
// no future change to the authenticated group can accidentally pull it under
// a login requirement. securityHeaders runs before lookup/render, so it
// covers the neutral 404 branch too.
func RegisterPublicRoutes(r *gin.Engine, h *PublicHandler) {
	g := r.Group("/public/statements", securityHeaders())
	g.GET("/:token", h.get)
	g.GET("/:token/qr.png", h.qrImage)
}
