package protocol

import "fmt"

// ErrCodeNotLoggedIn is the inner error code Zalo answers with when a request
// rides a session it no longer honours. It is the only code a caller acts on
// by name: it means the credentials are dead, not that this one request went
// wrong.
const ErrCodeNotLoggedIn = -3

// APIError is a Zalo response whose envelope carried a non-zero error code.
// Carrying the code as data lets a caller distinguish "this session is dead"
// (ErrCodeNotLoggedIn) from every other refusal without parsing error text.
// The message deliberately holds nothing but the operation name and the code
// — Zalo's own error strings can quote the request that produced them.
type APIError struct {
	// Op names the request that was refused, e.g. "send".
	Op string
	// Code is Zalo's inner error_code.
	Code int
}

func (e *APIError) Error() string {
	return fmt.Sprintf("zalo_personal: %s error code %d", e.Op, e.Code)
}
