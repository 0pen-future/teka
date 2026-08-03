package students

import "errors"

// Domain errors the repository reports; the service translates them onto the
// API error contract.
var (
	ErrNotFound        = errors.New("student not found")
	ErrContactNotOwned = errors.New("contact does not belong to this teacher")
)
