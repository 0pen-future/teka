// Package id generates UUIDv7 primary keys for domain rows. The domain schema
// declares bare UUID PRIMARY KEY columns with no DB-side default, so every
// insert must supply its id from here. Version 7 keys sort by creation time,
// which keeps B-tree index locality good under append-heavy load.
package id

import "github.com/google/uuid"

// New returns a fresh UUIDv7.
func New() uuid.UUID { return uuid.Must(uuid.NewV7()) }

// NewString returns a fresh UUIDv7 in canonical string form.
func NewString() string { return New().String() }
