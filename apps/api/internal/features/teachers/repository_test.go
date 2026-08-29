//go:build integration

package teachers_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"teka/apps/api/internal/database"
	"teka/apps/api/internal/features/centers"
	"teka/apps/api/internal/features/teachers"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/testutil"
)

// createInCenter mirrors how invitations.Accept actually drives
// AccountOnboarder.CreateInCenter: paired with MembershipOpener.OpenMembership
// inside one ambient transaction. teachers.center_id carries a DEFERRABLE FK
// into center_members (fk_teachers_membership) checked at commit, so calling
// CreateInCenter on its own — outside a transaction that also opens the
// membership — violates that constraint by design; CreateInCenter is not
// meant to be a self-contained membership grant.
func createInCenter(t *testing.T, db *gorm.DB, txMgr database.TxManager, centersSvc *centers.Service, phone, fullName, password string, centerID uuid.UUID) (uuid.UUID, error) {
	t.Helper()
	var accountID uuid.UUID
	err := txMgr.WithinTx(context.Background(), func(ctx context.Context) error {
		teachersSvc := teachers.NewService(teachers.NewRepository(db))
		id, cerr := teachersSvc.CreateInCenter(ctx, phone, fullName, password, centerID)
		if cerr != nil {
			return cerr
		}
		accountID = id
		return centersSvc.OpenMembership(ctx, id, centerID)
	})
	return accountID, err
}

func TestCreateInCenterPhoneFormsCollide(t *testing.T) {
	t.Parallel()
	db := testutil.StartPostgres(t)
	svc := teachers.NewService(teachers.NewRepository(db))
	txMgr := database.NewTxManager(db)
	centersSvc := centers.NewService(centers.NewRepository(db), txMgr, nil)
	ctx := context.Background()
	_, owner := testutil.Teacher(t, db)

	accountID, err := createInCenter(t, db, txMgr, centersSvc, "0901234567", "One", "password-123", owner.CenterID)
	require.NoError(t, err)
	p, err := svc.GetByID(ctx, accountID)
	require.NoError(t, err)
	require.Equal(t, "+84901234567", p.Account.Phone)
	require.Equal(t, p.Account.ID, p.Teacher.ID, "account and teacher must share one id")
	require.Equal(t, owner.CenterID, p.Teacher.CenterID)

	// Both accepted spellings normalize to one stored number, so a second
	// registration must hit the unique index regardless of input form.
	for _, phone := range []string{"0901234567", "+84901234567"} {
		_, err := createInCenter(t, db, txMgr, centersSvc, phone, "Two", "password-123", owner.CenterID)
		require.Equal(t, apperror.CodeConflict, apperror.From(err).Code, "phone form %q", phone)
	}
}

func TestGetByPhoneAcceptsBothFormsAndSkipsSoftDeleted(t *testing.T) {
	t.Parallel()
	db := testutil.StartPostgres(t)
	svc := teachers.NewService(teachers.NewRepository(db))
	ctx := context.Background()

	acct, _ := testutil.Teacher(t, db, testutil.WithPhone("+84912345678"))

	for _, phone := range []string{"+84912345678", "0912345678"} {
		p, err := svc.GetByPhone(ctx, phone)
		require.NoError(t, err, "lookup with %q", phone)
		require.Equal(t, acct.ID, p.Account.ID)
	}

	// Soft-deleting the account must hide it from lookups: the partial unique
	// index frees the phone and the auth flow must not resurrect the account.
	require.NoError(t, db.Delete(&teachers.Account{ID: acct.ID}).Error)
	_, err := svc.GetByPhone(ctx, "+84912345678")
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code)
	_, err = svc.GetByID(ctx, acct.ID)
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code)
}
