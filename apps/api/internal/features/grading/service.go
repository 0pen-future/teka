package grading

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

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
	// Get is the write gate: own classes only for a member.
	// SyncTemplateComponents resolves through it.
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

// Service owns the grading rules: the per-class component snapshot copied from
// the applied program template, and the per-student component scores teachers
// and the owner enter in the classbook.
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

// ─── Class snapshot (template copy + shared read) ───────────────────────────

// TemplateScoreGroup is one score group of a program template version as the
// class-program feature hands it over: the group title and its component
// labels in order. grading owns the shape so it never imports library.
type TemplateScoreGroup struct {
	Title  string
	Labels []string
}

// SnapshotOutcome reports what SyncTemplateComponents did to a class's
// snapshot.
type SnapshotOutcome string

// The four SyncTemplateComponents outcomes.
const (
	// SnapshotReplaced: the class had no score, so its snapshot now holds the
	// template's flattened components.
	SnapshotReplaced SnapshotOutcome = "replaced"
	// SnapshotUnchanged: the snapshot already carried the same names in the
	// same order, so nothing was written.
	SnapshotUnchanged SnapshotOutcome = "unchanged"
	// SnapshotKeptScored: the class already carries a score, so its snapshot
	// and grades were left alone.
	SnapshotKeptScored SnapshotOutcome = "kept_scored"
	// SnapshotKeptEmptyTemplate: the template defines no component, so the
	// class keeps whatever snapshot it had.
	SnapshotKeptEmptyTemplate SnapshotOutcome = "kept_empty_template"
)

// maxComponentNameRunes is class_score_components.name's VARCHAR(50) bound,
// which Postgres counts in characters, not bytes.
const maxComponentNameRunes = 50

// componentGroupSeparator joins a group title and a component label once the
// template has more than one non-empty group.
const componentGroupSeparator = " · "

// SyncTemplateComponents copies a program template's score groups into the
// class's snapshot (class_score_components). Owner only; classprogram calls it
// inside its Apply transaction, which the nested WithinTx joins, so a failure
// here rolls the whole apply back.
//
// The snapshot is replaced only while the class has no recorded score:
// replacing it cascade-deletes student_scores, so a scored class keeps its
// components and grades and the apply still succeeds. A template without
// components keeps whatever the class has, and an identical flattened list
// writes nothing. Lock → check → replace run in one tx, the same atomicity the
// score write relies on.
func (s *Service) SyncTemplateComponents(ctx context.Context, sc authctx.Scope, classID uuid.UUID, groups []TemplateScoreGroup) (SnapshotOutcome, error) {
	if !sc.IsOwner {
		return "", ownerOnly()
	}
	class, err := s.resolveClass(ctx, sc, classID)
	if err != nil {
		return "", err
	}
	names := flattenTemplateComponents(groups)
	if len(names) == 0 {
		return SnapshotKeptEmptyTemplate, nil
	}
	var outcome SnapshotOutcome
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.repo.LockClassForScoring(ctx, classID); err != nil {
			return err
		}
		scored, err := s.repo.ClassHasScores(ctx, sc, classID)
		if err != nil {
			return err
		}
		if scored {
			outcome = SnapshotKeptScored
			return nil
		}
		current, err := s.repo.GetClassComponents(ctx, sc, classID)
		if err != nil {
			return err
		}
		if sameComponents(current, names) {
			outcome = SnapshotUnchanged
			return nil
		}
		snapshot := make([]ClassComponent, len(names))
		for i, name := range names {
			snapshot[i] = ClassComponent{
				ID:       id.New(),
				ClassID:  class.ID,
				CenterID: class.CenterID,
				Name:     name,
				Position: int16(i), //nolint:gosec // at most 10 groups × 20 components
			}
		}
		if err := s.repo.ReplaceClassComponents(ctx, classID, snapshot); err != nil {
			return err
		}
		outcome = SnapshotReplaced
		return nil
	})
	if err != nil {
		return "", txError(err)
	}
	return outcome, nil
}

// GetClassComponents returns a class's snapshot components — the read behind
// the classbook score grid. Read gate is
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
	// validates against is exactly the set a concurrent template apply would swap,
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
// inside the closure (a 422 validation) passes through as-is; a
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

// flattenTemplateComponents turns a template's score groups into the ordered
// snapshot names (position = index). A lone non-empty group keeps its bare
// labels; two or more prefix each label with its group title so "Giữa kỳ ·
// Nghe" and "Cuối kỳ · Nghe" stay apart. Each name is cut to the column's 50
// characters, and a name that repeats an earlier one case-insensitively gets a
// " (2)", " (3)", … suffix, its base cut further so the whole still fits.
func flattenTemplateComponents(groups []TemplateScoreGroup) []string {
	nonEmpty := 0
	total := 0
	for _, g := range groups {
		if len(g.Labels) > 0 {
			nonEmpty++
			total += len(g.Labels)
		}
	}
	out := make([]string, 0, total)
	seen := make(map[string]bool, total)
	for _, g := range groups {
		title := strings.TrimSpace(g.Title)
		for _, label := range g.Labels {
			base := strings.TrimSpace(label)
			if nonEmpty > 1 {
				base = title + componentGroupSeparator + base
			}
			name := truncateRunes(base, maxComponentNameRunes)
			for n := 2; seen[strings.ToLower(name)]; n++ {
				suffix := fmt.Sprintf(" (%d)", n)
				name = truncateRunes(base, maxComponentNameRunes-utf8.RuneCountInString(suffix)) + suffix
			}
			seen[strings.ToLower(name)] = true
			out = append(out, name)
		}
	}
	return out
}

// truncateRunes cuts s to at most limit runes, dropping any whitespace the cut
// leaves at the end.
func truncateRunes(s string, limit int) string {
	if utf8.RuneCountInString(s) <= limit {
		return s
	}
	return strings.TrimRight(string([]rune(s)[:limit]), " ")
}

// sameComponents reports whether the class's current snapshot already holds
// exactly names, in order.
func sameComponents(current []ClassComponent, names []string) bool {
	if len(current) != len(names) {
		return false
	}
	for i, comp := range current {
		if comp.Name != names[i] || int(comp.Position) != i {
			return false
		}
	}
	return true
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
	appErr := apperror.Forbidden("only the center owner can change class score components")
	appErr.Err = ErrOwnerOnly
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
