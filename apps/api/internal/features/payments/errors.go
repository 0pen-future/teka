package payments

import "errors"

// ErrPaymentNotFound covers both a missing row and another teacher's — the
// caller cannot tell them apart, by design.
var ErrPaymentNotFound = errors.New("payment not found")
