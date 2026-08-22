package invitations

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"teka/apps/api/internal/config"
	"teka/apps/api/internal/features/teachers"
	"teka/apps/api/internal/features/zalo"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/id"
	"teka/apps/api/internal/shared/token"
)

// --- fakeRepository ---

// fakeRepository is an in-memory Repository mirroring the SQL layer's
// invariants: center-scoped reads/writes and "at most one pending row per
// (center, phone)" (enforced only for the plain Create path — the
// concurrent-race path is covered by integration_test.go against the real
// partial unique index).
type fakeRepository struct {
	rows map[uuid.UUID]Invitation
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{rows: map[uuid.UUID]Invitation{}}
}

func (f *fakeRepository) Create(_ context.Context, inv *Invitation) error {
	for _, r := range f.rows {
		if r.CenterID == inv.CenterID && r.Phone == inv.Phone && r.Status == StatusPending {
			return ErrPendingExists
		}
	}
	f.rows[inv.ID] = *inv
	return nil
}

func (f *fakeRepository) RevokePendingForPhone(_ context.Context, centerID uuid.UUID, phone string) error {
	now := time.Now()
	for rowID, r := range f.rows {
		if r.CenterID == centerID && r.Phone == phone && r.Status == StatusPending {
			r.Status = StatusRevoked
			r.RevokedAt = &now
			f.rows[rowID] = r
		}
	}
	return nil
}

func (f *fakeRepository) GetPendingByPhone(_ context.Context, centerID uuid.UUID, phone string) (*Invitation, error) {
	for _, r := range f.rows {
		if r.CenterID == centerID && r.Phone == phone && r.Status == StatusPending {
			cp := r
			return &cp, nil
		}
	}
	return nil, ErrNotFound
}

func (f *fakeRepository) SetTokenHash(_ context.Context, invID uuid.UUID, hash string) error {
	r, ok := f.rows[invID]
	if !ok {
		return ErrNotFound
	}
	r.TokenHash = hash
	f.rows[invID] = r
	return nil
}

func (f *fakeRepository) List(_ context.Context, centerID uuid.UUID) ([]Invitation, error) {
	out := make([]Invitation, 0)
	for _, r := range f.rows {
		if r.CenterID == centerID {
			out = append(out, r)
		}
	}
	return out, nil
}

func (f *fakeRepository) GetByID(_ context.Context, centerID, invID uuid.UUID) (*Invitation, error) {
	r, ok := f.rows[invID]
	if !ok || r.CenterID != centerID {
		return nil, ErrNotFound
	}
	cp := r
	return &cp, nil
}

func (f *fakeRepository) Revoke(_ context.Context, centerID, invID uuid.UUID) error {
	r, ok := f.rows[invID]
	if !ok || r.CenterID != centerID {
		return ErrNotFound
	}
	if r.Status == StatusPending {
		now := time.Now()
		r.Status = StatusRevoked
		r.RevokedAt = &now
		f.rows[invID] = r
	}
	return nil
}

func (f *fakeRepository) GetByTokenHash(_ context.Context, hash string) (*Invitation, error) {
	for _, r := range f.rows {
		if r.TokenHash == hash {
			cp := r
			return &cp, nil
		}
	}
	return nil, ErrNotFound
}

// GetByTokenHashForUpdate reads the same way GetByTokenHash does: this fake
// runs single-goroutine, so it has no real row lock to emulate. The
// concurrent-accept race against an actual SELECT ... FOR UPDATE lock is
// integration_test.go's job, against real Postgres.
func (f *fakeRepository) GetByTokenHashForUpdate(ctx context.Context, hash string) (*Invitation, error) {
	return f.GetByTokenHash(ctx, hash)
}

func (f *fakeRepository) MarkAccepted(_ context.Context, invID uuid.UUID, at time.Time) error {
	r, ok := f.rows[invID]
	if !ok {
		return ErrNotFound
	}
	r.Status = StatusAccepted
	r.AcceptedAt = &at
	f.rows[invID] = r
	return nil
}

func (f *fakeRepository) GetCenterName(_ context.Context, _ uuid.UUID) (string, error) {
	return "Test Center", nil
}

// --- fakeZaloSender ---

