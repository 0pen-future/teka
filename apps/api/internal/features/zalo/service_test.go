package zalo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"teka/apps/api/internal/features/zalo/protocol"
	"teka/apps/api/internal/shared/secrets"
)

// testCredKey protects nothing real, so a literal here carries none of the risk
// a production key would.
var testCredKey = []byte("unit-test-zalo-credential-key-32b")

type fakeRepo struct {
	mu       sync.Mutex
	accounts map[uuid.UUID]*Account
	upserts  int
	statuses []string
	verified int
	getErr   error
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{accounts: map[uuid.UUID]*Account{}}
}

func (f *fakeRepo) Upsert(_ context.Context, acc *Account) error {
	if acc.ConsentVersion == "" {
		return ErrConsentVersionRequired
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.upserts++
	stored := *acc
	f.accounts[acc.TeacherID] = &stored
	return nil
}

func (f *fakeRepo) GetByTeacher(_ context.Context, teacherID uuid.UUID) (*Account, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.getErr != nil {
		return nil, f.getErr
	}
	acc, ok := f.accounts[teacherID]
	if !ok {
		return nil, ErrAccountNotFound
	}
	stored := *acc
	return &stored, nil
}

func (f *fakeRepo) Delete(_ context.Context, teacherID uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.accounts[teacherID]; !ok {
		return ErrAccountNotFound
	}
	delete(f.accounts, teacherID)
	return nil
}

func (f *fakeRepo) UpdateStatus(_ context.Context, teacherID uuid.UUID, status string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	acc, ok := f.accounts[teacherID]
	if !ok {
		return ErrAccountNotFound
	}
	acc.Status = status
	f.statuses = append(f.statuses, status)
	return nil
}

func (f *fakeRepo) MarkVerified(_ context.Context, teacherID uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	acc, ok := f.accounts[teacherID]
	if !ok {
		return ErrAccountNotFound
	}
	now := time.Now()
	acc.LastVerifiedAt = &now
	f.verified++
	return nil
}

func (f *fakeRepo) ListLinked(_ context.Context) ([]uuid.UUID, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var ids []uuid.UUID
	for id, acc := range f.accounts {
		if acc.Status == StatusLinked {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

func (f *fakeRepo) counts() (upserts, verified int, statuses []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.upserts, f.verified, append([]string(nil), f.statuses...)
}

func (f *fakeRepo) stored(t *testing.T, teacherID uuid.UUID) *Account {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	acc, ok := f.accounts[teacherID]
	require.True(t, ok, "no account stored for teacher")
	return acc
}

func testCipher(t *testing.T) *secrets.Cipher {
	t.Helper()
	c, err := secrets.New(testCredKey)
	require.NoError(t, err)
	return c
}

// sealCredentials writes a stored account the way a successful link would.
func sealCredentials(t *testing.T, cred protocol.Credentials) []byte {
	t.Helper()
	raw, err := json.Marshal(cred)
	require.NoError(t, err)
	out, err := testCipher(t).Seal(raw)
	require.NoError(t, err)
	return out
}

type reloginSpy struct {
	mu    sync.Mutex
	calls int
	got   protocol.Credentials
	err   error
}

func (r *reloginSpy) relogin(_ context.Context, sess *protocol.Session, cred protocol.Credentials) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	r.got = cred
	if r.err != nil {
		return r.err
	}
	sess.UID = "zalo-uid"
	// A cookie login always yields the service map; the QR handshake never
	// does. The spy mirrors that guarantee so tests can tell the two apart.
	sess.LoginInfo = &protocol.LoginInfo{ZpwServiceMapV3: protocol.ZpwServiceMapV3{
		Chat:    []string{"https://chat.example"},
		Profile: []string{"https://profile.example"},
	}}
	return nil
}

func (r *reloginSpy) snapshot() (int, protocol.Credentials) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls, r.got
}

func newTestService(t *testing.T, repo Repository, opts ServiceOptions) *Service {
	t.Helper()
	svc := NewService(repo, testCipher(t), opts)
	t.Cleanup(svc.Close)
	return svc
}

func TestSessionForServesTheCachedSessionWithoutContactingZalo(t *testing.T) {
	t.Parallel()

	repo := newFakeRepo()
	spy := &reloginSpy{}
	svc := newTestService(t, repo, ServiceOptions{Relogin: spy.relogin})

	teacherID := uuid.New()
	cached := protocol.NewSession()
	svc.cache.Put(teacherID, cached)

	got, err := svc.sessionFor(context.Background(), teacherID)
	require.NoError(t, err)
	require.Same(t, cached, got)

	calls, _ := spy.snapshot()
	require.Zero(t, calls, "a cache hit must not re-login")
}

func TestSessionForRelogsInFromStoredCredentialsAndCachesTheResult(t *testing.T) {
	t.Parallel()

	cred := protocol.Credentials{IMEI: "stored-imei", UserAgent: "stored-ua"}
	repo := newFakeRepo()
	teacherID := uuid.New()
	repo.accounts[teacherID] = &Account{
		TeacherID:            teacherID,
		EncryptedCredentials: sealCredentials(t, cred),
		Status:               StatusLinked,
		ConsentVersion:       testConsentVersion,
	}

	spy := &reloginSpy{}
	svc := newTestService(t, repo, ServiceOptions{Relogin: spy.relogin})

	sess, err := svc.sessionFor(context.Background(), teacherID)
	require.NoError(t, err)
	require.Equal(t, "zalo-uid", sess.UID)

	calls, got := spy.snapshot()
	require.Equal(t, 1, calls)
	require.Equal(t, cred, got, "the sealed blob must decrypt back to the stored credentials")

	cachedSession, ok := svc.cache.Get(teacherID)
	require.True(t, ok, "a restored session must be cached for the next caller")
	require.Same(t, sess, cachedSession)

	_, verified, _ := repo.counts()
	require.Equal(t, 1, verified, "a successful re-login stamps last_verified_at")
}

func TestSessionForMarksTheAccountExpiredWhenZaloRejectsTheCredentials(t *testing.T) {
	t.Parallel()

	repo := newFakeRepo()
	teacherID := uuid.New()
	repo.accounts[teacherID] = &Account{
		TeacherID:            teacherID,
		EncryptedCredentials: sealCredentials(t, protocol.Credentials{IMEI: "imei", UserAgent: "ua"}),
		Status:               StatusLinked,
		ConsentVersion:       testConsentVersion,
	}

	spy := &reloginSpy{err: errors.New("session rejected")}
	svc := newTestService(t, repo, ServiceOptions{Relogin: spy.relogin})
	svc.cache.Put(teacherID, protocol.NewSession())
	svc.cache.Evict(teacherID)

	_, err := svc.sessionFor(context.Background(), teacherID)
	require.ErrorIs(t, err, ErrLinkExpired)

	_, _, statuses := repo.counts()
	require.Equal(t, []string{StatusExpired}, statuses)
	_, cached := svc.cache.Get(teacherID)
	require.False(t, cached, "a rejected session must not stay cached")
}

func TestSessionForReportsAnUnlinkedTeacher(t *testing.T) {
	t.Parallel()

	svc := newTestService(t, newFakeRepo(), ServiceOptions{Relogin: (&reloginSpy{}).relogin})

	_, err := svc.sessionFor(context.Background(), uuid.New())
	require.ErrorIs(t, err, ErrNotLinked)
}

// A rotated credential key leaves undecryptable rows behind. That is
// unrecoverable, so it must surface as "re-link", never as a server error.
func TestSessionForTreatsUndecryptableCredentialsAsExpired(t *testing.T) {
	t.Parallel()

	repo := newFakeRepo()
	teacherID := uuid.New()
	repo.accounts[teacherID] = &Account{
		TeacherID:            teacherID,
		EncryptedCredentials: []byte("sealed under a key this process no longer has"),
		Status:               StatusLinked,
		ConsentVersion:       testConsentVersion,
	}

	spy := &reloginSpy{}
	svc := newTestService(t, repo, ServiceOptions{Relogin: spy.relogin})

	_, err := svc.sessionFor(context.Background(), teacherID)
	require.ErrorIs(t, err, ErrLinkExpired)

	calls, _ := spy.snapshot()
	require.Zero(t, calls, "there is nothing to log in with")
	_, _, statuses := repo.counts()
	require.Equal(t, []string{StatusExpired}, statuses)
}

// A session flagged expired by the health probe can come back — a network blip
// looks exactly like a dead session at the moment it happens.
func TestSessionForRestoresLinkedStatusWhenAnExpiredAccountLogsInAgain(t *testing.T) {
	t.Parallel()

	repo := newFakeRepo()
	teacherID := uuid.New()
	repo.accounts[teacherID] = &Account{
		TeacherID:            teacherID,
		EncryptedCredentials: sealCredentials(t, protocol.Credentials{IMEI: "imei", UserAgent: "ua"}),
		Status:               StatusExpired,
		ConsentVersion:       testConsentVersion,
	}

	svc := newTestService(t, repo, ServiceOptions{Relogin: (&reloginSpy{}).relogin})

	_, err := svc.sessionFor(context.Background(), teacherID)
	require.NoError(t, err)
	require.Equal(t, StatusLinked, repo.stored(t, teacherID).Status)
}

func TestStatusReportsTheLinkWithoutTouchingZalo(t *testing.T) {
	t.Parallel()

	repo := newFakeRepo()
	linked, expired, absent := uuid.New(), uuid.New(), uuid.New()
	name := "Cô Lan"
	repo.accounts[linked] = &Account{TeacherID: linked, Status: StatusLinked, DisplayName: &name}
	repo.accounts[expired] = &Account{TeacherID: expired, Status: StatusExpired, DisplayName: &name}

	spy := &reloginSpy{}
	svc := newTestService(t, repo, ServiceOptions{Relogin: spy.relogin})
	ctx := context.Background()

	got, err := svc.Status(ctx, linked)
	require.NoError(t, err)
	require.Equal(t, AccountStatus{Linked: true, Status: StatusLinked, DisplayName: name}, got)

	got, err = svc.Status(ctx, expired)
	require.NoError(t, err)
	require.Equal(t, AccountStatus{Linked: true, Status: StatusExpired, DisplayName: name}, got)

	got, err = svc.Status(ctx, absent)
	require.NoError(t, err, "having no linked account is a normal state, not an error")
	require.Equal(t, AccountStatus{}, got)

	calls, _ := spy.snapshot()
	require.Zero(t, calls, "reading the status is a database read")
}

func TestUnlinkEvictsTheSessionAndRemovesTheRow(t *testing.T) {
	t.Parallel()

	repo := newFakeRepo()
	teacherID := uuid.New()
	repo.accounts[teacherID] = &Account{TeacherID: teacherID, Status: StatusLinked}

	svc := newTestService(t, repo, ServiceOptions{Relogin: (&reloginSpy{}).relogin})
	svc.cache.Put(teacherID, protocol.NewSession())

	require.NoError(t, svc.Unlink(context.Background(), teacherID))

	_, cached := svc.cache.Get(teacherID)
	require.False(t, cached)
	_, err := repo.GetByTeacher(context.Background(), teacherID)
	require.ErrorIs(t, err, ErrAccountNotFound)

	// Asking again is not an error: the account is already gone.
	require.NoError(t, svc.Unlink(context.Background(), teacherID))
}

func TestStartLinkRequiresAnAcknowledgedConsentVersion(t *testing.T) {
	t.Parallel()

	svc := newTestService(t, newFakeRepo(), ServiceOptions{Relogin: (&reloginSpy{}).relogin})

	_, err := svc.StartLink(uuid.New(), "")
	require.ErrorIs(t, err, ErrConsentVersionRequired)
}

func TestStartLinkSealsTheCredentialsItPersistsAndCachesTheSession(t *testing.T) {
	t.Parallel()

	cred := protocol.Credentials{IMEI: "fresh-imei-secret", UserAgent: "ua"}
	var qrSess *protocol.Session
	login := func(_ context.Context, sess *protocol.Session, cb protocol.QRCallbacks) (*protocol.Credentials, error) {
		cb.OnQR([]byte("png"))
		sess.UID = "qr-session-uid"
		sess.DisplayName = "Cô Lan"
		qrSess = sess
		return &cred, nil
	}

	repo := newFakeRepo()
	spy := &reloginSpy{}
	svc := newTestService(t, repo, ServiceOptions{Login: login, Relogin: spy.relogin})
	teacherID := uuid.New()

	linkID, err := svc.StartLink(teacherID, testConsentVersion)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		snap, err := svc.LinkStatus(teacherID, linkID)
		return err == nil && snap.State == LinkStateLinked
	}, 2*time.Second, 5*time.Millisecond)

	acc := repo.stored(t, teacherID)
	require.Equal(t, testConsentVersion, acc.ConsentVersion)
	require.Equal(t, StatusLinked, acc.Status)
	require.NotNil(t, acc.DisplayName)
	require.Equal(t, "Cô Lan", *acc.DisplayName)
	require.NotNil(t, acc.ZaloUID)
	require.Equal(t, "zalo-uid", *acc.ZaloUID, "the UID Zalo reports at credential login wins over the QR session's")
	require.NotContains(t, string(acc.EncryptedCredentials), cred.IMEI, "credentials must never be stored in the clear")

	plain, err := testCipher(t).Open(acc.EncryptedCredentials)
	require.NoError(t, err)
	var back protocol.Credentials
	require.NoError(t, json.Unmarshal(plain, &back))
	require.Equal(t, cred, back)

	// The QR handshake never fetches Zalo's service map, so the QR session
	// cannot send messages or list friends. The cached session must be the one
	// the credential login produced, never the bare QR session.
	cachedSess, cached := svc.cache.Get(teacherID)
	require.True(t, cached, "the session just created must be usable without a re-login")
	require.NotSame(t, qrSess, cachedSess)
	require.NotEmpty(t, protocol.ServiceURL(cachedSess, "profile"),
		"a cached session without the service map cannot list friends or send")

	calls, got := spy.snapshot()
	require.Equal(t, 1, calls, "linking must prove the persisted credentials can log in")
	require.Equal(t, cred, got)

	upserts, _, _ := repo.counts()
	require.Equal(t, 1, upserts)
}

