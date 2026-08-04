package notifications

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"teka/apps/api/internal/config"
	"teka/apps/api/internal/database"
	"teka/apps/api/internal/features/statements"
	"teka/apps/api/internal/shared/apperror"
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
	Generate(ctx context.Context, teacherID, periodID uuid.UUID) (*statements.GenerateResult, error)
	PeriodFigures(ctx context.Context, teacherID, periodID uuid.UUID) (map[uuid.UUID]statements.ContactFigures, error)
	ToResponse(row statements.Row) statements.StatementResponse
}

// Service owns notification queueing, sending, and status.
type Service struct {
	repo       Repository
	tx         database.TxManager
	statements StatementsSource
	cfg        config.NotificationsConfig
}

// NewService builds the notifications service.
func NewService(repo Repository, tx database.TxManager, statementsSvc StatementsSource, cfg config.NotificationsConfig) *Service {
	return &Service{repo: repo, tx: tx, statements: statementsSvc, cfg: cfg}
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
func (s *Service) BulkSend(ctx context.Context, teacherID, periodID uuid.UUID, req BulkSendRequest) (*BulkSendResponse, error) {
	purpose := normalizePurpose(req.Purpose)
	channel := req.Channel
	if channel == "" {
		channel = s.cfg.DefaultChannel
	}
	sender, err := resolveSender(channel)
	if err != nil {
		return nil, err
	}

	var resp BulkSendResponse
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		genResult, err := s.statements.Generate(ctx, teacherID, periodID)
		if err != nil {
			return err
		}

		figures, err := s.statements.PeriodFigures(ctx, teacherID, periodID)
		if err != nil {
			return err
		}

		rows := make([]*Notification, 0, len(genResult.Statements))
		outRows := make([]BulkSendRow, 0, len(genResult.Statements))
		texts := make([]string, 0, len(genResult.Statements))
		skippedPaid := 0
		collapsedCount := 0

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

			// statements.ChildFigures and statements.ChildSummary carry the same
			// fields in the same order by design (PeriodFigures produces exactly
			// what Build consumes) — a plain conversion, not a second field-by-
			// field mapping to keep in sync.
			children := make([]statements.ChildSummary, 0, len(cf.Children))
			for _, c := range cf.Children {
				children = append(children, statements.ChildSummary(c))
			}

			url := s.statements.ToResponse(target).URL
			text, collapsed := statements.Build(statements.MessageInput{
				ContactName:     cf.ContactName,
				PeriodLabel:     cf.PeriodLabel,
				Children:        children,
				OpeningBalance:  cf.OpeningBalance,
				AdjustmentTotal: cf.AdjustmentTotal,
				TotalDue:        cf.TotalDue,
				Outstanding:     cf.Outstanding,
				URL:             url,
			}, s.cfg.MaxMessageLen)
			if collapsed {
				collapsedCount++
			}

			n := &Notification{
				ID:          id.New(),
				TeacherID:   teacherID,
				StatementID: target.ID,
				Channel:     channel,
				Purpose:     purpose,
				Status:      StatusQueued,
			}
			if err := sender.Send(ctx, n); err != nil {
				return apperror.BadRequest(channel + " channel is not available: " + err.Error())
			}

			rows = append(rows, n)
			texts = append(texts, text)
			outRows = append(outRows, BulkSendRow{
				NotificationID: n.ID,
				ContactID:      target.ContactID,
				ContactName:    cf.ContactName,
				Phone:          target.ContactPhone,
				Channel:        channel,
				Purpose:        purpose,
				Status:         n.Status,
				MessageText:    text,
				URL:            url,
				Collapsed:      collapsed,
			})
		}

		if err := s.repo.InsertBatch(ctx, rows); err != nil {
			return apperror.From(err)
		}

		resp = BulkSendResponse{
			QueuedCount:      len(rows),
			SkippedPaidCount: skippedPaid,
			CollapsedCount:   collapsedCount,
			BulkText:         strings.Join(texts, "\n\n"),
			Rows:             outRows,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

// List returns one billing period's notification ledger, optionally
// narrowed by filter.
func (s *Service) List(ctx context.Context, teacherID, periodID uuid.UUID, filter ListFilter) ([]NotificationResponse, error) {
	rows, err := s.repo.ListByPeriod(ctx, teacherID, periodID, filter)
	if err != nil {
		return nil, apperror.From(err)
	}
	out := make([]NotificationResponse, 0, len(rows))
	for _, r := range rows {
		out = append(out, fromListRow(r))
	}
	return out, nil
}

// MarkSent marks every id in ids sent, for ids that still belong to
// teacherID and are still queued. Idempotent: an id already sent is silently
// left alone rather than erroring, so a teacher tapping "mark sent" twice
// never fails the second time.
func (s *Service) MarkSent(ctx context.Context, teacherID uuid.UUID, ids []uuid.UUID) error {
	if err := s.repo.MarkSent(ctx, teacherID, ids); err != nil {
		return apperror.From(err)
	}
	return nil
}