// fakeZaloSender is a scripted ZaloSender: each field controls one leg of the
// LookupPhone -> SendDM flow. onLookup, when set, runs synchronously inside
// LookupPhone — used to observe repository state at the exact moment DM
// delivery starts.
type fakeZaloSender struct {
	lookupUID    string
	lookupOK     bool
	lookupErr    error
	sendErr      error
	lookupCalled bool
	sendCalled   bool
	onLookup     func()
}

func (f *fakeZaloSender) LookupPhone(_ context.Context, _ uuid.UUID, _ string) (string, bool, error) {
	f.lookupCalled = true
	if f.onLookup != nil {
		f.onLookup()
	}
	return f.lookupUID, f.lookupOK, f.lookupErr
}

func (f *fakeZaloSender) SendDM(_ context.Context, _ uuid.UUID, _, _ string) (string, error) {
	f.sendCalled = true
	if f.sendErr != nil {
		return "", f.sendErr
	}
	return "msg-1", nil
}

// --- fakeOnboarder ---

// fakeOnboarder is a scripted AccountOnboarder keyed by phone, mirroring the
// account-lifecycle slice Accept consumes without a real teachers.Service.
type fakeOnboarder struct {
	byPhone       map[string]teachers.Account
	createErr     error
	reactivateErr error
	created       []uuid.UUID // accountIDs created via CreateInCenter, call order
	reactivated   []uuid.UUID // accountIDs passed to Reactivate, call order
}

func newFakeOnboarder() *fakeOnboarder {
	return &fakeOnboarder{byPhone: map[string]teachers.Account{}}
}

func (f *fakeOnboarder) FindByPhone(_ context.Context, phone string) (teachers.Account, error) {
	acct, ok := f.byPhone[phone]
	if !ok {
		return teachers.Account{}, teachers.ErrNotFound
	}
	return acct, nil
}

func (f *fakeOnboarder) CreateInCenter(_ context.Context, phone, _, _ string, _ uuid.UUID) (uuid.UUID, error) {
	if f.createErr != nil {
		return uuid.UUID{}, f.createErr
	}
	accountID := id.New()
	f.byPhone[phone] = teachers.Account{ID: accountID, Phone: phone, Status: teachers.StatusActive}
	f.created = append(f.created, accountID)
	return accountID, nil
}

func (f *fakeOnboarder) Reactivate(_ context.Context, accountID uuid.UUID, _, _ string) error {
	if f.reactivateErr != nil {
		return f.reactivateErr
	}
	for phone, acct := range f.byPhone {
		if acct.ID != accountID {
			continue
		}
		if acct.Status != teachers.StatusDisabled {
			return teachers.ErrNotDisabled
		}
		acct.Status = teachers.StatusActive
		f.byPhone[phone] = acct
	}
	f.reactivated = append(f.reactivated, accountID)
	return nil
}

// --- fakeOpener ---

// fakeOpener is a scripted MembershipOpener: everMember maps an accountID to
// whether it was ever a member of the inviting center, mirroring the
// real WasEverMember check Accept gates reactivation on.
type fakeOpener struct {
	everMember map[uuid.UUID]bool
	openErr    error
	switchErr  error
	opened     []uuid.UUID
	switched   []uuid.UUID
}

func (f *fakeOpener) OpenMembership(_ context.Context, teacherID, _ uuid.UUID) error {
	if f.openErr != nil {
		return f.openErr
	}
	f.opened = append(f.opened, teacherID)
	return nil
}

func (f *fakeOpener) SwitchTeacherCenter(_ context.Context, teacherID, _ uuid.UUID) error {
	if f.switchErr != nil {
		return f.switchErr
	}
	f.switched = append(f.switched, teacherID)
	return nil
}

func (f *fakeOpener) WasEverMember(_ context.Context, teacherID, _ uuid.UUID) (bool, error) {
	return f.everMember[teacherID], nil
}

// --- noopTxManager ---

// noopTxManager satisfies database.TxManager without a database: the fake
// repository has no real transaction boundary to join.
type noopTxManager struct{}

func (noopTxManager) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(ctx)
}

// --- helpers ---

func testOnboardingConfig() config.OnboardingConfig {
	return config.OnboardingConfig{InviteTTL: 72 * time.Hour}
}

func ownerScope() authctx.Scope {
	return authctx.Scope{TeacherID: id.New(), CenterID: id.New(), IsOwner: true}
}

func memberScopeIn(centerID uuid.UUID) authctx.Scope {
	return authctx.Scope{TeacherID: id.New(), CenterID: centerID, IsOwner: false}
}

