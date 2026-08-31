package authctx

import (
	"strings"

	"teka/apps/api/internal/shared/apperror"
)

// PermKind classifies a catalog entry: crud keys gate one canonical verb on
// one resource, scope keys widen repository visibility from own rows to the
// whole center, special keys gate a named domain command.
type PermKind string

// The three catalog kinds.
const (
	PermKindCRUD    PermKind = "crud"
	PermKindScope   PermKind = "scope"
	PermKindSpecial PermKind = "special"
)

// PermRisk drives UI emphasis and review pressure; it never changes
// enforcement.
type PermRisk string

// The three risk grades.
const (
	RiskLow    PermRisk = "low"
	RiskMedium PermRisk = "medium"
	RiskHigh   PermRisk = "high"
)

// PermDef is one code-owned catalog entry. The database stores assignments
// only; definitions are frozen per release and serialized in Order.
type PermDef struct {
	Key         string
	Resource    string
	Action      string
	Kind        PermKind
	Label       string
	Description string
	Risk        PermRisk
	Grantable   bool
	Deprecated  bool
	Order       int
}

// Resource-action catalog keys. Legacy identity keys (reports.send,
// members.manage, center.manage, invitations.manage, audit.read, imports.run,
// dashboard.view, teaching.review_queue) and the deprecated
// data.view_center_wide live in permissions.go.
const (
	PermClassesCreate  = "classes.create"
	PermClassesList    = "classes.list"
	PermClassesRead    = "classes.read"
	PermClassesEdit    = "classes.edit"
	PermClassesDelete  = "classes.delete"
	PermClassesArchive = "classes.archive"
	PermClassesViewAll = "classes.view_all"

	PermSchedulesCreate = "schedules.create"
	PermSchedulesEdit   = "schedules.edit"
	PermSchedulesDelete = "schedules.delete"

	PermContactsCreate   = "contacts.create"
	PermContactsList     = "contacts.list"
	PermContactsRead     = "contacts.read"
	PermContactsEdit     = "contacts.edit"
	PermContactsDelete   = "contacts.delete"
	PermContactsLinkZalo = "contacts.link_zalo"
	PermContactsViewAll  = "contacts.view_all"

	PermStudentsCreate  = "students.create"
	PermStudentsList    = "students.list"
	PermStudentsRead    = "students.read"
	PermStudentsEdit    = "students.edit"
	PermStudentsDelete  = "students.delete"
	PermStudentsViewAll = "students.view_all"

	PermEnrollmentsCreate  = "enrollments.create"
	PermEnrollmentsList    = "enrollments.list"
	PermEnrollmentsRead    = "enrollments.read"
	PermEnrollmentsDelete  = "enrollments.delete"
	PermEnrollmentsEnd     = "enrollments.end"
	PermEnrollmentsViewAll = "enrollments.view_all"

	PermSessionsCreate    = "sessions.create"
	PermSessionsList      = "sessions.list"
	PermSessionsRead      = "sessions.read"
	PermSessionsDelete    = "sessions.delete"
	PermSessionsLifecycle = "sessions.lifecycle"
	PermSessionsViewAll   = "sessions.view_all"

	PermAttendanceRead    = "attendance.read"
	PermAttendanceConfirm = "attendance.confirm"
	PermAttendanceViewAll = "attendance.view_all"

	PermScoresRead    = "scores.read"
	PermScoresEdit    = "scores.edit"
	PermScoresViewAll = "scores.view_all"

	PermTeachingRead    = "teaching.read"
	PermTeachingEdit    = "teaching.edit"
	PermTeachingViewAll = "teaching.view_all"

	PermBillingCreate        = "billing.create"
	PermBillingList          = "billing.list"
	PermBillingRead          = "billing.read"
	PermBillingDraft         = "billing.draft"
	PermBillingClose         = "billing.close"
	PermBillingVoidInvoice   = "billing.void_invoice"
	PermBillingAdjustInvoice = "billing.adjust_invoice"
	PermBillingViewAll       = "billing.view_all"

	PermPaymentsCreate   = "payments.create"
	PermPaymentsList     = "payments.list"
	PermPaymentsRead     = "payments.read"
	PermPaymentsAllocate = "payments.allocate"
	PermPaymentsReverse  = "payments.reverse"
	PermPaymentsViewAll  = "payments.view_all"

	PermStatementsList     = "statements.list"
	PermStatementsRead     = "statements.read"
	PermStatementsGenerate = "statements.generate"
	PermStatementsRevoke   = "statements.revoke"
	PermStatementsViewAll  = "statements.view_all"

	PermNotificationsMarkSent = "notifications.mark_sent"
	PermNotificationsViewAll  = "notifications.view_all"
)

