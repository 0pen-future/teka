package imports

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"teka/apps/api/internal/database"
)

// commitStatementTimeout bounds a single statement inside the import
// transaction. The
// server's WriteTimeout is 30s and the client gives up earlier still, so a
// statement that outruns this budget is going to lose its response either way
// — better to fail it and release the connection than to hold one while
// nobody is listening.
// It is spliced into the SET statement rather than bound: Postgres parses SET
// before parameters exist, so a placeholder there is a syntax error. The value
// is this constant and never anything a caller supplies.
const commitStatementTimeout = "'20s'"

// Locker guards a center against two imports running at once.
type Locker interface {
	// TryLockCenter takes a transaction-scoped advisory lock, returning false
	// rather than waiting when another import already holds it.
	TryLockCenter(ctx context.Context, centerID uuid.UUID) (bool, error)
	// SetStatementTimeout bounds each statement for the rest of the
	// transaction.
	SetStatementTimeout(ctx context.Context) error
}

type gormLocker struct {
	db *gorm.DB
}

// NewLocker returns the GORM-backed Locker.
func NewLocker(db *gorm.DB) Locker { return &gormLocker{db: db} }

// TryLockCenter serialises imports within one center.
//
// Three of the five natural keys this feature reuses (class, schedule,
// student) have no unique index behind them, so their create-or-reuse
// pre-checks are only sound while nothing else is writing the same center.
//
// It is the TRY variant deliberately. pg_advisory_xact_lock waits forever, and
// waiting holds a pooled connection: the pool is shared by every tenant
// (DB_MAX_OPEN_CONNS defaults to 25), so one owner retrying a slow import
// could park enough connections to stall every other center. Refusing with a
// clear 409 costs that owner one retry and costs everyone else nothing.
//
// The lock releases on commit or rollback, so there is no unlock path to
// forget.
func (l *gormLocker) TryLockCenter(ctx context.Context, centerID uuid.UUID) (bool, error) {
	var locked bool
	// hashtext takes text; center_id is a uuid, hence the cast.
	err := database.FromContext(ctx, l.db).
		Raw(`SELECT pg_try_advisory_xact_lock(hashtext(?::text))`, centerID.String()).
		Scan(&locked).Error
	return locked, err
}

// SetStatementTimeout applies commitStatementTimeout for the remainder of the
// transaction. SET LOCAL is scoped to the transaction, so it cannot leak onto
// the next request that borrows this pooled connection.
func (l *gormLocker) SetStatementTimeout(ctx context.Context) error {
	return database.FromContext(ctx, l.db).
		Exec(`SET LOCAL statement_timeout = ` + commitStatementTimeout).Error
}