func TestStartLinkFailsWhenTheStoredCredentialsCannotLogIn(t *testing.T) {
	t.Parallel()

	cred := protocol.Credentials{IMEI: "fresh-imei-secret", UserAgent: "ua"}
	login := func(_ context.Context, sess *protocol.Session, cb protocol.QRCallbacks) (*protocol.Credentials, error) {
		cb.OnQR([]byte("png"))
		sess.UID = "zalo-uid"
		return &cred, nil
	}

	repo := newFakeRepo()
	spy := &reloginSpy{err: errors.New("zalo said no")}
	svc := newTestService(t, repo, ServiceOptions{Login: login, Relogin: spy.relogin})
	teacherID := uuid.New()

	linkID, err := svc.StartLink(teacherID, testConsentVersion)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		snap, err := svc.LinkStatus(teacherID, linkID)
		return err == nil && snap.State == LinkStateError
	}, 2*time.Second, 5*time.Millisecond)

	// Credentials that cannot restore a session must not be stored: the teacher
	// would see a linked account whose every send fails.
	upserts, _, _ := repo.counts()
	require.Zero(t, upserts)
	_, cached := svc.cache.Get(teacherID)
	require.False(t, cached)
}

func TestUnlinkCancelsAnAttemptStillInFlight(t *testing.T) {
	t.Parallel()

	started := make(chan struct{})
	login := func(ctx context.Context, _ *protocol.Session, cb protocol.QRCallbacks) (*protocol.Credentials, error) {
		cb.OnQR([]byte("png"))
		close(started)
		<-ctx.Done()
		return nil, ctx.Err()
	}

	repo := newFakeRepo()
	teacherID := uuid.New()
	repo.accounts[teacherID] = &Account{TeacherID: teacherID, Status: StatusLinked}

	svc := newTestService(t, repo, ServiceOptions{Login: login, Relogin: (&reloginSpy{}).relogin})
	linkID, err := svc.StartLink(teacherID, testConsentVersion)
	require.NoError(t, err)
	<-started

	require.NoError(t, svc.Unlink(context.Background(), teacherID))

	_, err = svc.LinkStatus(teacherID, linkID)
	require.ErrorIs(t, err, ErrLinkNotFound)
}

