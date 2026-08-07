package zalo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"

	"teka/apps/api/internal/features/zalo/protocol"
	"teka/apps/api/internal/shared/secrets"
)

// ReloginFunc restores a session from stored credentials.
// protocol.LoginWithCredentials is the production value; tests inject a fake.
type ReloginFunc func(ctx context.Context, sess *protocol.Session, cred protocol.Credentials) error

// SendFunc sends a text DM over an established session and returns the message
// id Zalo assigned. protocol.SendMessage is the production value.
type SendFunc func(ctx context.Context, sess *protocol.Session, toUID, text string) (string, error)

// FriendsFunc lists the account's Zalo friends over an established session.
// protocol.FetchFriends is the production value.
type FriendsFunc func(ctx context.Context, sess *protocol.Session) ([]protocol.FriendInfo, error)

// Friend is one entry of the teacher's Zalo friend list, as the contact-mapping
// picker consumes it. It mirrors protocol.FriendInfo so nothing outside this
// package has to import the protocol.
type Friend struct {
	UserID      string
	DisplayName string
	ZaloName    string
	Avatar      string
}

// AccountStatus is everything the API may tell a teacher about their link. It
// deliberately has no field that could carry credential material.
type AccountStatus struct {
	// Linked reports whether a Zalo account is attached at all, regardless of
	// whether its session still works — Status distinguishes those two.
	Linked bool
	// Status is StatusLinked or StatusExpired; empty when nothing is linked.
	Status string
	// DisplayName is the Zalo profile name, when it is known. A link restored
	// from cookies alone never learns one.
	DisplayName string
	// LinkedAt is when the account was linked; zero when nothing is linked.
	LinkedAt time.Time
}

// ServiceOptions supplies the collaborators a Service cannot build itself. The
// zero value uses the real Zalo protocol, which is what production wants and
// what no test may use.
type ServiceOptions struct {
	Login   LoginFunc
	Relogin ReloginFunc
	Send    SendFunc
	Friends FriendsFunc
	Logger  *slog.Logger
	Link    LinkOptions
}

// Service is the Zalo linking feature: starting and following a QR link,
// reporting and removing a link, and handing out live sessions to whatever
// needs to act as the teacher on Zalo.
type Service struct {
	repo    Repository
	cipher  *secrets.Cipher
	cache   *SessionCache
	links   *LinkManager
	relogin ReloginFunc
	send    SendFunc
	friends FriendsFunc
	log     *slog.Logger

	// The health probe is optional and owned here rather than by the caller, so
	// that stopping the service stops everything it started.
	probeMu   sync.Mutex
	probeStop context.CancelFunc
	probeDone chan struct{}
}

// NewService wires the service together with its own session cache and link
// manager. Call Close at shutdown to stop the background attempts it owns.
func NewService(repo Repository, cipher *secrets.Cipher, opts ServiceOptions) *Service {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	if opts.Login == nil {
		opts.Login = protocol.LoginQR
	}
	if opts.Relogin == nil {
		opts.Relogin = protocol.LoginWithCredentials
	}
	if opts.Send == nil {
		opts.Send = protocol.SendMessage
	}
	if opts.Friends == nil {
		opts.Friends = protocol.FetchFriends
	}

	svc := &Service{
		repo:    repo,
		cipher:  cipher,
		cache:   NewSessionCache(),
		relogin: opts.Relogin,
		send:    opts.Send,
		friends: opts.Friends,
		log:     opts.Logger,
	}
	svc.links = NewLinkManager(opts.Login, svc.persistLink, opts.Logger, opts.Link)
	return svc
}

// StartHealthProbe runs the periodic session check in the background until ctx
// is cancelled or Close is called, whichever comes first. Starting it is
// optional: everything else works without it, the profile page just learns
// about a dead session later. Calling it a second time does nothing.
func (s *Service) StartHealthProbe(ctx context.Context, opts ProbeOptions) {
	s.probeMu.Lock()
	defer s.probeMu.Unlock()
	if s.probeDone != nil {
		return
	}

	probeCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	s.probeStop, s.probeDone = cancel, done

	probe := NewHealthProbe(s.LinkedTeachers, s.VerifyAccount, s.log, opts)
	go func() {
		defer close(done)
		probe.Run(probeCtx)
	}()
}

