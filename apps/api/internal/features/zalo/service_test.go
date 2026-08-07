package zalo

import (
	"context"
	"encoding/json"
	"errors"
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