// Revoking consent has to be final. An attempt the teacher left open in
// another tab can finish its scan at any moment, and the write that follows a
// successful scan deliberately ignores cancellation so a slow database cannot
// lose a link. Unlinking must therefore outlast that write rather than race it,
// or the teacher is silently linked again right after asking to be unlinked.
func TestUnlinkOutlastsAScanThatLandsWhileItRuns(t *testing.T) {
	t.Parallel()

	holding := make(chan struct{})
	release := make(chan struct{})
	login := func(_ context.Context, sess *protocol.Session, cb protocol.QRCallbacks) (*protocol.Credentials, error) {
		cb.OnQR([]byte("png"))
		close(holding)
		// Held at the instant the account holder approves on their phone.
		<-release
		sess.UID = "zalo-uid"
		sess.DisplayName = "Cô Lan"
		return &protocol.Credentials{IMEI: "imei-secret", UserAgent: "ua"}, nil
	}

	repo := newFakeRepo()
	teacherID := uuid.New()
	repo.accounts[teacherID] = &Account{TeacherID: teacherID, Status: StatusLinked}

	svc := newTestService(t, repo, ServiceOptions{Login: login, Relogin: (&reloginSpy{}).relogin})
	_, err := svc.StartLink(teacherID, testConsentVersion)
	require.NoError(t, err)
	<-holding

	// The scan completes a moment after the unlink begins — the window in which
	// an attempt could write over the removal.
	go func() {
		time.Sleep(50 * time.Millisecond)
		close(release)
	}()
	require.NoError(t, svc.Unlink(context.Background(), teacherID))

	require.Never(t, func() bool {
		_, err := repo.GetByTeacher(context.Background(), teacherID)
		return err == nil
	}, time.Second, 10*time.Millisecond, "the scan must not restore the row the teacher just removed")
	_, cached := svc.cache.Get(teacherID)
	require.False(t, cached, "a revoked account must not be left usable from the cache")
}