// Close stops everything this service started — the health probe and any link
// attempt in flight — and waits for it to finish. It is safe to call twice.
func (s *Service) Close() {
	s.stopProbe()
	s.links.Close()
}

// stopProbe cancels the sweep loop on the service's own derived context, so
// Close does not depend on whoever supplied the context cancelling it first.
func (s *Service) stopProbe() {
	s.probeMu.Lock()
	stop, done := s.probeStop, s.probeDone
	s.probeStop, s.probeDone = nil, nil
	s.probeMu.Unlock()

	if stop == nil {
		return
	}
	stop()
	<-done
}

// StartLink begins a QR link attempt and returns its id right away; the login
// happens in the background and is followed with LinkStatus. consentVersion is
// the consent text the teacher acknowledged and is stored with the account, so
// an attempt without one is refused before any credential is ever fetched.
func (s *Service) StartLink(teacherID uuid.UUID, consentVersion string) (uuid.UUID, error) {
	if consentVersion == "" {
		return uuid.Nil, ErrConsentVersionRequired
	}
	return s.links.Begin(teacherID, consentVersion), nil
}

// LinkStatus reports how the teacher's attempt is going.
func (s *Service) LinkStatus(teacherID, linkID uuid.UUID) (LinkSnapshot, error) {
	return s.links.Status(teacherID, linkID)
}

// Status reports the teacher's link from the database alone — no request ever
// reaches Zalo, so the profile page stays fast and cannot be slowed by Zalo
// being unreachable. Having no linked account is a normal state, not an error.
func (s *Service) Status(ctx context.Context, teacherID uuid.UUID) (AccountStatus, error) {
	acc, err := s.repo.GetByTeacher(ctx, teacherID)
	if errors.Is(err, ErrAccountNotFound) {
		return AccountStatus{}, nil
	}
	if err != nil {
		return AccountStatus{}, err
	}

	status := AccountStatus{Linked: true, Status: acc.Status, LinkedAt: acc.LinkedAt}
	if acc.DisplayName != nil {
		status.DisplayName = *acc.DisplayName
	}
	return status, nil
}

// unlinkTimeout bounds the removal itself, which runs detached from the request.
const unlinkTimeout = 10 * time.Second

// Unlink detaches the teacher's Zalo account: any attempt in flight is
// cancelled, the live session is dropped, and the stored credentials go away.
// A QR the teacher starts and completes after this call has begun is a new
// consent, not a survivor of the old one, and links the account again — which
// is what the teacher just asked for by scanning.
func (s *Service) Unlink(ctx context.Context, teacherID uuid.UUID) error {
	s.links.Cancel(teacherID)
	s.cache.Evict(teacherID)

	// Cancel can spend seconds waiting out a scan that landed mid-request, and a
	// teacher who gives up and closes the tab in that window must still end up
	// unlinked: the wait exists precisely because a write may just have stored
	// credentials. So the removal runs on a context the client cannot cancel.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), unlinkTimeout)
	defer cancel()

	// Unlinking is idempotent: the caller asked for the account to be gone, and
	// it is. Reporting "not found" would only turn a double-click, or a retry
	// after a dropped response, into an error the teacher cannot act on.
	err := s.repo.Delete(ctx, teacherID)
	if errors.Is(err, ErrAccountNotFound) {
		return nil
	}
	return err
}

// SendDM sends a text message as the teacher to one Zalo user and returns the
// message id Zalo assigned. The session comes from sessionFor, so a missing
// link is ErrNotLinked and dead credentials are ErrLinkExpired before any send
// is attempted. One send failure is acted on by name: Zalo's not-logged-in
// code means the cached session died since it was restored — the only path
// that ever sees it, since a cache hit skips the relogin — so the account is
// expired the same way a rejected relogin would have. Every other failure is
// returned as-is; the protocol cannot tell what it means for the link.
func (s *Service) SendDM(ctx context.Context, teacherID uuid.UUID, toUID, text string) (string, error) {
	sess, err := s.sessionFor(ctx, teacherID)
	if err != nil {
		return "", err
	}
	msgID, err := s.send(ctx, sess, toUID, text)
	if err != nil {
		return "", s.expireIfLoggedOut(ctx, teacherID, "send", err)
	}
	return msgID, nil
}

