package collections

import "github.com/gin-gonic/gin"

// RegisterRoutes mounts the collection board endpoints under
// /billing-periods and the month-wide reads under /collections, behind
// authentication and scope resolution.
func RegisterRoutes(rg *gin.RouterGroup, h *Handler, auth ...gin.HandlerFunc) {
	g := rg.Group("/billing-periods", auth...)
	g.GET("/:id/collections", h.list)
	g.GET("/:id/collections/summary", h.summary)

	m := rg.Group("/collections", auth...)
	m.GET("/contact-balances", h.contactBalances)
}
