//go:build integration

package teachers_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"teka/apps/api/internal/database"
	"teka/apps/api/internal/features/centers"
	"teka/apps/api/internal/features/teachers"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/testutil"
)

// newTeachersService wires the service the way the router does: the centers
// provisioner supplies the personal center CreateTeacher now requires, and
// creation runs inside a transaction (deferred owner FK).
func newTeachersService(t *testing.T, db *gorm.DB) (*teachers.Service, database.TxManager) {
	t.Helper()
	svc := teachers.NewService(teachers.NewRepository(db))
	txMgr := database.NewTxManager(db)
	svc.SetCenterProvisioner(centers.NewService(centers.NewRepository(db), svc, txMgr))
	return svc, txMgr
}

func createTeacher(svc *teachers.Service, txMgr database.TxManager, req teachers.CreateRequest) (*teachers.Profile, error) {
	var p *teachers.Profile
	err := txMgr.WithinTx(context.Background(), func(ctx context.Context) error {
		var err error
		p, err = svc.CreateTeacher(ctx, req)
		return err
	})
	return p, err
}

func TestCreateTeacherPhoneFormsCollide(t *testing.T) {
	t.Parallel()
	db := testutil.StartPostgres(t)
	svc, txMgr := newTeachersService(t, db)

	p, err := createTeacher(svc, txMgr, teachers.CreateRequest{
		Phone: "0901234567", Password: "password-123", FullName: "One",
	})
	require.NoError(t, err)
	require.Equal(t, "+84901234567", p.Account.Phone)
	require.Equal(t, p.Account.ID, p.Teacher.ID, "account and teacher must share one id")

	// Both accepted spellings normalize to one stored number, so the second
	// registration must hit the unique index regardless of input form.
	for _, phone := range []string{"0901234567", "+84901234567"} {
		_, err := createTeacher(svc, txMgr, teachers.CreateRequest{
			Phone: phone, Password: "password-123", FullName: "Two",
		})
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
