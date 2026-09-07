// Package gorm is a minimal stand-in for gorm.io/gorm, stubbed at its real
// import path so the analyzer's type-based checks resolve against it exactly
// as they do on the real dependency.
package gorm

// Session mirrors gorm.io/gorm's Session options struct; only NewDB matters
// to the analyzer under test.
type Session struct {
	NewDB bool
}

// DB mirrors the handful of *gorm.DB methods the analyzer's testdata cases
// chain calls through.
type DB struct {
	Error error
}

func (d *DB) Where(query string, args ...any) *DB { return d }
func (d *DB) Table(name string) *DB               { return d }
func (d *DB) Session(s *Session) *DB              { return d }
func (d *DB) Unscoped() *DB                       { return d }
func (d *DB) Raw(sql string, values ...any) *DB   { return d }
func (d *DB) Joins(query string, args ...any) *DB { return d }
func (d *DB) Find(dest any) *DB                   { return d }
func (d *DB) Take(dest any) *DB                   { return d }
