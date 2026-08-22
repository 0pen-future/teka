package teachers

import "errors"

// ErrNotFound is returned when no matching account or teacher exists; the
// service maps it onto the API error contract.
var ErrNotFound = errors.New("teacher not found")

// ErrDuplicatePhone is returned when the partial unique phone index rejects a
// write — the concurrency guard against double registration.
var ErrDuplicatePhone = errors.New("phone already registered")

// ErrNotDisabled is returned by Reactivate when the target account is not
// currently disabled — a defensive invariant check, since callers (the
// invitations accept flow) are expected to have already confirmed disabled
// status before calling.
var ErrNotDisabled = errors.New("teacher account is not disabled")