// newTestService builds a Service for the owner-only Create/List/Revoke
// tests, which never touch AccountOnboarder/MembershipOpener — fresh no-op
// fakes are enough to satisfy the constructor.
func newTestService(repo Repository, sender ZaloSender) *Service {
	return NewService(repo, sender, newFakeOnboarder(), &fakeOpener{}, noopTxManager{}, testOnboardingConfig(), "https://app.example.com")
}

// newAcceptTestService builds a Service for Preview/Accept tests, where the
// onboarder and opener are the fakes under test.
func newAcceptTestService(repo Repository, onboarder AccountOnboarder, opener MembershipOpener) *Service {
	return NewService(repo, &fakeZaloSender{}, onboarder, opener, noopTxManager{}, testOnboardingConfig(), "https://app.example.com")
}

// mintInvitation inserts a pending (by default) invitation directly into the
// fake repository with a real token so a test can drive Preview/Accept
// exactly the way an HTTP caller would: knowing only the plaintext.
func mintInvitation(t *testing.T, repo *fakeRepository, centerID uuid.UUID, phone, status string, expiresAt time.Time) (plaintext string, invID uuid.UUID) {
	t.Helper()
	pt, hash, err := token.New()
	require.NoError(t, err)
	inv := Invitation{
		ID:        id.New(),
		CenterID:  centerID,
		Phone:     phone,
		TokenHash: hash,
		Status:    status,
		ExpiresAt: expiresAt,
		CreatedAt: time.Now(),
	}
	repo.rows[inv.ID] = inv
	return pt, inv.ID
}

// --- tests ---

func TestCreateReturnsLinkAndCommitsBeforeAttemptingDM(t *testing.T) {
	t.Parallel()

	repo := newFakeRepository()
	sc := ownerScope()

	sender := &fakeZaloSender{lookupUID: "u1", lookupOK: true}
	sender.onLookup = func() {
		// The DM attempt must only start once the invitation is already
		// committed and visible — never before.
		rows, err := repo.List(context.Background(), sc.CenterID)
		require.NoError(t, err)
		require.Len(t, rows, 1)
		require.Equal(t, StatusPending, rows[0].Status)
	}
	svc := newTestService(repo, sender)

	resp, err := svc.Create(context.Background(), sc, CreateRequest{Phone: "0901234567"})
	require.NoError(t, err)
	require.NotEmpty(t, resp.Link)
	require.Contains(t, resp.Link, "https://app.example.com/invite/")
	require.Equal(t, DMStatusSent, resp.DMStatus)
	require.True(t, sender.lookupCalled)
	require.True(t, sender.sendCalled)
}

func TestCreateSupersedesPreviousPendingInvite(t *testing.T) {
	t.Parallel()

	repo := newFakeRepository()
	sc := ownerScope()
	sender := &fakeZaloSender{lookupOK: true, lookupUID: "u1"}
	svc := newTestService(repo, sender)

	first, err := svc.Create(context.Background(), sc, CreateRequest{Phone: "0901234567"})
	require.NoError(t, err)
	second, err := svc.Create(context.Background(), sc, CreateRequest{Phone: "0901234567"})
	require.NoError(t, err)

	require.NotEqual(t, first.ID, second.ID)
	rows, err := repo.List(context.Background(), sc.CenterID)
	require.NoError(t, err)
	require.Len(t, rows, 2)

	byID := map[uuid.UUID]Invitation{}
	for _, r := range rows {
		byID[r.ID] = r
	}
	require.Equal(t, StatusRevoked, byID[first.ID].Status)
	require.Equal(t, StatusPending, byID[second.ID].Status)
}

func TestCreateForbidsNonOwner(t *testing.T) {
	t.Parallel()

	repo := newFakeRepository()
	svc := newTestService(repo, &fakeZaloSender{})

	sc := memberScopeIn(id.New())
	_, err := svc.Create(context.Background(), sc, CreateRequest{Phone: "0901234567"})
	require.Error(t, err)
	require.Equal(t, apperror.CodeForbidden, apperror.From(err).Code)
}

