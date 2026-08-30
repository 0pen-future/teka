package grading

import "errors"

var (
	// ErrScoreSetNotFound covers a missing set, a soft-deleted one, or one in
	// another center — indistinguishable to the caller by design.
	ErrScoreSetNotFound = errors.New("score set not found")
	// ErrClassNotFound covers both a missing class and another teacher's, same
	// as the classes feature's own 404 — resolution is the read gate.
	ErrClassNotFound = errors.New("class not found")
	// ErrSessionNotFound mirrors sessions.ErrNotFound normalised into this
	// package's 404 contract.
	ErrSessionNotFound = errors.New("session not found")
	// ErrOwnerOnly rejects score-set CRUD and class assign/clear for non-owner
	// members — the owner gate is a plain Scope.IsOwner check, no perm key.
	ErrOwnerOnly = errors.New("owner-only action")
	// ErrClassHasScores blocks assigning a different set to (or clearing the
	// set of) a class that already carries ≥1 student score: the snapshot's
	// components are the scores' parents, so replacing them would silently
	// cascade-delete recorded grades.
	ErrClassHasScores = errors.New("class already has recorded scores")
)
