package centers

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"teka/apps/api/internal/shared/authctx"
)

// directoryRepo satisfies Repository by embedding it: only ListMembers is
// implemented, so any other call would panic loudly rather than pass silently.
type directoryRepo struct {
	Repository
	gotCenterID uuid.UUID
	rows        []MemberRow
	err         error
}

func (r *directoryRepo) ListMembers(_ context.Context, centerID uuid.UUID) ([]MemberRow, error) {
	r.gotCenterID = centerID
	return r.rows, r.err
}

func TestMemberIDsByPhoneKeysOnStorageForm(t *testing.T) {
	t.Parallel()
	teacher, other := uuid.New(), uuid.New()
	repo := &directoryRepo{rows: []MemberRow{
		// ListMembers reads user_accounts.phone, which is stored E.164; the
		// local spelling is included to prove normalisation is applied to the
		// stored side too, not only to the caller's lookup key.
		{ID: teacher, FullName: "Nguyễn Văn Nam", Phone: "+84912345678"},
		{ID: other, FullName: "Trần Thị Lan", Phone: "0987654321"},
	}}
	svc := NewService(repo, nil)
	scope := authctx.Scope{TeacherID: teacher, CenterID: uuid.New(), IsOwner: true}

	dir, err := svc.MemberIDsByPhone(context.Background(), scope)
	require.NoError(t, err)

	require.Equal(t, scope.CenterID, repo.gotCenterID,
		"the directory must be read for the caller's own center, never one from input")
	require.Equal(t, teacher, dir["+84912345678"])
	require.Equal(t, other, dir["+84987654321"],
		"a locally-spelled stored phone resolves under its E.164 key")
	require.Len(t, dir, 2)
}

func TestMemberIDsByPhoneOmitsNonMembers(t *testing.T) {
	t.Parallel()
	// ListMembers is already filtered to this center and to active accounts,
	// so a teacher from another center or a removed member simply never
	// appears. The absence IS the authorization result — there is no second
	// check to forget.
	repo := &directoryRepo{rows: nil}
	svc := NewService(repo, nil)

	dir, err := svc.MemberIDsByPhone(context.Background(), authctx.Scope{CenterID: uuid.New()})
	require.NoError(t, err)
	require.Empty(t, dir)
	require.NotNil(t, dir, "an empty center yields an empty map, not nil")
}

func TestMemberIDsByPhoneRejectsSharedPhone(t *testing.T) {
	t.Parallel()
	// uq_users_phone should make this unreachable. If it ever happens, keeping
	// the last row would anchor imported rows on an arbitrary one of two
	// teachers, so the call fails instead of guessing.
	repo := &directoryRepo{rows: []MemberRow{
		{ID: uuid.New(), Phone: "0912345678"},
		{ID: uuid.New(), Phone: "+84912345678"},
	}}
	svc := NewService(repo, nil)

	_, err := svc.MemberIDsByPhone(context.Background(), authctx.Scope{CenterID: uuid.New()})
	require.Error(t, err)
}

func TestMemberIDsByPhoneToleratesRepeatedRowForSameTeacher(t *testing.T) {
	t.Parallel()
	id := uuid.New()
	repo := &directoryRepo{rows: []MemberRow{
		{ID: id, Phone: "0912345678"},
		{ID: id, Phone: "+84912345678"},
	}}
	svc := NewService(repo, nil)

	dir, err := svc.MemberIDsByPhone(context.Background(), authctx.Scope{CenterID: uuid.New()})
	require.NoError(t, err, "the same teacher under both spellings is not a conflict")
	require.Equal(t, id, dir["+84912345678"])
}

func TestMemberIDsByPhonePropagatesRepositoryFailure(t *testing.T) {
	t.Parallel()
	repo := &directoryRepo{err: errors.New("boom")}
	svc := NewService(repo, nil)

	_, err := svc.MemberIDsByPhone(context.Background(), authctx.Scope{CenterID: uuid.New()})
	require.Error(t, err)
}
