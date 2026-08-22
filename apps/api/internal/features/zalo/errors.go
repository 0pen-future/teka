package zalo

import "errors"

// ErrNotLinked reports that the teacher has no Zalo account attached. Callers
// that need a session translate this into "scan a QR code first", not a failure.
var ErrNotLinked = errors.New("zalo: teacher has no linked account")

// ErrLinkExpired reports that stored credentials exist but Zalo no longer
// accepts them — the teacher has to scan a new QR code. It is also what an
// undecryptable blob surfaces as: from the teacher's side the two are the same
// situation with the same fix.
var ErrLinkExpired = errors.New("zalo: linked session expired")

// ErrLinkNotFound reports that a link attempt id is unknown to this process:
// it finished long enough ago to be swept, was superseded by a newer attempt,
// belongs to another teacher, or was never issued.
var ErrLinkNotFound = errors.New("zalo: link attempt not found")