func TestCreateDMStatusSkippedWhenOwnerHasNoLinkedZalo(t *testing.T) {
	t.Parallel()

	repo := newFakeRepository()
	sender := &fakeZaloSender{lookupErr: zalo.ErrNotLinked}
	svc := newTestService(repo, sender)

	resp, err := svc.Create(context.Background(), ownerScope(), CreateRequest{Phone: "0901234567"})
	require.NoError(t, err)
	require.Equal(t, DMStatusSkipped, resp.DMStatus)
	require.False(t, sender.sendCalled)
}

func TestCreateDMStatusSkippedWhenPhoneIsNotAFriend(t *testing.T) {
	t.Parallel()

	repo := newFakeRepository()
	sender := &fakeZaloSender{lookupOK: false}
	svc := newTestService(repo, sender)

	resp, err := svc.Create(context.Background(), ownerScope(), CreateRequest{Phone: "0901234567"})
	require.NoError(t, err)
	require.Equal(t, DMStatusSkipped, resp.DMStatus)
	require.False(t, sender.sendCalled)
}

func TestCreateDMStatusFailedWhenSendDMErrors(t *testing.T) {
	t.Parallel()

	repo := newFakeRepository()
	sender := &fakeZaloSender{lookupOK: true, lookupUID: "u1", sendErr: errors.New("zalo: send rejected")}
	svc := newTestService(repo, sender)

	resp, err := svc.Create(context.Background(), ownerScope(), CreateRequest{Phone: "0901234567"})
	require.NoError(t, err)
	require.Equal(t, DMStatusFailed, resp.DMStatus)
}

func TestCreateDMStatusFailedOnTimeoutWithoutFailingCreate(t *testing.T) {
	t.Parallel()

	repo := newFakeRepository()
	// context.DeadlineExceeded stands in for the bounded dmTimeout tripping —
	// attemptDM treats it like any other lookup error: DM "failed", create
	// still succeeds.
	sender := &fakeZaloSender{lookupErr: context.DeadlineExceeded}
	svc := newTestService(repo, sender)

	resp, err := svc.Create(context.Background(), ownerScope(), CreateRequest{Phone: "0901234567"})
	require.NoError(t, err)
	require.Equal(t, DMStatusFailed, resp.DMStatus)
}

func TestCreateNormalizesPhoneToE164(t *testing.T) {
	t.Parallel()

	repo := newFakeRepository()
	sc := ownerScope()
	svc := newTestService(repo, &fakeZaloSender{lookupOK: true, lookupUID: "u1"})

	resp, err := svc.Create(context.Background(), sc, CreateRequest{Phone: "0901234567"})
	require.NoError(t, err)

	rows, err := repo.List(context.Background(), sc.CenterID)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "+84901234567", rows[0].Phone)
	require.Equal(t, "+84901234567", resp.Phone)
}

func TestRevokeIsIdempotentOnAlreadyRevoked(t *testing.T) {
	t.Parallel()

	repo := newFakeRepository()
	sc := ownerScope()
	svc := newTestService(repo, &fakeZaloSender{lookupOK: true, lookupUID: "u1"})

	created, err := svc.Create(context.Background(), sc, CreateRequest{Phone: "0901234567"})
	require.NoError(t, err)

	require.NoError(t, svc.Revoke(context.Background(), sc, created.ID))
	require.NoError(t, svc.Revoke(context.Background(), sc, created.ID), "revoking an already-revoked invite must still succeed")
}

func TestRevokeOtherCenterReportsNotFound(t *testing.T) {
	t.Parallel()

	repo := newFakeRepository()
	owner := ownerScope()
	svc := newTestService(repo, &fakeZaloSender{lookupOK: true, lookupUID: "u1"})

	created, err := svc.Create(context.Background(), owner, CreateRequest{Phone: "0901234567"})
	require.NoError(t, err)

	otherOwner := ownerScope() // different CenterID
	err = svc.Revoke(context.Background(), otherOwner, created.ID)
	require.Error(t, err)
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code)
}

func TestListDerivesExpiredStatusAndSerializesEmptyAsEmptySlice(t *testing.T) {
	t.Parallel()

	repo := newFakeRepository()
	sc := ownerScope()
	svc := newTestService(repo, &fakeZaloSender{})

	rows, err := svc.List(context.Background(), sc)
	require.NoError(t, err)
	require.NotNil(t, rows)
	require.Empty(t, rows)

	inv := Invitation{
		ID:        id.New(),
		CenterID:  sc.CenterID,
		Phone:     "+84901234567",
		TokenHash: "hash",
		Status:    StatusPending,
		ExpiresAt: time.Now().Add(-time.Hour),
		CreatedAt: time.Now(),
	}
	repo.rows[inv.ID] = inv

	rows, err = svc.List(context.Background(), sc)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "expired", rows[0].Status)
}

