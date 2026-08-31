package billing

import "github.com/gin-gonic/gin"

// RegisterRoutes mounts the billing period endpoints under /billing-periods
// and the invoice void endpoint under /invoices, all behind authentication
// and center-scope resolution.
func RegisterRoutes(rg *gin.RouterGroup, h *Handler, auth ...gin.HandlerFunc) {
	g := rg.Group("/billing-periods", auth...)
	g.POST("", h.ensurePeriod)
	g.GET("", h.list)
	g.GET("/:id", h.get)
	g.GET("/:id/preview", h.preview)
	g.POST("/:id/draft", h.draft)
	g.POST("/:id/close", h.close)

	invoices := rg.Group("/invoices", auth...)
	invoices.POST("/:id/void", h.voidInvoice)
	invoices.POST("/:id/adjustments", h.addAdjustment)
	invoices.GET("/:id/adjustments", h.listAdjustments)
}