// def builds a grantable, non-deprecated catalog entry; resource and action
// derive from the key so they can never drift apart.
func def(key string, kind PermKind, risk PermRisk, label, description string) PermDef {
	resource, action, _ := strings.Cut(key, ".")
	return PermDef{
		Key: key, Resource: resource, Action: action,
		Kind: kind, Risk: risk, Label: label, Description: description,
		Grantable: true,
	}
}

func viewAll(key, label string) PermDef {
	d := def(key, PermKindScope, RiskHigh, label,
		"Mở rộng phạm vi dữ liệu từ phần mình phụ trách sang toàn trung tâm.")
	return d
}

// permCatalog is the full code-owned catalog in display/serialization order.
// Order fields are assigned in init from position.
var permCatalog = []PermDef{
	def(PermClassesCreate, PermKindCRUD, RiskLow, "Tạo lớp học", "Tạo lớp học mới trong trung tâm."),
	def(PermClassesList, PermKindCRUD, RiskLow, "Xem danh sách lớp học", "Xem danh sách lớp học trong phạm vi được thấy."),
	def(PermClassesRead, PermKindCRUD, RiskLow, "Xem chi tiết lớp học", "Xem chi tiết lớp học và danh sách nhân sự của lớp."),
	def(PermClassesEdit, PermKindCRUD, RiskLow, "Sửa lớp học", "Cập nhật thông tin lớp học."),
	def(PermClassesDelete, PermKindCRUD, RiskMedium, "Xóa lớp học", "Xóa lớp học khỏi trung tâm."),
	def(PermClassesArchive, PermKindSpecial, RiskMedium, "Lưu trữ lớp học", "Chuyển lớp học sang trạng thái lưu trữ."),
	viewAll(PermClassesViewAll, "Xem mọi lớp học"),

	def(PermSchedulesCreate, PermKindCRUD, RiskLow, "Tạo lịch học", "Thêm lịch học định kỳ cho lớp."),
	def(PermSchedulesEdit, PermKindCRUD, RiskLow, "Sửa lịch học", "Cập nhật lịch học định kỳ của lớp."),
	def(PermSchedulesDelete, PermKindCRUD, RiskLow, "Xóa lịch học", "Xóa lịch học định kỳ của lớp."),

	def(PermContactsCreate, PermKindCRUD, RiskLow, "Tạo liên hệ", "Tạo liên hệ phụ huynh/học viên mới."),
	def(PermContactsList, PermKindCRUD, RiskLow, "Xem danh sách liên hệ", "Xem danh sách liên hệ trong phạm vi được thấy."),
	def(PermContactsRead, PermKindCRUD, RiskLow, "Xem chi tiết liên hệ", "Xem chi tiết một liên hệ."),
	def(PermContactsEdit, PermKindCRUD, RiskLow, "Sửa liên hệ", "Cập nhật thông tin liên hệ."),
	def(PermContactsDelete, PermKindCRUD, RiskMedium, "Xóa liên hệ", "Xóa liên hệ khỏi trung tâm."),
	def(PermContactsLinkZalo, PermKindSpecial, RiskLow, "Liên kết Zalo", "Gán hoặc gỡ liên kết Zalo của một liên hệ."),
	viewAll(PermContactsViewAll, "Xem mọi liên hệ"),

	def(PermStudentsCreate, PermKindCRUD, RiskLow, "Tạo học viên", "Tạo hồ sơ học viên mới."),
	def(PermStudentsList, PermKindCRUD, RiskLow, "Xem danh sách học viên", "Xem danh sách học viên trong phạm vi được thấy."),
	def(PermStudentsRead, PermKindCRUD, RiskLow, "Xem chi tiết học viên", "Xem chi tiết hồ sơ học viên."),
	def(PermStudentsEdit, PermKindCRUD, RiskLow, "Sửa học viên", "Cập nhật hồ sơ học viên."),
	def(PermStudentsDelete, PermKindCRUD, RiskHigh, "Xóa học viên", "Ẩn danh hóa và xóa hồ sơ học viên — không khôi phục được."),
	viewAll(PermStudentsViewAll, "Xem mọi học viên"),

	def(PermEnrollmentsCreate, PermKindCRUD, RiskLow, "Ghi danh học viên", "Ghi danh học viên vào lớp, gồm cả danh sách chọn học viên."),
	def(PermEnrollmentsList, PermKindCRUD, RiskLow, "Xem danh sách ghi danh", "Xem danh sách ghi danh trong phạm vi được thấy."),
	def(PermEnrollmentsRead, PermKindCRUD, RiskLow, "Xem chi tiết ghi danh", "Xem chi tiết một lượt ghi danh."),
	def(PermEnrollmentsDelete, PermKindCRUD, RiskMedium, "Xóa ghi danh", "Xóa một lượt ghi danh."),
	def(PermEnrollmentsEnd, PermKindSpecial, RiskMedium, "Kết thúc ghi danh", "Kết thúc lượt ghi danh của học viên trong lớp."),
	viewAll(PermEnrollmentsViewAll, "Xem mọi ghi danh"),

	def(PermSessionsCreate, PermKindCRUD, RiskLow, "Tạo buổi học", "Tạo buổi học cho lớp."),
	def(PermSessionsList, PermKindCRUD, RiskLow, "Xem danh sách buổi học", "Xem danh sách buổi học, gồm cả danh sách buổi chờ xử lý."),
	def(PermSessionsRead, PermKindCRUD, RiskLow, "Xem chi tiết buổi học", "Xem chi tiết một buổi học."),
	def(PermSessionsDelete, PermKindCRUD, RiskMedium, "Xóa buổi học", "Xóa một buổi học."),
	def(PermSessionsLifecycle, PermKindSpecial, RiskMedium, "Đổi trạng thái buổi học", "Hủy, bỏ hủy hoặc tạm hoãn một buổi học."),
	viewAll(PermSessionsViewAll, "Xem mọi buổi học"),

	def(PermAttendanceRead, PermKindCRUD, RiskLow, "Xem điểm danh", "Xem điểm danh của buổi học."),
	def(PermAttendanceConfirm, PermKindSpecial, RiskMedium, "Xác nhận điểm danh", "Ghi nhận và xác nhận điểm danh của buổi học."),
	viewAll(PermAttendanceViewAll, "Xem mọi điểm danh"),

	def(PermScoresRead, PermKindCRUD, RiskLow, "Xem điểm số", "Xem điểm số và cấu phần điểm của lớp."),
	def(PermScoresEdit, PermKindCRUD, RiskMedium, "Sửa điểm số", "Nhập và cập nhật điểm số của buổi học."),
	// Scores and teaching rows are reached through their class/session, so
	// their repositories scope via class resolution (classes/sessions
	// view_all) and consult no scope key of their own yet. These two keys
	// are reserved for a direct center-wide surface (e.g. a cross-class score
	// report); until one exists, granting or denying them changes nothing —
	// wire or retire them in the legacy-cleanup phase.
	viewAll(PermScoresViewAll, "Xem mọi điểm số"),

	def(PermTeachingRead, PermKindCRUD, RiskLow, "Xem giảng dạy", "Xem giáo trình, giáo án và nhận xét của lớp."),
	def(PermTeachingEdit, PermKindCRUD, RiskLow, "Sửa giảng dạy", "Cập nhật giáo trình, giáo án, ghi chú và nhận xét buổi học."),
	def(PermTeachingReviewQueue, PermKindSpecial, RiskLow, "Xem hàng chờ duyệt giáo án", "Xem hàng chờ duyệt giáo án của trung tâm."),
	viewAll(PermTeachingViewAll, "Xem mọi dữ liệu giảng dạy"),

	def(PermBillingCreate, PermKindCRUD, RiskLow, "Tạo kỳ học phí", "Khởi tạo kỳ học phí theo tháng."),
	def(PermBillingList, PermKindCRUD, RiskLow, "Xem danh sách kỳ học phí", "Xem danh sách kỳ học phí."),
	def(PermBillingRead, PermKindCRUD, RiskLow, "Xem chi tiết học phí", "Xem chi tiết kỳ học phí, hóa đơn, điều chỉnh và tình hình thu."),
	def(PermBillingDraft, PermKindSpecial, RiskMedium, "Tạo nháp học phí", "Tính toán lại hóa đơn nháp của kỳ học phí."),
	def(PermBillingClose, PermKindSpecial, RiskHigh, "Chốt kỳ học phí", "Chốt kỳ học phí — khóa hóa đơn của kỳ."),
	def(PermBillingVoidInvoice, PermKindSpecial, RiskHigh, "Hủy hóa đơn", "Hủy hiệu lực một hóa đơn đã phát hành."),
	def(PermBillingAdjustInvoice, PermKindSpecial, RiskHigh, "Điều chỉnh hóa đơn", "Thêm khoản điều chỉnh vào hóa đơn."),
	viewAll(PermBillingViewAll, "Xem mọi dữ liệu học phí"),

	def(PermPaymentsCreate, PermKindCRUD, RiskLow, "Ghi nhận thanh toán", "Ghi nhận một khoản thanh toán."),
	def(PermPaymentsList, PermKindCRUD, RiskLow, "Xem danh sách thanh toán", "Xem danh sách thanh toán."),
	def(PermPaymentsRead, PermKindCRUD, RiskLow, "Xem chi tiết thanh toán", "Xem chi tiết một khoản thanh toán."),
	def(PermPaymentsAllocate, PermKindSpecial, RiskMedium, "Phân bổ thanh toán", "Phân bổ khoản thanh toán vào hóa đơn."),
	def(PermPaymentsReverse, PermKindSpecial, RiskHigh, "Hoàn tác thanh toán", "Hoàn tác một khoản thanh toán đã ghi nhận."),
	viewAll(PermPaymentsViewAll, "Xem mọi thanh toán"),

	def(PermStatementsList, PermKindCRUD, RiskLow, "Xem danh sách sao kê", "Xem danh sách sao kê học phí của kỳ."),
	def(PermStatementsRead, PermKindCRUD, RiskLow, "Xem chi tiết sao kê", "Xem chi tiết một sao kê học phí."),
	def(PermStatementsGenerate, PermKindSpecial, RiskHigh, "Phát hành sao kê", "Phát hành sao kê học phí cho kỳ."),
	def(PermStatementsRevoke, PermKindSpecial, RiskHigh, "Thu hồi sao kê", "Thu hồi một sao kê đã phát hành."),
	viewAll(PermStatementsViewAll, "Xem mọi sao kê"),

	def(PermNotificationsMarkSent, PermKindSpecial, RiskLow, "Đánh dấu đã gửi thông báo", "Đánh dấu thông báo học phí đã được gửi tay."),
	viewAll(PermNotificationsViewAll, "Xem mọi thông báo"),

	def(PermReportsSend, PermKindSpecial, RiskHigh, "Gửi báo cáo học phí", "Gửi thông báo học phí hàng loạt và theo dõi lượt gửi."),
	def(PermMembersManage, PermKindSpecial, RiskHigh, "Quản lý thành viên", "Gỡ thành viên khỏi trung tâm."),
	def(PermCenterManage, PermKindSpecial, RiskMedium, "Quản lý trung tâm", "Cập nhật thông tin trung tâm."),
	def(PermInvitationsManage, PermKindSpecial, RiskMedium, "Quản lý lời mời", "Tạo, xem và thu hồi lời mời tham gia trung tâm."),
	def(PermAuditRead, PermKindCRUD, RiskMedium, "Xem nhật ký hoạt động", "Xem nhật ký hoạt động của trung tâm."),
	def(PermImportsRun, PermKindSpecial, RiskHigh, "Import dữ liệu", "Import danh sách lớp, học viên và liên hệ từ file."),
	def(PermDashboardView, PermKindSpecial, RiskMedium, "Xem dashboard trung tâm", "Xem dashboard tổng hợp tài chính và vận hành của trung tâm."),

	// Deprecated single-axis scope key: kept known so legacy assignment rows
	// stay effective during the compatibility window, but no longer
	// assignable — new writes use the per-resource view_all keys.
	{
		Key: PermDataViewCenterWide, Resource: "data", Action: "view_center_wide",
		Kind: PermKindScope, Risk: RiskHigh,
		Label:       "Xem dữ liệu toàn trung tâm",
		Description: "Khóa phạm vi cũ, đã thay bằng các quyền “Xem mọi …” theo từng loại dữ liệu.",
		Grantable:   false, Deprecated: true,
	},
}