// The same guarantee, for the attempt the teacher left behind. Opening the QR
// modal a second time supersedes the first attempt without stopping it, so a
// scan of the older code can still be in flight — and nothing about it is
// visible, since a superseded record no longer reports its state anywhere.
func TestUnlinkOutlastsASupersededScanThatLandsWhileItRuns(t *testing.T) {
	t.Parallel()

	scanned := make(chan struct{})
	release := make(chan struct{})
	var attempts atomic.Int32
	login := func(ctx context.Context, sess *protocol.Session, cb protocol.QRCallbacks) (*protocol.Credentials, error) {
		if attempts.Add(1) > 1 {
			// The replacement code is on screen and nobody ever scans it.
			<-ctx.Done()
			return nil, ctx.Err()
		}
		cb.OnQR([]byte("png"))
		close(scanned)
		<-release
		sess.UID = "zalo-uid"
		sess.DisplayName = "Cô Lan"
		return &protocol.Credentials{IMEI: "imei-secret", UserAgent: "ua"}, nil
	}

	repo := newFakeRepo()
	teacherID := uuid.New()
	repo.accounts[teacherID] = &Account{TeacherID: teacherID, Status: StatusLinked}

	svc := newTestService(t, repo, ServiceOptions{Login: login, Relogin: (&reloginSpy{}).relogin})
	_, err := svc.StartLink(teacherID, testConsentVersion)
	require.NoError(t, err)
	<-scanned

	// Reopening the modal supersedes the first attempt, which is by then already
	// past the point of no return on the teacher's phone.
	_, err = svc.StartLink(teacherID, testConsentVersion)
	require.NoError(t, err)

	go func() {
		time.Sleep(50 * time.Millisecond)
		close(release)
	}()
	require.NoError(t, svc.Unlink(context.Background(), teacherID))

	require.Never(t, func() bool {
		_, err := repo.GetByTeacher(context.Background(), teacherID)
		return err == nil
	}, time.Second, 10*time.Millisecond, "an abandoned attempt must not restore the row the teacher just removed")
	_, cached := svc.cache.Get(teacherID)
	require.False(t, cached, "a revoked account must not be left usable from the cache")
}

// Revocation has to reach the cache too, not only the stored row. The health
// probe re-logs-in over the network, which takes seconds, and deleting our row
// revokes nothing on Zalo's side — so a probe that started before an unlink can
// come back holding a session that still works.
func TestUnlinkDuringAHealthCheckLeavesNoUsableSession(t *testing.T) {
	t.Parallel()

	entered := make(chan struct{})
	release := make(chan struct{})
	relogin := func(_ context.Context, sess *protocol.Session, _ protocol.Credentials) error {
		close(entered)
		<-release
		sess.UID = "zalo-uid"
		return nil
	}

	repo := newFakeRepo()
	teacherID := uuid.New()
	repo.accounts[teacherID] = &Account{
		TeacherID:            teacherID,
		EncryptedCredentials: sealCredentials(t, protocol.Credentials{IMEI: "imei", UserAgent: "ua"}),
		Status:               StatusLinked,
		ConsentVersion:       testConsentVersion,
	}

	svc := newTestService(t, repo, ServiceOptions{Relogin: relogin})
	checked := make(chan struct{})
	go func() {
		defer close(checked)
		_ = svc.VerifyAccount(context.Background(), teacherID)
	}()
	<-entered

	// The teacher revokes while the probe is mid-login.
	require.NoError(t, svc.Unlink(context.Background(), teacherID))
	close(release)
	<-checked

	_, err := repo.GetByTeacher(context.Background(), teacherID)
	require.ErrorIs(t, err, ErrAccountNotFound)
	_, cached := svc.cache.Get(teacherID)
	require.False(t, cached, "a session restored for an account revoked mid-check must not be kept")
}

