package server

import (
	"log/slog"
	"time"

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
	"teka/apps/api/internal/features/audit"
	"teka/apps/api/internal/features/auth"
	"teka/apps/api/internal/features/billing"
	"teka/apps/api/internal/features/centers"
	"teka/apps/api/internal/features/classes"
	"teka/apps/api/internal/features/collections"
	"teka/apps/api/internal/features/contacts"
	"teka/apps/api/internal/features/enrollments"
	"teka/apps/api/internal/features/handoff"
	"teka/apps/api/internal/features/imports"
	"teka/apps/api/internal/features/invitations"
	"teka/apps/api/internal/features/notifications"
	"teka/apps/api/internal/features/payments"
	"teka/apps/api/internal/features/sessions"
	"teka/apps/api/internal/features/statements"
	"teka/apps/api/internal/features/students"
	"teka/apps/api/internal/features/teachers"
	"teka/apps/api/internal/features/teaching"
	"teka/apps/api/internal/features/zalo"
	"teka/apps/api/internal/middleware"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/events"
	"teka/apps/api/internal/shared/response"
)

// NewRouter builds the Gin engine: middleware stack, health probes, and the
// versioned API group that feature modules mount into. Every feature is
// constructed from db here except zalo, statements, notifications, teachers,
// centers, and auth, which are passed in already built: zalo and
// notifications own background goroutines the app lifecycle has to stop,
// statements is a constructor dependency of notifications, and
// teachers/centers/auth are built once in app.Container so the operator
// CLI's onboarding commands share the exact same identity wiring instead of
// duplicating it.
func NewRouter(cfg *config.Config, log *slog.Logger, db *gorm.DB, zaloSvc *zalo.Service, statementsSvc *statements.Service, notificationsSvc *notifications.Service, teachersSvc *teachers.Service, centersSvc *centers.Service, authSvc *auth.Service, bus events.Bus) *gin.Engine {
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
	// After the global stack, before any route registers: RequestEvents does
	// its work post-c.Next(), so the principal/scope the per-route auth
	// middleware resolves is already on the context when it publishes.
	v1.Use(middleware.RequestEvents(bus))
	registerFeatures(v1, cfg, db, zaloSvc, statementsSvc, notificationsSvc, teachersSvc, centersSvc, authSvc, bus)

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
func registerFeatures(v1 *gin.RouterGroup, cfg *config.Config, db *gorm.DB, zaloSvc *zalo.Service, statementsSvc *statements.Service, notificationsSvc *notifications.Service, teachersSvc *teachers.Service, centersSvc *centers.Service, authSvc *auth.Service, bus events.Bus) {
	requireAuth := middleware.RequireAuth(cfg.JWT)
	txMgr := database.NewTxManager(db)

	// teachersSvc, centersSvc, and authSvc arrive already built and
	// cross-wired (SetAccountDisabler/SetTokenRevoker) from app.Container —
	// see its NewContainer for that wiring; only route registration happens
	// here.
	// resolveScope re-reads center membership from the database on every
	// request (never from JWT claims) so removals bite immediately.
	resolveScope := middleware.ResolveScope(centersSvc)
	centers.RegisterRoutes(v1, centers.NewHandler(centersSvc), requireAuth, resolveScope)

	auditSvc := audit.NewService(audit.NewRepository(db))
	audit.RegisterRoutes(v1, audit.NewHandler(auditSvc), requireAuth, resolveScope)

	authHandler := auth.NewHandler(authSvc, cfg)
	auth.RegisterRoutes(v1, authHandler)
	// The reset token and the target phone are the only credentials guarding
	// these two routes — the caller has no session yet, so each gets its own
	// rate limit keyed on the request body, not the caller's IP (same
	// reasoning as invitations.RegisterPublicRoutes below).
	auth.RegisterPublicRoutes(v1, authHandler,
		middleware.RateLimit(middleware.JSONBodyKey("phone"), 5, time.Minute),
		middleware.RateLimit(middleware.JSONBodyKey("token"), 10, time.Minute))
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

	// The roster import drives classes, contacts, students and enrollments
	// through their own services, so it mounts after all four exist. It writes
	// nothing directly: every row goes through the feature that owns the
	// table, under an anchor scope naming the class's teacher. It reads
	// the center's phone -> teacher directory through centers.MemberDirectory:
	// that lookup is scoped to the caller's own center, which is what keeps an
	// owner's workbook from naming a teacher outside it.
	//
	// The upload is the most expensive endpoint in the product and the
	// connection pool is shared across every tenant, so it carries its own
	// per-teacher rate limit rather than relying on the caller to behave.
	// centerLocker is the single advisory-lock instance imports and handoff
	// share: both take pg_try_advisory_xact_lock on the same center key, so a
	// class handoff and an in-flight import exclude each other rather than
	// interleaving mid-transaction. One instance keeps that key in one place.
	centerLocker := imports.NewLocker(db)
	importsSvc := imports.NewService(centersSvc, classesSvc, contactsSvc, studentsSvc, enrollmentsSvc,
		centerLocker, txMgr)
	imports.RegisterRoutes(v1, imports.NewHandler(importsSvc), requireAuth, resolveScope,
		middleware.RateLimit(middleware.TeacherKey(), 10, time.Minute))

	// sessions consumes classes, teachers, and enrollments through consumer
	// interfaces (ClassSource, TeacherSource, EnrollmentSource) rather than
	// their repository types, so all three services must exist first.
	sessionsSvc := sessions.NewService(sessions.NewRepository(db), classesSvc, teachersSvc, enrollmentsSvc)
	sessions.RegisterRoutes(v1, sessions.NewHandler(sessionsSvc), requireAuth, resolveScope)

	// handoff reassigns a class to another teacher. It coordinates classes and
	// sessions through consumer interfaces and validates the target through
	// centersSvc, so it mounts after all three exist. It shares centerLocker
	// with imports so the two center-wide writers exclude each other. It owns no
	// tables: the class and its schedules move through classesSvc, the future
	// planned sessions through sessionsSvc, in one transaction.
	handoffSvc := handoff.NewService(classesSvc, sessionsSvc, centersSvc, centerLocker, txMgr)
	handoff.RegisterRoutes(v1, handoff.NewHandler(handoffSvc), requireAuth, resolveScope)

	// attendance consumes enrollments and sessions through consumer
	// interfaces (RosterSource, SessionStore) rather than their repository
	// types, so both services must exist first. Confirming attendance runs
	// sessions.MarkHeldAndConfirmed inside attendance's own transaction so
	// attendance records and the session's held+confirmed status commit
	// atomically.
	attendanceSvc := attendance.NewService(attendance.NewRepository(db), enrollmentsSvc, sessionsSvc, txMgr)
	attendance.RegisterRoutes(v1, attendance.NewHandler(attendanceSvc), requireAuth, resolveScope)

	// teaching consumes classes, sessions, and enrollments through consumer
	// interfaces (ClassSource, SessionSource, RosterSource) — class/session
	// resolution doubles as its authorization gates — so all three services
	// must exist first. The marks batch upserts and deletes rows inside one
	// transaction via txMgr.
	teachingSvc := teaching.NewService(teaching.NewRepository(db), classesSvc, sessionsSvc, enrollmentsSvc, txMgr)
	teaching.RegisterRoutes(v1, teaching.NewHandler(teachingSvc), requireAuth, resolveScope)

	// The owner dashboard reads through classes, sessions, and attendance
	// (ClassReader, SessionReader, AttendanceReader), so it mounts here —
	// after all three exist — rather than next to the membership routes.
	centersDashboard := centers.NewDashboard(centers.NewRepository(db), classesSvc, sessionsSvc, attendanceSvc)
	centers.RegisterDashboardRoutes(v1, centers.NewDashboardHandler(centersDashboard), requireAuth, resolveScope)

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

	statements.RegisterRoutes(v1, statements.NewHandler(statementsSvc), requireAuth, resolveScope)

	notifications.RegisterRoutes(v1, notifications.NewHandler(notificationsSvc), requireAuth, resolveScope)

	zalo.RegisterRoutes(v1, zalo.NewHandler(zaloSvc), requireAuth, resolveScope)

	// invitations consumes zaloSvc through invitations.ZaloSender (its
	// LookupPhone adapter) for the best-effort DM, teachersSvc as
	// AccountOnboarder and centersSvc as MembershipOpener for the accept flow
	// (both already exist by this point, so these are plain constructor
	// parameters, not setters — no construction cycle to break), and reuses
	// Statements.PublicBaseURL rather than a second base-URL env var.
	invitationsSvc := invitations.NewService(
		invitations.NewRepository(db), zaloSvc, teachersSvc, centersSvc, txMgr, cfg.Onboarding, cfg.Statements.PublicBaseURL, bus)
	invitationsHandler := invitations.NewHandler(invitationsSvc)
	invitations.RegisterRoutes(v1, invitationsHandler, requireAuth, resolveScope)
	// The token is the only credential guarding an accept — these two routes
	// are the sole unauthenticated write surface in the product, so each gets
	// its own per-token rate limit keyed on the request body, not the caller's
	// IP (an invitee is not authenticated yet, so IP is the only other option
	// and is far easier to spoof/rotate than the random token itself).
	invitations.RegisterPublicRoutes(v1, invitationsHandler,
		middleware.RateLimit(middleware.JSONBodyKey("token"), 20, time.Minute),
		middleware.RateLimit(middleware.JSONBodyKey("token"), 10, time.Minute))
}