// permAliases maps a deprecated key to its canonical equivalence class. Alias
// expansion is symmetric: a grant of the legacy key grants every canonical
// key, a deny removes every canonical key. A single canonical key never
// expands back to the legacy key.
var permAliases = map[string][]string{
	PermDataViewCenterWide: {
		PermClassesViewAll,
		PermContactsViewAll,
		PermStudentsViewAll,
		PermEnrollmentsViewAll,
		PermSessionsViewAll,
		PermAttendanceViewAll,
		PermScoresViewAll,
		PermTeachingViewAll,
		PermBillingViewAll,
		PermPaymentsViewAll,
		PermStatementsViewAll,
		PermNotificationsViewAll,
	},
}

// permIndex maps key → catalog position; permRegistry and permLabels in
// permissions.go are derived views over the same catalog.
var permIndex = make(map[string]int, len(permCatalog))

func init() {
	for i := range permCatalog {
		d := &permCatalog[i]
		if _, dup := permIndex[d.Key]; dup {
			panic("authctx: duplicate catalog key " + d.Key)
		}
		d.Order = i
		permIndex[d.Key] = i
		permRegistry = append(permRegistry, d.Key)
		permLabels[d.Key] = d.Label
	}
	for legacy, canonical := range permAliases {
		if _, ok := permIndex[legacy]; !ok {
			panic("authctx: alias source " + legacy + " missing from catalog")
		}
		for _, key := range canonical {
			if _, ok := permIndex[key]; !ok {
				panic("authctx: alias target " + key + " missing from catalog")
			}
		}
	}
}

