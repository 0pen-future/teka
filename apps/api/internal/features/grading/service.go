package grading

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"teka/apps/api/internal/database"
	"teka/apps/api/internal/features/classes"
	"teka/apps/api/internal/features/enrollments"
	"teka/apps/api/internal/features/sessions"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/id"
)

// maxScoreEntries bounds one score batch: a class roster (~30) times its
// components (≤10) is ~300, so 500 leaves headroom while refusing an unbounded
// write.
const maxScoreEntries = 500

// ClassSource is the slice of the classes feature grading needs: resolving one
// class under the caller's scope — the read/authz gate (non-owners cannot
// resolve another teacher's class). *classes.Service satisfies this.
type ClassSource interface {
	// Get is the write gate: own classes only for a member. Score-set
	// assignment and clearing resolve through it.
	Get(ctx context.Context, sc authctx.Scope, classID uuid.UUID) (*classes.Class, error)
	// GetReadable is the read port: classes the caller holds a class_staff
	// stint on (ended included) or sees center-wide.
	GetReadable(ctx context.Context, sc authctx.Scope, classID uuid.UUID) (*classes.Class, error)
}

// SessionSource is the slice of the sessions feature grading needs: resolving
// one session under the caller's scope. *sessions.Service satisfies this.
type SessionSource interface {
	// GetWritable is the write gate — PutSessionScores resolves through it
	// with the scores capability, so only staff whose active role carries
	// that capability (or the owner) reach the score write path. Readable but
	// not writable resolves to 403, unreadable to 404.
	GetWritable(ctx context.Context, sc authctx.Scope, sessionID uuid.UUID, capability authctx.ClassCapability) (*sessions.Session, error)
	// GetReadableByID is the read port — the score-grid GET resolves through
	// it, so any staff assignment on the class can read scores.
	GetReadableByID(ctx context.Context, sc authctx.Scope, sessionID uuid.UUID) (*sessions.Session, error)
}

// RosterSource is the slice of the enrollments feature grading needs: the
// students enrolled in a class on a given date — a new score cell refuses a
// student who was not on the session's roster, mirroring teaching.PutMarks.
// *enrollments.Service satisfies this.
type RosterSource interface {
	ActiveOn(ctx context.Context, sc authctx.Scope, classID uuid.UUID, on time.Time) ([]enrollments.Enrollment, error)
}

// Service owns the grading rules: owner-curated score-set templates, the
// per-class snapshot taken at assignment, and the per-student component scores
// teachers and the owner enter in the classbook.
type Service struct {
	repo     Repository
	classes  ClassSource
	sessions SessionSource
	roster   RosterSource
	tx       database.TxManager
}

// NewService wires the grading service to its dependencies.
func NewService(repo Repository, classSource ClassSource, sessionSource SessionSource, roster RosterSource, tx database.TxManager) *Service {
	return &Service{repo: repo, classes: classSource, sessions: sessionSource, roster: roster, tx: tx}
}

// ─── Score sets (owner CRUD) ────────────────────────────────────────────────

// ListSets returns the center's live score sets with their component names.
// Owner only — the sets are a center-configuration surface.
func (s *Service) ListSets(ctx context.Context, sc authctx.Scope) ([]ScoreSetResponse, error) {
	if !sc.IsOwner {
		return nil, ownerOnly()
	}
	sets, err := s.repo.ListSets(ctx, sc)
	if err != nil {
		return nil, apperror.Internal(err)
	}
	ids := make([]uuid.UUID, len(sets))
	for i, set := range sets {
		ids[i] = set.ID
	}
	components, err := s.repo.ListComponentsForSets(ctx, ids)
	if err != nil {
		return nil, apperror.Internal(err)
	}
	bySet := make(map[uuid.UUID][]string, len(sets))
	for _, comp := range components {
		bySet[comp.SetID] = append(bySet[comp.SetID], comp.Name)
	}
	out := make([]ScoreSetResponse, len(sets))
	for i, set := range sets {
		names := bySet[set.ID]
		if names == nil {
			names = []string{}
		}
		out[i] = ScoreSetResponse{ID: set.ID, Name: set.Name, Components: names}
	}
	return out, nil
}

