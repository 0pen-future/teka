// Package handoff reassigns a class to another teacher in the same center. It
// is a coordinating feature that owns no tables: an owner's request moves the
// class and its schedules (owned by classes) together with the class's future
// planned sessions (owned by sessions) in one transaction, driving each feature
// through a consumer-defined interface — the same shape imports uses.
//
// It lives outside classes deliberately: sessions is constructed with classes
// as a dependency (router wiring), so classes cannot depend on sessions. A
// class handoff needs both, so it sits above them and is wired after both
// exist.
package handoff

import (
	"context"

	"github.com/google/uuid"

	"teka/apps/api/internal/database"
	"teka/apps/api/internal/features/classes"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
)

// ClassReassigner is the slice of the classes feature this one drives: reading
// the class under the caller's scope, and moving the class plus its schedule
// rows to a new teacher. *classes.Service satisfies it.
type ClassReassigner interface {
	Get(ctx context.Context, sc authctx.Scope, classID uuid.UUID) (*classes.Class, error)
	ReassignTeacher(ctx context.Context, sc authctx.Scope, classID, newTeacherID uuid.UUID) error
}

// SessionReassigner is the slice of the sessions feature this one drives:
// moving the class's future planned sessions to the new teacher. oldTeacherID
// is the class's current teacher, whose timezone defines the "today" boundary
// for which sessions count as future. *sessions.Service satisfies it.
type SessionReassigner interface {
	ReassignPlanned(ctx context.Context, sc authctx.Scope, classID, oldTeacherID, newTeacherID uuid.UUID) (int64, error)
}

// MemberChecker validates the target teacher against the caller's own center.
// *centers.Service satisfies it. The scope is the authorization boundary: there
// is no cross-center variant, so an owner cannot hand a class to a teacher
// outside their center.
type MemberChecker interface {
	IsActiveMember(ctx context.Context, sc authctx.Scope, teacherID uuid.UUID) (bool, error)
}

// Locker guards a center against a concurrent write. It is the same advisory
// lock imports takes, keyed on the same center — the two features exclude each
// other so a handoff cannot interleave with an in-flight import mid-transaction.
// The concrete locker is shared (see router wiring); this interface is
// consumer-defined so the feature stays decoupled from imports at compile time.
type Locker interface {
	TryLockCenter(ctx context.Context, centerID uuid.UUID) (bool, error)
	SetStatementTimeout(ctx context.Context) error
}

// Result is the outcome of a successful handoff: the class and its new teacher,
// plus how many future planned sessions moved with it.
type Result struct {
	ClassID              uuid.UUID
	TeacherID            uuid.UUID
	MovedPlannedSessions int64
}

// Service coordinates a class handoff. It owns no repository — every write goes
// through the feature that owns the table.
type Service struct {
	classes  ClassReassigner
	sessions SessionReassigner
	members  MemberChecker
	locker   Locker
	tx       database.TxManager
}

// NewService builds the handoff service.
func NewService(
	classSvc ClassReassigner,
	sessionSvc SessionReassigner,
	memberChecker MemberChecker,
	locker Locker,
	tx database.TxManager,
) *Service {
	return &Service{
		classes:  classSvc,
		sessions: sessionSvc,
		members:  memberChecker,
		locker:   locker,
		tx:       tx,
	}
}

// Reassign hands classID to newTeacherID. Only the owner may do it; the target
// must be an active member of the same center. Handing a class to its current
// teacher is an idempotent no-op. Everything else — the class, its schedules,
// and its future planned sessions — moves in one transaction under the center
// lock; held/cancelled and past planned sessions, attendance, and billing keep
// the old teacher.
func (s *Service) Reassign(ctx context.Context, sc authctx.Scope, classID, newTeacherID uuid.UUID) (*Result, error) {
	if !sc.IsOwner {
		return nil, apperror.Forbidden("chỉ chủ trung tâm được bàn giao lớp")
	}

	// Fetch under the owner's scope: a class outside the center is a clean 404,
	// and the read gives us the current teacher to detect the no-op.
	class, err := s.classes.Get(ctx, sc, classID)
	if err != nil {
		return nil, err
	}

	if newTeacherID == class.TeacherID {
		// Already this teacher's class: nothing to move, and no membership
		// check — the current teacher owns it regardless of their present
		// roster status, so re-affirming it must never fail.
		return &Result{ClassID: classID, TeacherID: newTeacherID}, nil
	}

	member, err := s.members.IsActiveMember(ctx, sc, newTeacherID)
	if err != nil {
		return nil, err
	}
	if !member {
		return nil, apperror.Invalid("giáo viên này không thuộc trung tâm của bạn",
			map[string]string{"teacher_id": "không thuộc trung tâm"})
	}

	var moved int64
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		// Same center key as imports: refuse rather than wait, so one tenant's
		// slow import cannot park a pooled connection the handoff is blocked on.
		locked, err := s.locker.TryLockCenter(ctx, sc.CenterID)
		if err != nil {
			return err
		}
		if !locked {
			return apperror.Conflict("một thao tác khác của trung tâm đang chạy; thử lại sau")
		}
		if err := s.locker.SetStatementTimeout(ctx); err != nil {
			return err
		}
		if err := s.classes.ReassignTeacher(ctx, sc, classID, newTeacherID); err != nil {
			return err
		}
		// class.TeacherID is the pre-handoff teacher (read before the tx); its
		// timezone anchors the future-session boundary.
		moved, err = s.sessions.ReassignPlanned(ctx, sc, classID, class.TeacherID, newTeacherID)
		return err
	})
	if err != nil {
		return nil, err
	}

	return &Result{
		ClassID:              classID,
		TeacherID:            newTeacherID,
		MovedPlannedSessions: moved,
	}, nil
}