// PermDefs returns the full catalog in serialization order; callers get a
// copy and cannot mutate the definitions.
func PermDefs() []PermDef {
	out := make([]PermDef, len(permCatalog))
	copy(out, permCatalog)
	return out
}

// PermDefOf returns the definition of a catalog key.
func PermDefOf(key string) (PermDef, bool) {
	i, ok := permIndex[key]
	if !ok {
		return PermDef{}, false
	}
	return permCatalog[i], true
}

// GrantableKeys returns the keys assignment writes may reference, in catalog
// order. Deprecated keys stay effective for existing rows but are rejected on
// new writes.
func GrantableKeys() []string {
	out := make([]string, 0, len(permCatalog))
	for _, d := range permCatalog {
		if d.Grantable {
			out = append(out, d.Key)
		}
	}
	return out
}

// CatalogVersion is the CAS anchor for permission-assignment writes: reads
// carry it, replacement writes echo it, and a mismatch is a 409 — a client
// that rendered an older catalog must reload before writing. Version 1 was
// the legacy 9-key registry; 2 is the resource-action catalog. Bump on any
// change that alters what a stored assignment means.
const CatalogVersion = 2

// legacyIdentitySet is the pre-catalog identity keys: operations that were
// permission-gated before the resource-action catalog existed. They stay out
// of the default baseline — membership never granted them, so backfilling
// them would escalate.
var legacyIdentitySet = map[string]bool{
	PermReportsSend:         true,
	PermMembersManage:       true,
	PermCenterManage:        true,
	PermInvitationsManage:   true,
	PermAuditRead:           true,
	PermImportsRun:          true,
	PermDashboardView:       true,
	PermTeachingReviewQueue: true,
}