// CreateSet inserts a named set and its components in one transaction. Owner
// only; a duplicate live name in the center is a 409.
func (s *Service) CreateSet(ctx context.Context, sc authctx.Scope, req ScoreSetRequest) (*ScoreSetResponse, error) {
	if !sc.IsOwner {
		return nil, ownerOnly()
	}
	names, msg := normalizeComponentNames(req.Components)
	if msg != "" {
		return nil, componentInvalid(msg)
	}
	set := &ScoreSet{ID: id.New(), CenterID: sc.CenterID, Name: req.Name}
	components := buildSetComponents(set.ID, names)
	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.repo.CreateSet(ctx, set); err != nil {
			return err
		}
		return s.repo.ReplaceSetComponents(ctx, set.ID, components)
	})
	if err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return nil, duplicateSetName()
		}
		return nil, apperror.Internal(err)
	}
	return &ScoreSetResponse{ID: set.ID, Name: set.Name, Components: names}, nil
}

// UpdateSet renames a set and whole-replaces its components. Owner only. The
// per-class snapshots copied earlier are untouched — that is the whole point of
// the two-tier design.
func (s *Service) UpdateSet(ctx context.Context, sc authctx.Scope, setID uuid.UUID, req ScoreSetRequest) (*ScoreSetResponse, error) {
	if !sc.IsOwner {
		return nil, ownerOnly()
	}
	names, msg := normalizeComponentNames(req.Components)
	if msg != "" {
		return nil, componentInvalid(msg)
	}
	set, err := s.repo.GetSet(ctx, sc, setID)
	if err != nil {
		return nil, apperror.Internal(err)
	}
	if set == nil {
		return nil, scoreSetNotFound()
	}
	set.Name = req.Name
	components := buildSetComponents(set.ID, names)
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.repo.UpdateSet(ctx, sc, set); err != nil {
			return err
		}
		return s.repo.ReplaceSetComponents(ctx, set.ID, components)
	})
	if err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return nil, duplicateSetName()
		}
		return nil, apperror.Internal(err)
	}
	return &ScoreSetResponse{ID: set.ID, Name: set.Name, Components: names}, nil
}

// DeleteSet soft-deletes a set. Owner only. Assigned classes keep their
// snapshot; source_set_id on those snapshots stays valid (soft delete, so the
// FK is never violated).
func (s *Service) DeleteSet(ctx context.Context, sc authctx.Scope, setID uuid.UUID) error {
	if !sc.IsOwner {
		return ownerOnly()
	}
	set, err := s.repo.GetSet(ctx, sc, setID)
	if err != nil {
		return apperror.Internal(err)
	}
	if set == nil {
		return scoreSetNotFound()
	}
	if err := s.repo.SoftDeleteSet(ctx, sc, setID); err != nil {
		return apperror.Internal(err)
	}
	return nil
}

// ─── Class snapshot (owner assign/clear + shared read) ──────────────────────

// AssignScoreSet snapshots a set onto a class: it copies the set's components
// into class_score_components, replacing whatever the class had. Owner only. A
// class that already carries any score refuses with 409 — replacing the
// components would cascade-delete recorded grades.
func (s *Service) AssignScoreSet(ctx context.Context, sc authctx.Scope, classID, setID uuid.UUID) (*ClassComponentsResponse, error) {
	if !sc.IsOwner {
		return nil, ownerOnly()
	}
	class, err := s.resolveClass(ctx, sc, classID)
	if err != nil {
		return nil, err
	}
	set, err := s.repo.GetSet(ctx, sc, setID)
	if err != nil {
		return nil, apperror.Internal(err)
	}
	if set == nil {
		return nil, scoreSetNotFound()
	}
	srcComponents, err := s.repo.ListComponentsForSets(ctx, []uuid.UUID{setID})
	if err != nil {
		return nil, apperror.Internal(err)
	}
	snapshot := make([]ClassComponent, len(srcComponents))
	for i, comp := range srcComponents {
		src := setID
		snapshot[i] = ClassComponent{
			ID:          id.New(),
			ClassID:     class.ID,
			CenterID:    class.CenterID,
			Name:        comp.Name,
			Position:    comp.Position,
			SourceSetID: &src,
		}
	}
	// Lock → guard → replace, all in one tx: the guard's "no scores yet" reading
	// and the cascade-deleting replace must be atomic against a concurrent score
	// write, or a grade committed in the gap is silently cascade-deleted.
	if err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.repo.LockClassForScoring(ctx, classID); err != nil {
			return err
		}
		if err := s.guardNoScores(ctx, sc, classID); err != nil {
			return err
		}
		return s.repo.ReplaceClassComponents(ctx, classID, snapshot)
	}); err != nil {
		return nil, txError(err)
	}
	return classComponentsResponse(classID, snapshot), nil
}

