//go:build integration

package statements_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/pagination"
	"teka/apps/api/internal/testutil"
)

// A can_send_reports member reads statements center-wide — list, get, and the
// period figures the send flow consumes — but the flag never widens a
// statement WRITE: the standalone generate endpoint and revoke on another
// member's data still answer the same neutral 404 a peer gets.
func TestSecretaryReadsStatementsCenterWideButCannotGenerateOrRevoke(t *testing.T) {
	t.Parallel()
	statementsSvc, billingSvc, db := newIntegrationDeps(t)
	ctx := context.Background()
	owner, _ := testutil.Teacher(t, db)
	member, _ := testutil.Teacher(t, db)
	ownerCenter := testutil.ScopeFor(t, db, owner.ID).CenterID
	testutil.JoinCenter(t, db, member.ID, ownerCenter)
	_, secretary := testutil.Secretary(t, db, ownerCenter)
	memberScope := testutil.ScopeFor(t, db, member.ID)
	secScope := testutil.ScopeFor(t, db, secretary.ID)
	require.True(t, secScope.CanSendReports)
	require.False(t, secScope.IsOwner)

	contact := testutil.Contact(t, db, member.ID)
	seedChild(t, db, member.ID, contact.ID, "SecretaryRead", date("2026-03-01"), 1)
	period, err := billingSvc.EnsurePeriod(ctx, memberScope, 2026, 3)
	require.NoError(t, err)
	_, err = billingSvc.Close(ctx, memberScope, period.ID)
	require.NoError(t, err)

	// The standalone generate endpoint stays owner/self-only: the flag holder
	// is refused with the same neutral 404 before any statement exists.
	_, err = statementsSvc.Generate(ctx, secScope, period.ID)
	require.Error(t, err)
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code,
		"the send-reports flag must not unlock the standalone generate endpoint")

	result, err := statementsSvc.Generate(ctx, memberScope, period.ID)
	require.NoError(t, err)
	require.Len(t, result.Statements, 1)
	statementID := result.Statements[0].ID

	// Center-wide reads: list, get, and the figures the send preview uses.
	rows, total, err := statementsSvc.List(ctx, secScope, period.ID, pagination.Params{Page: 1, PerPage: 20})
	require.NoError(t, err, "a send-reports holder must list any member's statements")
	require.EqualValues(t, 1, total)
	require.Len(t, rows, 1)
	require.Equal(t, contact.ID, rows[0].ContactID)

	got, err := statementsSvc.Get(ctx, secScope, statementID)
	require.NoError(t, err, "a send-reports holder must read any member's statement")
	require.Equal(t, contact.ID, got.ContactID)

	figures, err := statementsSvc.PeriodFigures(ctx, secScope, period.ID)
	require.NoError(t, err, "a send-reports holder must read any member's period figures")
	require.Len(t, figures, 1)
	require.EqualValues(t, 100_000, figures[contact.ID].TotalDue)

	// Revoke stays a write: refused neutrally, and the statement survives.
	err = statementsSvc.Revoke(ctx, secScope, statementID)
	require.Error(t, err)
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code,
		"the send-reports flag must not unlock statement revocation")
	var stmtRow struct {
		RevokedAt *time.Time
	}
	require.NoError(t, db.Table("statements").Select("revoked_at").
		Where("id = ?", statementID).Take(&stmtRow).Error)
	require.Nil(t, stmtRow.RevokedAt, "the refused revoke must not have touched the statement")
}
