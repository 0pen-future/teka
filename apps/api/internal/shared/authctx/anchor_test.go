package authctx

import (
	"reflect"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// An Anchor names whose rows a service acts on and nothing more. It must not
// carry, or be convertible to, anything that widens a query: no IsOwner, no
// Perms, no path back to Scope.
func TestAnchorCarriesIdentityOnly(t *testing.T) {
	anchorType := reflect.TypeOf(Anchor{})
	require.Equal(t, 2, anchorType.NumField())
	_, hasOwner := anchorType.FieldByName("IsOwner")
	_, hasPerms := anchorType.FieldByName("Perms")
	_, hasSend := anchorType.FieldByName("CanSendReports")
	require.False(t, hasOwner || hasPerms || hasSend)
	require.Zero(t, anchorType.NumMethod(), "an Anchor has no widening helpers")
	require.False(t, reflect.TypeOf(Scope{}).ConvertibleTo(anchorType))
	require.False(t, anchorType.ConvertibleTo(reflect.TypeOf(Scope{})))
}

func TestAnchorToKeepsTheCallersCenter(t *testing.T) {
	sc := Scope{TeacherID: uuid.New(), CenterID: uuid.New(), IsOwner: true, CanSendReports: true,
		Perms: BuildPermSet(nil, []string{PermBillingViewAll}, nil)}
	other := uuid.New()

	a := sc.AnchorTo(other)
	require.Equal(t, Anchor{TeacherID: other, CenterID: sc.CenterID}, a)
	require.Equal(t, Anchor{TeacherID: sc.TeacherID, CenterID: sc.CenterID}, sc.Self())
}

// OwnerAnchor is a proof, not a convenience: it embeds a plain Anchor so
// anchored repository helpers accept it, while a plain Anchor can never be
// passed where an OwnerAnchor is required.
func TestOwnerAnchorIsDistinctFromAnchor(t *testing.T) {
	owner, center := uuid.New(), uuid.New()
	oa := MintOwnerAnchor(owner, center)
	require.Equal(t, Anchor{TeacherID: owner, CenterID: center}, oa.Anchor)
	require.False(t, reflect.TypeOf(Anchor{}).AssignableTo(reflect.TypeOf(OwnerAnchor{})))
}
