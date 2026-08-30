package grading

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
)

func ptr(v float64) *float64 { return &v }

// The batch validator guards the shape a fake or real repo never gets to see:
// the entry cap, a duplicate cell, and the 0–10 scale. Precision (one decimal)
// is intentionally NOT checked here — the NUMERIC(4,1) column owns it.
func TestValidateScoreEntries(t *testing.T) {
	t.Parallel()
	student := uuid.New()
	comp := uuid.New()

	t.Run("valid batch passes", func(t *testing.T) {
		err := validateScoreEntries([]ScoreEntryRequest{
			{StudentID: student, ComponentID: comp, Score: ptr(8.5)},
			{StudentID: student, ComponentID: uuid.New(), Score: nil}, // null = clear cell
		})
		require.NoError(t, err)
	})

	t.Run("over the entry cap is rejected", func(t *testing.T) {
		entries := make([]ScoreEntryRequest, maxScoreEntries+1)
		for i := range entries {
			entries[i] = ScoreEntryRequest{StudentID: uuid.New(), ComponentID: comp, Score: ptr(5)}
		}
		err := validateScoreEntries(entries)
		require.Equal(t, apperror.CodeValidation, apperror.From(err).Code)
		require.NotEmpty(t, apperror.From(err).Fields["scores"])
	})

	t.Run("a duplicate cell is rejected", func(t *testing.T) {
		err := validateScoreEntries([]ScoreEntryRequest{
			{StudentID: student, ComponentID: comp, Score: ptr(7)},
			{StudentID: student, ComponentID: comp, Score: ptr(8)},
		})
		require.Equal(t, apperror.CodeValidation, apperror.From(err).Code)
	})

	t.Run("a score outside 0–10 is rejected", func(t *testing.T) {
		for _, bad := range []float64{-0.1, 10.5} {
			err := validateScoreEntries([]ScoreEntryRequest{
				{StudentID: student, ComponentID: comp, Score: ptr(bad)},
			})
			require.Equal(t, apperror.CodeValidation, apperror.From(err).Code, "score %.1f must fail", bad)
		}
	})

	t.Run("the 0 and 10 bounds are allowed", func(t *testing.T) {
		err := validateScoreEntries([]ScoreEntryRequest{
			{StudentID: student, ComponentID: comp, Score: ptr(0)},
			{StudentID: uuid.New(), ComponentID: comp, Score: ptr(10)},
		})
		require.NoError(t, err)
	})
}

// normalizeComponentNames trims, keeps input order (position = index), and
// rejects both blanks and case-insensitive duplicates.
func TestNormalizeComponentNames(t *testing.T) {
	t.Parallel()

	names, msg := normalizeComponentNames([]string{"  Listening ", "Speaking"})
	require.Empty(t, msg)
	require.Equal(t, []string{"Listening", "Speaking"}, names, "trimmed and order-preserving")

	_, msg = normalizeComponentNames([]string{"Reading", "  "})
	require.NotEmpty(t, msg, "a blank name must be rejected")

	_, msg = normalizeComponentNames([]string{"Writing", "writing"})
	require.NotEmpty(t, msg, "a case-insensitive duplicate must be rejected")
}

// Every owner-configuration surface refuses a non-owner before it touches a
// dependency — so a Service with nil deps is enough to prove the gate.
func TestOwnerGatesShortCircuit(t *testing.T) {
	t.Parallel()
	svc := NewService(nil, nil, nil, nil, nil)
	ctx := context.Background()
	member := authctx.Scope{TeacherID: uuid.New(), CenterID: uuid.New(), IsOwner: false}

	isForbidden := func(err error) {
		t.Helper()
		require.Equal(t, apperror.CodeForbidden, apperror.From(err).Code)
	}

	_, err := svc.ListSets(ctx, member)
	isForbidden(err)
	_, err = svc.CreateSet(ctx, member, ScoreSetRequest{Name: "IELTS", Components: []string{"Listening"}})
	isForbidden(err)
	_, err = svc.UpdateSet(ctx, member, uuid.New(), ScoreSetRequest{Name: "x", Components: []string{"a"}})
	isForbidden(err)
	err = svc.DeleteSet(ctx, member, uuid.New())
	isForbidden(err)
	_, err = svc.AssignScoreSet(ctx, member, uuid.New(), uuid.New())
	isForbidden(err)
	err = svc.ClearScoreSet(ctx, member, uuid.New())
	isForbidden(err)
}
