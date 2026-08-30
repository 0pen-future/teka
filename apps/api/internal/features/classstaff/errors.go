package classstaff

import "errors"

// ErrNotFound marks an assignment that does not exist under the caller's
// scope; the service maps it onto the API 404 contract.
var ErrNotFound = errors.New("staff assignment not found")
