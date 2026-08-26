//go:build integration

package audit_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"teka/apps/api/internal/features/audit"
	"teka/apps/api/internal/shared/id"
	"teka/apps/api/internal/testutil"
)

// TestInsertBatchRoundTrip proves N rows land in one insert and every column
// — including the jsonb metadata — reads back intact.
func TestInsertBatchRoundTrip(t *testing.T) {
	db := testutil.StartPostgres(t)
	repo := audit.NewRepository(db)

	centerID := uuid.New()
	actorID := uuid.New()
	at := time.Now().UTC().Truncate(time.Microsecond)

	rows := []audit.Log{
		{
			ID:          id.New(),
			OccurredAt:  at,
			CenterID:    &centerID,
			ActorUserID: &actorID,
			ActorRole:   "owner",
			Action:      "class.create",
			Method:      "POST",
			Path:        "/api/v1/classes",
			EntityType:  "class",
			EntityID:    "e-1",
			StatusCode:  201,
			RequestID:   "req-1",
			IP:          "10.0.0.1",
			UserAgent:   "go-test",
			Metadata:    audit.Metadata{"k": "v"},
		},
		{
			// Auth-shaped row: no center, no actor, masked phone metadata.
			ID:         id.New(),
			OccurredAt: at.Add(time.Second),
			Action:     "auth.login_fail",
			Metadata:   audit.Metadata{"phone_masked": "090***123"},
		},
	}
	require.NoError(t, repo.InsertBatch(context.Background(), rows))

	var got []audit.Log
	require.NoError(t, db.Order("occurred_at asc").Find(&got).Error)
	require.Len(t, got, 2)

	full := got[0]
	require.Equal(t, rows[0].ID, full.ID)
	require.True(t, full.OccurredAt.Equal(at))
	require.NotNil(t, full.CenterID)
	require.Equal(t, centerID, *full.CenterID)
	require.NotNil(t, full.ActorUserID)
	require.Equal(t, actorID, *full.ActorUserID)
	require.Equal(t, "owner", full.ActorRole)
	require.Equal(t, "class.create", full.Action)
	require.Equal(t, "POST", full.Method)
	require.Equal(t, "/api/v1/classes", full.Path)
	require.Equal(t, "class", full.EntityType)
	require.Equal(t, "e-1", full.EntityID)
	require.Equal(t, 201, full.StatusCode)
	require.Equal(t, "req-1", full.RequestID)
	require.Equal(t, "10.0.0.1", full.IP)
	require.Equal(t, "go-test", full.UserAgent)
	require.Equal(t, audit.Metadata{"k": "v"}, full.Metadata)

	bare := got[1]
	require.Nil(t, bare.CenterID)
	require.Nil(t, bare.ActorUserID)
	require.Equal(t, audit.Metadata{"phone_masked": "090***123"}, bare.Metadata)
}

// TestInsertBatchEmpty proves an empty batch is a no-op, not an error — the
// flush path calls this on timers regardless of traffic.
func TestInsertBatchEmpty(t *testing.T) {
	db := testutil.StartPostgres(t)
	repo := audit.NewRepository(db)
	require.NoError(t, repo.InsertBatch(context.Background(), nil))
}