// The probe must not conclude "healthy" from a cached session; only Zalo can
// say whether the stored credentials still work.
func TestVerifyAccountIgnoresTheCacheAndLogsInAgain(t *testing.T) {
	t.Parallel()

	repo := newFakeRepo()
	teacherID := uuid.New()
	repo.accounts[teacherID] = &Account{
		TeacherID:            teacherID,
		EncryptedCredentials: sealCredentials(t, protocol.Credentials{IMEI: "imei", UserAgent: "ua"}),
		Status:               StatusLinked,
		ConsentVersion:       testConsentVersion,
	}

	spy := &reloginSpy{}
	svc := newTestService(t, repo, ServiceOptions{Relogin: spy.relogin})
	svc.cache.Put(teacherID, protocol.NewSession())

	require.NoError(t, svc.VerifyAccount(context.Background(), teacherID))

	calls, _ := spy.snapshot()
	require.Equal(t, 1, calls)
}

// storeLinkedAccount stores a decryptable linked account and returns its
// teacher id, which is all the send/friends paths need to reach a session.
func storeLinkedAccount(t *testing.T, repo *fakeRepo) uuid.UUID {
	t.Helper()
	teacherID := uuid.New()
	repo.accounts[teacherID] = &Account{
		TeacherID:            teacherID,
		EncryptedCredentials: sealCredentials(t, protocol.Credentials{IMEI: "imei", UserAgent: "ua"}),
		Status:               StatusLinked,
		ConsentVersion:       testConsentVersion,
	}
	return teacherID
}

func TestSendDMSendsThroughTheTeachersSessionAndReturnsTheMessageID(t *testing.T) {
	t.Parallel()

	repo := newFakeRepo()
	teacherID := storeLinkedAccount(t, repo)

	var gotSess *protocol.Session
	var gotUID, gotText string
	send := func(_ context.Context, sess *protocol.Session, toUID, text string) (string, error) {
		gotSess, gotUID, gotText = sess, toUID, text
		return "msg-991", nil
	}

	svc := newTestService(t, repo, ServiceOptions{Relogin: (&reloginSpy{}).relogin, Send: send})

	msgID, err := svc.SendDM(context.Background(), teacherID, "friend-uid", "Học phí tháng 8")
	require.NoError(t, err)
	require.Equal(t, "msg-991", msgID)
	require.Equal(t, "friend-uid", gotUID)
	require.Equal(t, "Học phí tháng 8", gotText)

	cached, ok := svc.cache.Get(teacherID)
	require.True(t, ok)
	require.Same(t, cached, gotSess, "the send must ride the session sessionFor restored")
}

func TestSendDMReportsExpiredWhenZaloRejectsTheStoredCredentials(t *testing.T) {
	t.Parallel()

	repo := newFakeRepo()
	teacherID := storeLinkedAccount(t, repo)

	sendCalls := 0
	send := func(_ context.Context, _ *protocol.Session, _, _ string) (string, error) {
		sendCalls++
		return "", nil
	}
	spy := &reloginSpy{err: errors.New("session rejected")}
	svc := newTestService(t, repo, ServiceOptions{Relogin: spy.relogin, Send: send})

	_, err := svc.SendDM(context.Background(), teacherID, "friend-uid", "hi")
	require.ErrorIs(t, err, ErrLinkExpired)
	require.Zero(t, sendCalls, "a dead session must not be handed to the send path")

	_, _, statuses := repo.counts()
	require.Equal(t, []string{StatusExpired}, statuses)
}

func TestSendDMForAnUnlinkedTeacherReportsNotLinked(t *testing.T) {
	t.Parallel()

	svc := newTestService(t, newFakeRepo(), ServiceOptions{Relogin: (&reloginSpy{}).relogin})

	_, err := svc.SendDM(context.Background(), uuid.New(), "friend-uid", "hi")
	require.ErrorIs(t, err, ErrNotLinked)
}

func TestSendDMPropagatesASendFailure(t *testing.T) {
	t.Parallel()

	repo := newFakeRepo()
	teacherID := storeLinkedAccount(t, repo)

	sendErr := errors.New("zalo_personal: send error code 216")
	send := func(_ context.Context, _ *protocol.Session, _, _ string) (string, error) {
		return "", sendErr
	}
	svc := newTestService(t, repo, ServiceOptions{Relogin: (&reloginSpy{}).relogin, Send: send})

	_, err := svc.SendDM(context.Background(), teacherID, "friend-uid", "hi")
	require.ErrorIs(t, err, sendErr)
}

func TestSendDMTreatsANotLoggedInRejectionAsExpired(t *testing.T) {
	t.Parallel()

	repo := newFakeRepo()
	teacherID := storeLinkedAccount(t, repo)

	// Zalo answers a send from a session it no longer honours with inner error
	// code -3; only the send path ever sees it, because the cached session
	// skips the relogin that would otherwise catch the dead credentials.
	send := func(_ context.Context, _ *protocol.Session, _, _ string) (string, error) {
		return "", &protocol.APIError{Op: "send", Code: protocol.ErrCodeNotLoggedIn}
	}
	svc := newTestService(t, repo, ServiceOptions{Relogin: (&reloginSpy{}).relogin, Send: send})

	// First send restores and caches a session; the -3 must evict it and
	// expire the account rather than surfacing as a generic send failure.
	_, err := svc.SendDM(context.Background(), teacherID, "friend-uid", "hi")
	require.ErrorIs(t, err, ErrLinkExpired)

	_, ok := svc.cache.Get(teacherID)
	require.False(t, ok, "a session Zalo rejected must not stay cached")
	_, _, statuses := repo.counts()
	require.Equal(t, []string{StatusExpired}, statuses)
}

