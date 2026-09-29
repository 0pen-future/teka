//go:build integration

package collections_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"teka/apps/api/internal/features/collections"
	"teka/apps/api/internal/features/payments"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/testutil"
)

// Billing periods are per teacher, so one family can owe on several periods
// of the same month. The monthly read sums them per contact, leaves out other
// months, other centers and settled families, and is gated on billing.view_all.
func TestContactOutstandingByMonthSumsEveryPeriodOfTheMonth(t *testing.T) {
	t.Parallel()
	collectionsSvc, billingSvc, paymentsSvc, db := newIntegrationDeps(t)
	ctx := context.Background()

	owner, _ := testutil.Teacher(t, db)
	member, _ := testutil.Teacher(t, db)
	ownerScope := testutil.ScopeFor(t, db, owner.ID)
	testutil.JoinCenter(t, db, member.ID, ownerScope.CenterID)
	memberScope := testutil.ScopeFor(t, db, member.ID)

	// The owing family has a child with each teacher in January (2 + 1
	// sessions) and another child billed in February.
	owing := testutil.Contact(t, db, owner.ID, testutil.WithContactFullName("Owing"))
	seedChild(t, db, owner.ID, owing.ID, "OwnerJan", date("2026-01-01"), 2)
	seedChild(t, db, member.ID, owing.ID, "MemberJan", date("2026-01-01"), 1)
	seedChild(t, db, owner.ID, owing.ID, "OwnerFeb", date("2026-02-01"), 1)

	// The settled family pays its January invoice in full.
	settled := testutil.Contact(t, db, owner.ID, testutil.WithContactFullName("Settled"))
	seedChild(t, db, owner.ID, settled.ID, "SettledJan", date("2026-01-01"), 1)

	// Another center's family owes in the same month.
	outsider, _ := testutil.Teacher(t, db)
	outsiderScope := testutil.ScopeFor(t, db, outsider.ID)
	require.NotEqual(t, ownerScope.CenterID, outsiderScope.CenterID)
	outsiderContact := testutil.Contact(t, db, outsider.ID)
	seedChild(t, db, outsider.ID, outsiderContact.ID, "OutsiderJan", date("2026-01-01"), 1)

	closePeriod := func(sc authctx.Scope, year, month int) uuid.UUID {
		t.Helper()
		period, err := billingSvc.EnsurePeriod(ctx, sc, year, month)
		require.NoError(t, err)
		_, err = billingSvc.Close(ctx, sc, period.ID)
		require.NoError(t, err)
		return period.ID
	}
	closePeriod(ownerScope, 2026, 1)
	closePeriod(memberScope, 2026, 1)
	closePeriod(outsiderScope, 2026, 1)

	// Settle before February closes, so nothing of January carries into it
	// as an opening balance for the settled family.
	_, err := paymentsSvc.Record(ctx, ownerScope, payments.RecordPaymentRequest{
		ContactID: settled.ID, Amount: 100_000, Method: payments.MethodCash, ReceivedOn: "2026-01-20",
	})
	require.NoError(t, err)
	febPeriodID := closePeriod(ownerScope, 2026, 2)

	rows, err := collectionsSvc.ContactOutstandingByMonth(ctx, ownerScope, 2026, 1)
	require.NoError(t, err)
	require.Equal(t, []collections.ContactOutstanding{{ContactID: owing.ID, Outstanding: 300_000}}, rows,
		"both teachers' January periods add up; February, the settled family and the other center stay out")

	// February reads February's own invoices only (whatever January left
	// unpaid is carried there as an opening balance, as on the board).
	var febOutstanding int64
	require.NoError(t, db.Raw(`SELECT COALESCE(SUM(total_due - paid_amount), 0) FROM invoices
		WHERE period_id = ? AND contact_id = ? AND status <> 'void'`, febPeriodID, owing.ID).Scan(&febOutstanding).Error)
	require.Positive(t, febOutstanding)
	feb, err := collectionsSvc.ContactOutstandingByMonth(ctx, ownerScope, 2026, 2)
	require.NoError(t, err)
	require.Equal(t, []collections.ContactOutstanding{{ContactID: owing.ID, Outstanding: febOutstanding}}, feb)

	empty, err := collectionsSvc.ContactOutstandingByMonth(ctx, ownerScope, 2025, 6)
	require.NoError(t, err)
	require.NotNil(t, empty, "a month with no period is an empty list, not null")
	require.Empty(t, empty)

	_, err = collectionsSvc.ContactOutstandingByMonth(ctx, memberScope, 2026, 1)
	require.Equal(t, apperror.CodeForbidden, apperror.From(err).Code,
		"a teacher without billing.view_all cannot read center-wide balances")

	granted := memberScope
	granted.Perms = authctx.BuildPermSet(nil, []string{authctx.PermBillingViewAll}, nil)
	viaGrant, err := collectionsSvc.ContactOutstandingByMonth(ctx, granted, 2026, 1)
	require.NoError(t, err)
	require.Equal(t, rows, viaGrant, "billing.view_all reads the same center-wide figures as the owner")
}

// The query parameters are bound and validated at the handler: an
// out-of-range or missing value is a 422, a non-numeric one a 400, and a
// valid month returns the enveloped list.
func TestContactBalancesHandlerValidatesTheMonth(t *testing.T) {
	t.Parallel()
	collectionsSvc, _, _, db := newIntegrationDeps(t)

	owner, _ := testutil.Teacher(t, db)
	ownerScope := testutil.ScopeFor(t, db, owner.ID)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	collections.RegisterRoutes(r.Group("/api/v1"), collections.NewHandler(collectionsSvc),
		func(c *gin.Context) { authctx.SetScope(c, ownerScope) })

	get := func(query string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/collections/contact-balances"+query, nil))
		return w
	}

	for _, q := range []string{"?year=2026&month=13", "?year=2026&month=0", "?year=2026", "?month=1", "?year=1999&month=1"} {
		w := get(q)
		require.Equal(t, http.StatusUnprocessableEntity, w.Code, "query %q", q)
	}
	require.Equal(t, http.StatusBadRequest, get("?year=abc&month=1").Code,
		"a non-numeric value is a malformed request, not a range error")

	w := get("?year=2026&month=1")
	require.Equal(t, http.StatusOK, w.Code)
	var body struct {
		Data []struct {
			ContactID   uuid.UUID `json:"contact_id"`
			Outstanding int64     `json:"outstanding"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.NotNil(t, body.Data, "an empty month serializes as [], not null")
	require.Empty(t, body.Data)
}
