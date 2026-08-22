//go:build integration

package statements_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/pagination"
	"teka/apps/api/internal/testutil"
)

// TestOwnerGeneratesAndReadsMembersStatementsAnchoredOnMember proves owner
// oversight end to end: an owner can generate and read a member's closed-period
// statements through the scoped path with real content coming back, but the
// created rows stay anchored on the member's own teacher/center — never
// reassigned to the generating owner.
func TestOwnerGeneratesAndReadsMembersStatementsAnchoredOnMember(t *testing.T) {
	t.Parallel()
	statementsSvc, billingSvc, db := newIntegrationDeps(t)
	ctx := context.Background()
	owner, _ := testutil.Teacher(t, db)
	member, _ := testutil.Teacher(t, db)
	ownerScope := testutil.ScopeFor(t, db, owner.ID)
	testutil.JoinCenter(t, db, member.ID, ownerScope.CenterID)
	memberScope := testutil.ScopeFor(t, db, member.ID)
	require.Equal(t, ownerScope.CenterID, memberScope.CenterID, "member must have joined the owner's center")

	contact := testutil.Contact(t, db, member.ID, testutil.WithContactFullName("Member Contact"))
	seedChild(t, db, member.ID, contact.ID, "OwnerOversight", date("2026-01-01"), 1)

	period, err := billingSvc.EnsurePeriod(ctx, memberScope, 2026, 1)
	require.NoError(t, err)
	_, err = billingSvc.Close(ctx, memberScope, period.ID)
	require.NoError(t, err)

	result, err := statementsSvc.Generate(ctx, ownerScope, period.ID)
	require.NoError(t, err, "owner must be able to generate a member's closed-period statements")
	require.Equal(t, 1, result.Created)
	require.Len(t, result.Statements, 1)
	require.Equal(t, contact.ID, result.Statements[0].ContactID)
	require.EqualValues(t, 100_000, result.Statements[0].TotalDue, "generated statement must carry the member's real total, not a blank")

	rows, total, err := statementsSvc.List(ctx, ownerScope, period.ID, pagination.Params{Page: 1, PerPage: 20})
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, rows, 1)
	require.Equal(t, contact.ID, rows[0].ContactID)
	require.EqualValues(t, 100_000, rows[0].TotalDue)

	got, err := statementsSvc.Get(ctx, ownerScope, rows[0].ID)
	require.NoError(t, err)
	require.Equal(t, contact.ID, got.ContactID)
	require.EqualValues(t, 100_000, got.TotalDue)

	var dbRow struct {
		TeacherID uuid.UUID
		CenterID  uuid.UUID
	}
	require.NoError(t, db.Table("statements").
		Select("teacher_id, center_id").
		Where("id = ?", rows[0].ID).
		Take(&dbRow).Error)
	require.Equal(t, member.ID, dbRow.TeacherID, "a statement the owner generates for a member must stay anchored on the member's own teacher id, not the owner's")
	require.Equal(t, ownerScope.CenterID, dbRow.CenterID)
}

// TestPeerInSameCenterCannotListOrGetAnotherMembersStatements proves center
// scope grants the owner oversight, not peer-to-peer access: two non-owning
// members in the same center stay isolated from each other's statements.
func TestPeerInSameCenterCannotListOrGetAnotherMembersStatements(t *testing.T) {
	t.Parallel()
	statementsSvc, billingSvc, db := newIntegrationDeps(t)
	ctx := context.Background()
	owner, _ := testutil.Teacher(t, db)
	memberA, _ := testutil.Teacher(t, db)
	memberB, _ := testutil.Teacher(t, db)
	ownerCenter := testutil.ScopeFor(t, db, owner.ID).CenterID
	testutil.JoinCenter(t, db, memberA.ID, ownerCenter)
	testutil.JoinCenter(t, db, memberB.ID, ownerCenter)
	scopeA := testutil.ScopeFor(t, db, memberA.ID)
	scopeB := testutil.ScopeFor(t, db, memberB.ID)

	contact := testutil.Contact(t, db, memberA.ID)
	seedChild(t, db, memberA.ID, contact.ID, "PeerIsolation", date("2026-02-01"), 1)

	period, err := billingSvc.EnsurePeriod(ctx, scopeA, 2026, 2)
	require.NoError(t, err)
	_, err = billingSvc.Close(ctx, scopeA, period.ID)
	require.NoError(t, err)

	result, err := statementsSvc.Generate(ctx, scopeA, period.ID)
	require.NoError(t, err)
	require.Len(t, result.Statements, 1)
	statementID := result.Statements[0].ID

	_, _, err = statementsSvc.List(ctx, scopeB, period.ID, pagination.Params{})
	require.Error(t, err)
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code, "a peer must not list another member's statements")

	_, err = statementsSvc.Get(ctx, scopeB, statementID)
	require.Error(t, err)
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code, "a peer must not read another member's statement")
}

// TestCrossCenterStatementIsNotFoundButPublicTokenStillResolves proves a
// teacher in a different center is refused with the same neutral 404 as a
// missing resource on every authenticated path, and that the unauthenticated
// public token path — which derives its scope from the loaded statement row,
// never the caller — still resolves the real statement after the re-key.
func TestCrossCenterStatementIsNotFoundButPublicTokenStillResolves(t *testing.T) {
	t.Parallel()
	statementsSvc, billingSvc, db := newIntegrationDeps(t)
	ctx := context.Background()
	teacherA, _ := testutil.Teacher(t, db)
	teacherB, _ := testutil.Teacher(t, db)
	scopeA := testutil.ScopeFor(t, db, teacherA.ID)
	scopeB := testutil.ScopeFor(t, db, teacherB.ID)

	contact := testutil.Contact(t, db, teacherA.ID)
	seedChild(t, db, teacherA.ID, contact.ID, "CrossCenter", date("2026-03-01"), 1)

	period, err := billingSvc.EnsurePeriod(ctx, scopeA, 2026, 3)
	require.NoError(t, err)
	_, err = billingSvc.Close(ctx, scopeA, period.ID)
	require.NoError(t, err)

	result, err := statementsSvc.Generate(ctx, scopeA, period.ID)
	require.NoError(t, err)
	require.Len(t, result.Statements, 1)
	row := result.Statements[0]

	_, err = statementsSvc.Generate(ctx, scopeB, period.ID)
	require.Error(t, err)
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code, "a teacher in another center must not see this period at all")

	_, _, err = statementsSvc.List(ctx, scopeB, period.ID, pagination.Params{})
	require.Error(t, err)
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code)

	_, err = statementsSvc.Get(ctx, scopeB, row.ID)
	require.Error(t, err)
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code)

	// The public token path must still resolve correctly after the re-key:
	// scope derived from the loaded statement row's own anchors, never the
	// (absent) caller's, so token issuance and cross-center scoping never
	// interact.
	token := tokenOf(t, statementsSvc, row)
	stmt, err := statementsSvc.LookupPublic(ctx, token)
	require.NoError(t, err)
	payload, _, err := statementsSvc.RenderPublic(ctx, stmt)
	require.NoError(t, err)
	require.Len(t, payload.Children, 1)
	require.EqualValues(t, 100_000, payload.Totals.TotalDue)
}
