package attendance

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	"teka/apps/api/internal/database"
	"teka/apps/api/internal/features/enrollments"
	"teka/apps/api/internal/features/sessions"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/id"
)

// RosterSource is the slice of the enrollments feature attendance needs: the
// roster active on a session's date. *enrollments.Service satisfies this.
type RosterSource interface {
	ActiveOn(ctx context.Context, teacherID, classID uuid.UUID, on time.Time) ([]enrollments.Enrollment, error)
}

// SessionStore is the slice of the sessions feature attendance needs:
// resolving one session, and transitioning it to held+confirmed inside the
// caller's transaction. *sessions.Service satisfies this.
type SessionStore interface {
	GetByID(ctx context.Context, teacherID, sessionID uuid.UUID) (*sessions.Session, error)
	MarkHeldAndConfirmed(ctx context.Context, teacherID, sessionID uuid.UUID, at time.Time) error
}

// ReconciliationEntry is one student's carried adjustment produced by a
// post-close reconciliation (plan 04): the invoice it landed on and the
// signed amount posted.
type ReconciliationEntry struct {
	StudentID uuid.UUID `json:"student_id"`
	PeriodID  uuid.UUID `json:"period_id"`
	InvoiceID uuid.UUID `json:"invoice_id"`
	Amount    int64     `json:"amount"`
}

// Reconciliation is what a BillingReconciler call produces: zero or more
// carried adjustments, one per affected student whose delta was non-zero.
// Empty (not nil-checked by callers) means the edit needed no correction —
// either the session's date is still inside an open period, or every
// affected student's live billable count came out exactly as billed.
type Reconciliation struct {
	Adjustments []ReconciliationEntry `json:"adjustments,omitempty"`
}

// BillingReconciler is the slice of the billing feature attendance needs:
// carrying a post-close attendance edit's money delta onto the next open
// period. Declared here, in attendance, rather than in billing, because
// billing already imports attendance for TallyByEnrollment — declaring the
// interface in billing would need billing to import attendance AND
// attendance to import billing to call it, an import cycle. *billing.Service
// satisfies this; attendance itself never imports billing.
type BillingReconciler interface {
	ReconcileSession(ctx context.Context, teacherID, sessionID uuid.UUID) (Reconciliation, error)
}

// Service implements one-touch attendance (PRD R2): reading the roster
// attendance sheet for a session, and confirming it in a single write that
// also transitions the session to held.
type Service struct {
	repo     Repository
	roster   RosterSource
	sessions SessionStore
	tx       database.TxManager
	// reconciler is nil until SetReconciler wires it in router.go. Confirm
	// guards every call on it being non-nil so this package stays usable
	// (e.g. in tests) without billing ever being constructed.
	reconciler BillingReconciler
}

// NewService wires the attendance service to its dependencies.
func NewService(repo Repository, roster RosterSource, sessionStore SessionStore, tx database.TxManager) *Service {
	return &Service{repo: repo, roster: roster, sessions: sessionStore, tx: tx}
}

// SetReconciler wires the billing package's post-close reconciliation in
// after both services exist (router.go: attendance is constructed before
// billing, so this cannot be a NewService parameter). Safe to leave unset —
// Confirm no-ops the reconciliation step when it is nil.
func (s *Service) SetReconciler(r BillingReconciler) {
	s.reconciler = r
}

// TallyByEnrollment passes through the repository's batched attendance tally
// for a date window. This is the one entry point plan 04's billing package
// uses to price a period — it never re-aggregates attendance_records itself.
func (s *Service) TallyByEnrollment(ctx context.Context, teacherID uuid.UUID, from, to time.Time) ([]EnrollmentTally, error) {
	return s.repo.TallyByEnrollment(ctx, teacherID, from, to)
}

// SessionAttendance passes through the repository's already-recorded rows
// for one session. This is plan 04's entry point for discovering which
// students a post-close reconciliation must consider — it never scans
// attendance_records by anything but session_id.
func (s *Service) SessionAttendance(ctx context.Context, teacherID, sessionID uuid.UUID) ([]Record, error) {
	return s.repo.ListBySession(ctx, teacherID, sessionID)
}