// ClearScoreSet removes a class's snapshot — the fix for a wrong assignment.
// Owner only, and gated by the same no-scores guard: clearing the components
// would cascade-delete any recorded grades.
func (s *Service) ClearScoreSet(ctx context.Context, sc authctx.Scope, classID uuid.UUID) error {
	if !sc.IsOwner {
		return ownerOnly()
	}
	if _, err := s.resolveClass(ctx, sc, classID); err != nil {
		return err
	}
	// Same lock → guard → replace atomicity as AssignScoreSet: clearing the
	// snapshot cascade-deletes student_scores, so the guard and the delete must
	// be serialised against a concurrent score write.
	if err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.repo.LockClassForScoring(ctx, classID); err != nil {
			return err
		}
		if err := s.guardNoScores(ctx, sc, classID); err != nil {
			return err
		}
		return s.repo.ReplaceClassComponents(ctx, classID, nil)
	}); err != nil {
		return txError(err)
	}
	return nil
}

// GetClassComponents returns a class's snapshot components — the shared read
// behind both the owner config page and the classbook score grid. Read gate is
// readable class resolution: the class's own teacher, any center-wide reader,
// and any class_staff assignment holder (ended included) see it; a member with
// no relationship to the class gets the class's own 404.
func (s *Service) GetClassComponents(ctx context.Context, sc authctx.Scope, classID uuid.UUID) (*ClassComponentsResponse, error) {
	if _, err := s.resolveReadableClass(ctx, sc, classID); err != nil {
		return nil, err
	}
	components, err := s.repo.GetClassComponents(ctx, sc, classID)
	if err != nil {
		return nil, apperror.Internal(err)
	}
	return classComponentsResponse(classID, components), nil
}

// ─── Student scores (teacher-or-owner write, session read) ──────────────────

// GetSessionScores returns the class's component columns and every recorded
// cell for the session, in one round-trip the grid rebuilds from. Read gate is
// readable session resolution (same widening as the class read).
func (s *Service) GetSessionScores(ctx context.Context, sc authctx.Scope, sessionID uuid.UUID) (*SessionScoresResponse, error) {
	session, err := s.resolveReadableSession(ctx, sc, sessionID)
	if err != nil {
		return nil, err
	}
	components, err := s.repo.GetClassComponents(ctx, sc, session.ClassID)
	if err != nil {
		return nil, apperror.Internal(err)
	}
	scores, err := s.repo.ListScoresBySession(ctx, sc, sessionID)
	if err != nil {
		return nil, apperror.Internal(err)
	}
	out := &SessionScoresResponse{
		Components: make([]ClassComponentResponse, len(components)),
		Scores:     make([]ScoreResponse, len(scores)),
	}
	for i, comp := range components {
		out.Components[i] = ClassComponentResponse{ID: comp.ID, Name: comp.Name, Position: comp.Position}
	}
	for i, row := range scores {
		out.Scores[i] = ScoreResponse{StudentID: row.StudentID, ComponentID: row.ComponentID, Score: row.Score}
	}
	return out, nil
}