// --- Preview ---

func TestPreviewReturnsMaskedPhoneAndCenterName(t *testing.T) {
	t.Parallel()

	repo := newFakeRepository()
	svc := newAcceptTestService(repo, newFakeOnboarder(), &fakeOpener{})
	plaintext, _ := mintInvitation(t, repo, id.New(), "+84901234567", StatusPending, time.Now().Add(time.Hour))

	resp, err := svc.Preview(context.Background(), PreviewRequest{Token: plaintext})
	require.NoError(t, err)
	require.Equal(t, "Test Center", resp.CenterName)
	require.Equal(t, "+84******567", resp.PhoneMasked)
}

func TestPreviewUnknownTokenIsGenericNotFound(t *testing.T) {
	t.Parallel()

	repo := newFakeRepository()
	svc := newAcceptTestService(repo, newFakeOnboarder(), &fakeOpener{})

	_, err := svc.Preview(context.Background(), PreviewRequest{Token: "does-not-exist"})
	require.Error(t, err)
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code)
}

func TestPreviewNonPendingOrExpiredIsGenericNotFound(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		status    string
		expiresAt time.Time
	}{
		"expired":  {StatusPending, time.Now().Add(-time.Hour)},
		"accepted": {StatusAccepted, time.Now().Add(time.Hour)},
		"revoked":  {StatusRevoked, time.Now().Add(time.Hour)},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			repo := newFakeRepository()
			svc := newAcceptTestService(repo, newFakeOnboarder(), &fakeOpener{})
			plaintext, _ := mintInvitation(t, repo, id.New(), "+84901234567", tc.status, tc.expiresAt)

			_, err := svc.Preview(context.Background(), PreviewRequest{Token: plaintext})
			require.Error(t, err)
			require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code)
		})
	}
}

// --- Accept ---

func TestAcceptNewPhoneCreatesAccountOpensMembershipAndMarksAccepted(t *testing.T) {
	t.Parallel()

	repo := newFakeRepository()
	onboarder := newFakeOnboarder()
	opener := &fakeOpener{}
	svc := newAcceptTestService(repo, onboarder, opener)
	plaintext, invID := mintInvitation(t, repo, id.New(), "+84901234567", StatusPending, time.Now().Add(time.Hour))

	err := svc.Accept(context.Background(), AcceptRequest{Token: plaintext, FullName: "Nguyễn Văn A", Password: "matkhau123"})
	require.NoError(t, err)

	require.Len(t, onboarder.created, 1, "a new phone must create exactly one account")
	require.Empty(t, onboarder.reactivated, "a new phone must never call Reactivate")
	require.Equal(t, onboarder.created, opener.opened, "the created account must be the one whose membership opens")
	require.Empty(t, opener.switched, "a brand-new account never switches center — CreateInCenter already points it at the inviting center")

	row := repo.rows[invID]
	require.Equal(t, StatusAccepted, row.Status)
	require.NotNil(t, row.AcceptedAt)
}

func TestAcceptDisabledEverMemberReactivatesOpensAndSwitches(t *testing.T) {
	t.Parallel()

	repo := newFakeRepository()
	onboarder := newFakeOnboarder()
	centerID := id.New()
	accountID := id.New()
	phone := "+84901234567"
	onboarder.byPhone[phone] = teachers.Account{ID: accountID, Phone: phone, Status: teachers.StatusDisabled}
	opener := &fakeOpener{everMember: map[uuid.UUID]bool{accountID: true}}
	svc := newAcceptTestService(repo, onboarder, opener)
	plaintext, invID := mintInvitation(t, repo, centerID, phone, StatusPending, time.Now().Add(time.Hour))

	err := svc.Accept(context.Background(), AcceptRequest{Token: plaintext, FullName: "Nguyễn Văn A", Password: "matkhau123"})
	require.NoError(t, err)

	require.Equal(t, []uuid.UUID{accountID}, onboarder.reactivated)
	require.Empty(t, onboarder.created, "a re-invite must never create a second account")
	require.Equal(t, []uuid.UUID{accountID}, opener.opened)
	require.Equal(t, []uuid.UUID{accountID}, opener.switched)

	row := repo.rows[invID]
	require.Equal(t, StatusAccepted, row.Status)
}

