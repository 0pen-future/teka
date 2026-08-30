package collections

import (
	"context"

	"github.com/google/uuid"

	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/pagination"
)

// Service validates a request against the caller's own billing period, then
// delegates straight to Repository — this package has nothing to write, so
// there is no transaction manager and no domain invariant beyond tenancy and
// the view/class_id contract.
type Service struct {
	repo Repository
}

// NewService builds a Service over repo.
func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// ListResult holds exactly one of ContactRows or ClassRows, chosen by the
// view that produced it — kept as two typed slices instead of an untyped
// result so callers never need a type assertion.
type ListResult struct {
	ContactRows []ContactBalanceRow
	ClassRows   []ClassCollectionRow
	Total       int64
}

// List validates periodID belongs to sc's tenancy and that view is a known
// value, then dispatches to the matching repository query. view == ViewClass
// additionally requires filter.ClassID.
func (s *Service) List(ctx context.Context, sc authctx.Scope, periodID uuid.UUID, view string, filter Filter, p pagination.Params) (*ListResult, error) {
	if err := s.ensurePeriod(ctx, sc, periodID); err != nil {
		return nil, err
	}

	switch view {
	case ViewContact:
		rows, total, err := s.repo.ContactBalances(ctx, sc, periodID, filter, p)
		if err != nil {
			return nil, err
		}
		// The one phone rule: null the phone unless sc is owner/oversight or
		// the row carries the caller's hoc_vu grant.
		for i := range rows {
			if !sc.PhoneVisible(rows[i].PhoneVisible) {
				rows[i].Phone = nil
			}
		}
		return &ListResult{ContactRows: rows, Total: total}, nil
	case ViewClass:
		if filter.ClassID == nil {
			return nil, apperror.Invalid("validation failed", map[string]string{"class_id": "required when view is class"})
		}
		rows, total, err := s.repo.ClassCollections(ctx, sc, periodID, filter, p)
		if err != nil {
			return nil, err
		}
		return &ListResult{ClassRows: rows, Total: total}, nil
	default:
		return nil, apperror.Invalid("validation failed", map[string]string{"view": "must be contact or class"})
	}
}

// Summary validates periodID belongs to sc's tenancy, then returns the whole
// period's aggregate.
func (s *Service) Summary(ctx context.Context, sc authctx.Scope, periodID uuid.UUID) (*SummaryResponse, error) {
	if err := s.ensurePeriod(ctx, sc, periodID); err != nil {
		return nil, err
	}
	return s.repo.PeriodSummary(ctx, sc, periodID)
}

func (s *Service) ensurePeriod(ctx context.Context, sc authctx.Scope, periodID uuid.UUID) error {
	exists, err := s.repo.PeriodExists(ctx, sc, periodID)
	if err != nil {
		return err
	}
	if !exists {
		return apperror.NotFound("billing period")
	}
	return nil
}
