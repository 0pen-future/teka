package imports

import "github.com/gin-gonic/gin"

// RegisterRoutes mounts the import endpoints under /imports, behind the auth
// chain (authentication, scope resolution, route policy). The service keeps
// its own owner check so the rule is testable without HTTP.
//
// rateLimit guards the upload route. It is the most expensive endpoint in the
// product and the connection pool is shared across every tenant, so an
// unthrottled retry loop by one owner is an availability problem for all of
// them.
func RegisterRoutes(rg *gin.RouterGroup, h *Handler, rateLimit gin.HandlerFunc, auth ...gin.HandlerFunc) {
	g := rg.Group("/imports", auth...)
	g.GET("/roster/template", h.template)
	g.POST("/roster", rateLimit, h.importRoster)
}
