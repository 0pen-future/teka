package contacts

import "errors"

var (
	// ErrNotFound covers both a missing row and another teacher's row — the
	// caller cannot tell them apart, by design.
	ErrNotFound = errors.New("contact not found")
	// ErrDuplicatePhone surfaces the uq_contacts_phone partial unique index:
	// one phone per teacher among non-deleted contacts.
	ErrDuplicatePhone = errors.New("phone already used by another contact")
	// ErrHasStudents blocks soft-deleting a contact that still has live
	// students referencing it.
	ErrHasStudents = errors.New("contact still has students")
)
