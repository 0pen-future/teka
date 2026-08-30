package authctx

import (
	"testing"
)

func TestValidStaffRole(t *testing.T) {
	for _, key := range []string{StaffRoleGiaoVien, StaffRoleHocVu, StaffRoleTroGiang} {
		if !ValidStaffRole(key) {
			t.Errorf("ValidStaffRole(%q) = false, want true", key)
		}
	}
	for _, key := range []string{"", "owner", "thu_ky", "GIAO_VIEN", "giao_vien "} {
		if ValidStaffRole(key) {
			t.Errorf("ValidStaffRole(%q) = true, want false", key)
		}
	}
}

func TestStaffRoleKeysIsACopy(t *testing.T) {
	keys := StaffRoleKeys()
	if len(keys) != 3 {
		t.Fatalf("StaffRoleKeys() = %v, want 3 keys", keys)
	}
	keys[0] = "mutated"
	if StaffRoleKeys()[0] != StaffRoleGiaoVien {
		t.Error("mutating the returned slice must not touch the registry")
	}
}

func TestStaffRoleLabelCoversRegistry(t *testing.T) {
	for _, key := range StaffRoleKeys() {
		if StaffRoleLabel(key) == "" {
			t.Errorf("StaffRoleLabel(%q) = \"\", every registry key needs a label", key)
		}
	}
	if StaffRoleLabel("unknown") != "" {
		t.Error("StaffRoleLabel of an unknown key must be empty")
	}
}

// The capability map is the write-authorization authority; this pins the
// agreed matrix so an accidental edit fails a test, not a security review.
func TestStaffCapabilityMatrix(t *testing.T) {
	cases := []struct {
		cap  ClassCapability
		role string
		want bool
	}{
		{CapAttendanceWrite, StaffRoleGiaoVien, true},
		{CapAttendanceWrite, StaffRoleTroGiang, true},
		{CapAttendanceWrite, StaffRoleHocVu, false},
		{CapScoresWrite, StaffRoleGiaoVien, true},
		{CapScoresWrite, StaffRoleTroGiang, false},
		{CapScoresWrite, StaffRoleHocVu, false},
		{CapRemarksWrite, StaffRoleGiaoVien, true},
		{CapRemarksWrite, StaffRoleHocVu, false},
		{CapLessonPlanWrite, StaffRoleGiaoVien, true},
		{CapLessonPlanWrite, StaffRoleTroGiang, false},
		{CapEnrollmentWrite, StaffRoleGiaoVien, true},
		{CapEnrollmentWrite, StaffRoleHocVu, false},
		{CapStatementSend, StaffRoleHocVu, true},
		{CapStatementSend, StaffRoleGiaoVien, false},
		{CapStatementSend, StaffRoleTroGiang, false},
		{CapSessionsWrite, StaffRoleGiaoVien, true},
		{CapSessionsWrite, StaffRoleTroGiang, false},
		{CapSessionsWrite, StaffRoleHocVu, false},
	}
	for _, tc := range cases {
		if got := StaffRoleCan(tc.role, tc.cap); got != tc.want {
			t.Errorf("StaffRoleCan(%q, %q) = %v, want %v", tc.role, tc.cap, got, tc.want)
		}
	}
}

func TestStaffRolesForUnknownCapability(t *testing.T) {
	if roles := StaffRolesFor("nonexistent.write"); len(roles) != 0 {
		t.Errorf("StaffRolesFor(unknown) = %v, want empty", roles)
	}
}