func TestListFriendsReturnsTheTeachersFriends(t *testing.T) {
	t.Parallel()

	repo := newFakeRepo()
	teacherID := storeLinkedAccount(t, repo)

	friends := func(_ context.Context, _ *protocol.Session) ([]protocol.FriendInfo, error) {
		return []protocol.FriendInfo{
			{UserID: "111", DisplayName: "Mẹ bé An", ZaloName: "Lan Nguyễn", Avatar: "https://a/1.jpg"},
			{UserID: "222", DisplayName: "Bố bé Bình"},
		}, nil
	}
	svc := newTestService(t, repo, ServiceOptions{Relogin: (&reloginSpy{}).relogin, Friends: friends})

	got, err := svc.ListFriends(context.Background(), teacherID)
	require.NoError(t, err)
	require.Equal(t, []Friend{
		{UserID: "111", DisplayName: "Mẹ bé An", ZaloName: "Lan Nguyễn", Avatar: "https://a/1.jpg"},
		{UserID: "222", DisplayName: "Bố bé Bình"},
	}, got)
}

func TestListFriendsForAnUnlinkedTeacherReportsNotLinked(t *testing.T) {
	t.Parallel()

	svc := newTestService(t, newFakeRepo(), ServiceOptions{Relogin: (&reloginSpy{}).relogin})

	_, err := svc.ListFriends(context.Background(), uuid.New())
	require.ErrorIs(t, err, ErrNotLinked)
}

func TestLinkedTeachersListsOnlyLiveAccounts(t *testing.T) {
	t.Parallel()

	repo := newFakeRepo()
	linked, expired := uuid.New(), uuid.New()
	repo.accounts[linked] = &Account{TeacherID: linked, Status: StatusLinked}
	repo.accounts[expired] = &Account{TeacherID: expired, Status: StatusExpired}

	svc := newTestService(t, repo, ServiceOptions{Relogin: (&reloginSpy{}).relogin})

	ids, err := svc.LinkedTeachers(context.Background())
	require.NoError(t, err)
	require.Equal(t, []uuid.UUID{linked}, ids)
}

// --- Auto-map: phone normalization, matching, friend requests ---

func TestNormalizePhoneCanonicalizesVietnameseNumbers(t *testing.T) {
	t.Parallel()

	cases := []struct{ in, want string }{
		{"0901234567", "0901234567"},
		{" 0901 234 567 ", "0901234567"},
		{"090.123.4567", "0901234567"},
		{"+84901234567", "0901234567"},
		{"84901234567", "0901234567"},
		{"+84 901 234 567", "0901234567"},
		{"", ""},
		{"   ", ""},
	}
	for _, tc := range cases {
		require.Equal(t, tc.want, normalizePhone(tc.in), "input %q", tc.in)
	}
}

func TestMatchFriendsLabelsRowsInRequestOrder(t *testing.T) {
	t.Parallel()

	repo := newFakeRepo()
	teacherID := storeLinkedAccount(t, repo)

	var lookedUp []string
	findUser := func(_ context.Context, _ *protocol.Session, phones []string) (map[string]protocol.FoundUser, error) {
		lookedUp = append(lookedUp, phones...)
		return map[string]protocol.FoundUser{
			"84901234567": {UID: "111", DisplayName: "Lan Nguyễn", ZaloName: "Lan", Avatar: "https://a/1.jpg"},
			"84908888777": {UID: "333", DisplayName: "Hoa Trần"},
		}, nil
	}
	friends := func(_ context.Context, _ *protocol.Session) ([]protocol.FriendInfo, error) {
		return []protocol.FriendInfo{{UserID: "111", DisplayName: "Mẹ bé An"}}, nil
	}
	svc := newTestService(t, repo, ServiceOptions{
		Relogin: (&reloginSpy{}).relogin, FindUser: findUser, Friends: friends,
	})

	rows, err := svc.MatchFriends(context.Background(), teacherID,
		[]string{"+84 901 234 567", "0907654321", "84908888777", "   "})
	require.NoError(t, err)

	// The lookup travels in the country-code form Zalo resolves and skips
	// blanks; rows echo the phone exactly as the caller sent it, in request
	// order.
	require.Equal(t, []string{"84901234567", "84907654321", "84908888777"}, lookedUp)
	require.Equal(t, []FriendMatch{
		{Phone: "+84 901 234 567", Matched: true, UserID: "111", DisplayName: "Lan Nguyễn", ZaloName: "Lan", Avatar: "https://a/1.jpg", IsFriend: true},
		{Phone: "0907654321"},
		{Phone: "84908888777", Matched: true, UserID: "333", DisplayName: "Hoa Trần"},
		{Phone: "   "},
	}, rows)
}

func TestMatchFriendsChunksLookupsAndPacesBetweenChunks(t *testing.T) {
	t.Parallel()

	repo := newFakeRepo()
	teacherID := storeLinkedAccount(t, repo)

	var batches [][]string
	findUser := func(_ context.Context, _ *protocol.Session, phones []string) (map[string]protocol.FoundUser, error) {
		batches = append(batches, append([]string(nil), phones...))
		return map[string]protocol.FoundUser{}, nil
	}
	friends := func(_ context.Context, _ *protocol.Session) ([]protocol.FriendInfo, error) {
		return nil, nil
	}
	paces := 0
	svc := newTestService(t, repo, ServiceOptions{
		Relogin:  (&reloginSpy{}).relogin,
		FindUser: findUser,
		Friends:  friends,
		Pace:     func(_ context.Context) { paces++ },
	})

	phones := make([]string, 61)
	for i := range phones {
		phones[i] = fmt.Sprintf("09%08d", i)
	}
	rows, err := svc.MatchFriends(context.Background(), teacherID, phones)
	require.NoError(t, err)
	require.Len(t, rows, 61)

	require.Len(t, batches, 3, "61 phones must travel as 30+30+1")
	require.Len(t, batches[0], 30)
	require.Len(t, batches[1], 30)
	require.Len(t, batches[2], 1)
	require.Equal(t, 2, paces, "pacing sits between chunks, not after the last")
}

