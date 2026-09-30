package grading

import "errors"

var (
	// ErrClassNotFound covers both a missing class and another teacher's, same
	// as the classes feature's own 404 — resolution is the read gate.
	ErrClassNotFound = errors.New("class not found")
	// ErrSessionNotFound mirrors sessions.ErrNotFound normalised into this
	// package's 404 contract.
	ErrSessionNotFound = errors.New("session not found")
	// ErrOwnerOnly rejects a non-owner member changing a class's score
	// components — the owner gate is a plain Scope.IsOwner check, no perm key.
	ErrOwnerOnly = errors.New("owner-only action")
)
