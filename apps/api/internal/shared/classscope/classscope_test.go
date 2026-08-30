package classscope_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"teka/apps/api/internal/shared/classscope"
)

// The fragment is glued into repository WHERE clauses by hand, so its shape is
// contract: the caller's column expression must appear verbatim, the join must
// drop soft-deleted classes (forgetting it widens reads onto deleted classes),
// and the placeholder count must match what argCount promises or every caller
// mis-binds its scope arguments.
func TestReadExistsShape(t *testing.T) {
	t.Parallel()
	frag, args := classscope.ReadExists("enrollments.class_id")

	require.Equal(t, 2, args)
	require.Equal(t, args, strings.Count(frag, "?"))
	require.Contains(t, frag, "cs.class_id = enrollments.class_id")
	require.Contains(t, frag, "c2.deleted_at IS NULL")
	require.Contains(t, frag, "cs.teacher_id = ?")
	require.Contains(t, frag, "cs.center_id = ?")
	// Any stint grants read — ended ones keep history access (R4.1), so the
	// fragment must never filter on ended_at.
	require.NotContains(t, frag, "ended_at")
}

// WriteExists is the write gate: unlike the read fragments it must demand an
// ACTIVE stint (ended_at IS NULL) and one of the capability's roles — losing
// either filter would hand write access to ended stints or to every role.
func TestWriteExistsShape(t *testing.T) {
	t.Parallel()
	frag, args := classscope.WriteExists("sessions.class_id")

	require.Equal(t, 3, args)
	require.Equal(t, args, strings.Count(frag, "?"))
	require.Contains(t, frag, "cs.class_id = sessions.class_id")
	require.Contains(t, frag, "c2.deleted_at IS NULL")
	require.Contains(t, frag, "cs.teacher_id = ?")
	require.Contains(t, frag, "cs.center_id = ?")
	require.Contains(t, frag, "cs.ended_at IS NULL")
	require.Contains(t, frag, "cs.role_key IN ?")
}

func TestReadExistsViaEnrollmentShape(t *testing.T) {
	t.Parallel()
	frag, args := classscope.ReadExistsViaEnrollment("students.id")

	require.Equal(t, 2, args)
	require.Equal(t, args, strings.Count(frag, "?"))
	require.Contains(t, frag, "e2.student_id = students.id")
	require.Contains(t, frag, "e2.deleted_at IS NULL")
	require.Contains(t, frag, "c2.deleted_at IS NULL")
	require.NotContains(t, frag, "ended_at")
}
