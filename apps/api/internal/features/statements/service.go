package statements

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"teka/apps/api/internal/config"
	"teka/apps/api/internal/database"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/id"
	"teka/apps/api/internal/shared/pagination"
)

// These mirror the billing_periods.status and invoices.status values this
// package reads. Sharing the table's own vocabulary, rather than importing
// the billing package's Go types, keeps statements decoupled from billing's
// internals — the same choice collections makes for the same reason.
const (
	periodStatusClosed = "closed"
	invoiceStatusVoid  = "void"
)

// statementTTL is how long a generated link stays valid before it can no
// longer be resolved, independent of revocation.
const statementTTL = 90 * 24 * time.Hour

// statementSendRoles is the capability map's answer to which class-staff
// roles may send statements, resolved once here — the service layer is the
// capability map's home. Repository SQL only ever binds this slice; it never
// consults the map itself.
var statementSendRoles = authctx.StaffRolesFor(authctx.CapStatementSend)

// Service owns statement generation and the teacher-facing reads/writes
// around it: token derivation is keyed on cfg.TokenKey, and links are built
// against cfg.PublicBaseURL. Both are required to construct a Service, which
// is why they travel together as one config sub-struct rather than as loose
// parameters. bank and qr back the public, unauthenticated view's payment QR
// block (see render.go/qr.go) — an unconfigured bank is a supported state,
// not a construction error.
type Service struct {
	repo Repository
	tx   database.TxManager
	cfg  config.StatementsConfig
	bank BankConfig
	qr   QRBuilder
	now  func() time.Time
}

// NewService builds the statements service.
func NewService(repo Repository, tx database.TxManager, cfg config.StatementsConfig, bank BankConfig, qr QRBuilder) *Service {
	return &Service{repo: repo, tx: tx, cfg: cfg, bank: bank, qr: qr, now: time.Now}
}

// GenerateResult summarises one Generate call: how many statements were
// newly created, how many pre-existing ones had their total_due refreshed,
// and how many were left untouched because they were already revoked.
// Statements holds only the created and refreshed rows — a skipped, revoked
// statement was not touched, and surfacing it here would read as "generated"
// when nothing about it changed.
type GenerateResult struct {
	Created        int
	Refreshed      int
	SkippedRevoked int
	Statements     []Row
}

// Generate creates or refreshes one statement per contact with at least one
// non-void invoice in periodID. The period must be closed: statements are a
// closed period's output, not a preview of one still being billed. Existing,
// not-yet-revoked statements have only their total_due refreshed — their
// token, and therefore their link, never changes underneath a parent who
// already received it. An existing, revoked statement is left alone rather
// than resurrected: revoking a link is meant to kill it for good.
//
// sc authorizes the call (owner may act on any teacher's period in the
// center; a member only on their own), but every write is anchored on
// periodScope — the period's own owning teacher, not necessarily sc's —
// so an owner generating a member's statements never reassigns them to
// itself.
func (s *Service) Generate(ctx context.Context, sc authctx.Scope, periodID uuid.UUID) (*GenerateResult, error) {
	return s.generate(ctx, sc, periodID, false)
}

// GenerateForSend is Generate for the delegated send path only: the period is
// resolved with reports oversight, so a reports.send holder can (re)issue
// statements as part of a bulk send on another teacher's period. The
// standalone generate endpoint keeps calling Generate — delegation includes
// no standalone generate right (plan decision D4).
func (s *Service) GenerateForSend(ctx context.Context, sc authctx.Scope, periodID uuid.UUID) (*GenerateResult, error) {
	return s.generate(ctx, sc, periodID, true)
}

// AuthorizeClassSend is the one authorization gate every class-scoped
// statement path (and notifications' class-scoped send) runs: reports
// oversight (owner or reports.send holder) passes outright, as does an
// active sending-role stint on the class. A caller who can read the class
// (any stint, ended included) but may not send gets an honest 403; anyone
// else — including a caller probing a class that exists but was never theirs
// — gets the same neutral 404 a nonexistent class produces.
func (s *Service) AuthorizeClassSend(ctx context.Context, sc authctx.Scope, classID uuid.UUID) error {
	sendable, readable, err := s.repo.ClassSendAccess(ctx, sc, classID, statementSendRoles)
	if err != nil {
		return s.translate(err)
	}
	if sc.ReportsOversight() || sendable {
		return nil
	}
	if readable {
		return apperror.Forbidden("your role on this class does not allow this action")
	}
	return apperror.NotFound("class")
}