// PutSessionScores merges a batch of per-cell entries into the session's score
// rows. Per entry, a value upserts the cell and null deletes it; the table
// never holds empty cells. Returns the session's full score set after the
// write so the client can reconcile.
//
// WRITE GATE — the capability model decides who writes: the owner
// (center-wide scope) always passes, and a member passes only with an active
// class_staff stint whose role carries the scores capability. Who actually
// entered a score is traced through the audit log, not a column, so
// PUT /sessions/:id/scores must stay registered in audit/action.go.
func (s *Service) PutSessionScores(ctx context.Context, sc authctx.Scope, sessionID uuid.UUID, entries []ScoreEntryRequest) ([]ScoreResponse, error) {
	session, err := s.resolveSession(ctx, sc, sessionID)
	if err != nil {
		return nil, err
	}
	if err := validateScoreEntries(entries); err != nil {
		return nil, err
	}

	// One tx, taking the per-class lock first: the component set this write
	// validates against is exactly the set a concurrent assign/clear would swap,
	// so the read, the validation, and the write must all see the same snapshot.
	// Losing the race yields a clean 422 (the client's stale component ids no
	// longer belong to the class) instead of an FK 500 or a silent cascade.
	var out []ScoreResponse
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.repo.LockClassForScoring(ctx, session.ClassID); err != nil {
			return err
		}

		components, err := s.repo.GetClassComponents(ctx, sc, session.ClassID)
		if err != nil {
			return err
		}
		validComponent := make(map[uuid.UUID]bool, len(components))
		for _, comp := range components {
			validComponent[comp.ID] = true
		}
		for _, entry := range entries {
			if !validComponent[entry.ComponentID] {
				return apperror.Invalid("validation failed", map[string]string{
					"scores": fmt.Sprintf("component %s does not belong to this class", entry.ComponentID),
				})
			}
		}

		existing, err := s.repo.ListScoresBySession(ctx, sc, sessionID)
		if err != nil {
			return err
		}
		byKey := make(map[componentKey]StudentScore, len(existing))
		scoredStudents := make(map[uuid.UUID]bool, len(existing))
		for _, row := range existing {
			byKey[componentKey{row.ComponentID, row.StudentID}] = row
			scoredStudents[row.StudentID] = true
		}

		// Roster gate mirrors teaching.PutMarks: a NEW cell requires the student
		// to have been on the session's roster; a student who already has any
		// recorded score stays correctable/clearable after their enrollment
		// ends, so a wrong grade never becomes immutable history.
		roster, err := s.roster.ActiveOn(ctx, sc, session.ClassID, session.SessionDate)
		if err != nil {
			return err
		}
		enrolled := make(map[uuid.UUID]bool, len(roster))
		for _, enrollment := range roster {
			enrolled[enrollment.StudentID] = true
		}
		for _, entry := range entries {
			if _, existed := byKey[componentKey{entry.ComponentID, entry.StudentID}]; existed {
				continue
			}
			if !enrolled[entry.StudentID] && !scoredStudents[entry.StudentID] {
				return apperror.Invalid("validation failed", map[string]string{
					"scores": fmt.Sprintf("student %s was not on the session's roster", entry.StudentID),
				})
			}
		}

		var upserts []StudentScore
		var deletes []uuid.UUID
		for _, entry := range entries {
			key := componentKey{entry.ComponentID, entry.StudentID}
			row, existed := byKey[key]
			if entry.Score == nil {
				if existed {
					deletes = append(deletes, row.ID)
				}
				continue
			}
			if !existed {
				row = StudentScore{
					ID:          id.New(),
					ClassID:     session.ClassID,
					SessionID:   sessionID,
					ComponentID: entry.ComponentID,
					StudentID:   entry.StudentID,
					// teacher_id/center_id anchor the row in the session's own
					// teacher and center even when the owner is the writer — the
					// audit log records who actually entered it.
					TeacherID: session.TeacherID,
					CenterID:  session.CenterID,
				}
			}
			row.Score = *entry.Score
			upserts = append(upserts, row)
		}

		if err := s.repo.UpsertScores(ctx, upserts); err != nil {
			return err
		}
		if err := s.repo.DeleteScores(ctx, sc, deletes); err != nil {
			return err
		}

		current, err := s.repo.ListScoresBySession(ctx, sc, sessionID)
		if err != nil {
			return err
		}
		out = make([]ScoreResponse, len(current))
		for i, row := range current {
			out[i] = ScoreResponse{StudentID: row.StudentID, ComponentID: row.ComponentID, Score: row.Score}
		}
		return nil
	})
	if err != nil {
		return nil, txError(err)
	}
	return out, nil
}

// ─── Helpers ────────────────────────────────────────────────────────────────

// txError normalises an error escaping WithinTx: a domain *AppError raised
// inside the closure (the 409 guard, a 422 validation) passes through as-is; a
// raw repo/driver error becomes a 500. Lets the closures return typed errors
// without every caller unwrapping.
func txError(err error) error {
	if err == nil {
		return nil
	}
	var appErr *apperror.AppError
	if errors.As(err, &appErr) {
		return appErr
	}
	return apperror.Internal(err)
}

// guardNoScores blocks assign/clear when the class already has ≥1 score.
func (s *Service) guardNoScores(ctx context.Context, sc authctx.Scope, classID uuid.UUID) error {
	has, err := s.repo.ClassHasScores(ctx, sc, classID)
	if err != nil {
		return apperror.Internal(err)
	}
	if has {
		return classHasScores()
	}
	return nil
}

// validateScoreEntries bounds the batch shape: within the entry cap, no
// duplicate (student, component) cell (two would race inside one statement),
// and every value on the 0–10 scale. Decimal precision is left to the
// NUMERIC(4,1) column, exactly as session_marks does.
func validateScoreEntries(entries []ScoreEntryRequest) error {
	if len(entries) > maxScoreEntries {
		return apperror.Invalid("validation failed", map[string]string{
			"scores": fmt.Sprintf("batch of %d entries exceeds the %d-entry limit", len(entries), maxScoreEntries),
		})
	}
	seen := make(map[componentKey]bool, len(entries))
	for _, entry := range entries {
		key := componentKey{entry.ComponentID, entry.StudentID}
		if seen[key] {
			return apperror.Invalid("validation failed", map[string]string{
				"scores": fmt.Sprintf("student %s / component %s appears more than once", entry.StudentID, entry.ComponentID),
			})
		}
		seen[key] = true
		if entry.Score != nil {
			if score := *entry.Score; score < 0 || score > 10 {
				return apperror.Invalid("validation failed", map[string]string{
					"scores": fmt.Sprintf("score %.1f is outside the 0–10 scale", score),
				})
			}
		}
	}
	return nil
}

