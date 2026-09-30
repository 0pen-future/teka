package grading

import (
	"context"
	"strings"
	"testing"
	"unicode/utf8"

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

// flattenTemplateComponents keeps group then component order (position =
// index), prefixes the group title only when two or more groups carry
// components, and keeps every name within the column's 50 characters and
// unique case-insensitively.
func TestFlattenTemplateComponents(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("Đ", 60)

	cases := []struct {
		name   string
		groups []TemplateScoreGroup
		want   []string
	}{
		{
			name:   "a single group keeps its bare trimmed labels",
			groups: []TemplateScoreGroup{{Title: "Giữa kỳ", Labels: []string{" Nghe ", "Nói"}}},
			want:   []string{"Nghe", "Nói"},
		},
		{
			name: "empty groups do not count towards prefixing",
			groups: []TemplateScoreGroup{
				{Title: "Trống", Labels: nil},
				{Title: "Cuối kỳ", Labels: []string{"Viết"}},
			},
			want: []string{"Viết"},
		},
		{
			name: "several groups prefix each label with the group title in order",
			groups: []TemplateScoreGroup{
				{Title: "Giữa kỳ", Labels: []string{"Nghe", "Nói"}},
				{Title: "Cuối kỳ", Labels: []string{"Nghe"}},
			},
			want: []string{"Giữa kỳ · Nghe", "Giữa kỳ · Nói", "Cuối kỳ · Nghe"},
		},
		{
			name:   "a 60-rune label is cut to 50 runes",
			groups: []TemplateScoreGroup{{Title: "G", Labels: []string{long}}},
			want:   []string{strings.Repeat("Đ", 50)},
		},
		{
			name:   "a case-insensitive repeat gets a numbered suffix",
			groups: []TemplateScoreGroup{{Title: "G", Labels: []string{"Nghe", "nghe", "NGHE"}}},
			want:   []string{"Nghe", "nghe (2)", "NGHE (3)"},
		},
		{
			name:   "names equal only after the cut are deduplicated within 50 runes",
			groups: []TemplateScoreGroup{{Title: "G", Labels: []string{long + "a", long + "b"}}},
			want:   []string{strings.Repeat("Đ", 50), strings.Repeat("Đ", 46) + " (2)"},
		},
		{
			name: "a long group title is cut so the labels stay apart",
			groups: []TemplateScoreGroup{
				{Title: "Kiểm tra định kỳ giữa học kỳ I năm học 2026-2027", Labels: []string{"Nghe", "Nói"}},
				{Title: "Cuối kỳ", Labels: []string{"Viết"}},
			},
			want: []string{
				"Kiểm tra định kỳ giữa học kỳ I năm học 2026 · Nghe",
				"Kiểm tra định kỳ giữa học kỳ I năm học 2026- · Nói",
				"Cuối kỳ · Viết",
			},
		},
		{
			name:   "no groups flatten to nothing",
			groups: nil,
			want:   []string{},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := flattenTemplateComponents(tc.groups)
			require.Equal(t, tc.want, got)
			for _, name := range got {
				require.LessOrEqual(t, utf8.RuneCountInString(name), maxComponentNameRunes)
			}
		})
	}
}

// Changing a class's components refuses a non-owner before it touches a
// dependency — so a Service with nil deps is enough to prove the gate.
func TestOwnerGatesShortCircuit(t *testing.T) {
	t.Parallel()
	svc := NewService(nil, nil, nil, nil, nil)
	member := authctx.Scope{TeacherID: uuid.New(), CenterID: uuid.New(), IsOwner: false}

	_, err := svc.SyncTemplateComponents(context.Background(), member, uuid.New(), []TemplateScoreGroup{
		{Title: "Giữa kỳ", Labels: []string{"Nghe"}},
	})
	require.Equal(t, apperror.CodeForbidden, apperror.From(err).Code)
}
