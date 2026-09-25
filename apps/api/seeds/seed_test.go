//go:build integration

package seeds_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"regexp"
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
	).Row().Scan(&demoTeacherID), "demo teacher account not seeded")
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

	var draftLessons int64
	require.NoError(t, db.Raw(`
		SELECT count(*) FROM template_lessons l
		JOIN program_template_versions v ON v.id = l.version_id
		JOIN program_templates t ON t.id = v.template_id
		WHERE t.code = ?`, "MAU-VAN9",
	).Scan(&draftLessons).Error)
	require.Equal(t, int64(4), draftLessons, "MAU-VAN9 must keep its 3 seeded lessons plus the new self-study lesson")

	assertLibraryBankSeeded(t, db)
	assertDraftTemplateV5ContentSeeded(t, db)

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
	require.Equal(t, []string{"courses.edit", "library.edit"}, grants, "demo teacher must hold exactly the menu's two optIn grants")

	var invitationStatuses []string
	require.NoError(t, db.Raw(
		"SELECT status FROM class_invitations WHERE teacher_id = ? ORDER BY status", demoTeacherID,
	).Scan(&invitationStatuses).Error)
	require.Equal(t, []string{"accepted", "pending"}, invitationStatuses, "demo teacher must hold one pending and one accepted invitation")
}

// exerciseCodePattern matches the auto-generated "BT-0001"-style code the
// library service assigns when an exercise is created without one.
var exerciseCodePattern = regexp.MustCompile(`^BT-\d{4}$`)

// assertLibraryBankSeeded checks the center-wide library bank content the
// v5 demo data adds: three materials of the new kinds plus one inactive
// material, and three exercises with an auto-generated code, skill and
// level. Both banks are scoped to the demo center via program_templates,
// the same join pattern the rest of this file uses.
func assertLibraryBankSeeded(t *testing.T, db *gorm.DB) {
	t.Helper()

	var centerID uuid.UUID
	require.NoError(t, db.Raw(
		"SELECT center_id FROM program_templates WHERE code = ? AND deleted_at IS NULL", "MAU-VAN9",
	).Row().Scan(&centerID), "demo center not found via MAU-VAN9")

	var activeKinds []string
	require.NoError(t, db.Raw(
		`SELECT kind FROM library_materials
		 WHERE center_id = ? AND deleted_at IS NULL AND active = TRUE AND kind IN ('video', 'doc', 'note')
		 ORDER BY kind`, centerID,
	).Scan(&activeKinds).Error)
	require.Equal(t, []string{"doc", "note", "video"}, activeKinds, "seed must create one active material of each of the video/doc/note kinds")

	var inactiveMaterials int64
	require.NoError(t, db.Raw(
		"SELECT count(*) FROM library_materials WHERE center_id = ? AND deleted_at IS NULL AND active = FALSE", centerID,
	).Scan(&inactiveMaterials).Error)
	require.Equal(t, int64(1), inactiveMaterials, "seed must create exactly one inactive material")

	var exerciseCodes []string
	require.NoError(t, db.Raw(
		"SELECT code FROM library_exercises WHERE center_id = ? AND deleted_at IS NULL ORDER BY code", centerID,
	).Scan(&exerciseCodes).Error)
	require.Len(t, exerciseCodes, 3, "seed must create exactly three library exercises")
	for _, code := range exerciseCodes {
		require.Regexp(t, exerciseCodePattern, code, "exercise code must follow the auto-generated BT-0001 shape")
	}

	var exercisesWithSkillLevel int64
	require.NoError(t, db.Raw(
		`SELECT count(*) FROM library_exercises
		 WHERE center_id = ? AND deleted_at IS NULL AND skill IS NOT NULL AND level IS NOT NULL`, centerID,
	).Scan(&exercisesWithSkillLevel).Error)
	require.Equal(t, int64(3), exercisesWithSkillLevel, "every seeded exercise must carry a skill and a level")
}

// assertDraftTemplateV5ContentSeeded checks the v5 fields added to the
// MAU-VAN9 draft: one exercise group with one exercise assigned to it, one
// self_study lesson with unit "Unit 1", a two-group score set and a
// student-kind log field.
func assertDraftTemplateV5ContentSeeded(t *testing.T, db *gorm.DB) {
	t.Helper()

	var groupCount int64
	require.NoError(t, db.Raw(`
		SELECT count(*) FROM template_exercise_groups g
		JOIN program_template_versions v ON v.id = g.version_id
		JOIN program_templates t ON t.id = v.template_id
		WHERE t.code = ?`, "MAU-VAN9",
	).Scan(&groupCount).Error)
	require.Equal(t, int64(1), groupCount, "MAU-VAN9 draft must have exactly one exercise group")

	var assignedExercises int64
	require.NoError(t, db.Raw(`
		SELECT count(*) FROM template_lesson_exercises le
		JOIN template_exercise_groups g ON g.id = le.group_id
		JOIN program_template_versions v ON v.id = g.version_id
		JOIN program_templates t ON t.id = v.template_id
		WHERE t.code = ?`, "MAU-VAN9",
	).Scan(&assignedExercises).Error)
	require.Equal(t, int64(1), assignedExercises, "exactly one exercise must be assigned into the exercise group")

	var selfStudyLessons int64
	require.NoError(t, db.Raw(`
		SELECT count(*) FROM template_lessons l
		JOIN program_template_versions v ON v.id = l.version_id
		JOIN program_templates t ON t.id = v.template_id
		WHERE t.code = ? AND l.mode = 'self_study' AND l.unit = 'Unit 1'`, "MAU-VAN9",
	).Scan(&selfStudyLessons).Error)
	require.Equal(t, int64(1), selfStudyLessons, `MAU-VAN9 draft must have one self_study lesson with unit "Unit 1"`)

	var scoreSetJSON string
	require.NoError(t, db.Raw(`
		SELECT v.score_set::text FROM program_template_versions v
		JOIN program_templates t ON t.id = v.template_id
		WHERE t.code = ? AND v.status = 'draft'`, "MAU-VAN9",
	).Row().Scan(&scoreSetJSON))
	var scoreSet []map[string]any
	require.NoError(t, json.Unmarshal([]byte(scoreSetJSON), &scoreSet))
	require.Len(t, scoreSet, 2, "MAU-VAN9 draft must have two score set groups")

	var studentLogFields int64
	require.NoError(t, db.Raw(`
		SELECT count(*) FROM template_log_fields f
		JOIN program_template_versions v ON v.id = f.version_id
		JOIN program_templates t ON t.id = v.template_id
		WHERE t.code = ? AND f.kind = 'student'`, "MAU-VAN9",
	).Scan(&studentLogFields).Error)
	require.Equal(t, int64(1), studentLogFields, "MAU-VAN9 draft must have a student-kind log field")
}
