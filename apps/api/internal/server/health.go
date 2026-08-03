// Package server assembles the Gin router and runs the HTTP server with
// graceful shutdown.
package server

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"teka/apps/api/internal/database"
)

// registerHealth mounts liveness and readiness probes. These return plain
// bodies (no envelope): their consumers are orchestrators, not API clients.
func registerHealth(r *gin.Engine, db *gorm.DB) {
	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	r.GET("/readyz", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), time.Second)
		defer cancel()
		if err := database.Ping(ctx, db); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unavailable", "reason": "database unreachable"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
}
