package server

import (
	"log/slog"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"teka/apps/api/internal/config"
	"teka/apps/api/internal/middleware"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/response"
)

// NewRouter builds the Gin engine: middleware stack, health probes, and the
// versioned API group that feature modules mount into.
func NewRouter(cfg *config.Config, log *slog.Logger, db *gorm.DB) *gin.Engine {
	if cfg.IsProduction() {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	// Trust no proxy: ClientIP() uses the socket address instead of a
	// client-forgeable X-Forwarded-For. Revisit when a load balancer fronts
	// the API and its address range is known.
	_ = r.SetTrustedProxies(nil)
	r.Use(
		middleware.RequestID(),
		middleware.Logger(log),
		middleware.Recovery(),
		middleware.CORS(cfg),
	)

	registerHealth(r, db)

	v1 := r.Group("/api/v1")
	registerFeatures(v1, cfg, db)

	r.NoRoute(func(c *gin.Context) {
		response.Err(c, apperror.NotFound("route"))
	})

	return r
}

// registerFeatures wires feature modules into the versioned group. Feature
// construction (repository → service → handler) happens here so features stay
// decoupled from bootstrap.
func registerFeatures(v1 *gin.RouterGroup, cfg *config.Config, db *gorm.DB) {
	_ = v1
	_ = cfg
	_ = db
	// Feature modules are provisioned in Phase 3 (auth, users).
}
