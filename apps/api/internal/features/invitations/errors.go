package invitations

import (
	"errors"

	"teka/apps/api/internal/shared/apperror"
)

var (
	// ErrNotFound covers a missing row and another center's row alike — the
	// caller cannot tell them apart, by design (no cross-center leakage).
	ErrNotFound = errors.New("invitation not found")
	// ErrPendingExists surfaces the uq_invitations_pending_phone partial
	// unique index: a concurrent Create for the same (center, phone) already
	// holds the one live pending slot. The service treats this as a race to
	// resolve, not a client error.
	ErrPendingExists = errors.New("a pending invitation for this phone already exists")
)

// errAcceptRejected is the single anti-enumeration outcome Accept returns for
// every rejection branch: unknown/used/expired/revoked token, an account
// that was never a member of the inviting center, or an already-active
// account. Reused as one package-level value (not rebuilt per call) so every
// branch's response body is byte-for-byte identical — a caller must never be
// able to distinguish "wrong token" from "right token, wrong account state"
// by diffing responses.
var errAcceptRejected = apperror.BadRequest("unable to accept this invitation")
