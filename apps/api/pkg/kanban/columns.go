package kanban

import (
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
)

// DefaultColumnSpec describes one column to seed via DefaultColumns. The
// core does not know what names to use — see README.md "Localization
// boundary" — so an adapter supplies its own localized column names.
type DefaultColumnSpec struct {
	Name   string
	IsDone bool
}

// DefaultColumns builds one Column per spec, in order, with Position set to
// its index (0..len(specs)-1) and a fresh ID from idGen. TenantID and
// timestamps are left zero: the caller sets TenantID (and, for adapters
// using database-generated timestamps, drops CreatedAt/UpdatedAt) before
// persisting. It is a free function, not a Service method, because it reads
// nothing from a Service but an id generator — building a whole Service with
// every other port nil just to reach this one helper would make an
// unrelated future change to DefaultColumns a nil-pointer trap for callers
// that never wired those ports.
func DefaultColumns(idGen func() uuid.UUID, specs []DefaultColumnSpec) []Column {
	cols := make([]Column, len(specs))
	for i, spec := range specs {
		cols[i] = Column{
			ID:       ColumnID(idGen()),
			Name:     spec.Name,
			Position: i,
			IsDone:   spec.IsDone,
		}
	}
	return cols
}

// validateName trims name and returns the trimmed form, rejecting it as
// ErrEmptyName when empty after trimming, or ErrInvalidInput when longer than
// maxLen.
func validateName(name string, maxLen int) (string, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "", ErrEmptyName
	}
	if utf8.RuneCountInString(trimmed) > maxLen {
		return "", ErrInvalidInput
	}
	return trimmed, nil
}

// validatePermutation reports ErrInvalidPermutation unless incoming contains
// exactly the same set of ids as current, comparing as sets so a caller
// cannot smuggle in a duplicate id in place of a missing one at the same
// length.
func validatePermutation(current, incoming []ColumnID) error {
	if len(current) != len(incoming) {
		return ErrInvalidPermutation
	}
	want := make(map[ColumnID]struct{}, len(current))
	for _, id := range current {
		want[id] = struct{}{}
	}
	seen := make(map[ColumnID]struct{}, len(incoming))
	for _, id := range incoming {
		if _, ok := want[id]; !ok {
			return ErrInvalidPermutation
		}
		if _, dup := seen[id]; dup {
			return ErrInvalidPermutation
		}
		seen[id] = struct{}{}
	}
	return nil
}
