package authctx

// Class-staff roles are the per-class staffing axis, deliberately independent
// of center_roles even though the key strings coincide: a center role decides
// what a member may do center-wide, a staff role decides what they may do
// inside one class they are assigned to. The two always intersect (AND) — a
// permission deny in center_member_permissions wins over any class assignment.
//
// The vocabulary is code-owned like the permission registry: class_staff rows
// store the key with no FK or CHECK, so adding a role is a code change and an
// unknown key read back from the database simply matches no capability.
const (
	StaffRoleGiaoVien = "giao_vien"
	StaffRoleHocVu    = "hoc_vu"
	StaffRoleTroGiang = "tro_giang"
)

// staffRoleRegistry is the closed set of assignable staff roles, in stable
// display order.
var staffRoleRegistry = []string{
	StaffRoleGiaoVien,
	StaffRoleHocVu,
	StaffRoleTroGiang,
}

// staffRoleLabels carries the Vietnamese display label per role key. The API
// response is the single label source — the web renders what it receives.
var staffRoleLabels = map[string]string{
	StaffRoleGiaoVien: "Giáo viên",
	StaffRoleHocVu:    "Học vụ",
	StaffRoleTroGiang: "Trợ giảng",
}

// StaffRoleKeys returns the registered staff role keys in stable order;
// callers get a copy and cannot mutate the registry.
func StaffRoleKeys() []string {
	out := make([]string, len(staffRoleRegistry))
	copy(out, staffRoleRegistry)
	return out
}

// StaffRoleLabel returns the Vietnamese display label of a role key ("" for
// keys outside the registry).
func StaffRoleLabel(key string) string {
	return staffRoleLabels[key]
}

// ValidStaffRole reports whether key belongs to the staff role registry.
func ValidStaffRole(key string) bool {
	for _, k := range staffRoleRegistry {
		if k == key {
			return true
		}
	}
	return false
}

// ClassCapability names one kind of WRITE inside a class. Reads need no map:
// every assignment — active or ended — grants read on the class's history.
// Writes require an ACTIVE assignment whose role appears in the capability's
// role list below.
type ClassCapability string

// The capability → writing-roles map: the one authority on which staff role
// may perform which class write.
const (
	CapAttendanceWrite ClassCapability = "attendance.write"
	CapScoresWrite     ClassCapability = "scores.write"
	CapRemarksWrite    ClassCapability = "remarks.write"
	CapLessonPlanWrite ClassCapability = "lesson_plan.write"
	CapEnrollmentWrite ClassCapability = "enrollment.write"
	CapSessionsWrite   ClassCapability = "sessions.write"
	CapStatementSend   ClassCapability = "statement.send"
)

var staffCapabilities = map[ClassCapability][]string{
	CapAttendanceWrite: {StaffRoleGiaoVien, StaffRoleTroGiang},
	CapScoresWrite:     {StaffRoleGiaoVien},
	CapRemarksWrite:    {StaffRoleGiaoVien},
	CapLessonPlanWrite: {StaffRoleGiaoVien},
	CapEnrollmentWrite: {StaffRoleGiaoVien},
	CapSessionsWrite:   {StaffRoleGiaoVien},
	CapStatementSend:   {StaffRoleHocVu},
}

// StaffRolesFor returns the roles allowed to perform the capability, in
// registry order; callers get a copy. An unknown capability yields nil — no
// role may write.
func StaffRolesFor(capability ClassCapability) []string {
	roles := staffCapabilities[capability]
	out := make([]string, len(roles))
	copy(out, roles)
	return out
}

// StaffRoleCan reports whether roleKey may perform the capability. It is the
// single branching point feature gates call, so the map above stays the one
// authority on who writes what.
func StaffRoleCan(roleKey string, capability ClassCapability) bool {
	for _, r := range staffCapabilities[capability] {
		if r == roleKey {
			return true
		}
	}
	return false
}