// expireIfLoggedOut turns Zalo's not-logged-in code into ErrLinkExpired and
// records the expiry; any other error passes through untouched. Only calls made
// over a cached session can see this code — a fresh relogin already failed
// inside sessionFor — so it always means the cached session died since it was
// restored.
func (s *Service) expireIfLoggedOut(ctx context.Context, teacherID uuid.UUID, op string, err error) error {
	var apiErr *protocol.APIError
	if errors.As(err, &apiErr) && apiErr.Code == protocol.ErrCodeNotLoggedIn {
		s.log.Warn("zalo: "+op+" rejected as not logged in", "teacher_id", teacherID)
		s.expire(ctx, teacherID)
		return ErrLinkExpired
	}
	return err
}

// ListFriends lists the teacher's Zalo friends for the contact-mapping picker.
func (s *Service) ListFriends(ctx context.Context, teacherID uuid.UUID) ([]Friend, error) {
	sess, err := s.sessionFor(ctx, teacherID)
	if err != nil {
		return nil, err
	}
	infos, err := s.friends(ctx, sess)
	if err != nil {
		return nil, s.expireIfLoggedOut(ctx, teacherID, "friends fetch", err)
	}
	friends := make([]Friend, len(infos))
	for i, f := range infos {
		friends[i] = Friend{
			UserID:      f.UserID,
			DisplayName: f.DisplayName,
			ZaloName:    f.ZaloName,
			Avatar:      f.Avatar,
		}
	}
	return friends, nil
}

// LinkedTeachers lists the teachers whose account is currently healthy. The
// health probe sweeps exactly this set.
func (s *Service) LinkedTeachers(ctx context.Context) ([]uuid.UUID, error) {
	return s.repo.ListLinked(ctx)
}

// VerifyAccount checks that the teacher's stored credentials are still accepted
// by Zalo, and records the outcome. It deliberately ignores the session cache:
// a cached session proves only that a login once worked, and the whole point of
// the check is to learn whether it still does.
func (s *Service) VerifyAccount(ctx context.Context, teacherID uuid.UUID) error {
	s.cache.Evict(teacherID)
	_, err := s.sessionFor(ctx, teacherID)
	return err
}

// sessionFor returns a live Zalo session for the teacher, restoring one from
// the stored credentials when none is cached. A session Zalo rejects is not an
// internal failure: the credentials are dead, so the account is marked expired
// and the caller gets ErrLinkExpired, whose only fix is scanning a new QR code.
func (s *Service) sessionFor(ctx context.Context, teacherID uuid.UUID) (*protocol.Session, error) {
	if sess, ok := s.cache.Get(teacherID); ok {
		return sess, nil
	}

	// Everything below can take seconds — a credential read plus a login against
	// Zalo — and an unlink may land inside it. Noting where the cache stood
	// before starting is what lets the restore be thrown away if it does.
	since := s.cache.Evictions(teacherID)

	acc, err := s.repo.GetByTeacher(ctx, teacherID)
	if errors.Is(err, ErrAccountNotFound) {
		return nil, ErrNotLinked
	}
	if err != nil {
		return nil, err
	}

	cred, err := s.openCredentials(acc)
	if err != nil {
		// Unreadable means the credential key changed. Nothing can recover the
		// blob, so present it as the situation it is: a link that must be redone.
		s.log.Error("zalo: stored credentials could not be opened", "teacher_id", teacherID, "error", err)
		s.expire(ctx, teacherID)
		return nil, ErrLinkExpired
	}

	sess := protocol.NewSession()
	if err := s.relogin(ctx, sess, *cred); err != nil {
		s.log.Warn("zalo: stored session was rejected", "teacher_id", teacherID, "error", err)
		s.expire(ctx, teacherID)
		return nil, ErrLinkExpired
	}

	if !s.cache.PutUnlessEvicted(teacherID, sess, since) {
		// The teacher unlinked, or the account was expired, while this login was
		// in flight. The session is real and Zalo would still honour it, which is
		// exactly why it must not be kept or handed back.
		return nil, ErrNotLinked
	}
	s.recordHealthy(ctx, acc)
	return sess, nil
}

