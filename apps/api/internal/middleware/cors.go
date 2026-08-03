// Package middleware provides the cross-cutting Gin middleware chain:
// request id, request logging, panic recovery, and CORS.
package middleware

import (
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"

	"teka/apps/api/internal/config"
)

// CORS allows the configured web origins with credentials (the refresh-token
// cookie crosses origins in host-mode development).
func CORS(cfg *config.Config) gin.HandlerFunc {
	return cors.New(cors.Config{
		AllowOrigins:     cfg.CORSOrigins,
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization", RequestIDHeader},
		ExposeHeaders:    []string{RequestIDHeader},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	})
}
