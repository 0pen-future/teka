package invitations

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"teka/apps/api/internal/features/teachers"
	"teka/apps/api/internal/shared/events"
	"teka/apps/api/internal/shared/id"
)

// busRecorder captures every event the service publishes through a SyncBus.
type busRecorder struct {
	events []events.Event
}

func (r *busRecorder) bus() events.Bus {
	b := events.NewSync()
	b.Subscribe("test", 0, func(_ context.Context, e events.Event) {
		r.events = append(r.events, e)
	})
	return b
}

var testClientMeta = ClientMeta{IP: "10.0.0.7", UserAgent: "go-test"}

// commitFailingTxManager runs the transaction body successfully but fails the
// commit — the case that separates publish-after-commit from
// publish-inside-tx.
type commitFailingTxManager struct{}

func (commitFailingTxManager) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error {
	if err := fn(ctx); err != nil {
		return err
	}
	return errors.New("commit failed")
}

// TestAcceptPublishesMemberJoinedForNewAccount proves a successful accept of a
// brand-new phone emits exactly one MemberJoined tying the created account to
// the inviting center and the redeemed invitation.
func TestAcceptPublishesMemberJoinedForNewAccount(t *testing.T) {
	t.Parallel()

	rec := &busRecorder{}
	repo := newFakeRepository()
	onboarder := newFakeOnboarder()
	svc := newAcceptTestService(repo, onboarder, &fakeOpener{})
	svc.bus = rec.bus()
	centerID := id.New()
	plaintext, invID := mintInvitation(t, repo, centerID, "+84901234567", StatusPending, time.Now().Add(time.Hour))

	err := svc.Accept(context.Background(), AcceptRequest{Token: plaintext, FullName: "Nguyễn Văn A", Password: "matkhau123"}, testClientMeta)
	require.NoError(t, err)

	require.Len(t, rec.events, 1)
	e, ok := rec.events[0].(MemberJoined)
	require.True(t, ok, "event type = %T, want MemberJoined", rec.events[0])
	require.Equal(t, centerID, e.CenterID)
	require.Equal(t, invID, e.InvitationID)
	require.Len(t, onboarder.created, 1)
	require.Equal(t, onboarder.created[0], e.UserID, "event must carry the created account")
	require.Equal(t, testClientMeta.IP, e.IP)
	require.Equal(t, testClientMeta.UserAgent, e.UserAgent)
	require.False(t, e.OccurredAt.IsZero())
}

// TestAcceptPublishesMemberJoinedForReactivation proves the disabled-account
// reactivation branch publishes too, carrying the reactivated account.
func TestAcceptPublishesMemberJoinedForReactivation(t *testing.T) {
	t.Parallel()

	rec := &busRecorder{}
	repo := newFakeRepository()
	onboarder := newFakeOnboarder()
	centerID := id.New()
	accountID := id.New()
	phone := "+84901234567"
	onboarder.byPhone[phone] = teachers.Account{ID: accountID, Phone: phone, Status: teachers.StatusDisabled}
	svc := newAcceptTestService(repo, onboarder, &fakeOpener{everMember: map[uuid.UUID]bool{accountID: true}})
	svc.bus = rec.bus()
	plaintext, invID := mintInvitation(t, repo, centerID, phone, StatusPending, time.Now().Add(time.Hour))

	err := svc.Accept(context.Background(), AcceptRequest{Token: plaintext, FullName: "Nguyễn Văn A", Password: "matkhau123"}, testClientMeta)
	require.NoError(t, err)

	require.Len(t, rec.events, 1)
	e := rec.events[0].(MemberJoined)
	require.Equal(t, accountID, e.UserID)
	require.Equal(t, centerID, e.CenterID)
	require.Equal(t, invID, e.InvitationID)
}

// TestRejectedAcceptPublishesNothing proves rejection branches stay silent —
// nothing joined, nothing to audit here (the failed HTTP request itself is a
// different concern).
func TestRejectedAcceptPublishesNothing(t *testing.T) {
	t.Parallel()

	rec := &busRecorder{}
	repo := newFakeRepository()
	onboarder := newFakeOnboarder()
	phone := "+84901234567"
	onboarder.byPhone[phone] = teachers.Account{ID: id.New(), Phone: phone, Status: teachers.StatusActive}
	svc := newAcceptTestService(repo, onboarder, &fakeOpener{})
	svc.bus = rec.bus()
	plaintext, _ := mintInvitation(t, repo, id.New(), phone, StatusPending, time.Now().Add(time.Hour))

	require.Error(t, svc.Accept(context.Background(), AcceptRequest{Token: "unknown-token", FullName: "A B", Password: "matkhau123"}, testClientMeta))
	require.Error(t, svc.Accept(context.Background(), AcceptRequest{Token: plaintext, FullName: "A B", Password: "matkhau123"}, testClientMeta))
	require.Empty(t, rec.events)
}

// TestFailedCommitPublishesNothing proves MemberJoined is published strictly
// after the transaction commits — a rolled-back membership must never reach
// the audit trail.
func TestFailedCommitPublishesNothing(t *testing.T) {
	t.Parallel()

	rec := &busRecorder{}
	repo := newFakeRepository()
	svc := NewService(repo, &fakeZaloSender{}, newFakeOnboarder(), &fakeOpener{}, commitFailingTxManager{}, testOnboardingConfig(), "https://app.example.com", nil)
	svc.bus = rec.bus()
	plaintext, _ := mintInvitation(t, repo, id.New(), "+84901234567", StatusPending, time.Now().Add(time.Hour))

	require.Error(t, svc.Accept(context.Background(), AcceptRequest{Token: plaintext, FullName: "A B", Password: "matkhau123"}, testClientMeta))
	require.Empty(t, rec.events)
}

// TestAcceptNilBusIsSafe proves a service built without a bus never panics on
// a successful accept.
func TestAcceptNilBusIsSafe(t *testing.T) {
	t.Parallel()

	repo := newFakeRepository()
	svc := newAcceptTestService(repo, newFakeOnboarder(), &fakeOpener{})
	plaintext, _ := mintInvitation(t, repo, id.New(), "+84901234567", StatusPending, time.Now().Add(time.Hour))

	require.NoError(t, svc.Accept(context.Background(), AcceptRequest{Token: plaintext, FullName: "A B", Password: "matkhau123"}, testClientMeta))
}
