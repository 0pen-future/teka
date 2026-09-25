// Package classprogram links a class to one published program template
// version. Applying a program copies the version's lesson titles into the
// class curriculum (owned by teaching); removing it drops only the link, so
// the curriculum and every lesson plan survive. It is a coordinating feature
// over classes (the read gate), teaching (the curriculum) and library (the
// published version), each driven through a consumer-defined interface.
package classprogram

import (
	"context"
	"errors"
	"slices"

	"github.com/google/uuid"

	"teka/apps/api/internal/database"
	"teka/apps/api/internal/features/classes"
	"teka/apps/api/internal/features/library"
	"teka/apps/api/internal/features/teaching"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
)

// ClassSource is the slice of classes this feature drives: resolving the
// class under the caller's read port (owner, primary teacher, or any staff
// stint). *classes.Service satisfies it.
type ClassSource interface {
	GetReadable(ctx context.Context, sc authctx.Scope, classID uuid.UUID) (*classes.Class, error)
}

// CurriculumStore is the slice of teaching this feature drives: reading and
// whole-replacing the class curriculum. *teaching.Service satisfies it.
type CurriculumStore interface {
	GetCurriculum(ctx context.Context, sc authctx.Scope, classID uuid.UUID) (*teaching.CurriculumResponse, error)
	// LockCurriculum locks the class's curriculum row inside Apply's
	// transaction so its CURRICULUM_DIFFERS decision re-reads under the lock
	// instead of racing a snapshot taken before the transaction opened.
	LockCurriculum(ctx context.Context, sc authctx.Scope, classID uuid.UUID) error
	PutCurriculum(ctx context.Context, sc authctx.Scope, classID uuid.UUID, req teaching.PutCurriculumRequest) (*teaching.CurriculumResponse, error)
}

// LibrarySource is the slice of library this feature drives, without the
// library.read gate: PublishedVersion gates what may be applied, while
// ReleasedVersion also accepts an archived version so a class keeps reading
// the program it already follows. *library.Service satisfies it.
type LibrarySource interface {
	PublishedVersion(ctx context.Context, sc authctx.Scope, versionID uuid.UUID) (*library.VersionResponse, []library.LessonDetailResponse, error)
	ReleasedVersion(ctx context.Context, sc authctx.Scope, versionID uuid.UUID) (*library.VersionResponse, []library.LessonDetailResponse, error)
	// LockTemplateForVersion locks the version's template row inside Apply's
	// transaction so applying a version serialises with a concurrent
	// DeleteTemplate of the same template instead of racing it.
	LockTemplateForVersion(ctx context.Context, sc authctx.Scope, versionID uuid.UUID) error
}

// Service applies and removes class programs.
type Service struct {
	repo      Repository
	classes   ClassSource
	curricula CurriculumStore
	library   LibrarySource
	tx        database.TxManager
}

// NewService wires the repository and its collaborators.
func NewService(repo Repository, classSrc ClassSource, curricula CurriculumStore, lib LibrarySource, tx database.TxManager) *Service {
	return &Service{repo: repo, classes: classSrc, curricula: curricula, library: lib, tx: tx}
}

// Get returns the class's applied program, or nil when it applies none.
func (s *Service) Get(ctx context.Context, sc authctx.Scope, classID uuid.UUID) (*ProgramResponse, error) {
	if err := s.resolveClass(ctx, sc, classID); err != nil {
		return nil, err
	}
	row, err := s.repo.Get(ctx, sc, classID)
	if err != nil {
		return nil, apperror.Internal(err)
	}
	if row == nil {
		return nil, nil
	}
	return programResponse(row), nil
}

// Lessons returns the applied version's lessons with their materials and
// exercises, or an empty list when the class applies no program.
func (s *Service) Lessons(ctx context.Context, sc authctx.Scope, classID uuid.UUID) ([]library.LessonDetailResponse, error) {
	if err := s.resolveClass(ctx, sc, classID); err != nil {
		return nil, err
	}
	row, err := s.repo.Get(ctx, sc, classID)
	if err != nil {
		return nil, apperror.Internal(err)
	}
	if row == nil {
		return []library.LessonDetailResponse{}, nil
	}
	_, lessons, err := s.library.ReleasedVersion(ctx, sc, row.TemplateVersionID)
	if err != nil {
		return nil, err
	}
	return lessons, nil
}