// GenerateForSendClass mints/refreshes CLASS-scoped statement copies for one
// class in one period — the class staff's counterpart of GenerateForSend.
// Authorization is the class-send gate, never period ownership: the period is
// resolved center-wide because a class's students can be billed under any
// teacher's period after a handoff, and it is the caller's active stint on
// the class (or oversight) that entitles them to act. Targeting requires a
// still-active class enrollment; the money written is the period's class
// charges as invoiced (see the repository's class query doc comments). Every
// class copy derives its token WITH the class id, so its link can never
// resolve the family statement, and upserts land on the class partial unique
// — a class send never touches the family row.
func (s *Service) GenerateForSendClass(ctx context.Context, sc authctx.Scope, periodID, classID uuid.UUID) (*GenerateResult, error) {
	var result GenerateResult
	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.AuthorizeClassSend(ctx, sc, classID); err != nil {
			return err
		}
		info, err := s.repo.GetPeriodStatusCenter(ctx, sc, periodID)
		if err != nil {
			return s.translate(err)
		}
		if info.Status != periodStatusClosed {
			return apperror.Conflict("period is not closed")
		}
		periodScope := authctx.Scope{TeacherID: info.TeacherID, CenterID: sc.CenterID}

		targets, err := s.repo.TargetContactsClass(ctx, periodScope, sc, periodID, classID)
		if err != nil {
			return apperror.From(err)
		}
		totals, err := s.repo.ContactClassTotals(ctx, periodScope, periodID, classID)
		if err != nil {
			return apperror.From(err)
		}

		now := s.now()
		rows := make([]Row, 0, len(targets))
		for _, target := range targets {
			statementID := id.New()
			classRef := classID
			candidate := &Statement{
				ID:        statementID,
				ContactID: target.ContactID,
				PeriodID:  periodID,
				ClassID:   &classRef,
				TokenHash: hashToken(deriveToken(s.cfg.TokenKey, statementID, &classRef)),
				ExpiresAt: now.Add(statementTTL),
				TotalDue:  totals[target.ContactID],
			}

			created, skippedRevoked, err := s.repo.UpsertStatement(ctx, periodScope, candidate)
			if err != nil {
				return apperror.From(err)
			}
			if skippedRevoked {
				result.SkippedRevoked++
				continue
			}
			if created {
				result.Created++
			} else {
				result.Refreshed++
			}
			rows = append(rows, Row{
				Statement:       *candidate,
				ContactFullName: target.FullName,
				ContactPhone:    target.Phone,
				PhoneVisible:    target.PhoneVisible,
			})
		}
		result.Statements = rows
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (s *Service) generate(ctx context.Context, sc authctx.Scope, periodID uuid.UUID, oversight bool) (*GenerateResult, error) {
	var result GenerateResult
	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		lookup := s.repo.GetPeriodStatus
		if oversight {
			lookup = s.repo.GetPeriodStatusRead
		}
		info, err := lookup(ctx, sc, periodID)
		if err != nil {
			return s.translate(err)
		}
		if info.Status != periodStatusClosed {
			return apperror.Conflict("period is not closed")
		}
		periodScope := authctx.Scope{TeacherID: info.TeacherID, CenterID: sc.CenterID}

		targets, err := s.repo.TargetContacts(ctx, periodScope, sc, periodID)
		if err != nil {
			return apperror.From(err)
		}
		totals, err := s.repo.ContactTotals(ctx, periodScope, periodID)
		if err != nil {
			return apperror.From(err)
		}

		now := s.now()
		rows := make([]Row, 0, len(targets))
		for _, target := range targets {
			statementID := id.New()
			candidate := &Statement{
				ID:        statementID,
				ContactID: target.ContactID,
				PeriodID:  periodID,
				// This path mints family statements only (ClassID nil), so the
				// token is derived without a class component.
				TokenHash: hashToken(deriveToken(s.cfg.TokenKey, statementID, nil)),
				ExpiresAt: now.Add(statementTTL),
				TotalDue:  totals[target.ContactID],
			}

			created, skippedRevoked, err := s.repo.UpsertStatement(ctx, periodScope, candidate)
			if err != nil {
				return apperror.From(err)
			}
			if skippedRevoked {
				result.SkippedRevoked++
				continue
			}
			if created {
				result.Created++
			} else {
				result.Refreshed++
			}
			rows = append(rows, Row{
				Statement:       *candidate,
				ContactFullName: target.FullName,
				ContactPhone:    target.Phone,
				PhoneVisible:    target.PhoneVisible,
			})
		}
		result.Statements = rows
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// List returns a page of one period's statements, center-scoped by sc with
// reports oversight (owner or reports.send holder). periodID must be
// visible to sc; an unknown or out-of-tenancy id is reported as if the
// period does not exist.
func (s *Service) List(ctx context.Context, sc authctx.Scope, periodID uuid.UUID, p pagination.Params) ([]Row, int64, error) {
	if _, err := s.repo.GetPeriodStatusRead(ctx, sc, periodID); err != nil {
		return nil, 0, s.translate(err)
	}
	rows, total, err := s.repo.ListByPeriod(ctx, sc, periodID, p)
	if err != nil {
		return nil, 0, apperror.From(err)
	}
	return rows, total, nil
}

// ListClass returns a page of one period's CLASS-scoped statement copies for
// one class. Gated by AuthorizeClassSend (so it 404s/403s exactly like the
// class send itself), then resolved center-wide like every class-scoped path
// — see GenerateForSendClass.
func (s *Service) ListClass(ctx context.Context, sc authctx.Scope, periodID, classID uuid.UUID, p pagination.Params) ([]Row, int64, error) {
	if err := s.AuthorizeClassSend(ctx, sc, classID); err != nil {
		return nil, 0, err
	}
	if _, err := s.repo.GetPeriodStatusCenter(ctx, sc, periodID); err != nil {
		return nil, 0, s.translate(err)
	}
	rows, total, err := s.repo.ListByPeriodClass(ctx, sc, periodID, classID, p)
	if err != nil {
		return nil, 0, apperror.From(err)
	}
	return rows, total, nil
}

// Get returns one statement, center-scoped by sc with owner oversight. A
// statement outside sc's tenancy is reported identically to a missing one.
func (s *Service) Get(ctx context.Context, sc authctx.Scope, statementID uuid.UUID) (*Row, error) {
	row, err := s.repo.GetByID(ctx, sc, statementID)
	if err != nil {
		return nil, s.translate(err)
	}
	return row, nil
}

// Revoke kills one statement's link, center-scoped by sc with owner
// oversight. Idempotent: revoking an already-revoked statement is a no-op
// success, not a conflict.
func (s *Service) Revoke(ctx context.Context, sc authctx.Scope, statementID uuid.UUID) error {
	if err := s.repo.Revoke(ctx, sc, statementID); err != nil {
		return s.translate(err)
	}
	return nil
}

// ToResponse builds the teacher-facing DTO for one statement row, recomputing
// its plaintext token (and the URL built from it) on the fly from
// s.cfg.TokenKey — the token itself is never persisted, so this is the only
// place a caller can ever see it again. This method, and therefore the
// token/URL fields it produces, must only ever be reached from a
// teacher-authenticated request path.
//
// sc masks the privacy-sensitive fields: the phone follows the one phone rule
// (Scope.PhoneVisible over the row's derived grant), and the URL — a public
// bearer token for the whole family's statement — is derived only for
// owner/oversight callers; for anyone else the token is never even computed.
func (s *Service) ToResponse(sc authctx.Scope, row Row) StatementResponse {
	resp := StatementResponse{
		ID:            row.ID,
		ContactID:     row.ContactID,
		ContactName:   row.ContactFullName,
		ClassID:       row.ClassID,
		TotalDue:      row.TotalDue,
		ExpiresAt:     row.ExpiresAt,
		RevokedAt:     row.RevokedAt,
		ViewCount:     row.ViewCount,
		FirstViewedAt: row.FirstViewedAt,
		LastViewedAt:  row.LastViewedAt,
	}
	if sc.PhoneVisible(row.PhoneVisible) {
		resp.Phone = &row.ContactPhone
	}
	if sc.ReportsOversight() {
		resp.URL = s.linkFor(row)
	}
	return resp
}

// linkFor derives one row's public link. Callers gate WHO may see it; this
// only builds it.
func (s *Service) linkFor(row Row) *string {
	url := s.cfg.PublicBaseURL + "/s/" + deriveToken(s.cfg.TokenKey, row.ID, row.ClassID)
	return &url
}

// ToResponseForSend is ToResponse with the URL unconditionally derived — for
// callers that have ALREADY passed a send authorization (reports oversight,
// the period's own teacher sending their own statements, or the class-send
// gate). The notifications sender uses it to put each family's link into the
// message it queues; it must never back a plain read endpoint.
func (s *Service) ToResponseForSend(sc authctx.Scope, row Row) StatementResponse {
	resp := s.ToResponse(sc, row)
	if resp.URL == nil {
		resp.URL = s.linkFor(row)
	}
	return resp
}

// ToResponseAuthorized is ToResponse plus the class-send grant resolved live:
// a caller below reports oversight still gets a CLASS copy's URL when they
// hold an active sending-role stint on that class — the same right that let
// them mint the copy lets them re-read its link. A failed (or erroring) gate
// leaves the URL null rather than failing the read; the row itself was
// already visible through scopedRead.
func (s *Service) ToResponseAuthorized(ctx context.Context, sc authctx.Scope, row Row) StatementResponse {
	resp := s.ToResponse(sc, row)
	if resp.URL == nil && row.ClassID != nil {
		if err := s.AuthorizeClassSend(ctx, sc, *row.ClassID); err == nil {
			resp.URL = s.linkFor(row)
		}
	}
	return resp
}

// LookupPublic resolves a plaintext token to its statement for the public,
// unauthenticated view: hash it, look it up, then check revocation and
// expiry (soft-deletion is already excluded by GetByTokenHash's own
// GORM-managed deleted_at filter). Every failure branch — an unknown,
// malformed, revoked, expired, or soft-deleted token — returns the identical
// ErrNotFound, so the HTTP layer's neutral 404 never differs by cause;
// distinguishing them would already confirm a token once existed.
func (s *Service) LookupPublic(ctx context.Context, token string) (*Statement, error) {
	stmt, err := s.repo.GetByTokenHash(ctx, hashToken(token))
	if errors.Is(err, ErrNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, apperror.Internal(err)
	}
	if stmt.RevokedAt != nil || !s.now().Before(stmt.ExpiresAt) {
		return nil, ErrNotFound
	}
	return stmt, nil
}

// RenderPublic assembles a resolved statement's parent-facing payload. Every
// read is scoped by a scope derived straight from stmt's own
// TeacherID/CenterID/ContactID/PeriodID — never anything the HTTP caller
// supplies directly — so a forged or guessed path can never pull another
// family's data, and there is no owner bypass on this path. Returns
// ErrNotFound (the same neutral 404 LookupPublic's failures produce) when the
// family has no non-void invoice left for the period, or when it has already
// finished paying: a statement expires the moment its outstanding balance
// reaches zero, independent of ExpiresAt. The second return value is the raw
// VietQR payload string for the qr.png route to render; it is empty exactly
// when PublicStatement.QR is nil.
func (s *Service) RenderPublic(ctx context.Context, stmt *Statement) (*PublicStatement, string, error) {
	scope := authctx.Scope{TeacherID: stmt.TeacherID, CenterID: stmt.CenterID}

	invoiceRows, err := s.repo.InvoicesWithLines(ctx, scope, stmt.ContactID, stmt.PeriodID)
	if err != nil {
		return nil, "", apperror.Internal(err)
	}
	if len(invoiceRows) == 0 {
		return nil, "", ErrNotFound
	}

	sessionRows, err := s.repo.LiveSessions(ctx, scope, stmt.ContactID, stmt.PeriodID)
	if err != nil {
		return nil, "", apperror.Internal(err)
	}

	var payload PublicStatement
	if stmt.ClassID != nil {
		// A class copy shows only its own class's charges: no opening
		// balance, no adjustments (they are family-level money), no payment
		// breakdown. buildClassStatement zeroes Outstanding when the family
		// owes nothing across the class's invoices, so the shared
		// paid-up-means-gone rule below retires the class link exactly when
		// it retires the family one. Children empty means the class has no
		// line at all on this family's period — same neutral 404.
		payload = buildClassStatement(invoiceRows, sessionRows, *stmt.ClassID)
		if len(payload.Children) == 0 {
			return nil, "", ErrNotFound
		}
	} else {
		adjustmentRows, err := s.repo.Adjustments(ctx, scope, stmt.ContactID, stmt.PeriodID)
		if err != nil {
			return nil, "", apperror.Internal(err)
		}
		payload = buildPublicStatement(invoiceRows, sessionRows, adjustmentRows)
	}
	if payload.Totals.Outstanding <= 0 {
		return nil, "", ErrNotFound
	}

	token := deriveToken(s.cfg.TokenKey, stmt.ID, stmt.ClassID)
	qrPayload := attachQR(&payload, s.qr, s.bank, s.cfg.PublicBaseURL, token)
	return &payload, qrPayload, nil
}

// RenderQR turns a payload built by RenderPublic's second return value into
// a PNG image.
func (s *Service) RenderQR(payload string) ([]byte, error) {
	return s.qr.Render(payload)
}

// TouchView records one open of a statement's public link. Called after the
// response has already been written; the caller is expected to log a
// failure rather than let a view counter's own error affect what a parent
// sees. sc must be derived from the resolved statement row, never the
// (absent) caller's — this is reached only from the unauthenticated public
// path.
func (s *Service) TouchView(ctx context.Context, sc authctx.Scope, statementID uuid.UUID) error {
	return s.repo.TouchView(ctx, sc, statementID)
}

// ChildFigures is one child's session/amount summary for one period — the
// exact numbers a MessageInput.ChildSummary needs, read straight off the
// invoice_lines snapshot: BillableCount/AbsentCount/Amount summed across
// every class line on that child's own invoice.
type ChildFigures struct {
	StudentName   string
	BillableCount int
	AbsentCount   int
	Amount        int64
}

// ContactFigures is one contact's per-period message inputs: every child
// billed to them this period, plus the family-level money totals the public
// statement page itself shows (PublicTotals). Built once per period by
// PeriodFigures so a bulk notification send never re-derives these sums a
// second time and can never disagree with what RenderPublic shows a parent.
type ContactFigures struct {
	ContactID       uuid.UUID
	ContactName     string
	PeriodLabel     string
	Children        []ChildFigures
	OpeningBalance  int64
	AdjustmentTotal int64
	TotalDue        int64
	Outstanding     int64
}

// PeriodFigures reads every non-void invoice (with lines) for periodID, for
// every contact at once, in a single round trip, and aggregates it into one
// ContactFigures per contact. Independent of how many contacts the period
// has: this is always exactly one query, never one per contact — the
// property notifications.Service.BulkSend's scale requirement depends on.
// The sums it produces (opening balance, current charge, adjustments, total
// due, outstanding) are computed by the same grouping RenderPublic's
// buildPublicStatement uses, off the same invoices/invoice_lines snapshot, so
// a bulk-sent message's total can never disagree with the number a parent
// sees at their own statement link.
//
// sc authorizes the call with reports oversight (owner or reports.send
// holder may act on any teacher's period in the center; a plain member only
// on their own), then the read is derived from periodScope — the period's
// own owning teacher — so an oversight caller's bulk send over a member's
// period reads exactly that member's figures.
func (s *Service) PeriodFigures(ctx context.Context, sc authctx.Scope, periodID uuid.UUID) (map[uuid.UUID]ContactFigures, error) {
	info, err := s.repo.GetPeriodStatusRead(ctx, sc, periodID)
	if err != nil {
		return nil, s.translate(err)
	}
	periodScope := authctx.Scope{TeacherID: info.TeacherID, CenterID: sc.CenterID}

	rows, err := s.repo.PeriodInvoiceLines(ctx, periodScope, periodID)
	if err != nil {
		return nil, apperror.Internal(err)
	}
	return assemblePeriodFigures(rows), nil
}

// assemblePeriodFigures groups periodInvoiceLinesQuery's rows — ordered by
// contact_id, student_name, invoice id, line — into one ContactFigures per
// contact and one ChildFigures per invoice (one invoice per student per
// period), mirroring buildPublicStatement's own flush-on-boundary-change
// pass over the identical row shape. Pure: no I/O, so it is unit-testable
// over fixture rows without a database.
func assemblePeriodFigures(rows []InvoiceLineRow) map[uuid.UUID]ContactFigures {
	byContact := make(map[uuid.UUID]*ContactFigures)

	var currentContact *ContactFigures
	var currentChild *ChildFigures
	var currentInvoiceID uuid.UUID

	flushChild := func() {
		if currentChild != nil && currentContact != nil {
			currentContact.Children = append(currentContact.Children, *currentChild)
		}
		currentChild = nil
	}

	for _, row := range rows {
		if currentContact == nil || row.ContactID != currentContact.ContactID {
			flushChild()
			cf, ok := byContact[row.ContactID]
			if !ok {
				cf = &ContactFigures{
					ContactID:   row.ContactID,
					ContactName: row.ContactName,
					PeriodLabel: fmt.Sprintf("%02d/%d", row.PeriodMonth, row.PeriodYear),
				}
				byContact[row.ContactID] = cf
			}
			currentContact = cf
		}
		if currentChild == nil || row.InvoiceID != currentInvoiceID {
			flushChild()
			currentInvoiceID = row.InvoiceID
			currentChild = &ChildFigures{StudentName: row.StudentName, Amount: row.CurrentCharge}
			currentContact.OpeningBalance += row.OpeningBalance
			currentContact.AdjustmentTotal += row.AdjustmentTotal
			currentContact.TotalDue += row.TotalDue
			currentContact.Outstanding += row.TotalDue - row.PaidAmount
		}
		if row.LineID != nil {
			currentChild.BillableCount += *row.BillableCount
			currentChild.AbsentCount += *row.AbsentCount
		}
	}
	flushChild()

	out := make(map[uuid.UUID]ContactFigures, len(byContact))
	for contactID, cf := range byContact {
		out[contactID] = *cf
	}
	return out
}

// PeriodFiguresClass is PeriodFigures for one class's copies: the same
// single-round-trip guarantee, but figures assembled from the class's own
// lines only — matching what buildClassStatement shows a parent at the class
// link. Gated by AuthorizeClassSend and resolved center-wide, like every
// class-scoped path.
func (s *Service) PeriodFiguresClass(ctx context.Context, sc authctx.Scope, periodID, classID uuid.UUID) (map[uuid.UUID]ContactFigures, error) {
	if err := s.AuthorizeClassSend(ctx, sc, classID); err != nil {
		return nil, err
	}
	info, err := s.repo.GetPeriodStatusCenter(ctx, sc, periodID)
	if err != nil {
		return nil, s.translate(err)
	}
	periodScope := authctx.Scope{TeacherID: info.TeacherID, CenterID: sc.CenterID}

	rows, err := s.repo.PeriodClassInvoiceLines(ctx, periodScope, periodID, classID)
	if err != nil {
		return nil, apperror.Internal(err)
	}
	return assembleClassFigures(rows), nil
}

// assembleClassFigures groups periodClassInvoiceLinesQuery's rows into one
// ContactFigures per contact, mirroring assemblePeriodFigures' linear pass
// but with class-copy money semantics: a child's Amount and the contact's
// TotalDue are the CLASS lines' sum (never the invoice totals), opening
// balance and adjustments stay zero (family-level money), and Outstanding is
// all-or-nothing — the full class total while the family still owes anything
// across the class's invoices, zero once they've paid up, matching
// buildClassStatement. Pure: no I/O. Every row is a real class line (the
// query INNER JOINs), so the Line* pointers are never nil here.
func assembleClassFigures(rows []InvoiceLineRow) map[uuid.UUID]ContactFigures {
	byContact := make(map[uuid.UUID]*ContactFigures)
	familyOutstanding := make(map[uuid.UUID]int64)

	var currentContact *ContactFigures
	var currentChild *ChildFigures
	var currentInvoiceID uuid.UUID

	flushChild := func() {
		if currentChild != nil && currentContact != nil {
			currentContact.Children = append(currentContact.Children, *currentChild)
		}
		currentChild = nil
	}

	for _, row := range rows {
		if currentContact == nil || row.ContactID != currentContact.ContactID {
			flushChild()
			cf, ok := byContact[row.ContactID]
			if !ok {
				cf = &ContactFigures{
					ContactID:   row.ContactID,
					ContactName: row.ContactName,
					PeriodLabel: fmt.Sprintf("%02d/%d", row.PeriodMonth, row.PeriodYear),
				}
				byContact[row.ContactID] = cf
			}
			currentContact = cf
		}
		if currentChild == nil || row.InvoiceID != currentInvoiceID {
			flushChild()
			currentInvoiceID = row.InvoiceID
			currentChild = &ChildFigures{StudentName: row.StudentName}
			familyOutstanding[row.ContactID] += row.TotalDue - row.PaidAmount
		}
		currentChild.BillableCount += *row.BillableCount
		currentChild.AbsentCount += *row.AbsentCount
		currentChild.Amount += *row.LineAmount
		currentContact.TotalDue += *row.LineAmount
	}
	flushChild()

	out := make(map[uuid.UUID]ContactFigures, len(byContact))
	for contactID, cf := range byContact {
		if familyOutstanding[contactID] > 0 {
			cf.Outstanding = cf.TotalDue
		}
		out[contactID] = *cf
	}
	return out
}

// translate maps domain errors onto the API error contract, keeping the
// domain error as the cause so errors.Is still works on the result.
func (s *Service) translate(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrNotFound):
		return apperror.NotFound("statement")
	case errors.Is(err, ErrPeriodNotFound):
		return apperror.NotFound("billing period")
	case errors.Is(err, ErrClassNotFound):
		return apperror.NotFound("class")
	default:
		return apperror.From(err)
	}
}
