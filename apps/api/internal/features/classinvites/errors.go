package classinvites

import "errors"

// ErrNotFound marks an invitation that does not exist under the caller's
// scope, or is not in the state a transition requires; the service maps it
// onto 404 or 409 depending on what it already knows about the row.
var ErrNotFound = errors.New("class invitation not found")

// Validation error codes the web client branches on.
const (
	// CodeSelfInvite: the owner tried to invite themself.
	CodeSelfInvite = "SELF_INVITE"
	// CodeMemberInactive: the invitee is not an active member of the center.
	CodeMemberInactive = "MEMBER_INACTIVE"
)
