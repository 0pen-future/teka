package imports

import "github.com/gin-gonic/gin"

// RegisterRoutes mounts the import endpoints under /imports, behind
// authentication and center-scope resolution. The service enforces owner-only
// access; there is no owner middleware in this codebase and one endpoint is
// not reason enough to introduce one.
//
// rateLimit guards the upload route. It is the most expensive endpoint in the
// product and the connection pool is shared across every tenant, so an
// unthrottled retry loop by one owner is an availability problem for all of
// them.
func RegisterRoutes(rg *gin.RouterGroup, h *Handler, requireAuth, resolveScope, rateLimit gin.HandlerFunc) {
	g := rg.Group("/imports", requireAuth, resolveScope)
	g.GET("/roster/template", h.template)
	g.POST("/roster", rateLimit, h.importRoster)
}
