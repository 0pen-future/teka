package notifications

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"teka/apps/api/internal/config"
	"teka/apps/api/internal/database"
	"teka/apps/api/internal/features/statements"
	"teka/apps/api/internal/features/zalo"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/id"
)

// StatementsSource is the slice of the statements feature BulkSend needs:
// generating/refreshing this period's statements (so a bulk send is one
// teacher action, never "generate, then separately send"), reading every
// contact's per-period message figures in one round trip (so a message's
// total can never disagree with what RenderPublic itself shows a parent),
// and building the teacher-facing URL for one statement row.
// *statements.Service satisfies this — declared here, a consumer-defined
// interface, so notifications depends on statements' public service
// contract, never a second implementation of its sums.
type StatementsSource interface {
	Generate(ctx context.Context, sc authctx.Scope, periodID uuid.UUID) (*statements.GenerateResult, error)
	PeriodFigures(ctx context.Context, sc authctx.Scope, periodID uuid.UUID) (map[uuid.UUID]statements.ContactFigures, error)
	ToResponse(row statements.Row) statements.StatementResponse
}

// ZaloSender is the slice of the Zalo feature the personal channel needs:
// checking the teacher's session is alive before a run is created, and the DM
// send the run itself is made of. *zalo.Service satisfies this — a
// consumer-defined interface, so tests drive the channel without a Zalo
// session and the zalo package never learns notifications exists.
type ZaloSender interface {
	DMSender
	VerifyAccount(ctx context.Context, teacherID uuid.UUID) error
}

// Service owns notification queueing, sending, and status.
type Service struct {
	repo       Repository
	tx         database.TxManager
	statements StatementsSource
	zalo       ZaloSender
	runs       *RunManager
	log        *slog.Logger
	cfg        config.NotificationsConfig
}

// NewService builds the notifications service. A nil logger falls back to
// slog.Default.
func NewService(repo Repository, tx database.TxManager, statementsSvc StatementsSource, zaloSender ZaloSender, log *slog.Logger, cfg config.NotificationsConfig) *Service {
	if log == nil {
		log = slog.Default()
	}
	runs := NewRunManager(repo, zaloSender, log,
		time.Duration(cfg.PaceMinSeconds)*time.Second,
		time.Duration(cfg.PaceMaxSeconds)*time.Second)
	return &Service{repo: repo, tx: tx, statements: statementsSvc, zalo: zaloSender, runs: runs, log: log, cfg: cfg}
}

// Close stops any background sending run and waits for it to finish writing.
// Rows a stopped run never reached stay queued; the next boot's reconcile
// marks their run interrupted.
func (s *Service) Close() {
	s.runs.Close()
}