// buildSetComponents turns cleaned names into position-ordered rows.
func buildSetComponents(setID uuid.UUID, names []string) []SetComponent {
	out := make([]SetComponent, len(names))
	for i, name := range names {
		if i > 32767 {
			break // gosec: prevent int overflow; position must fit in int16
		}
		out[i] = SetComponent{ID: id.New(), SetID: setID, Name: name, Position: int16(i)} //nolint:gosec
	}
	return out
}

func classComponentsResponse(classID uuid.UUID, components []ClassComponent) *ClassComponentsResponse {
	out := &ClassComponentsResponse{ClassID: classID, Components: make([]ClassComponentResponse, len(components))}
	for i, comp := range components {
		out.Components[i] = ClassComponentResponse{ID: comp.ID, Name: comp.Name, Position: comp.Position}
	}
	return out
}

// resolveClass fetches the class through the ClassSource contract, normalising
// its error shape (a pre-translated *AppError from the real service, or a raw
// classes.ErrNotFound from a fake) into this package's 404 contract.
func (s *Service) resolveClass(ctx context.Context, sc authctx.Scope, classID uuid.UUID) (*classes.Class, error) {
	return normalizeClassErr(s.classes.Get(ctx, sc, classID))
}

// resolveReadableClass is resolveClass on the read port: a class_staff stint
// (active or ended) also resolves. GETs only — writes keep resolveClass.
func (s *Service) resolveReadableClass(ctx context.Context, sc authctx.Scope, classID uuid.UUID) (*classes.Class, error) {
	return normalizeClassErr(s.classes.GetReadable(ctx, sc, classID))
}

func normalizeClassErr(class *classes.Class, err error) (*classes.Class, error) {
	if err != nil {
		var appErr *apperror.AppError
		if errors.As(err, &appErr) {
			return nil, appErr
		}
		if errors.Is(err, classes.ErrNotFound) {
			return nil, classNotFound()
		}
		return nil, apperror.Internal(err)
	}
	return class, nil
}

// resolveSession is resolveClass's session counterpart, on the write port
// with the scores capability.
func (s *Service) resolveSession(ctx context.Context, sc authctx.Scope, sessionID uuid.UUID) (*sessions.Session, error) {
	return normalizeSessionErr(s.sessions.GetWritable(ctx, sc, sessionID, authctx.CapScoresWrite))
}

// resolveReadableSession is resolveSession on the read port: a class_staff
// stint (active or ended) on the session's class also resolves. GETs only.
func (s *Service) resolveReadableSession(ctx context.Context, sc authctx.Scope, sessionID uuid.UUID) (*sessions.Session, error) {
	return normalizeSessionErr(s.sessions.GetReadableByID(ctx, sc, sessionID))
}

func normalizeSessionErr(session *sessions.Session, err error) (*sessions.Session, error) {
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

func ownerOnly() error {
	appErr := apperror.Forbidden("only the center owner can configure score sets")
	appErr.Err = ErrOwnerOnly
	return appErr
}

func scoreSetNotFound() error {
	appErr := apperror.NotFound("score set")
	appErr.Err = ErrScoreSetNotFound
	return appErr
}

func classNotFound() error {
	appErr := apperror.NotFound("class")
	appErr.Err = ErrClassNotFound
	return appErr
}

func sessionNotFound() error {
	appErr := apperror.NotFound("session")
	appErr.Err = ErrSessionNotFound
	return appErr
}

// classHasScores is the 409 that blocks changing a scored class's components.
// The message is Vietnamese: it surfaces directly in the owner's config UI.
func classHasScores() error {
	appErr := apperror.Conflict("lớp đã có điểm thành phần, không thể đổi hoặc gỡ bộ điểm")
	appErr.Err = ErrClassHasScores
	return appErr
}

func duplicateSetName() error {
	return apperror.Conflict("a score set with this name already exists")
}

func componentInvalid(msg string) error {
	return apperror.Invalid("validation failed", map[string]string{"components": msg})
}
