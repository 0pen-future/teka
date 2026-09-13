package tasks

import (
	"errors"

	"gorm.io/gorm"

	"teka/apps/api/pkg/kanban"
)

// errIsRecordNotFound reports whether err is GORM's not-found sentinel — the
// one place this package still names a GORM type directly, since
// kanban.ErrColumnNotFound/ErrTaskNotFound are what every caller above the
// repository layer actually compares against.
func errIsRecordNotFound(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound)
}

// newRepositories builds the kanban.Repositories bundle NewService takes,
// wiring both GORM-backed repositories against the same *gorm.DB.
func newRepositories(db *gorm.DB) kanban.Repositories {
	return kanban.Repositories{
		Columns: newColumnRepository(db),
		Tasks:   newTaskRepository(db),
	}
}
