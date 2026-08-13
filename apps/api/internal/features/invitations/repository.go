package invitations

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"teka/apps/api/internal/database"
)

// Repository is the persistence contract for invitations; the service depends
// on this interface, tests supply a fake.
type Repository interface {
	// Create inserts a new pending invitation. It returns ErrPendingExists on
	// a uq_invitations_pending_phone violation — a concurrent Create for the
	// same (center, phone) won the race.
	Create(ctx context.Context, inv *Invitation) error
	// RevokePendingForPhone closes any pending invite for (centerID, phone).
	// It is a no-op, not an error, when none exists.
	RevokePendingForPhone(ctx context.Context, centerID uuid.UUID, phone string) error
	// GetPendingByPhone reloads the surviving pending row for (centerID,
	// phone) after a Create race. ErrNotFound when none is pending.
	GetPendingByPhone(ctx context.Context, centerID uuid.UUID, phone string) (*Invitation, error)
	// SetTokenHash rotates the stored token hash of an existing row in place.
	// Used only to recover from a Create race: the loser mints its own token
	// but the row already belongs to the winner, so the loser's response
	// still needs a hash that matches a plaintext it actually knows.
	SetTokenHash(ctx context.Context, invID uuid.UUID, hash string) error
	// List returns every invitation in the center, pending first.
	List(ctx context.Context, centerID uuid.UUID) ([]Invitation, error)
	// GetByID returns one invitation scoped to the center. ErrNotFound for a
	// missing row or one belonging to another center.
	GetByID(ctx context.Context, centerID, invID uuid.UUID) (*Invitation, error)
	// Revoke closes a pending invitation. Idempotent: a row that is already
	// revoked (or otherwise not pending) is left untouched and reported as
	// success; only a row missing from the center is ErrNotFound.
	Revoke(ctx context.Context, centerID, invID uuid.UUID) error
	// GetByTokenHash loads an invitation by its token hash for the public
	// preview read. ErrNotFound covers an unknown hash — the same answer a
	// stale/used one gets, since the caller decides pending-vs-not from the
	// returned row's Status/ExpiresAt, not from this method.
	GetByTokenHash(ctx context.Context, hash string) (*Invitation, error)
	// GetByTokenHashForUpdate is GetByTokenHash under SELECT ... FOR UPDATE,
	// for the accept flow: it serializes two concurrent accepts of the same
	// token inside their WithinTx so only one can win the pending->accepted
	// transition.
	GetByTokenHashForUpdate(ctx context.Context, hash string) (*Invitation, error)
	// MarkAccepted flips a pending invitation to accepted in the ambient tx.
	MarkAccepted(ctx context.Context, invID uuid.UUID, at time.Time) error
	// GetCenterName reads one center's display name for the preview response.
	// ErrNotFound when the center is gone (soft-deleted) — an invariant
	// violation for a still-pending invitation, not a normal outcome.
	GetCenterName(ctx context.Context, centerID uuid.UUID) (string, error)
}

type gormRepository struct {
	db *gorm.DB
}

// NewRepository returns the GORM-backed Repository.
func NewRepository(db *gorm.DB) Repository {
	return &gormRepository{db: db}
}

func (r *gormRepository) Create(ctx context.Context, inv *Invitation) error {
	return translateError(database.FromContext(ctx, r.db).Create(inv).Error)
}

func (r *gormRepository) RevokePendingForPhone(ctx context.Context, centerID uuid.UUID, phone string) error {
	return database.FromContext(ctx, r.db).
		Model(&Invitation{}).
		Where("center_id = ? AND phone = ? AND status = ?", centerID, phone, StatusPending).
		Updates(map[string]any{"status": StatusRevoked, "revoked_at": time.Now()}).Error
}

func (r *gormRepository) GetPendingByPhone(ctx context.Context, centerID uuid.UUID, phone string) (*Invitation, error) {
	var inv Invitation
	err := database.FromContext(ctx, r.db).
		Where("center_id = ? AND phone = ? AND status = ?", centerID, phone, StatusPending).
		Take(&inv).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &inv, nil
}

func (r *gormRepository) SetTokenHash(ctx context.Context, invID uuid.UUID, hash string) error {
	res := database.FromContext(ctx, r.db).
		Model(&Invitation{}).
		Where("id = ?", invID).
		Update("token_hash", hash)
	if res.Error != nil {
		return translateError(res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *gormRepository) List(ctx context.Context, centerID uuid.UUID) ([]Invitation, error) {
	var rows []Invitation
	err := database.FromContext(ctx, r.db).
		Where("center_id = ?", centerID).
		Order("CASE WHEN status = 'pending' THEN 0 ELSE 1 END, created_at DESC").
		Find(&rows).Error
	return rows, err
}

func (r *gormRepository) GetByID(ctx context.Context, centerID, invID uuid.UUID) (*Invitation, error) {
	var inv Invitation
	err := database.FromContext(ctx, r.db).
		Where("id = ? AND center_id = ?", invID, centerID).
		Take(&inv).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &inv, nil
}

func (r *gormRepository) Revoke(ctx context.Context, centerID, invID uuid.UUID) error {
	res := database.FromContext(ctx, r.db).
		Model(&Invitation{}).
		Where("id = ? AND center_id = ? AND status = ?", invID, centerID, StatusPending).
		Updates(map[string]any{"status": StatusRevoked, "revoked_at": time.Now()})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 1 {
		return nil
	}
	// Nothing changed: either the row does not exist in this center, or it
	// was already past pending (revoked/accepted). Distinguish the two so a
	// cross-center id still answers 404 while re-revoking stays idempotent.
	var exists bool
	err := database.FromContext(ctx, r.db).
		Raw(`SELECT EXISTS(SELECT 1 FROM invitations WHERE id = ? AND center_id = ?)`, invID, centerID).
		Scan(&exists).Error
	if err != nil {
		return err
	}
	if !exists {
		return ErrNotFound
	}
	return nil
}

func (r *gormRepository) GetByTokenHash(ctx context.Context, hash string) (*Invitation, error) {
	var inv Invitation
	err := database.FromContext(ctx, r.db).Where("token_hash = ?", hash).Take(&inv).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &inv, nil
}

func (r *gormRepository) GetByTokenHashForUpdate(ctx context.Context, hash string) (*Invitation, error) {
	var inv Invitation
	err := database.FromContext(ctx, r.db).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("token_hash = ?", hash).Take(&inv).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &inv, nil
}

func (r *gormRepository) MarkAccepted(ctx context.Context, invID uuid.UUID, at time.Time) error {
	res := database.FromContext(ctx, r.db).
		Model(&Invitation{}).
		Where("id = ?", invID).
		Updates(map[string]any{"status": StatusAccepted, "accepted_at": at})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *gormRepository) GetCenterName(ctx context.Context, centerID uuid.UUID) (string, error) {
	var name string
	res := database.FromContext(ctx, r.db).
		Raw(`SELECT name FROM centers WHERE id = ? AND deleted_at IS NULL`, centerID).
		Scan(&name)
	if res.Error != nil {
		return "", res.Error
	}
	if res.RowsAffected == 0 {
		return "", ErrNotFound
	}
	return name, nil
}

// translateError maps the token_hash and uq_invitations_pending_phone unique
// violations onto domain errors so callers stay driver-agnostic.
func translateError(err error) error {
	if !errors.Is(err, gorm.ErrDuplicatedKey) {
		return err
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.ConstraintName == "uq_invitations_pending_phone" {
		return ErrPendingExists
	}
	return err
}