func TestMatchFriendsForAnUnlinkedTeacherReportsNotLinked(t *testing.T) {
	t.Parallel()

	svc := newTestService(t, newFakeRepo(), ServiceOptions{Relogin: (&reloginSpy{}).relogin})

	_, err := svc.MatchFriends(context.Background(), uuid.New(), []string{"0901234567"})
	require.ErrorIs(t, err, ErrNotLinked)
}

func TestMatchFriendsReportsExpiredWhenZaloRejectsTheStoredCredentials(t *testing.T) {
	t.Parallel()

	repo := newFakeRepo()
	teacherID := storeLinkedAccount(t, repo)

	lookups := 0
	findUser := func(_ context.Context, _ *protocol.Session, _ []string) (map[string]protocol.FoundUser, error) {
		lookups++
		return nil, nil
	}
	spy := &reloginSpy{err: errors.New("session rejected")}
	svc := newTestService(t, repo, ServiceOptions{Relogin: spy.relogin, FindUser: findUser})

	_, err := svc.MatchFriends(context.Background(), teacherID, []string{"0901234567"})
	require.ErrorIs(t, err, ErrLinkExpired)
	require.Zero(t, lookups, "a dead session must not be handed to the lookup path")
}

// A cached session Zalo stopped honouring surfaces as -3 mid-lookup; like the
// send path, that means the link expired — not a generic lookup failure.
func TestMatchFriendsTreatsANotLoggedInLookupAsExpired(t *testing.T) {
	t.Parallel()

	repo := newFakeRepo()
	teacherID := storeLinkedAccount(t, repo)

	findUser := func(_ context.Context, _ *protocol.Session, _ []string) (map[string]protocol.FoundUser, error) {
		return nil, &protocol.APIError{Op: "lookup", Code: protocol.ErrCodeNotLoggedIn}
	}
	svc := newTestService(t, repo, ServiceOptions{Relogin: (&reloginSpy{}).relogin, FindUser: findUser})

	_, err := svc.MatchFriends(context.Background(), teacherID, []string{"0901234567"})
	require.ErrorIs(t, err, ErrLinkExpired)

	_, ok := svc.cache.Get(teacherID)
	require.False(t, ok, "a session Zalo rejected must not stay cached")
	_, _, statuses := repo.counts()
	require.Equal(t, []string{StatusExpired}, statuses)
}

func TestSendRequestSendsExactlyOneFriendRequest(t *testing.T) {
	t.Parallel()

	repo := newFakeRepo()
	teacherID := storeLinkedAccount(t, repo)

	var calls int
	var gotUID, gotMessage string
	sendReq := func(_ context.Context, _ *protocol.Session, userID, message string) error {
		calls++
		gotUID, gotMessage = userID, message
		return nil
	}
	svc := newTestService(t, repo, ServiceOptions{Relogin: (&reloginSpy{}).relogin, SendFriendRequest: sendReq})

	err := svc.SendRequest(context.Background(), teacherID, "target-uid", "Chào chị")
	require.NoError(t, err)
	require.Equal(t, 1, calls)
	require.Equal(t, "target-uid", gotUID)
	require.Equal(t, "Chào chị", gotMessage)
}

// A blank message still sends something the parent can recognise — Zalo shows
// the request text, and an empty one looks like spam.
func TestSendRequestDefaultsTheMessage(t *testing.T) {
	t.Parallel()

	repo := newFakeRepo()
	teacherID := storeLinkedAccount(t, repo)

	var gotMessage string
	sendReq := func(_ context.Context, _ *protocol.Session, _, message string) error {
		gotMessage = message
		return nil
	}
	svc := newTestService(t, repo, ServiceOptions{Relogin: (&reloginSpy{}).relogin, SendFriendRequest: sendReq})

	require.NoError(t, svc.SendRequest(context.Background(), teacherID, "target-uid", ""))
	require.Equal(t, defaultFriendRequestMessage, gotMessage)
	require.NotEmpty(t, gotMessage)
}

func TestSendRequestForAnUnlinkedTeacherReportsNotLinked(t *testing.T) {
	t.Parallel()

	svc := newTestService(t, newFakeRepo(), ServiceOptions{Relogin: (&reloginSpy{}).relogin})

	err := svc.SendRequest(context.Background(), uuid.New(), "target-uid", "hi")
	require.ErrorIs(t, err, ErrNotLinked)
}

func TestSendRequestTreatsANotLoggedInRejectionAsExpired(t *testing.T) {
	t.Parallel()

	repo := newFakeRepo()
	teacherID := storeLinkedAccount(t, repo)

	sendReq := func(_ context.Context, _ *protocol.Session, _, _ string) error {
		return &protocol.APIError{Op: "friend_request", Code: protocol.ErrCodeNotLoggedIn}
	}
	svc := newTestService(t, repo, ServiceOptions{Relogin: (&reloginSpy{}).relogin, SendFriendRequest: sendReq})

	err := svc.SendRequest(context.Background(), teacherID, "target-uid", "hi")
	require.ErrorIs(t, err, ErrLinkExpired)

	_, ok := svc.cache.Get(teacherID)
	require.False(t, ok, "a session Zalo rejected must not stay cached")
	_, _, statuses := repo.counts()
	require.Equal(t, []string{StatusExpired}, statuses)
}