// DefaultRoleKeys returns the baseline permission set every system role (and,
// via member grants, every role-less legacy stint) receives in the
// compatibility backfill, in catalog order. Before the catalog, membership
// alone granted all operational access — so the baseline is every grantable
// operational key: scope keys stay out (visibility must not widen) and the
// legacy identity keys stay out (already gated, granting would escalate).
// The SQL backfill artifacts must stay in parity with this list — the
// migrations package pins that with a checksum test.
func DefaultRoleKeys() []string {
	out := make([]string, 0, len(permCatalog))
	for _, d := range permCatalog {
		if d.Grantable && d.Kind != PermKindScope && !legacyIdentitySet[d.Key] {
			out = append(out, d.Key)
		}
	}
	return out
}

// CenterWideFor reports whether the caller sees the whole center's rows for
// one resource — the per-resource replacement for CenterWide(). key must be a
// <resource>.view_all catalog key; the legacy axis participates only through
// alias expansion at set-build time, never here.
func (s Scope) CenterWideFor(key string) bool {
	return s.IsOwner || s.Perms.HasKey(key)
}

// WriteWide reports whether the caller's mutations reach the whole center's
// rows. Only the owner passes: scope keys (<resource>.view_all) widen
// visibility, never writes — member writes were own-rows-only before the
// catalog existed, so a key widening writes would be an escalation.
// Repositories whose write path has no capability filter branch on this
// instead of IsOwner, keeping the write-scope policy in one home.
func (s Scope) WriteWide() bool {
	return s.IsOwner
}

// Require is the service-boundary authorization check for callers that do not
// pass through HTTP middleware. Same semantics as Scope.Has, as a typed
// apperror the response layer already knows how to render.
func Require(s Scope, key string) error {
	if s.Has(key) {
		return nil
	}
	return apperror.Forbidden("Bạn không có quyền thực hiện thao tác này")
}
