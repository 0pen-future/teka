package audit

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
)

const (
	defaultLimit = 50
	maxLimit     = 100
)

// ListQuery carries the caller's filters, already parsed by the handler.
// Zero values mean "no filter"; Cursor stays opaque until the service
// decodes it.
type ListQuery struct {
	ActorID uuid.UUID
	// Action filters by prefix, e.g. "class." matches class.create.
	Action string
	From   time.Time
	To     time.Time
	Cursor string
	Limit  int
}

// ListSpec is the normalized repository query: visibility center, clamped
// fetch size (one past the page to probe for a next page), and the decoded
// keyset position.
type ListSpec struct {
	CenterID     uuid.UUID
	ActorID      uuid.UUID
	ActionPrefix string
	From         time.Time
	To           time.Time
	CursorAt     time.Time
	CursorID     uuid.UUID
	Limit        int
}

// Row is one visible audit entry: the stored log plus the actor's display
// name resolved at read time (empty when the teacher row is gone).
type Row struct {
	Log       `gorm:"embedded"`
	ActorName string
}

// ListStore is the slice of the repository the read service needs.
type ListStore interface {
	List(ctx context.Context, spec ListSpec) ([]Row, error)
}

// Service answers the owner-only audit trail reads.
type Service struct {
	store ListStore
}

// NewService wires the read service to its store.
func NewService(store ListStore) *Service {
	return &Service{store: store}
}

// List returns one page of the center's audit trail, newest first, plus the
// cursor for the next page ("" when this is the last one). Reading the trail
// takes audit.read; visibility is bounded to the caller's center by the store.
func (s *Service) List(ctx context.Context, sc authctx.Scope, q ListQuery) ([]Row, string, error) {
	if !sc.Has(authctx.PermAuditRead) {
		return nil, "", apperror.Forbidden("you are not allowed to read the audit log")
	}

	limit := q.Limit
	if limit <= 0 {
		limit = defaultLimit
	}
	if limit > maxLimit {
		limit = maxLimit
	}

	spec := ListSpec{
		CenterID:     sc.CenterID,
		ActorID:      q.ActorID,
		ActionPrefix: q.Action,
		From:         q.From,
		To:           q.To,
		// One extra row proves or disproves a next page without a COUNT.
		Limit: limit + 1,
	}
	if q.Cursor != "" {
		at, cid, err := decodeCursor(q.Cursor)
		if err != nil {
			return nil, "", apperror.BadRequest("invalid cursor")
		}
		spec.CursorAt = at
		spec.CursorID = cid
	}

	rows, err := s.store.List(ctx, spec)
	if err != nil {
		return nil, "", err
	}
	next := ""
	if len(rows) > limit {
		rows = rows[:limit]
		last := rows[limit-1]
		next = encodeCursor(last.OccurredAt, last.ID)
	}
	return rows, next, nil
}

// encodeCursor packs the keyset position of the last row on a page. The
// format is internal: clients treat cursors as opaque strings.
func encodeCursor(at time.Time, id uuid.UUID) string {
	raw := at.UTC().Format(time.RFC3339Nano) + "|" + id.String()
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func decodeCursor(s string) (time.Time, uuid.UUID, error) {
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return time.Time{}, uuid.Nil, err
	}
	atRaw, idRaw, ok := strings.Cut(string(raw), "|")
	if !ok {
		return time.Time{}, uuid.Nil, fmt.Errorf("audit: cursor missing separator")
	}
	at, err := time.Parse(time.RFC3339Nano, atRaw)
	if err != nil {
		return time.Time{}, uuid.Nil, err
	}
	cid, err := uuid.Parse(idRaw)
	if err != nil {
		return time.Time{}, uuid.Nil, err
	}
	return at, cid, nil
}