// Get returns the attendance sheet for a session: one row per student
// enrolled as of the session date, with a null status if the session has
// never been confirmed.
func (s *Service) Get(ctx context.Context, teacherID, sessionID uuid.UUID) (*Response, error) {
	session, err := s.resolveSession(ctx, teacherID, sessionID)
	if err != nil {
		return nil, err
	}
	return s.buildResponse(ctx, teacherID, session)
}

// Confirm writes one attendance record per roster enrollment (present unless
// listed absent), drops records for students no longer on the roster, and
// transitions the session to held+confirmed — all inside one transaction so
// the write is atomic and re-confirming is idempotent (stable record ids,
// recorded_at preserved, updated_at advanced).
func (s *Service) Confirm(ctx context.Context, teacherID, sessionID uuid.UUID, req ConfirmRequest) (*Response, error) {
	session, err := s.resolveSession(ctx, teacherID, sessionID)
	if err != nil {
		return nil, err
	}
	if session.Status == sessions.StatusCancelled {
		return nil, cancelledConflict()
	}

	roster, err := s.roster.ActiveOn(ctx, teacherID, session.ClassID, session.SessionDate)
	if err != nil {
		return nil, apperror.Internal(err)
	}
	absentSet, err := validateAbsentees(req.AbsentStudentIDs, roster)
	if err != nil {
		return nil, err
	}

	note := trimmedNotePtr(req.Note)
	now := time.Now()
	records := make([]Record, 0, len(roster))
	keepIDs := make([]uuid.UUID, 0, len(roster))
	for _, e := range roster {
		status := StatusPresent
		if absentSet[e.StudentID] {
			status = StatusAbsent
		}
		records = append(records, Record{
			ID:           id.New(),
			TeacherID:    teacherID,
			SessionID:    sessionID,
			StudentID:    e.StudentID,
			EnrollmentID: e.ID,
			Status:       status,
			Billable:     true,
			Note:         note,
			RecordedAt:   now,
			UpdatedAt:    now,
		})
		keepIDs = append(keepIDs, e.StudentID)
	}

	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.repo.UpsertMany(ctx, records); err != nil {
			return err
		}
		// The only legitimate soft delete this package performs: a student
		// removed from the roster since the last confirmation. Absent
		// students stay in keepIDs because they are still enrolled — they
		// are marked status='absent' above, never soft-deleted.
		if err := s.repo.SoftDeleteMissing(ctx, teacherID, sessionID, keepIDs); err != nil {
			return err
		}
		// Runs inside this same transaction via database.FromContext, so the
		// session's held+confirmed transition commits atomically with the
		// attendance rows above.
		return s.sessions.MarkHeldAndConfirmed(ctx, teacherID, sessionID, now)
	})
	if err != nil {
		return nil, translateTxErr(err)
	}

	confirmed, err := s.resolveSession(ctx, teacherID, sessionID)
	if err != nil {
		return nil, err
	}
	resp, err := s.buildResponse(ctx, teacherID, confirmed)
	if err != nil {
		return nil, err
	}

	// Post-close reconciliation (plan 04, D7): this session's attendance is
	// already committed above, so a reconciliation failure here cannot roll
	// that back — it is surfaced as a warning on an otherwise successful
	// response rather than turning the whole confirm into a 5xx. Skipped
	// entirely when billing has not wired itself in (SetReconciler never
	// called), which keeps this package usable standalone.
	if s.reconciler != nil {
		if _, reconErr := s.reconciler.ReconcileSession(ctx, teacherID, sessionID); reconErr != nil {
			warning := "attendance saved, but the automatic billing adjustment for this change could not be applied; review the student's invoice manually"
			resp.Warning = &warning
		}
	}
	return resp, nil
}

