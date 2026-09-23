// Package classchat is the internal chat of a class: short messages between
// the center owner and the staff currently on the class. It is not synced
// to Zalo. Reading and posting need the caller to be the owner or to hold an
// active class_staff stint — a teacher whose stint ended still reads the
// class elsewhere but is out of its chat. Posting also needs the
// class_messages.post permission; retracting is for the author or the owner.
package classchat

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"

	"teka/apps/api/internal/features/classes"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/id"
)

// ClassSource is the slice of classes this feature drives: resolving the
// class under the caller's read port. *classes.Service satisfies it.
type ClassSource interface {
	GetReadable(ctx context.Context, sc authctx.Scope, classID uuid.UUID) (*classes.Class, error)
}

// StaffChecker answers whether a teacher currently holds a stint on the
// class. classstaff.Repository satisfies it structurally.
type StaffChecker interface {
	HasActiveAssignment(ctx context.Context, sc authctx.Scope, classID, teacherID uuid.UUID) (bool, error)
}

// Service reads, posts and retracts class messages.
type Service struct {
	repo    Repository
	classes ClassSource
	staff   StaffChecker
}

// NewService wires the repository and its collaborators.
func NewService(repo Repository, classSrc ClassSource, staff StaffChecker) *Service {
	return &Service{repo: repo, classes: classSrc, staff: staff}
}

// List returns one page of the class's messages, newest first.
func (s *Service) List(ctx context.Context, sc authctx.Scope, classID uuid.UUID, q ListQuery) (*ListResponse, error) {
	if err := s.resolveClass(ctx, sc, classID); err != nil {
		return nil, err
	}
	limit := q.Limit
	if limit <= 0 {
		limit = DefaultLimit
	}
	limit = min(limit, MaxLimit)
	// One extra row tells whether an older page exists without a count.
	rows, err := s.repo.List(ctx, sc, classID, q.Before, limit+1)
	if err != nil {
		return nil, apperror.Internal(err)
	}
	out := &ListResponse{Items: make([]MessageResponse, 0, len(rows))}
	if len(rows) > limit {
		rows = rows[:limit]
		out.NextCursor = rows[limit-1].ID.String()
	}
	for i := range rows {
		out.Items = append(out.Items, messageResponse(&rows[i]))
	}
	return out, nil
}

// Post adds a message to the class. The body is trimmed and must not end up
// blank; the caller needs class_messages.post on top of being on the class.
func (s *Service) Post(ctx context.Context, sc authctx.Scope, classID uuid.UUID, req PostRequest) (*MessageResponse, error) {
	if err := s.resolveClass(ctx, sc, classID); err != nil {
		return nil, err
	}
	if err := authctx.Require(sc, authctx.PermClassMessagesPost); err != nil {
		return nil, err
	}
	body := strings.TrimSpace(req.Body)
	if body == "" {
		return nil, errBlankBody()
	}
	msg := &Message{ID: id.New(), CenterID: sc.CenterID, ClassID: classID, AuthorID: sc.TeacherID, Body: body}
	if err := s.repo.Create(ctx, msg); err != nil {
		return nil, apperror.Internal(err)
	}
	row, err := s.repo.Get(ctx, sc, classID, msg.ID)
	if err != nil {
		return nil, apperror.Internal(err)
	}
	out := messageResponse(row)
	return &out, nil
}

// Delete retracts a message. Only its author or the owner may; a retracted
// or foreign message is not found.
func (s *Service) Delete(ctx context.Context, sc authctx.Scope, classID, messageID uuid.UUID) error {
	if err := s.resolveClass(ctx, sc, classID); err != nil {
		return err
	}
	row, err := s.repo.Get(ctx, sc, classID, messageID)
	if errors.Is(err, ErrNotFound) {
		return apperror.NotFound("class message")
	}
	if err != nil {
		return apperror.Internal(err)
	}
	if !sc.IsOwner && row.AuthorID != sc.TeacherID {
		return errNotAuthor()
	}
	deleted, err := s.repo.SoftDelete(ctx, sc, classID, messageID)
	if err != nil {
		return apperror.Internal(err)
	}
	if !deleted {
		return apperror.NotFound("class message")
	}
	return nil
}

// resolveClass is the chat gate: the class must be readable by the caller
// (else 404, as for a class of another center) and the caller must be the
// owner or currently on the class (else 403).
func (s *Service) resolveClass(ctx context.Context, sc authctx.Scope, classID uuid.UUID) error {
	if _, err := s.classes.GetReadable(ctx, sc, classID); err != nil {
		var appErr *apperror.AppError
		if errors.As(err, &appErr) {
			return appErr
		}
		if errors.Is(err, classes.ErrNotFound) {
			return apperror.NotFound("class")
		}
		return apperror.Internal(err)
	}
	if sc.IsOwner {
		return nil
	}
	active, err := s.staff.HasActiveAssignment(ctx, sc, classID, sc.TeacherID)
	if err != nil {
		return apperror.Internal(err)
	}
	if !active {
		return errNotOnClass()
	}
	return nil
}
