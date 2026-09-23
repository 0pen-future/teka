// Package idset compares id lists as sets, for reorder endpoints that must
// name every child of a parent exactly once.
package idset

import "github.com/google/uuid"

// Same reports whether want and got hold the same ids, each exactly once:
// the check a reorder request must pass before positions are rewritten,
// since a duplicate or a missing id would leave two rows on one position or
// a row without one.
func Same(want, got []uuid.UUID) bool {
	if len(want) != len(got) {
		return false
	}
	seen := make(map[uuid.UUID]bool, len(want))
	for _, id := range want {
		seen[id] = true
	}
	for _, id := range got {
		if !seen[id] {
			return false
		}
		delete(seen, id)
	}
	return len(seen) == 0
}