// BulkSend queues one notification per eligible contact in periodID, in a
// single transaction: it first regenerates/refreshes the period's statements
// (so totals are current and every contact has a live link), then builds and
// queues one message per eligible contact. purpose=PurposeStatements targets
// every contact with a non-void invoice in the period (Generate's own result
// already excludes voided-invoice-only contacts and skips revoked
// statements); purpose=PurposeReminder further narrows that set to
// outstanding > 0. A contact fully paid under a reminder send is counted in
// SkippedPaidCount, not queued. If the resolved channel cannot currently
// send (an unconfigured provider, or an unsupported channel), the whole call
// fails and nothing is written — including the statement refresh — so a bad
// channel choice never leaves partial state behind.
// Under channel=ChannelZaloPersonal the call additionally splits the target
// set by Zalo mapping: mapped contacts queue as zalo_personal rows attached
// to a notification run that a background goroutine sends one by one after
// commit, unmapped contacts fall back to zalo_manual copy-paste rows exactly
// as if the manual channel had been chosen for them. BulkText then carries
// only the fallback rows — the auto-sent messages have nothing to copy.
func (s *Service) BulkSend(ctx context.Context, sc authctx.Scope, periodID uuid.UUID, req BulkSendRequest) (*BulkSendResponse, error) {
	purpose := normalizePurpose(req.Purpose)
	channel := req.Channel
	if channel == "" {
		channel = s.cfg.DefaultChannel
	}
	sender, err := resolveSender(channel)
	if err != nil {
		return nil, err
	}

	personal := channel == ChannelZaloPersonal
	var reservation *RunReservation
	if personal {
		// Every check runs before the transaction: a dead session or a still-
		// sending run must refuse the call before anything — including the
		// statement refresh — is written. The reservation claims this teacher's
		// run slot for the same reason: two concurrent personal sends must not
		// both commit a run, so the loser is turned away while it still has
		// nothing to lose.
		if err := s.verifyPersonalSession(ctx, sc.TeacherID); err != nil {
			return nil, err
		}
		active, err := s.repo.HasActiveRun(ctx, sc)
		if err != nil {
			return nil, apperror.From(err)
		}
		if active {
			return nil, apperror.Conflict("a zalo_personal run is already sending; wait for it to finish")
		}
		if reservation, err = s.runs.Reserve(sc.TeacherID, sc.CenterID); err != nil {
			return nil, apperror.Conflict("a zalo_personal run is already sending; wait for it to finish")
		}
		defer reservation.Release()
	}

	runID := id.New()
	var runItems []RunItem
	var resp BulkSendResponse
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		genResult, err := s.statements.Generate(ctx, sc, periodID)
		if err != nil {
			return err
		}

		figures, err := s.statements.PeriodFigures(ctx, sc, periodID)
		if err != nil {
			return err
		}

		var mappings map[uuid.UUID]string
		if personal {
			contactIDs := make([]uuid.UUID, 0, len(genResult.Statements))
			for _, target := range genResult.Statements {
				contactIDs = append(contactIDs, target.ContactID)
			}
			if mappings, err = s.repo.ZaloMappings(ctx, sc, contactIDs); err != nil {
				return apperror.From(err)
			}
		}

		rows := make([]*Notification, 0, len(genResult.Statements))
		outRows := make([]BulkSendRow, 0, len(genResult.Statements))
		texts := make([]string, 0, len(genResult.Statements))
		skippedPaid := 0
		collapsedCount := 0
		fallbackManual := 0

		for _, target := range genResult.Statements {
			cf, ok := figures[target.ContactID]
			if !ok {
				// Generate's own target set is built from the same non-void
				// invoices PeriodFigures reads, so every target contact must
				// have figures. Skip defensively rather than panic if the two
				// ever disagree — a missing entry means nothing to say to
				// this contact, not a crash.
				continue
			}
			if purpose == PurposeReminder && cf.Outstanding <= 0 {
				skippedPaid++
				continue
			}

			text, url, collapsed := s.renderMessage(target, cf)
			if collapsed {
				collapsedCount++
			}

			rowChannel := channel
			toUID := ""
			if personal {
				if uid, mapped := mappings[target.ContactID]; mapped {
					toUID = uid
				} else {
					rowChannel = ChannelZaloManual
					fallbackManual++
				}
			}

			n := &Notification{
				ID: id.New(),
				// Stamps the caller as sender, not the statement's own
				// teacher — an owner sending for a member's period sends as
				// itself (see the package's tenancy design notes).
				TeacherID:   sc.TeacherID,
				CenterID:    sc.CenterID,
				StatementID: target.ID,
				Channel:     rowChannel,
				Purpose:     purpose,
				Status:      StatusQueued,
			}
			if toUID != "" {
				n.RunID = &runID
				runItems = append(runItems, RunItem{NotificationID: n.ID, ToUID: toUID, Text: text})
			}
			if err := sender.Send(ctx, n); err != nil {
				return apperror.BadRequest(channel + " channel is not available: " + err.Error())
			}

			rows = append(rows, n)
			// BulkText is the copy-paste bundle, so an auto-sent personal row
			// stays out of it.
			if toUID == "" {
				texts = append(texts, text)
			}
			outRows = append(outRows, BulkSendRow{
				NotificationID: n.ID,
				ContactID:      target.ContactID,
				ContactName:    cf.ContactName,
				Phone:          target.ContactPhone,
				Channel:        rowChannel,
				Purpose:        purpose,
				Status:         n.Status,
				MessageText:    text,
				URL:            url,
				Collapsed:      collapsed,
			})
		}

		if len(runItems) > s.cfg.MaxRunSize && s.cfg.MaxRunSize > 0 {
			return apperror.BadRequest("this send would auto-deliver to too many contacts at once; send in smaller batches")
		}

		if len(runItems) > 0 {
			run := &Run{
				ID:              runID,
				TeacherID:       sc.TeacherID,
				CenterID:        sc.CenterID,
				BillingPeriodID: periodID,
				Purpose:         purpose,
				Status:          RunStatusRunning,
			}
			if err := s.repo.CreateRun(ctx, run); err != nil {
				if errors.Is(err, ErrRunActive) {
					return apperror.Conflict("a zalo_personal run is already sending; wait for it to finish")
				}
				return apperror.From(err)
			}
		}

		if err := s.repo.InsertBatch(ctx, rows); err != nil {
			return apperror.From(err)
		}

		resp = BulkSendResponse{
			QueuedCount:         len(rows),
			SkippedPaidCount:    skippedPaid,
			CollapsedCount:      collapsedCount,
			PersonalQueuedCount: len(runItems),
			FallbackManualCount: fallbackManual,
			BulkText:            strings.Join(texts, "\n\n"),
			Rows:                outRows,
		}
		if len(runItems) > 0 {
			resp.RunID = &runID
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	if len(runItems) > 0 {
		reservation.Start(runID, runItems)
	}
	return &resp, nil
}

// renderMessage builds one contact's message text and statement URL.
// statements.ChildFigures and statements.ChildSummary carry the same fields in
// the same order by design (PeriodFigures produces exactly what Build
// consumes) — a plain conversion, not a second field-by-field mapping to keep
// in sync.
func (s *Service) renderMessage(target statements.Row, cf statements.ContactFigures) (text, url string, collapsed bool) {
	children := make([]statements.ChildSummary, 0, len(cf.Children))
	for _, c := range cf.Children {
		children = append(children, statements.ChildSummary(c))
	}
	url = s.statements.ToResponse(target).URL
	text, collapsed = statements.Build(statements.MessageInput{
		ContactName:     cf.ContactName,
		PeriodLabel:     cf.PeriodLabel,
		Children:        children,
		OpeningBalance:  cf.OpeningBalance,
		AdjustmentTotal: cf.AdjustmentTotal,
		TotalDue:        cf.TotalDue,
		Outstanding:     cf.Outstanding,
		URL:             url,
	}, s.cfg.MaxMessageLen)
	return text, url, collapsed
}

// verifyPersonalSession relogins the teacher's cached Zalo session and maps
// the two expected failure modes onto client errors: never linked is a bad
// request (the teacher skipped a setup step), expired is a conflict with the
// account's current state (relink, then retry).
func (s *Service) verifyPersonalSession(ctx context.Context, teacherID uuid.UUID) error {
	err := s.zalo.VerifyAccount(ctx, teacherID)
	switch {
	case err == nil:
		return nil
	case errors.Is(err, zalo.ErrNotLinked):
		return apperror.BadRequest("zalo_personal needs a linked Zalo account")
	case errors.Is(err, zalo.ErrLinkExpired):
		return apperror.Conflict("the linked Zalo session has expired; relink the account first")
	default:
		return apperror.From(err)
	}
}

// List returns one billing period's notification ledger, optionally
// narrowed by filter. An owner sees every member's rows in the center; a
// member sees only their own.
func (s *Service) List(ctx context.Context, sc authctx.Scope, periodID uuid.UUID, filter ListFilter) ([]NotificationResponse, error) {
	rows, err := s.repo.ListByPeriod(ctx, sc, periodID, filter)
	if err != nil {
		return nil, apperror.From(err)
	}
	out := make([]NotificationResponse, 0, len(rows))
	for _, r := range rows {
		out = append(out, fromListRow(r))
	}
	return out, nil
}

// MarkSent marks every id in ids sent, for ids visible to sc (center-scoped,
// and teacher-scoped unless sc is the center's owner) and still queued.
// Idempotent: an id already sent is silently left alone rather than erroring,
// so tapping "mark sent" twice never fails the second time.
func (s *Service) MarkSent(ctx context.Context, sc authctx.Scope, ids []uuid.UUID) error {
	if err := s.repo.MarkSent(ctx, sc, ids); err != nil {
		return apperror.From(err)
	}
	return nil
}

// RunSnapshot reports the period's latest run and its row-derived progress.
// A period that never had a run answers with the zero snapshot rather than an
// error — "no run" is an ordinary poll result, not a missing resource. An
// owner may read a member's run snapshot as oversight; a member sees only
// their own.
func (s *Service) RunSnapshot(ctx context.Context, sc authctx.Scope, periodID uuid.UUID) (*RunSnapshotResponse, error) {
	run, err := s.repo.LatestRunByPeriod(ctx, sc, periodID)
	if errors.Is(err, ErrRunNotFound) {
		return &RunSnapshotResponse{}, nil
	}
	if err != nil {
		return nil, apperror.From(err)
	}
	// runScope anchors on the run's own sender, not necessarily sc's own
	// teacher — an owner may be viewing a member's run — so the row count
	// still matches exactly this run regardless of who is asking.
	runScope := authctx.Scope{TeacherID: run.TeacherID, CenterID: sc.CenterID}
	counts, err := s.repo.RunCounts(ctx, runScope, run.ID)
	if err != nil {
		return nil, apperror.From(err)
	}
	return &RunSnapshotResponse{
		Active:  run.Status == RunStatusRunning,
		RunID:   &run.ID,
		Status:  run.Status,
		Purpose: run.Purpose,
		Total:   counts.Total,
		Sent:    counts.Sent,
		Failed:  counts.Failed,
	}, nil
}

// Ledger messages for rows a resume cannot re-send. Like the run manager's
// failure messages these are fixed constants: the ledger is teacher-facing and
// must never echo internal detail.
const (
	resumeUnmappedFailureMessage = "Chưa gán bạn Zalo"
	resumePaidFailureMessage     = "Đã thanh toán đủ"
	resumeStaleFailureMessage    = "Không còn dữ liệu để gửi"
)

// ResumeRun restarts the period's latest run after an interruption. Only a
// run in RunStatusInterrupted qualifies: a running one is already being sent
// and a completed/expired one has nothing left. The still-queued rows are
// re-rendered from live statement data — figures may have changed since the
// original send — and rows that can no longer be auto-sent (mapping removed,
// statement gone, or a reminder's balance since paid) are failed with a
// reason instead of silently dropped.
func (s *Service) ResumeRun(ctx context.Context, sc authctx.Scope, periodID uuid.UUID) (*RunSnapshotResponse, error) {
	// A resume re-occupies the acting teacher's own Zalo session and run
	// slot, never a member's, regardless of oversight rights — IsOwner is
	// deliberately stripped here so LatestRunByPeriod's owner-bypass template
	// degrades to a strict own-row match. An owner resuming a member's
	// interrupted run must not be possible: it would send through the
	// owner's own Zalo account under the member's run record.
	ownScope := authctx.Scope{TeacherID: sc.TeacherID, CenterID: sc.CenterID}
	run, err := s.repo.LatestRunByPeriod(ctx, ownScope, periodID)
	if errors.Is(err, ErrRunNotFound) {
		return nil, apperror.NotFound("notification run")
	}
	if err != nil {
		return nil, apperror.From(err)
	}
	if run.Status != RunStatusInterrupted {
		return nil, apperror.Conflict("only an interrupted run can be resumed")
	}
	if err := s.verifyPersonalSession(ctx, sc.TeacherID); err != nil {
		return nil, err
	}
	active, err := s.repo.HasActiveRun(ctx, sc)
	if err != nil {
		return nil, apperror.From(err)
	}
	if active {
		return nil, apperror.Conflict("a zalo_personal run is already sending; wait for it to finish")
	}
	// The slot is claimed before the transaction so a concurrent resume (or
	// bulk send) loses here, before this call touches the shared run record —
	// a loser that got as far as writing would flip a run the winner is
	// actively sending back to interrupted.
	reservation, err := s.runs.Reserve(sc.TeacherID, sc.CenterID)
	if err != nil {
		return nil, apperror.Conflict("a zalo_personal run is already sending; wait for it to finish")
	}
	defer reservation.Release()

	var items []RunItem
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		genResult, err := s.statements.Generate(ctx, sc, periodID)
		if err != nil {
			return err
		}
		figures, err := s.statements.PeriodFigures(ctx, sc, periodID)
		if err != nil {
			return err
		}
		queued, err := s.repo.QueuedRunRows(ctx, sc, run.ID)
		if err != nil {
			return apperror.From(err)
		}

		contactIDs := make([]uuid.UUID, 0, len(queued))
		for _, row := range queued {
			contactIDs = append(contactIDs, row.ContactID)
		}
		mappings, err := s.repo.ZaloMappings(ctx, sc, contactIDs)
		if err != nil {
			return apperror.From(err)
		}
		targets := make(map[uuid.UUID]statements.Row, len(genResult.Statements))
		for _, target := range genResult.Statements {
			targets[target.ContactID] = target
		}

		for _, row := range queued {
			fail := func(reason string) error {
				return s.repo.MarkOutcome(ctx, sc, row.NotificationID, StatusFailed, nil, ptr(reason))
			}
			uid, mapped := mappings[row.ContactID]
			if !mapped {
				if err := fail(resumeUnmappedFailureMessage); err != nil {
					return apperror.From(err)
				}
				continue
			}
			target, hasTarget := targets[row.ContactID]
			cf, hasFigures := figures[row.ContactID]
			if !hasTarget || !hasFigures {
				if err := fail(resumeStaleFailureMessage); err != nil {
					return apperror.From(err)
				}
				continue
			}
			if run.Purpose == PurposeReminder && cf.Outstanding <= 0 {
				if err := fail(resumePaidFailureMessage); err != nil {
					return apperror.From(err)
				}
				continue
			}
			text, _, _ := s.renderMessage(target, cf)
			items = append(items, RunItem{NotificationID: row.NotificationID, ToUID: uid, Text: text})
		}

		// A resume that finds nothing sendable left still resolves the run:
		// every queued row was just failed with its reason, so the run is done.
		status := RunStatusRunning
		if len(items) == 0 {
			status = RunStatusCompleted
		}
		if err := s.repo.UpdateRunStatus(ctx, sc, run.ID, status); err != nil {
			if errors.Is(err, ErrRunActive) {
				return apperror.Conflict("a zalo_personal run is already sending; wait for it to finish")
			}
			return apperror.From(err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	if len(items) > 0 {
		reservation.Start(run.ID, items)
	}
	return s.RunSnapshot(ctx, sc, periodID)
}

// ReconcileInterrupted marks every run a previous process left in
// RunStatusRunning as interrupted. Called once at boot, before requests are
// served: a run can only be "running" while a goroutine in this process is
// sending it, so any running row found at startup belongs to a process that
// died mid-run. Deliberately unscoped by tenant: there is no requesting
// teacher or center at boot, and a process crash is a system-wide event that
// can equally strand a run in any center — every affected row keeps its own
// existing teacher_id/center_id anchor, this sweep only ever touches status
// and finished_at.
func (s *Service) ReconcileInterrupted(ctx context.Context) error {
	count, err := s.repo.MarkInterrupted(ctx)
	if err != nil {
		return apperror.From(err)
	}
	if count > 0 {
		s.log.Warn("notifications: marked runs abandoned by a previous process as interrupted", "count", count)
	}
	return nil
}