// recordHealthy stamps a successful check and revives an account an earlier
// check gave up on — a login that works now proves the previous failure was the
// network, not the credentials.
func (s *Service) recordHealthy(ctx context.Context, acc *Account) {
	if err := s.repo.MarkVerified(ctx, acc.TeacherID); err != nil {
		s.log.Warn("zalo: could not record a successful session check", "teacher_id", acc.TeacherID, "error", err)
	}
	if acc.Status == StatusLinked {
		return
	}
	if err := s.repo.UpdateStatus(ctx, acc.TeacherID, StatusLinked); err != nil {
		s.log.Warn("zalo: could not restore linked status", "teacher_id", acc.TeacherID, "error", err)
	}
}

// expire drops the session and marks the account so the profile page tells the
// teacher the truth without anyone having to try a send first.
func (s *Service) expire(ctx context.Context, teacherID uuid.UUID) {
	s.cache.Evict(teacherID)
	if err := s.repo.UpdateStatus(ctx, teacherID, StatusExpired); err != nil && !errors.Is(err, ErrAccountNotFound) {
		s.log.Error("zalo: could not mark account expired", "teacher_id", teacherID, "error", err)
	}
}

// openCredentials unseals a stored account. Neither returned error carries any
// part of the plaintext: a json error quotes the input it choked on, which here
// would be the teacher's cookies.
func (s *Service) openCredentials(acc *Account) (*protocol.Credentials, error) {
	plain, err := s.cipher.Open(acc.EncryptedCredentials)
	if err != nil {
		return nil, err
	}
	var cred protocol.Credentials
	if json.Unmarshal(plain, &cred) != nil {
		return nil, errors.New("zalo: stored credentials are not valid credential json")
	}
	return &cred, nil
}

// persistLink seals a freshly obtained session and stores it. It is the link
// manager's OnLinked hook, so it runs on the attempt's goroutine.
func (s *Service) persistLink(
	ctx context.Context,
	teacherID uuid.UUID,
	consentVersion string,
	sess *protocol.Session,
	cred *protocol.Credentials,
) error {
	raw, err := json.Marshal(cred)
	if err != nil {
		return errors.New("zalo: credentials could not be serialized")
	}
	enc, err := s.cipher.Seal(raw)
	if err != nil {
		return err
	}

	// The QR handshake proves the teacher approved the login, but the session
	// it leaves behind never fetched Zalo's service map — only a cookie login
	// does — so it cannot send messages or list friends. Log in from the
	// credentials now: it proves the blob being persisted actually works, and
	// yields the session worth caching.
	live := protocol.NewSession()
	if err := s.relogin(ctx, live, *cred); err != nil {
		return fmt.Errorf("zalo: the linked credentials were rejected on their first login: %w", err)
	}
	live.DisplayName = sess.DisplayName

	now := time.Now()
	acc := &Account{
		TeacherID:            teacherID,
		EncryptedCredentials: enc,
		Status:               StatusLinked,
		ConsentVersion:       consentVersion,
		ConsentAt:            now,
		LinkedAt:             now,
		LastVerifiedAt:       &now,
	}
	// The credential login learns the account's UID from Zalo itself; the QR
	// session's UID is the fallback for the odd flow that only set it there.
	uid := live.UID
	if uid == "" {
		uid = sess.UID
	}
	if uid != "" {
		acc.ZaloUID = &uid
	}
	if sess.DisplayName != "" {
		name := sess.DisplayName
		acc.DisplayName = &name
	}

	if err := s.repo.Upsert(ctx, acc); err != nil {
		return err
	}
	s.cache.Put(teacherID, live)
	return nil
}
