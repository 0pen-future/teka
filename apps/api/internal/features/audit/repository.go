package audit

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Repository persists audit rows.
type Repository struct {
	db *gorm.DB
}

// NewRepository wires the repository to the shared connection pool.
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// InsertBatch writes all rows in a single multi-row INSERT. An empty batch
// is a no-op so timer-driven flushes cost nothing on idle traffic.
func (r *Repository) InsertBatch(ctx context.Context, rows []Log) error {
	if len(rows) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Create(&rows).Error
}

// likeEscaper neutralizes LIKE metacharacters in the user-supplied action
// prefix so "class_" cannot match "classX".
var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// List returns one page of audit rows visible to the given center, newest
// first, resolving the actor's display name only for the surviving page.
//
// Visibility has two legs, each pushed down as its own keyset-limited
// subquery so a page costs O(limit) index work instead of scanning and
// re-sorting the whole visible trail:
//   - rows stamped with the center, served in order by
//     idx_audit_logs_center_time;
//   - auth rows (center NULL) whose actor was a member of the center when
//     the event occurred. Bounding by the membership window keeps a
//     teacher's login history from following them to their next center and
//     keeps it in the old center's trail after they leave. Served per
//     member by idx_audit_logs_actor via a LATERAL join, so one center's
//     auth volume never taxes another's page.
//
// A failed login has no actor and so never surfaces here.
func (r *Repository) List(ctx context.Context, spec ListSpec) ([]Row, error) {
	query, args := listSQL(spec)
	var rows []Row
	err := r.db.WithContext(ctx).Raw(query, args...).Scan(&rows).Error
	return rows, err
}

// listSQL builds the page query. Caller-supplied filters apply inside both
// visibility legs — before each leg's LIMIT — so a filtered page can never
// miss rows that a leg's probe window would otherwise have cut off. Only
// constant fragments are concatenated; every user value stays a bind
// parameter.
func listSQL(spec ListSpec) (string, []any) {
	var cond strings.Builder
	var condArgs []any
	if spec.ActorID != uuid.Nil {
		cond.WriteString(" AND a.actor_user_id = ?")
		condArgs = append(condArgs, spec.ActorID)
	}
	if spec.ActionPrefix != "" {
		cond.WriteString(` AND a.action LIKE ? ESCAPE '\'`)
		condArgs = append(condArgs, likeEscaper.Replace(spec.ActionPrefix)+"%")
	}
	if !spec.From.IsZero() {
		cond.WriteString(" AND a.occurred_at >= ?")
		condArgs = append(condArgs, spec.From)
	}
	if !spec.To.IsZero() {
		cond.WriteString(" AND a.occurred_at <= ?")
		condArgs = append(condArgs, spec.To)
	}
	if !spec.CursorAt.IsZero() {
		// Row-value comparison keeps the keyset correct across equal
		// timestamps; both indexes end in occurred_at DESC (id DESC) and
		// serve exactly this order.
		cond.WriteString(" AND (a.occurred_at, a.id) < (?, ?)")
		condArgs = append(condArgs, spec.CursorAt, spec.CursorID)
	}
	filters := cond.String()

	query := `
SELECT logs.*, COALESCE(teachers.full_name, '') AS actor_name
FROM (
    (SELECT a.* FROM audit_logs a
     WHERE a.center_id = ?` + filters + `
     ORDER BY a.occurred_at DESC, a.id DESC
     LIMIT ?)
    UNION ALL
    (SELECT m.* FROM center_members cm
     CROSS JOIN LATERAL (
         SELECT a.* FROM audit_logs a
         WHERE a.actor_user_id = cm.teacher_id
           AND a.center_id IS NULL
           AND a.occurred_at >= cm.joined_at
           AND (cm.left_at IS NULL OR a.occurred_at < cm.left_at)` + filters + `
         ORDER BY a.occurred_at DESC, a.id DESC
         LIMIT ?) m
     WHERE cm.center_id = ?)
) logs
LEFT JOIN teachers ON teachers.id = logs.actor_user_id
ORDER BY logs.occurred_at DESC, logs.id DESC
LIMIT ?`

	args := make([]any, 0, 4+2*len(condArgs))
	args = append(args, spec.CenterID)
	args = append(args, condArgs...)
	args = append(args, spec.Limit)
	args = append(args, condArgs...)
	args = append(args, spec.Limit, spec.CenterID, spec.Limit)
	return query, args
}