// Apply links the class to a published version and copies its lesson titles
// into the class curriculum. When the class already keeps a different,
// non-empty lesson list the call stops with CURRICULUM_DIFFERS until the
// owner confirms; the curriculum pointer (current_index) is kept either way.
// Owner only — the manifest classifies the route the same way.
func (s *Service) Apply(ctx context.Context, sc authctx.Scope, classID uuid.UUID, req ApplyRequest) (*ProgramResponse, error) {
	if err := s.resolveClass(ctx, sc, classID); err != nil {
		return nil, err
	}
	if !sc.IsOwner {
		return nil, errOwnerOnly()
	}
	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		// Lock the template before reading the version: a concurrent
		// DeleteTemplate either waits for this transaction or has already
		// removed the template by the time the lock is granted, so
		// PublishedVersion below never applies a version whose template is
		// mid-delete.
		if err := s.library.LockTemplateForVersion(ctx, sc, req.TemplateVersionID); err != nil {
			return err
		}
		_, lessons, err := s.library.PublishedVersion(ctx, sc, req.TemplateVersionID)
		if err != nil {
			return err
		}
		if len(lessons) > teaching.MaxCurriculumLessons {
			return errTooManyLessons(len(lessons))
		}
		titles := make([]string, 0, len(lessons))
		for _, l := range lessons {
			titles = append(titles, l.Title)
		}
		// Lock the curriculum row before re-reading it: the CURRICULUM_DIFFERS
		// decision below must see the class's latest committed lesson list,
		// not a snapshot taken before this transaction opened.
		if err := s.curricula.LockCurriculum(ctx, sc, classID); err != nil {
			return err
		}
		current, err := s.curricula.GetCurriculum(ctx, sc, classID)
		if err != nil {
			return err
		}
		if len(current.Lessons) > 0 && !slices.Equal(current.Lessons, titles) && !req.Confirm {
			return errCurriculumDiffers(len(current.Lessons), len(titles))
		}
		if err := s.repo.Upsert(ctx, &Program{
			ClassID:           classID,
			CenterID:          sc.CenterID,
			TemplateVersionID: req.TemplateVersionID,
			AppliedBy:         sc.TeacherID,
		}); err != nil {
			return apperror.Internal(err)
		}
		_, err = s.curricula.PutCurriculum(ctx, sc, classID, teaching.PutCurriculumRequest{
			Lessons:      titles,
			CurrentIndex: current.CurrentIndex,
		})
		return err
	})
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, sc, classID)
}

// Remove drops the class's program link. The curriculum and lesson plans are
// untouched. Owner only; 404 when the class applies no program.
func (s *Service) Remove(ctx context.Context, sc authctx.Scope, classID uuid.UUID) error {
	if err := s.resolveClass(ctx, sc, classID); err != nil {
		return err
	}
	if !sc.IsOwner {
		return errOwnerOnly()
	}
	removed, err := s.repo.Delete(ctx, sc, classID)
	if err != nil {
		return apperror.Internal(err)
	}
	if !removed {
		return apperror.NotFound("class program")
	}
	return nil
}

// resolveClass is the read gate: a class outside the caller's reach (other
// center, or no stint on it) is indistinguishable from a missing one.
func (s *Service) resolveClass(ctx context.Context, sc authctx.Scope, classID uuid.UUID) error {
	_, err := s.classes.GetReadable(ctx, sc, classID)
	if err == nil {
		return nil
	}
	var appErr *apperror.AppError
	if errors.As(err, &appErr) {
		return appErr
	}
	if errors.Is(err, classes.ErrNotFound) {
		return apperror.NotFound("class")
	}
	return apperror.Internal(err)
}