// Refusals that are not about the session — already friends, blocked — reach
// the caller as they are; the protocol cannot tell what they mean for the link.
func TestSendRequestPropagatesOtherRefusals(t *testing.T) {
	t.Parallel()

	repo := newFakeRepo()
	teacherID := storeLinkedAccount(t, repo)

	refusal := &protocol.APIError{Op: "friend_request", Code: 225}
	sendReq := func(_ context.Context, _ *protocol.Session, _, _ string) error {
		return refusal
	}
	svc := newTestService(t, repo, ServiceOptions{Relogin: (&reloginSpy{}).relogin, SendFriendRequest: sendReq})

	err := svc.SendRequest(context.Background(), teacherID, "target-uid", "hi")
	require.ErrorIs(t, err, refusal)
}

// The friend-list leg of a match runs over the same cached session as the
// lookups, so -3 there means the same thing: the link expired.
func TestMatchFriendsTreatsANotLoggedInFriendsFetchAsExpired(t *testing.T) {
	t.Parallel()

	repo := newFakeRepo()
	teacherID := storeLinkedAccount(t, repo)

	findUser := func(_ context.Context, _ *protocol.Session, _ []string) (map[string]protocol.FoundUser, error) {
		return map[string]protocol.FoundUser{"84901234567": {UID: "111"}}, nil
	}
	friends := func(_ context.Context, _ *protocol.Session) ([]protocol.FriendInfo, error) {
		return nil, &protocol.APIError{Op: "friends", Code: protocol.ErrCodeNotLoggedIn}
	}
	svc := newTestService(t, repo, ServiceOptions{
		Relogin: (&reloginSpy{}).relogin, FindUser: findUser, Friends: friends,
	})

	_, err := svc.MatchFriends(context.Background(), teacherID, []string{"0901234567"})
	require.ErrorIs(t, err, ErrLinkExpired)

	_, ok := svc.cache.Get(teacherID)
	require.False(t, ok, "a session Zalo rejected must not stay cached")
	_, _, statuses := repo.counts()
	require.Equal(t, []string{StatusExpired}, statuses)
}

// GET /me/zalo/friends shares the session with everything else, so a -3 there
// must expire the link the same way — not surface as an internal error.
func TestListFriendsTreatsANotLoggedInFetchAsExpired(t *testing.T) {
	t.Parallel()

	repo := newFakeRepo()
	teacherID := storeLinkedAccount(t, repo)

	friends := func(_ context.Context, _ *protocol.Session) ([]protocol.FriendInfo, error) {
		return nil, &protocol.APIError{Op: "friends", Code: protocol.ErrCodeNotLoggedIn}
	}
	svc := newTestService(t, repo, ServiceOptions{Relogin: (&reloginSpy{}).relogin, Friends: friends})

	_, err := svc.ListFriends(context.Background(), teacherID)
	require.ErrorIs(t, err, ErrLinkExpired)

	_, ok := svc.cache.Get(teacherID)
	require.False(t, ok, "a session Zalo rejected must not stay cached")
	_, _, statuses := repo.counts()
	require.Equal(t, []string{StatusExpired}, statuses)
}

// A duplicated phone costs one lookup but still labels every row it appears in.
func TestMatchFriendsLooksUpDuplicatePhonesOnce(t *testing.T) {
	t.Parallel()

	repo := newFakeRepo()
	teacherID := storeLinkedAccount(t, repo)

	var lookedUp []string
	findUser := func(_ context.Context, _ *protocol.Session, phones []string) (map[string]protocol.FoundUser, error) {
		lookedUp = append(lookedUp, phones...)
		return map[string]protocol.FoundUser{"84901234567": {UID: "111", DisplayName: "Lan"}}, nil
	}
	friends := func(_ context.Context, _ *protocol.Session) ([]protocol.FriendInfo, error) {
		return nil, nil
	}
	svc := newTestService(t, repo, ServiceOptions{
		Relogin: (&reloginSpy{}).relogin, FindUser: findUser, Friends: friends,
	})

	rows, err := svc.MatchFriends(context.Background(), teacherID,
		[]string{"0901234567", "+84901234567"})
	require.NoError(t, err)
	require.Equal(t, []string{"84901234567"}, lookedUp, "one lookup per distinct normalized phone")
	require.Equal(t, []FriendMatch{
		{Phone: "0901234567", Matched: true, UserID: "111", DisplayName: "Lan"},
		{Phone: "+84901234567", Matched: true, UserID: "111", DisplayName: "Lan"},
	}, rows)
}

// Anything that does not normalize into a Vietnamese mobile number never
// travels to Zalo — it comes back unmatched instead of riding in a query
// string as arbitrary text.
func TestMatchFriendsNeverSendsANonPhoneToZalo(t *testing.T) {
	t.Parallel()

	repo := newFakeRepo()
	teacherID := storeLinkedAccount(t, repo)

	var lookedUp []string
	findUser := func(_ context.Context, _ *protocol.Session, phones []string) (map[string]protocol.FoundUser, error) {
		lookedUp = append(lookedUp, phones...)
		return nil, nil
	}
	friends := func(_ context.Context, _ *protocol.Session) ([]protocol.FriendInfo, error) {
		return nil, nil
	}
	svc := newTestService(t, repo, ServiceOptions{
		Relogin: (&reloginSpy{}).relogin, FindUser: findUser, Friends: friends,
	})

	rows, err := svc.MatchFriends(context.Background(), teacherID,
		[]string{"not-a-phone", "12345", "0201234567", "0901234567"})
	require.NoError(t, err)
	require.Equal(t, []string{"84901234567"}, lookedUp)
	require.Equal(t, []FriendMatch{
		{Phone: "not-a-phone"},
		{Phone: "12345"},
		{Phone: "0201234567"},
		{Phone: "0901234567"},
	}, rows)
}
