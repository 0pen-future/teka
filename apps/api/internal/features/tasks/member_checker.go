package tasks

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"teka/apps/api/internal/database"
	"teka/apps/api/pkg/kanban"
)

// memberChecker implements kanban.MemberChecker against center_members. It
// queries that table directly rather than going through the centers
// package: "is this a live member of this center" is a narrow, self-
// contained read this package owns the calling side of (CreateTask/
// UpdateTask's assignee validation), and centers exposes no port for it.
type memberChecker struct {
	db *gorm.DB
}

// newMemberChecker builds the GORM-backed kanban.MemberChecker.
func newMemberChecker(db *gorm.DB) *memberChecker {
	return &memberChecker{db: db}
}

var _ kanban.MemberChecker = (*memberChecker)(nil)

// IsMember reports whether actor currently holds a live (not left)
// membership stint in tenant.
func (c *memberChecker) IsMember(ctx context.Context, tenant kanban.TenantID, actor kanban.ActorID) (bool, error) {
	var exists bool
	err := database.FromContext(ctx, c.db).
		Raw(`SELECT EXISTS (SELECT 1 FROM center_members WHERE center_id = ? AND teacher_id = ? AND left_at IS NULL)`,
			uuid.UUID(tenant), uuid.UUID(actor)).
		Scan(&exists).Error
	return exists, err
}
