//go:build integration

package seeds_test

import (
	"context"
	"log/slog"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"teka/apps/api/internal/testutil"
	"teka/apps/api/seeds"
)

func TestRunIsIdempotent(t *testing.T) {
	t.Parallel()
	db := testutil.StartPostgres(t)
	ctx := context.Background()
	log := slog.New(slog.DiscardHandler)

	require.NoError(t, seeds.Run(ctx, db, log))

	var accounts, teachers int64
	require.NoError(t, db.Raw("SELECT count(*) FROM user_accounts").Scan(&accounts).Error)
	require.NoError(t, db.Raw("SELECT count(*) FROM teachers").Scan(&teachers).Error)
	require.Positive(t, accounts, "seed inserted no accounts")
	require.Equal(t, accounts, teachers, "every account needs a matching teachers row")

	assertTeachingMenuSeeded(t, db)

	// Second run must not add or modify anything.
	require.NoError(t, seeds.Run(ctx, db, log))
	var accountsAfter int64
	require.NoError(t, db.Raw("SELECT count(*) FROM user_accounts").Scan(&accountsAfter).Error)
	require.Equal(t, accounts, accountsAfter, "reseed must be a no-op")

	assertTeachingMenuSeeded(t, db)
}

// assertTeachingMenuSeeded checks every row the Giảng dạy menu seed
// (teaching_menu.go) is expected to have created, and is run after both the
// first and the second seeds.Run to also prove the reseed changed nothing.
func assertTeachingMenuSeeded(t *testing.T, db *gorm.DB) {
	t.Helper()

	var demoTeacherID uuid.UUID
	require.NoError(t, db.Raw(
		"SELECT id FROM user_accounts WHERE phone = ?", "+84901000004",
	).Scan(&demoTeacherID).Error)
	require.NotEqual(t, uuid.Nil, demoTeacherID, "demo teacher account not seeded")

	var publishedVersions int64
	require.NoError(t, db.Raw(`
		SELECT count(*) FROM program_template_versions v
		JOIN program_templates t ON t.id = v.template_id
		WHERE t.code = ? AND v.status = 'published'`, "MAU-TOAN8",
	).Scan(&publishedVersions).Error)
	require.Equal(t, int64(1), publishedVersions, "published template MAU-TOAN8 not seeded exactly once")

	var draftTemplates int64
	require.NoError(t, db.Raw(
		"SELECT count(*) FROM program_templates WHERE code = ? AND deleted_at IS NULL", "MAU-VAN9",
	).Scan(&draftTemplates).Error)
	require.Equal(t, int64(1), draftTemplates, "draft template MAU-VAN9 not seeded exactly once")

	var draftPrepStatuses []string
	require.NoError(t, db.Raw(`
		SELECT l.prep_status FROM template_lessons l
		JOIN program_template_versions v ON v.id = l.version_id
		JOIN program_templates t ON t.id = v.template_id
		WHERE t.code = ? ORDER BY l.position`, "MAU-VAN9",
	).Scan(&draftPrepStatuses).Error)
	require.Equal(t, []string{"doing", "review", "todo"}, draftPrepStatuses, "MAU-VAN9 lessons must keep their mixed prep status")

	for _, code := range []string{"TOAN8-CB", "VAN9-NC", "LY7-CB"} {
		var count int64
		require.NoError(t, db.Raw(
			"SELECT count(*) FROM courses WHERE code = ? AND deleted_at IS NULL", code,
		).Scan(&count).Error)
		require.Equal(t, int64(1), count, "course %s not seeded exactly once", code)
	}

	var pathStages int64
	require.NoError(t, db.Raw(`
		SELECT count(*) FROM path_stages s
		JOIN learning_paths p ON p.id = s.path_id
		WHERE p.code = ?`, "LO-TRINH-CB",
	).Scan(&pathStages).Error)
	require.Equal(t, int64(3), pathStages, "learning path LO-TRINH-CB must have exactly 3 stages")

	var linkedClasses int64
	require.NoError(t, db.Raw(`
		SELECT count(*) FROM classes
		WHERE name IN ('Toán 8 - Tối Thứ Ba', 'Văn 9 - Sáng Thứ Bảy', 'Lý 7 - Chiều Thứ Năm')
			AND course_id IS NOT NULL AND deleted_at IS NULL`,
	).Scan(&linkedClasses).Error)
	require.Equal(t, int64(3), linkedClasses, "every seeded class must carry a course_id")

	var appliedPrograms int64
	require.NoError(t, db.Raw(`
		SELECT count(*) FROM class_curricula cc
		JOIN classes c ON c.id = cc.class_id
		WHERE c.name = ?`, "Toán 8 - Tối Thứ Ba",
	).Scan(&appliedPrograms).Error)
	require.Equal(t, int64(1), appliedPrograms, "Toán 8 - Tối Thứ Ba must have the published program applied")

	var grants []string
	require.NoError(t, db.Raw(
		"SELECT permission_key FROM center_member_permissions WHERE teacher_id = ? AND allowed = TRUE ORDER BY permission_key",
		demoTeacherID,
	).Scan(&grants).Error)
	require.Equal(t, []string{"courses.edit", "library.edit", "prep.assign"}, grants, "demo teacher must hold exactly the menu's three optIn grants")

	var invitationStatuses []string
	require.NoError(t, db.Raw(
		"SELECT status FROM class_invitations WHERE teacher_id = ? ORDER BY status", demoTeacherID,
	).Scan(&invitationStatuses).Error)
	require.Equal(t, []string{"accepted", "pending"}, invitationStatuses, "demo teacher must hold one pending and one accepted invitation")
}
