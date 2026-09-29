//go:build integration

package zalo_test

import (
	"context"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"teka/apps/api/internal/features/zalo"
	"teka/apps/api/internal/features/zalo/protocol"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/secrets"
	"teka/apps/api/internal/testutil"
)

// The one phone rule on the zalo match surface: matching phones against Zalo
// necessarily sends them to a third party, so the endpoint stays open only to
// callers who may see phones at all: the owner or a contacts.view_all holder
// (oversight through the key it implies). Everyone else is refused before any
// phone leaves — a class teacher and an active hoc_vu alike.
func TestMatchFriendsScopedFollowsTheOnePhoneRule(t *testing.T) {
	t.Parallel()
	db := testutil.StartPostgres(t)
	ctx := context.Background()

	owner, _ := testutil.Teacher(t, db)
	member, _ := testutil.Teacher(t, db)
	hocVu, _ := testutil.Teacher(t, db)
	ownerScope := testutil.ScopeFor(t, db, owner.ID)
	center := ownerScope.CenterID
	testutil.JoinCenter(t, db, member.ID, center)
	testutil.JoinCenter(t, db, hocVu.ID, center)

	// Contact A's student sits in a class the hoc_vu is assigned to; contact
	// B's student sits in a class they are not. The stint opens neither.
	contactA := testutil.Contact(t, db, member.ID, testutil.WithContactPhone("+84903334444"))
	contactB := testutil.Contact(t, db, member.ID, testutil.WithContactPhone("+84907778888"))
	classA := testutil.Class(t, db, member.ID, testutil.WithClassName("ZaloReachA"))
	classB := testutil.Class(t, db, member.ID, testutil.WithClassName("ZaloReachB"))
	studentA := testutil.Student(t, db, member.ID, contactA.ID)
	studentB := testutil.Student(t, db, member.ID, contactB.ID)
	testutil.Enrollment(t, db, member.ID, studentA.ID, classA.ID, classA.StartDate)
	testutil.Enrollment(t, db, member.ID, studentB.ID, classB.ID, classB.StartDate)
	testutil.StaffAssignment(t, db, classA, hocVu.ID, "hoc_vu")

	repo := zalo.NewRepository(db)
	cipher, err := secrets.New(testCredKey)
	require.NoError(t, err)
	// Both callers hold a linked account so the gate under test is the scope,
	// never a missing link.
	require.NoError(t, repo.Upsert(ctx, account(t, hocVu.ID, `{"imei":"abc","userAgent":"ua"}`)))
	require.NoError(t, repo.Upsert(ctx, account(t, owner.ID, `{"imei":"abc","userAgent":"ua"}`)))
	require.NoError(t, repo.Upsert(ctx, account(t, member.ID, `{"imei":"abc","userAgent":"ua"}`)))

	relogin := func(_ context.Context, sess *protocol.Session, _ protocol.Credentials) error {
		sess.UID = "zalo-uid-1"
		sess.LoginInfo = &protocol.LoginInfo{ZpwServiceMapV3: protocol.ZpwServiceMapV3{
			Chat:    []string{"https://chat.example"},
			Profile: []string{"https://profile.example"},
		}}
		return nil
	}
	var mu sync.Mutex
	var lookedUp []string
	findUser := func(_ context.Context, _ *protocol.Session, phones []string) (map[string]protocol.FoundUser, error) {
		mu.Lock()
		defer mu.Unlock()
		out := make(map[string]protocol.FoundUser, len(phones))
		for _, p := range phones {
			lookedUp = append(lookedUp, p)
			out[p] = protocol.FoundUser{UID: "uid-" + p, DisplayName: "User " + p}
		}
		return out, nil
	}
	friends := func(_ context.Context, _ *protocol.Session) ([]protocol.FriendInfo, error) {
		return nil, nil
	}
	svc := zalo.NewService(repo, cipher, zalo.ServiceOptions{
		Relogin:  relogin,
		FindUser: findUser,
		Friends:  friends,
		Pace:     func(context.Context) {},
	})
	t.Cleanup(svc.Close)

	sawPhones := func() []string {
		mu.Lock()
		defer mu.Unlock()
		out := make([]string, len(lookedUp))
		copy(out, lookedUp)
		return out
	}
	resetSaw := func() {
		mu.Lock()
		defer mu.Unlock()
		lookedUp = nil
	}

	// Neither a plain class teacher nor an active hoc_vu may send anyone's
	// phone to Zalo.
	for name, id := range map[string]uuid.UUID{"giao_vien": member.ID, "hoc_vu": hocVu.ID} {
		_, err = svc.MatchFriendsScoped(ctx, testutil.ScopeFor(t, db, id), []string{"0903334444"})
		require.Equal(t, apperror.CodeForbidden, apperror.From(err).Code,
			"%s must not match phones against Zalo", name)
	}
	require.Empty(t, sawPhones(), "a refused call must not have touched Zalo")

	// contacts.view_all opens it with full reach.
	viewerScope := testutil.ScopeFor(t, db, hocVu.ID)
	viewerScope.Perms = authctx.BuildPermSet(nil, []string{authctx.PermContactsViewAll}, nil)
	rows, err := svc.MatchFriendsScoped(ctx, viewerScope, []string{"0903334444", "0907778888"})
	require.NoError(t, err)
	require.Len(t, rows, 2)
	require.Equal(t, "0903334444", rows[0].Phone, "rows keep request order and echo the phone as sent")
	require.True(t, rows[0].Matched)
	require.True(t, rows[1].Matched)
	require.ElementsMatch(t, []string{"84903334444", "84907778888"}, sawPhones(),
		"phones travel to Zalo in the country-code wire form")

	// The owner holds the key implicitly: every phone is forwarded.
	resetSaw()
	rows, err = svc.MatchFriendsScoped(ctx, ownerScope, []string{"0903334444", "0907778888"})
	require.NoError(t, err)
	require.Len(t, rows, 2)
	require.True(t, rows[0].Matched)
	require.True(t, rows[1].Matched)
	require.ElementsMatch(t, []string{"84903334444", "84907778888"}, sawPhones())
}
