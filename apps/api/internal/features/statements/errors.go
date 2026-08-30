package statements

import "errors"

var (
	// ErrNotFound covers both a missing statement and another teacher's — the
	// caller cannot tell them apart, by design.
	ErrNotFound = errors.New("statement not found")
	// ErrPeriodNotFound covers both a missing billing period and another
	// teacher's, for the period Generate/List validate against.
	ErrPeriodNotFound = errors.New("billing period not found")
	// ErrClassNotFound covers a class that does not exist in the caller's
	// center, is soft-deleted, or that the caller holds no stint on — the
	// class-scoped statement paths hide all three identically.
	ErrClassNotFound = errors.New("class not found")
)
