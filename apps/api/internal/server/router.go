package server

import (
	"log/slog"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"gorm.io/gorm"

	// Generated OpenAPI spec (make api-docs); imported for its side-effect
	// registration with gin-swagger.
	_ "teka/apps/api/docs"
	"teka/apps/api/internal/config"
	"teka/apps/api/internal/database"
	"teka/apps/api/internal/features/auth"
	"teka/apps/api/internal/features/classes"
	"teka/apps/api/internal/features/contacts"
	"teka/apps/api/internal/features/enrollments"
	"teka/apps/api/internal/features/students"
	"teka/apps/api/internal/features/teachers"
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

	// The OpenAPI UI ships only outside production.
	if !cfg.IsProduction() {
		r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
	}

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
	requireAuth := middleware.RequireAuth(cfg.JWT)
	txMgr := database.NewTxManager(db)

	teachersSvc := teachers.NewService(teachers.NewRepository(db))

	authSvc := auth.NewService(teachersSvc, auth.NewRepository(db), auth.NewTokenIssuer(cfg.JWT), txMgr)
	auth.RegisterRoutes(v1, auth.NewHandler(authSvc, cfg))
	teachers.RegisterRoutes(v1, teachers.NewHandler(teachersSvc), requireAuth)

	contactsSvc := contacts.NewService(contacts.NewRepository(db))
	contacts.RegisterRoutes(v1, contacts.NewHandler(contactsSvc), requireAuth)

	classesSvc := classes.NewService(classes.NewRepository(db), txMgr)
	classes.RegisterRoutes(v1, classes.NewHandler(classesSvc), requireAuth)

	// Construction order matters: the students service consumes the
	// enrollments service through students.EnrollmentEnder so deleting a
	// student closes their open enrollments in the same transaction.
	enrollmentsSvc := enrollments.NewService(enrollments.NewRepository(db))
	enrollments.RegisterRoutes(v1, enrollments.NewHandler(enrollmentsSvc), requireAuth)

	studentsSvc := students.NewService(students.NewRepository(db), enrollmentsSvc, txMgr)
	students.RegisterRoutes(v1, students.NewHandler(studentsSvc), requireAuth)
}