func TestAcceptPropagatesGenuineOnboarderErrorsUnmasked(t *testing.T) {
	t.Parallel()

	repo := newFakeRepository()
	onboarder := newFakeOnboarder()
	sentinel := apperror.Internal(errors.New("boom"))
	onboarder.createErr = sentinel
	svc := newAcceptTestService(repo, onboarder, &fakeOpener{})
	plaintext, _ := mintInvitation(t, repo, id.New(), "+84901234567", StatusPending, time.Now().Add(time.Hour))

	err := svc.Accept(context.Background(), AcceptRequest{Token: plaintext, FullName: "X", Password: "password1"})
	require.Same(t, sentinel, err, "a genuine onboarder failure must propagate, not be masked as the generic rejection")
}

// TestAcceptRejectionIsIdenticalAcrossEveryFailureReason proves the
// anti-enumeration guarantee: an unknown token, an expired token, a
// used (already-accepted) token, a revoked token, an already-active
// account, and a disabled account that was never a member of this center
// all answer the exact same shared errAcceptRejected value — never a
// distinguishable one.
func TestAcceptRejectionIsIdenticalAcrossEveryFailureReason(t *testing.T) {
	t.Parallel()

	centerID := id.New()
	activePhone := "+84901111111"
	neverMemberPhone := "+84902222222"
	neverMemberID := id.New()

	newScenario := func() (*Service, *fakeRepository) {
		repo := newFakeRepository()
		onboarder := newFakeOnboarder()
		onboarder.byPhone[activePhone] = teachers.Account{ID: id.New(), Phone: activePhone, Status: teachers.StatusActive}
		onboarder.byPhone[neverMemberPhone] = teachers.Account{ID: neverMemberID, Phone: neverMemberPhone, Status: teachers.StatusDisabled}
		opener := &fakeOpener{everMember: map[uuid.UUID]bool{}}
		return newAcceptTestService(repo, onboarder, opener), repo
	}

	scenarios := map[string]func(t *testing.T, svc *Service, repo *fakeRepository) error{
		"unknown token": func(_ *testing.T, svc *Service, _ *fakeRepository) error {
			return svc.Accept(context.Background(), AcceptRequest{Token: "does-not-exist", FullName: "X", Password: "password1"})
		},
		"expired token": func(t *testing.T, svc *Service, repo *fakeRepository) error {
			pt, _ := mintInvitation(t, repo, centerID, "+84903333333", StatusPending, time.Now().Add(-time.Hour))
			return svc.Accept(context.Background(), AcceptRequest{Token: pt, FullName: "X", Password: "password1"})
		},
		"already accepted (used) token": func(t *testing.T, svc *Service, repo *fakeRepository) error {
			pt, _ := mintInvitation(t, repo, centerID, "+84904444444", StatusAccepted, time.Now().Add(time.Hour))
			return svc.Accept(context.Background(), AcceptRequest{Token: pt, FullName: "X", Password: "password1"})
		},
		"revoked token": func(t *testing.T, svc *Service, repo *fakeRepository) error {
			pt, _ := mintInvitation(t, repo, centerID, "+84905555555", StatusRevoked, time.Now().Add(time.Hour))
			return svc.Accept(context.Background(), AcceptRequest{Token: pt, FullName: "X", Password: "password1"})
		},
		"already-active account": func(t *testing.T, svc *Service, repo *fakeRepository) error {
			pt, _ := mintInvitation(t, repo, centerID, activePhone, StatusPending, time.Now().Add(time.Hour))
			return svc.Accept(context.Background(), AcceptRequest{Token: pt, FullName: "X", Password: "password1"})
		},
		"disabled account never a member of this center": func(t *testing.T, svc *Service, repo *fakeRepository) error {
			pt, _ := mintInvitation(t, repo, centerID, neverMemberPhone, StatusPending, time.Now().Add(time.Hour))
			return svc.Accept(context.Background(), AcceptRequest{Token: pt, FullName: "X", Password: "password1"})
		},
	}

	for name, run := range scenarios {
		t.Run(name, func(t *testing.T) {
			svc, repo := newScenario()
			err := run(t, svc, repo)
			require.Error(t, err)
			require.Same(t, errAcceptRejected, err, "every rejection branch must answer the identical shared value")
		})
	}
}
