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
	"teka/apps/api/internal/features/attendance"
	"teka/apps/api/internal/features/auth"
	"teka/apps/api/internal/features/billing"
	"teka/apps/api/internal/features/centers"
	"teka/apps/api/internal/features/classes"
	"teka/apps/api/internal/features/collections"
	"teka/apps/api/internal/features/contacts"
	"teka/apps/api/internal/features/enrollments"
	"teka/apps/api/internal/features/notifications"
	"teka/apps/api/internal/features/payments"
	"teka/apps/api/internal/features/sessions"
	"teka/apps/api/internal/features/statements"
	"teka/apps/api/internal/features/students"
	"teka/apps/api/internal/features/teachers"
	"teka/apps/api/internal/features/zalo"
	"teka/apps/api/internal/middleware"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/response"
)

// NewRouter builds the Gin engine: middleware stack, health probes, and the
// versioned API group that feature modules mount into. Every feature is
// constructed from db here except zalo, statements, and notifications, which
// are passed in already built: zalo and notifications own background
// goroutines the app lifecycle has to stop, and statements is a constructor
// dependency of notifications.
func NewRouter(cfg *config.Config, log *slog.Logger, db *gorm.DB, zaloSvc *zalo.Service, statementsSvc *statements.Service, notificationsSvc *notifications.Service) *gin.Engine {
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
	registerFeatures(v1, cfg, db, zaloSvc, statementsSvc, notificationsSvc)

	// Deliberately outside v1 and outside requireAuth: the only unauthenticated
	// route in the product that serves child/money data.
	statements.RegisterPublicRoutes(r, statements.NewPublicHandler(statementsSvc))

	r.NoRoute(func(c *gin.Context) {
		response.Err(c, apperror.NotFound("route"))
	})

	return r
}

// registerFeatures wires feature modules into the versioned group. Feature
// construction (repository → service → handler) happens here so features stay
// decoupled from bootstrap; the process-lifetime services (zalo, statements,
// notifications) arrive already built from the container and are only
// mounted.
func registerFeatures(v1 *gin.RouterGroup, cfg *config.Config, db *gorm.DB, zaloSvc *zalo.Service, statementsSvc *statements.Service, notificationsSvc *notifications.Service) {
	requireAuth := middleware.RequireAuth(cfg.JWT)
	txMgr := database.NewTxManager(db)

	teachersSvc := teachers.NewService(teachers.NewRepository(db))

	// centers consumes teachers through centers.TeacherLookup (owner-phone
	// resolution), and teachers consumes centers back through
	// teachers.CenterProvisioner so registering a teacher provisions their
	// personal center in the same transaction — a setter breaks that cycle,
	// same as attendance.SetReconciler below.
	centersSvc := centers.NewService(centers.NewRepository(db), teachersSvc, txMgr)
	teachersSvc.SetCenterProvisioner(centersSvc)
	// resolveScope re-reads center membership from the database on every
	// request (never from JWT claims) so kicks and leaves bite immediately.
	resolveScope := middleware.ResolveScope(centersSvc)
	centers.RegisterRoutes(v1, centers.NewHandler(centersSvc), requireAuth, resolveScope)

	authSvc := auth.NewService(teachersSvc, auth.NewRepository(db), auth.NewTokenIssuer(cfg.JWT), txMgr)
	auth.RegisterRoutes(v1, auth.NewHandler(authSvc, cfg))
	teachers.RegisterRoutes(v1, teachers.NewHandler(teachersSvc), requireAuth, resolveScope)

	contactsSvc := contacts.NewService(contacts.NewRepository(db))
	contacts.RegisterRoutes(v1, contacts.NewHandler(contactsSvc), requireAuth, resolveScope)

	classesSvc := classes.NewService(classes.NewRepository(db), txMgr)
	classes.RegisterRoutes(v1, classes.NewHandler(classesSvc), requireAuth, resolveScope)

	// Construction order matters: the students service consumes the
	// enrollments service through students.EnrollmentEnder so deleting a
	// student closes their open enrollments in the same transaction.
	enrollmentsSvc := enrollments.NewService(enrollments.NewRepository(db))
	enrollments.RegisterRoutes(v1, enrollments.NewHandler(enrollmentsSvc), requireAuth, resolveScope)

	studentsSvc := students.NewService(students.NewRepository(db), enrollmentsSvc, txMgr)
	students.RegisterRoutes(v1, students.NewHandler(studentsSvc), requireAuth, resolveScope)

	// sessions consumes classes, teachers, and enrollments through consumer
	// interfaces (ClassSource, TeacherSource, EnrollmentSource) rather than
	// their repository types, so all three services must exist first.
	sessionsSvc := sessions.NewService(sessions.NewRepository(db), classesSvc, teachersSvc, enrollmentsSvc)
	sessions.RegisterRoutes(v1, sessions.NewHandler(sessionsSvc), requireAuth, resolveScope)

	// attendance consumes enrollments and sessions through consumer
	// interfaces (RosterSource, SessionStore) rather than their repository
	// types, so both services must exist first. Confirming attendance runs
	// sessions.MarkHeldAndConfirmed inside attendance's own transaction so
	// attendance records and the session's held+confirmed status commit
	// atomically.
	attendanceSvc := attendance.NewService(attendance.NewRepository(db), enrollmentsSvc, sessionsSvc, txMgr)
	attendance.RegisterRoutes(v1, attendance.NewHandler(attendanceSvc), requireAuth, resolveScope)

	// billing consumes attendance through billing.AttendanceSource — the
	// batched per-enrollment tally — rather than re-aggregating
	// attendance_records itself, sessions through billing.PendingSource — the
	// unconfirmed-sessions predicate its period close reuses — and
	// enrollments through billing.EnrollmentSource — the sanctioned roster
	// membership check its post-close reconciliation's rare no-invoice-line
	// case reuses — so it is constructed after all three.
	billingSvc := billing.NewService(billing.NewRepository(db, attendanceSvc), txMgr, sessionsSvc, enrollmentsSvc)
	billing.RegisterRoutes(v1, billing.NewHandler(billingSvc), requireAuth, resolveScope)

	// attendance.Confirm carries a post-close attendance edit's money delta
	// onto the next open period through billingSvc, which can only be wired in
	// after billing exists — hence a setter rather than a NewService
	// parameter, breaking what would otherwise be a construction cycle
	// (billing needs attendance for TallyByEnrollment; attendance needs
	// billing for reconciliation).
	attendanceSvc.SetReconciler(billingSvc)

	paymentsSvc := payments.NewService(payments.NewRepository(db), txMgr)
	payments.RegisterRoutes(v1, payments.NewHandler(paymentsSvc), requireAuth, resolveScope)

	// collections is read-only reporting over billing_periods/invoices/
	// payments — no writes, so no transaction manager.
	collectionsSvc := collections.NewService(collections.NewRepository(db))
	collections.RegisterRoutes(v1, collections.NewHandler(collectionsSvc), requireAuth, resolveScope)

	statements.RegisterRoutes(v1, statements.NewHandler(statementsSvc), requireAuth)

	notifications.RegisterRoutes(v1, notifications.NewHandler(notificationsSvc), requireAuth)

	zalo.RegisterRoutes(v1, zalo.NewHandler(zaloSvc), requireAuth)
}
