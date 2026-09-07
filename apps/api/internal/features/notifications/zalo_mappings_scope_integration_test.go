//go:build integration

package notifications_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"teka/apps/api/internal/features/notifications"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/testutil"
)

// grantContactsViewAll grants the teacher's contacts.view_all member override
// directly, the same way testutil.GrantSendReports flips reports.send —
// standalone here so a caller can hold the implied key without also holding
// reports.send itself.
func grantContactsViewAll(t *testing.T, db *gorm.DB, teacherID uuid.UUID) {
	t.Helper()
	var row struct{ CenterID uuid.UUID }
	require.NoError(t, db.Raw(
		"SELECT center_id FROM center_members WHERE teacher_id = ? AND left_at IS NULL",
		teacherID).Scan(&row).Error)
	require.NotEqual(t, uuid.Nil, row.CenterID, "fixture teacher must have a live membership")
	require.NoError(t, db.Exec(`
		INSERT INTO center_member_permissions (teacher_id, center_id, permission_key, allowed)
		VALUES (?, ?, ?, TRUE)
		ON CONFLICT (teacher_id, center_id, permission_key) DO UPDATE SET allowed = TRUE`,
		teacherID, row.CenterID, authctx.PermContactsViewAll).Error)
}

// TestZaloMappingsFollowsCenterWideOrStint proves the notifications
// repository's ZaloMappings query — read directly here since every service
// entry point that calls it (BulkSend, SendPreview, ResumeRun) requires
// reports.send oversight first, and reports.send now implies
// contacts.view_all, so a service-level call never exercises the stint arm on
// its own — resolves a contact's Zalo mapping through either of its two
// legitimate arms: contacts.view_all (direct grant or owner), or an active
// hoc_vu stint over one of the contact's actively enrolled students. A caller
// with neither sees nothing, matching the ledger's own phone-visibility rule
// this mirrors.
func TestZaloMappingsFollowsCenterWideOrStint(t *testing.T) {
	t.Parallel()
	d := newDeps(t)
	db := d.db
	ctx := context.Background()

	owner, _ := testutil.Teacher(t, db)
	member, _ := testutil.Teacher(t, db)
	viewAllHolder, _ := testutil.Teacher(t, db)
	stintHolder, _ := testutil.Teacher(t, db)
	stranger, _ := testutil.Teacher(t, db)
	ownerScope := testutil.ScopeFor(t, db, owner.ID)
	center := ownerScope.CenterID
	testutil.JoinCenter(t, db, member.ID, center)
	testutil.JoinCenter(t, db, viewAllHolder.ID, center)
	testutil.JoinCenter(t, db, stintHolder.ID, center)
	testutil.JoinCenter(t, db, stranger.ID, center)
	grantContactsViewAll(t, db, viewAllHolder.ID)

	contact := testutil.Contact(t, db, member.ID)
	classStart := date("2026-03-01")
	homeClass := testutil.Class(t, db, member.ID, testutil.WithClassName("ZaloMappingHome"), testutil.WithClassStartDate(classStart))
	student := testutil.Student(t, db, member.ID, contact.ID, testutil.WithStudentFullName("Zalo-mapping-student"))
	testutil.Enrollment(t, db, member.ID, student.ID, homeClass.ID, classStart)

	// The stint holder reaches this contact only through a stint on a
	// DIFFERENT class where the same student is actively enrolled — the same
	// shape TestLedgerPhoneAndSendGateFollowTheOnePhoneRule uses for the
	// ledger's own phone mask. The class is owned by member (any owner works;
	// what matters is stintHolder holding no *other* active stint on it), so
	// StaffAssignment's hoc_vu insert below is the stint holder's only row.
	stintClass := testutil.Class(t, db, member.ID, testutil.WithClassName("ZaloMappingStint"), testutil.WithClassStartDate(classStart))
	testutil.Enrollment(t, db, stintHolder.ID, student.ID, stintClass.ID, classStart)
	testutil.StaffAssignment(t, db, stintClass, stintHolder.ID, "hoc_vu")

	mapContact(t, db, contact.ID, "uid-zalo-mapping-scope")

	repo := notifications.NewRepository(db)
	contactIDs := []uuid.UUID{contact.ID}

	ownerMappings, err := repo.ZaloMappings(ctx, ownerScope, contactIDs)
	require.NoError(t, err)
	require.Equal(t, "uid-zalo-mapping-scope", ownerMappings[contact.ID], "the owner sees every mapping center-wide")

	viewAllMappings, err := repo.ZaloMappings(ctx, testutil.ScopeFor(t, db, viewAllHolder.ID), contactIDs)
	require.NoError(t, err)
	require.Equal(t, "uid-zalo-mapping-scope", viewAllMappings[contact.ID],
		"a direct contacts.view_all grant (or, transitively, reports.send's implied key) must see the mapping center-wide")

	stintMappings, err := repo.ZaloMappings(ctx, testutil.ScopeFor(t, db, stintHolder.ID), contactIDs)
	require.NoError(t, err)
	require.Equal(t, "uid-zalo-mapping-scope", stintMappings[contact.ID],
		"an active hoc_vu stint over the contact's actively enrolled student must see the mapping")

	strangerMappings, err := repo.ZaloMappings(ctx, testutil.ScopeFor(t, db, stranger.ID), contactIDs)
	require.NoError(t, err)
	_, strangerSees := strangerMappings[contact.ID]
	require.False(t, strangerSees, "a caller with neither contacts.view_all nor an active stint must not see the mapping")
}