// buildResponse assembles the roster + attendance-status read model shared
// by Get and the response of a successful Confirm.
func (s *Service) buildResponse(ctx context.Context, teacherID uuid.UUID, session *sessions.Session) (*Response, error) {
	roster, err := s.roster.ActiveOn(ctx, teacherID, session.ClassID, session.SessionDate)
	if err != nil {
		return nil, apperror.Internal(err)
	}
	studentIDs := make([]uuid.UUID, len(roster))
	for i, e := range roster {
		studentIDs[i] = e.StudentID
	}
	names, err := s.repo.StudentNames(ctx, teacherID, studentIDs)
	if err != nil {
		return nil, apperror.Internal(err)
	}
	records, err := s.repo.ListBySession(ctx, teacherID, session.ID)
	if err != nil {
		return nil, apperror.Internal(err)
	}
	byStudent := make(map[uuid.UUID]Record, len(records))
	for _, r := range records {
		byStudent[r.StudentID] = r
	}

	rows := make([]RowResponse, 0, len(roster))
	for _, e := range roster {
		name := names[e.StudentID]
		row := RowResponse{
			StudentID:    e.StudentID,
			StudentName:  name.FullName,
			DisplayNote:  name.DisplayNote,
			EnrollmentID: e.ID,
			Billable:     true,
		}
		if rec, ok := byStudent[e.StudentID]; ok {
			status := rec.Status
			row.Status = &status
			row.EnrollmentID = rec.EnrollmentID
			row.Billable = rec.Billable
			row.Note = rec.Note
		}
		rows = append(rows, row)
	}

	return &Response{
		SessionID:             session.ID,
		SessionDate:           session.SessionDate.Format(dateLayout),
		Status:                session.Status,
		AttendanceConfirmedAt: session.AttendanceConfirmedAt,
		Rows:                  rows,
	}, nil
}

// resolveSession fetches a session through the SessionStore contract and
// normalises whatever error shape it returns (a pre-translated *AppError
// from the real sessions.Service, or a raw sessions.ErrNotFound from a test
// fake) into this package's 404 contract.
func (s *Service) resolveSession(ctx context.Context, teacherID, sessionID uuid.UUID) (*sessions.Session, error) {
	session, err := s.sessions.GetByID(ctx, teacherID, sessionID)
	if err != nil {
		var appErr *apperror.AppError
		if errors.As(err, &appErr) {
			return nil, appErr
		}
		if errors.Is(err, sessions.ErrNotFound) {
			return nil, sessionNotFound()
		}
		return nil, apperror.Internal(err)
	}
	return session, nil
}

// validateAbsentees dedups the requested absent ids and confirms each is on
// the roster active for the session's date, returning a set for O(1) lookup
// while building records. Any id not on the roster is rejected by name
// rather than silently dropped — a typo must not leave a student billed for
// a session they missed.
func validateAbsentees(ids []uuid.UUID, roster []enrollments.Enrollment) (map[uuid.UUID]bool, error) {
	rosterSet := make(map[uuid.UUID]bool, len(roster))
	for _, e := range roster {
		rosterSet[e.StudentID] = true
	}
	seen := make(map[uuid.UUID]bool, len(ids))
	absent := make(map[uuid.UUID]bool, len(ids))
	var unknown []string
	for _, sid := range ids {
		if seen[sid] {
			continue
		}
		seen[sid] = true
		if !rosterSet[sid] {
			unknown = append(unknown, sid.String())
			continue
		}
		absent[sid] = true
	}
	if len(unknown) > 0 {
		return nil, studentNotEnrolled(unknown)
	}
	return absent, nil
}

// translateTxErr passes through an already-typed AppError (e.g. from
// sessions.MarkHeldAndConfirmed) unchanged, and wraps anything else as an
// internal error.
func translateTxErr(err error) error {
	var appErr *apperror.AppError
	if errors.As(err, &appErr) {
		return appErr
	}
	if errors.Is(err, sessions.ErrNotFound) {
		return sessionNotFound()
	}
	return apperror.Internal(err)
}

func sessionNotFound() error {
	appErr := apperror.NotFound("session")
	appErr.Err = ErrSessionNotFound
	return appErr
}

func cancelledConflict() error {
	appErr := apperror.Conflict("session is cancelled; attendance cannot be confirmed")
	appErr.Err = ErrSessionCancelled
	return appErr
}

func studentNotEnrolled(ids []string) error {
	appErr := apperror.Invalid("validation failed", map[string]string{
		"absent_student_ids": "not enrolled for this session: " + strings.Join(ids, ", "),
	})
	appErr.Err = ErrStudentNotEnrolled
	return appErr
}
