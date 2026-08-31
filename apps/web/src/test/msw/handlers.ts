import { http, HttpResponse } from "msw";

import type { Teacher } from "@/features/auth";
import type { Meta } from "@/lib/api/envelope";

/** Must match vitest.config.ts test.env.VITE_API_URL. */
export const API_URL = "http://localhost:8080/api/v1";

/**
 * Origin for the public statement routes, which the real API mounts at the
 * server root (`/public/statements`) — outside `/api/v1`. Mirrors the
 * `publicApiClient` base-URL derivation in `@/lib/api/public-client.ts`.
 */
export const PUBLIC_API_URL = API_URL.replace(/\/api\/v1\/?$/, "");

/** Version stamp of the mirrored catalog below (`authctx.CatalogVersion`). */
export const CATALOG_VERSION = 3;

function perm(
  key: string,
  label: string,
  kind: "crud" | "scope" | "special",
  risk: "low" | "medium" | "high",
  description: string,
) {
  const [resource = "", action = ""] = key.split(".");
  return { key, label, resource, action, kind, risk, description };
}

const SCOPE_DESC = "Mở rộng phạm vi dữ liệu từ phần mình phụ trách sang toàn trung tâm.";

/**
 * The full assignment catalog, mirroring `authctx/catalog.go` (keys,
 * Vietnamese labels, structured fields) in registry order — deprecated keys
 * stay out, exactly as `GET /centers/me/permissions` serializes it. The
 * owner's `/centers/me` body carries every key — the server folds the owner
 * bypass into the effective array.
 */
export const PERMISSION_CATALOG = [
  perm("classes.create", "Tạo lớp học", "crud", "low", "Tạo lớp học mới trong trung tâm."),
  perm(
    "classes.list",
    "Xem danh sách lớp học",
    "crud",
    "low",
    "Xem danh sách lớp học trong phạm vi được thấy.",
  ),
  perm(
    "classes.read",
    "Xem chi tiết lớp học",
    "crud",
    "low",
    "Xem chi tiết lớp học và danh sách nhân sự của lớp.",
  ),
  perm("classes.edit", "Sửa lớp học", "crud", "low", "Cập nhật thông tin lớp học."),
  perm("classes.delete", "Xóa lớp học", "crud", "medium", "Xóa lớp học khỏi trung tâm."),
  perm(
    "classes.archive",
    "Lưu trữ lớp học",
    "special",
    "medium",
    "Chuyển lớp học sang trạng thái lưu trữ.",
  ),
  perm("classes.view_all", "Xem mọi lớp học", "scope", "high", SCOPE_DESC),
  perm("schedules.create", "Tạo lịch học", "crud", "low", "Thêm lịch học định kỳ cho lớp."),
  perm("schedules.edit", "Sửa lịch học", "crud", "low", "Cập nhật lịch học định kỳ của lớp."),
  perm("schedules.delete", "Xóa lịch học", "crud", "low", "Xóa lịch học định kỳ của lớp."),
  perm("contacts.create", "Tạo liên hệ", "crud", "low", "Tạo liên hệ phụ huynh/học viên mới."),
  perm(
    "contacts.list",
    "Xem danh sách liên hệ",
    "crud",
    "low",
    "Xem danh sách liên hệ trong phạm vi được thấy.",
  ),
  perm("contacts.read", "Xem chi tiết liên hệ", "crud", "low", "Xem chi tiết một liên hệ."),
  perm("contacts.edit", "Sửa liên hệ", "crud", "low", "Cập nhật thông tin liên hệ."),
  perm("contacts.delete", "Xóa liên hệ", "crud", "medium", "Xóa liên hệ khỏi trung tâm."),
  perm(
    "contacts.link_zalo",
    "Liên kết Zalo",
    "special",
    "low",
    "Gán hoặc gỡ liên kết Zalo của một liên hệ.",
  ),
  perm("contacts.view_all", "Xem mọi liên hệ", "scope", "high", SCOPE_DESC),
  perm("students.create", "Tạo học viên", "crud", "low", "Tạo hồ sơ học viên mới."),
  perm(
    "students.list",
    "Xem danh sách học viên",
    "crud",
    "low",
    "Xem danh sách học viên trong phạm vi được thấy.",
  ),
  perm("students.read", "Xem chi tiết học viên", "crud", "low", "Xem chi tiết hồ sơ học viên."),
  perm("students.edit", "Sửa học viên", "crud", "low", "Cập nhật hồ sơ học viên."),
  perm(
    "students.delete",
    "Xóa học viên",
    "crud",
    "high",
    "Ẩn danh hóa và xóa hồ sơ học viên — không khôi phục được.",
  ),
  perm("students.view_all", "Xem mọi học viên", "scope", "high", SCOPE_DESC),
  perm(
    "enrollments.create",
    "Ghi danh học viên",
    "crud",
    "low",
    "Ghi danh học viên vào lớp, gồm cả danh sách chọn học viên.",
  ),
  perm(
    "enrollments.list",
    "Xem danh sách ghi danh",
    "crud",
    "low",
    "Xem danh sách ghi danh trong phạm vi được thấy.",
  ),
  perm(
    "enrollments.read",
    "Xem chi tiết ghi danh",
    "crud",
    "low",
    "Xem chi tiết một lượt ghi danh.",
  ),
  perm("enrollments.delete", "Xóa ghi danh", "crud", "medium", "Xóa một lượt ghi danh."),
  perm(
    "enrollments.end",
    "Kết thúc ghi danh",
    "special",
    "medium",
    "Kết thúc lượt ghi danh của học viên trong lớp.",
  ),
  perm("enrollments.view_all", "Xem mọi ghi danh", "scope", "high", SCOPE_DESC),
  perm("sessions.create", "Tạo buổi học", "crud", "low", "Tạo buổi học cho lớp."),
  perm(
    "sessions.list",
    "Xem danh sách buổi học",
    "crud",
    "low",
    "Xem danh sách buổi học, gồm cả danh sách buổi chờ xử lý.",
  ),
  perm("sessions.read", "Xem chi tiết buổi học", "crud", "low", "Xem chi tiết một buổi học."),
  perm("sessions.delete", "Xóa buổi học", "crud", "medium", "Xóa một buổi học."),
  perm(
    "sessions.lifecycle",
    "Đổi trạng thái buổi học",
    "special",
    "medium",
    "Hủy, bỏ hủy hoặc tạm hoãn một buổi học.",
  ),
  perm("sessions.view_all", "Xem mọi buổi học", "scope", "high", SCOPE_DESC),
  perm("attendance.read", "Xem điểm danh", "crud", "low", "Xem điểm danh của buổi học."),
  perm(
    "attendance.confirm",
    "Xác nhận điểm danh",
    "special",
    "medium",
    "Ghi nhận và xác nhận điểm danh của buổi học.",
  ),
  perm("attendance.view_all", "Xem mọi điểm danh", "scope", "high", SCOPE_DESC),
  perm("scores.read", "Xem điểm số", "crud", "low", "Xem điểm số và cấu phần điểm của lớp."),
  perm("scores.edit", "Sửa điểm số", "crud", "medium", "Nhập và cập nhật điểm số của buổi học."),
  perm(
    "teaching.read",
    "Xem giảng dạy",
    "crud",
    "low",
    "Xem giáo trình, giáo án và nhận xét của lớp.",
  ),
  perm(
    "teaching.edit",
    "Sửa giảng dạy",
    "crud",
    "low",
    "Cập nhật giáo trình, giáo án, ghi chú và nhận xét buổi học.",
  ),
  perm(
    "teaching.review_queue",
    "Xem hàng chờ duyệt giáo án",
    "special",
    "low",
    "Xem hàng chờ duyệt giáo án của trung tâm.",
  ),
  perm("billing.create", "Tạo kỳ học phí", "crud", "low", "Khởi tạo kỳ học phí theo tháng."),
  perm("billing.list", "Xem danh sách kỳ học phí", "crud", "low", "Xem danh sách kỳ học phí."),
  perm(
    "billing.read",
    "Xem chi tiết học phí",
    "crud",
    "low",
    "Xem chi tiết kỳ học phí, hóa đơn, điều chỉnh và tình hình thu.",
  ),
  perm(
    "billing.draft",
    "Tạo nháp học phí",
    "special",
    "medium",
    "Tính toán lại hóa đơn nháp của kỳ học phí.",
  ),
  perm(
    "billing.close",
    "Chốt kỳ học phí",
    "special",
    "high",
    "Chốt kỳ học phí — khóa hóa đơn của kỳ.",
  ),
  perm(
    "billing.void_invoice",
    "Hủy hóa đơn",
    "special",
    "high",
    "Hủy hiệu lực một hóa đơn đã phát hành.",
  ),
  perm(
    "billing.adjust_invoice",
    "Điều chỉnh hóa đơn",
    "special",
    "high",
    "Thêm khoản điều chỉnh vào hóa đơn.",
  ),
  perm("billing.view_all", "Xem mọi dữ liệu học phí", "scope", "high", SCOPE_DESC),
  perm("payments.create", "Ghi nhận thanh toán", "crud", "low", "Ghi nhận một khoản thanh toán."),
  perm("payments.list", "Xem danh sách thanh toán", "crud", "low", "Xem danh sách thanh toán."),
  perm(
    "payments.read",
    "Xem chi tiết thanh toán",
    "crud",
    "low",
    "Xem chi tiết một khoản thanh toán.",
  ),
  perm(
    "payments.allocate",
    "Phân bổ thanh toán",
    "special",
    "medium",
    "Phân bổ khoản thanh toán vào hóa đơn.",
  ),
  perm(
    "payments.reverse",
    "Hoàn tác thanh toán",
    "special",
    "high",
    "Hoàn tác một khoản thanh toán đã ghi nhận.",
  ),
  perm("payments.view_all", "Xem mọi thanh toán", "scope", "high", SCOPE_DESC),
  perm(
    "statements.list",
    "Xem danh sách sao kê",
    "crud",
    "low",
    "Xem danh sách sao kê học phí của kỳ.",
  ),
  perm("statements.read", "Xem chi tiết sao kê", "crud", "low", "Xem chi tiết một sao kê học phí."),
  perm(
    "statements.generate",
    "Phát hành sao kê",
    "special",
    "high",
    "Phát hành sao kê học phí cho kỳ.",
  ),
  perm(
    "statements.revoke",
    "Thu hồi sao kê",
    "special",
    "high",
    "Thu hồi một sao kê đã phát hành.",
  ),
  perm("statements.view_all", "Xem mọi sao kê", "scope", "high", SCOPE_DESC),
  perm(
    "notifications.mark_sent",
    "Đánh dấu đã gửi thông báo",
    "special",
    "low",
    "Đánh dấu thông báo học phí đã được gửi tay.",
  ),
  perm("notifications.view_all", "Xem mọi thông báo", "scope", "high", SCOPE_DESC),
  perm(
    "reports.send",
    "Gửi báo cáo học phí",
    "special",
    "high",
    "Gửi thông báo học phí hàng loạt và theo dõi lượt gửi.",
  ),
  perm("members.manage", "Quản lý thành viên", "special", "high", "Gỡ thành viên khỏi trung tâm."),
  perm("center.manage", "Quản lý trung tâm", "special", "medium", "Cập nhật thông tin trung tâm."),
  perm(
    "invitations.manage",
    "Quản lý lời mời",
    "special",
    "medium",
    "Tạo, xem và thu hồi lời mời tham gia trung tâm.",
  ),
  perm(
    "audit.read",
    "Xem nhật ký hoạt động",
    "crud",
    "medium",
    "Xem nhật ký hoạt động của trung tâm.",
  ),
  perm(
    "imports.run",
    "Import dữ liệu",
    "special",
    "high",
    "Import danh sách lớp, học viên và liên hệ từ file.",
  ),
  perm(
    "dashboard.view",
    "Xem dashboard trung tâm",
    "special",
    "medium",
    "Xem dashboard tổng hợp tài chính và vận hành của trung tâm.",
  ),
];

export const ALL_PERMISSION_KEYS = PERMISSION_CATALOG.map((p) => p.key);

/**
 * Default RBAC read model for `GET /centers/me/permissions`: the three
 * system roles born with empty permission sets (v1 parity) and no member
 * rows. Tests override with `server.use` for populated states.
 */
export const DEFAULT_ROLES = {
  giaoVien: {
    id: "50000000-0000-4000-8000-000000000001",
    key: "giao_vien",
    name: "Giáo viên",
    permissions: [] as string[],
    assignment_version: 1,
  },
  hocVu: {
    id: "50000000-0000-4000-8000-000000000002",
    key: "hoc_vu",
    name: "Học vụ",
    permissions: [] as string[],
    assignment_version: 1,
  },
  troGiang: {
    id: "50000000-0000-4000-8000-000000000003",
    key: "tro_giang",
    name: "Trợ giảng",
    permissions: [] as string[],
    assignment_version: 1,
  },
};

export const DEFAULT_CENTER_PERMISSIONS = {
  catalog: PERMISSION_CATALOG,
  roles: [DEFAULT_ROLES.giaoVien, DEFAULT_ROLES.hocVu, DEFAULT_ROLES.troGiang],
  members: [],
  catalog_version: CATALOG_VERSION,
};

// --- Envelope builders (mirror the Go API's response shape exactly) ---

export function ok<T>(data: T, meta?: Meta) {
  return meta === undefined ? { success: true, data } : { success: true, data, meta };
}

/**
 * `details` mirrors `response.ErrWithDetails` — structured context the flat
 * `fields` map cannot carry (the roster import's per-row error list). Both
 * are omitted when unset, exactly as the Go envelope's `omitempty` does.
 */
export function fail(
  code: string,
  message: string,
  fields?: Record<string, string>,
  details?: unknown,
) {
  const error: Record<string, unknown> = { code, message };
  if (fields !== undefined) {
    error.fields = fields;
  }
  if (details !== undefined) {
    error.details = details;
  }
  return { success: false, error };
}

export function listMeta(total: number, page = 1, perPage = 20): Meta {
  return {
    page,
    per_page: perPage,
    total,
    total_pages: Math.max(1, Math.ceil(total / perPage)),
  };
}

// --- Fixtures ---

let teacherCounter = 0;

export function makeTeacher(overrides: Partial<Teacher> = {}): Teacher {
  teacherCounter += 1;
  return {
    id: `00000000-0000-4000-8000-${String(teacherCounter).padStart(12, "0")}`,
    phone: `+8490100${String(teacherCounter).padStart(4, "0")}`,
    full_name: `Teacher ${teacherCounter}`,
    timezone: "Asia/Ho_Chi_Minh",
    status: "active",
    created_at: "2026-08-01T10:00:00Z",
    ...overrides,
  };
}

export const primaryTeacher = makeTeacher({ full_name: "Cô Lan" });
export const secondaryTeacher = makeTeacher({ full_name: "Thầy Minh" });

export function makeSession(teacher: Teacher) {
  return {
    access_token: "test-access-token",
    token_type: "Bearer",
    expires_in: 900,
    teacher,
  };
}

interface PendingSession {
  session_id: string;
  class_id: string;
  class_name: string;
  session_date: string;
  start_time: string | null;
  status: string;
  expected_student_count: number;
  days_overdue: number;
}

export function makePendingSession(overrides: Partial<PendingSession> = {}): PendingSession {
  return {
    session_id: "10000000-0000-4000-8000-000000000001",
    class_id: "20000000-0000-4000-8000-000000000001",
    class_name: "Toán 6A",
    session_date: "2026-07-15",
    start_time: "18:00:00",
    status: "held",
    expected_student_count: 10,
    days_overdue: 1,
    ...overrides,
  };
}

export const defaultPendingSessions: PendingSession[] = [];

let classCounter = 0;

/** `classes.ClassResponse` fixture — one Monday-19:00 schedule by default. */
export function makeClass(overrides: Record<string, unknown> = {}) {
  classCounter += 1;
  const id = `20000000-0000-4000-8000-${String(classCounter).padStart(12, "0")}`;
  return {
    id,
    name: `Toán 9A${classCounter}`,
    teacher_id: `22000000-0000-4000-8000-${String(classCounter).padStart(12, "0")}`,
    start_date: "2026-08-01",
    end_date: null,
    default_unit_price: 60000,
    status: "active",
    schedules: [
      {
        id: `21000000-0000-4000-8000-${String(classCounter).padStart(12, "0")}`,
        weekday: 1,
        start_time: "19:00",
        duration_min: 90,
        effective_from: "2026-08-01",
        effective_to: null,
      },
    ],
    created_at: "2026-08-01T10:00:00Z",
    ...overrides,
  };
}

/** `sessions.SessionResponse` fixture for `GET /classes/:id/sessions`. */
export function makeClassSession(overrides: Record<string, unknown> = {}) {
  return {
    id: "10000000-0000-4000-8000-000000000099",
    class_id: "20000000-0000-4000-8000-000000000001",
    class_name: "Toán 9A1",
    session_date: "2026-08-03",
    start_time: "19:00:00",
    status: "held",
    cancel_reason: null,
    attendance_confirmed_at: "2026-08-03T21:00:00Z",
    student_count: 10,
    attendance_summary: null,
    created_at: "2026-08-01T10:00:00Z",
    ...overrides,
  };
}

/** `billing.PreviewResponse` fixture — zeroed totals unless overridden. */
export function makePreview(overrides: Record<string, unknown> = {}) {
  return {
    invoices: [],
    totals: {
      student_count: 0,
      total_opening: 0,
      total_charge: 0,
      total_adjustment: 0,
      total_due: 0,
    },
    ...overrides,
  };
}

/** `collections` summary fixture — zeroed unless overridden. */
export function makeCollectionsSummary(overrides: Record<string, unknown> = {}) {
  return {
    student_count: 0,
    contact_count: 0,
    total_due: 0,
    total_paid: 0,
    total_outstanding: 0,
    paid_contact_count: 0,
    unpaid_contact_count: 0,
    partial_contact_count: 0,
    unallocated_credit: 0,
    ...overrides,
  };
}

export function makePeriod(overrides: Record<string, unknown> = {}) {
  return {
    id: "30000000-0000-4000-8000-000000000001",
    year: 2026,
    month: 8,
    period_start: "2026-08-01",
    period_end: "2026-08-31",
    status: "open",
    closed_at: null,
    ...overrides,
  };
}

// --- Public statement fixtures (GET /public/statements/:token) ---

const publicStatementFixtureOneChild = {
  contact_name: "Chị Hoa",
  period: "08/2026",
  children: [
    {
      student_name: "Nguyễn Văn An",
      display_note: null,
      opening_balance: 0,
      classes: [
        {
          class_name: "Toán 6A",
          unit_price: 150000,
          billable_count: 12,
          absent_count: 1,
          amount: 1800000,
          sessions: [
            { date: "2026-08-03", status: "present", counted: true },
            { date: "2026-08-05", status: "present", counted: true },
            { date: "2026-08-07", status: "absent", counted: false },
          ],
        },
      ],
      adjustments: [],
      carried_adjustment: null,
      subtotal: 1800000,
    },
  ],
  totals: {
    opening_balance: 0,
    current_charge: 1800000,
    adjustment_total: 0,
    total_due: 1800000,
    paid: 0,
    outstanding: 1800000,
  },
  payments: {
    total_paid: 0,
    by_invoice: [
      { student_name: "Nguyễn Văn An", total_due: 1800000, paid: 0, outstanding: 1800000 },
    ],
  },
  qr: {
    image_url: "https://img.vietqr.io/image/example-an.png",
    amount: 1800000,
    note: "HP An T8",
  },
};

const publicStatementFixtureTwoChildren = {
  contact_name: "Anh Minh",
  period: "08/2026",
  children: [
    {
      student_name: "Trần Thị Bích",
      display_note: "Lớp 6",
      opening_balance: 500000,
      classes: [
        {
          class_name: "Toán 6A",
          unit_price: 150000,
          billable_count: 10,
          absent_count: 0,
          amount: 1500000,
          sessions: [
            { date: "2026-08-03", status: "present", counted: true },
            { date: "2026-08-05", status: "present", counted: true },
          ],
        },
        {
          class_name: "Lý 6A",
          unit_price: 130000,
          billable_count: 8,
          absent_count: 2,
          amount: 1040000,
          sessions: [
            { date: "2026-08-04", status: "present", counted: true },
            { date: "2026-08-06", status: "absent", counted: false },
          ],
        },
      ],
      adjustments: [],
      carried_adjustment: null,
      subtotal: 3040000,
    },
    {
      student_name: "Trần Văn Cường",
      display_note: null,
      opening_balance: 0,
      classes: [
        {
          class_name: "Văn 9A",
          unit_price: 160000,
          billable_count: 9,
          absent_count: 0,
          amount: 1440000,
          sessions: [{ date: "2026-08-02", status: "present", counted: true }],
        },
      ],
      adjustments: [],
      carried_adjustment: null,
      subtotal: 1440000,
    },
  ],
  totals: {
    opening_balance: 500000,
    current_charge: 3980000,
    adjustment_total: 0,
    total_due: 4480000,
    paid: 0,
    outstanding: 4480000,
  },
  payments: {
    total_paid: 0,
    by_invoice: [
      { student_name: "Trần Thị Bích", total_due: 3040000, paid: 0, outstanding: 3040000 },
      { student_name: "Trần Văn Cường", total_due: 1440000, paid: 0, outstanding: 1440000 },
    ],
  },
  qr: {
    image_url: "https://img.vietqr.io/image/example-minh.png",
    amount: 4480000,
    note: "HP Bich Cuong T8",
  },
};

const publicStatementFixtureCancelledSession = {
  contact_name: "Chị Lan",
  period: "08/2026",
  children: [
    {
      student_name: "Phạm Thị Dung",
      display_note: null,
      opening_balance: 0,
      classes: [
        {
          class_name: "Tiếng Anh 7A",
          unit_price: 140000,
          billable_count: 7,
          absent_count: 1,
          amount: 980000,
          sessions: [
            { date: "2026-08-01", status: "present", counted: true },
            { date: "2026-08-08", status: "absent", counted: false },
            { date: "2026-08-15", status: "cancelled", counted: false },
          ],
        },
      ],
      adjustments: [],
      carried_adjustment: null,
      subtotal: 980000,
    },
  ],
  totals: {
    opening_balance: 0,
    current_charge: 980000,
    adjustment_total: 0,
    total_due: 980000,
    paid: 0,
    outstanding: 980000,
  },
  payments: {
    total_paid: 0,
    by_invoice: [
      { student_name: "Phạm Thị Dung", total_due: 980000, paid: 0, outstanding: 980000 },
    ],
  },
  qr: {
    image_url: "https://img.vietqr.io/image/example-dung.png",
    amount: 980000,
    note: "HP Dung T8",
  },
};

const publicStatementFixtureNoQr = {
  contact_name: "Anh Tuấn",
  period: "08/2026",
  children: [
    {
      student_name: "Lê Văn Em",
      display_note: null,
      opening_balance: 0,
      classes: [
        {
          class_name: "Toán 5A",
          unit_price: 120000,
          billable_count: 8,
          absent_count: 0,
          amount: 960000,
          sessions: [{ date: "2026-08-02", status: "present", counted: true }],
        },
      ],
      adjustments: [],
      carried_adjustment: null,
      subtotal: 960000,
    },
  ],
  totals: {
    opening_balance: 0,
    current_charge: 960000,
    adjustment_total: 0,
    total_due: 960000,
    paid: 0,
    outstanding: 960000,
  },
  payments: {
    total_paid: 0,
    by_invoice: [{ student_name: "Lê Văn Em", total_due: 960000, paid: 0, outstanding: 960000 }],
  },
  qr: null,
};

/** Reachable by token value from `publicStatementHandler` below; any other token 404s. */
const publicStatementFixturesByToken: Record<string, unknown> = {
  "valid-token": publicStatementFixtureOneChild,
  "two-child-token": publicStatementFixtureTwoChildren,
  "cancelled-session-token": publicStatementFixtureCancelledSession,
  "no-qr-token": publicStatementFixtureNoQr,
};

// --- Default happy-path handlers; tests override per case with server.use() ---

export const handlers = [
  http.post(`${API_URL}/auth/login`, () => HttpResponse.json(ok(makeSession(primaryTeacher)))),
  // No refresh cookie in tests by default: a fresh visitor has no session.
  http.post(`${API_URL}/auth/refresh`, () =>
    HttpResponse.json(fail("UNAUTHORIZED", "invalid refresh token"), { status: 401 }),
  ),
  http.post(`${API_URL}/auth/logout`, () => HttpResponse.json(ok({ message: "logged out" }))),
  http.get(`${API_URL}/me`, () => HttpResponse.json(ok(primaryTeacher))),
  // `GET /centers/me` is role-shaped; the default is the owner body, since
  // any page that role-gates renders its owner branch by default. Tests that
  // need the member body (`{center_name}`) override this with server.use.
  http.get(`${API_URL}/centers/me`, () =>
    HttpResponse.json(
      ok({
        center: {
          id: "30000000-0000-4000-8000-000000000001",
          name: "Trung Tâm Bình Minh",
          is_owner: true,
        },
        members: [],
        permissions: ALL_PERMISSION_KEYS,
      }),
    ),
  ),
  // Owner-only RBAC read model; role/override tests override with server.use.
  http.get(`${API_URL}/centers/me/permissions`, () =>
    HttpResponse.json(ok(DEFAULT_CENTER_PERMISSIONS)),
  ),
  // RBAC writes succeed silently by default; tests asserting payloads
  // override these with server.use and capture the body.
  http.put(
    `${API_URL}/centers/me/roles/:roleId/permissions`,
    () => new HttpResponse(null, { status: 204 }),
  ),
  http.put(
    `${API_URL}/centers/me/members/:teacherId/role`,
    () => new HttpResponse(null, { status: 204 }),
  ),
  http.put(
    `${API_URL}/centers/me/members/:teacherId/overrides`,
    () => new HttpResponse(null, { status: 204 }),
  ),
  // A test teacher has no linked Zalo account unless the test says otherwise.
  http.get(`${API_URL}/me/zalo`, () => HttpResponse.json(ok({ linked: false }))),
  http.get(`${API_URL}/sessions/pending`, () =>
    HttpResponse.json(ok({ total: defaultPendingSessions.length, items: defaultPendingSessions })),
  ),
  http.post(`${API_URL}/billing-periods`, () =>
    HttpResponse.json(ok(makePeriod()), { status: 201 }),
  ),
  // Dashboard aggregation defaults: an empty roster with nothing billed.
  http.get(`${API_URL}/classes`, () => HttpResponse.json(ok([], listMeta(0)))),
  http.get(`${API_URL}/classes/:id/sessions`, () => HttpResponse.json(ok([]))),
  // Teaching defaults: nothing saved yet. Stateful round-trip handlers live in
  // `@/features/teaching/__tests__/teaching-handlers.ts`; tests that write
  // register those with server.use().
  http.get(`${API_URL}/classes/:id/curriculum`, () =>
    HttpResponse.json(ok({ lessons: [], current_index: 0 })),
  ),
  http.get(`${API_URL}/classes/:id/lesson-plans`, () => HttpResponse.json(ok([]))),
  http.get(`${API_URL}/classes/:id/marks`, () =>
    HttpResponse.json(ok({ session_notes: [], marks: [] })),
  ),
  http.get(`${API_URL}/teaching/review-queue`, () => HttpResponse.json(ok([]))),
  // No configured score components by default — the classbook scores tab
  // renders its plain general-score block; a test opting into the
  // per-component grid overrides this with server.use().
  http.get(`${API_URL}/classes/:id/score-components`, ({ params }) =>
    HttpResponse.json(ok({ class_id: params.id as string, components: [] })),
  ),
  http.get(`${API_URL}/sessions/:id/scores`, () =>
    HttpResponse.json(ok({ components: [], scores: [] })),
  ),
  http.get(`${API_URL}/students`, () => HttpResponse.json(ok([], listMeta(0)))),
  http.get(`${API_URL}/billing-periods/:id/preview`, () => HttpResponse.json(ok(makePreview()))),
  http.get(`${API_URL}/billing-periods/:id/collections/summary`, () =>
    HttpResponse.json(ok(makeCollectionsSummary())),
  ),
  // Always the same generic body — the real endpoint never reveals whether
  // the phone matched an eligible account (anti-enumeration).
  http.post(`${API_URL}/auth/forgot-password`, () =>
    HttpResponse.json(ok({ message: "if this phone is registered, a reset link has been sent" })),
  ),
  http.post(`${API_URL}/auth/reset-password`, () => new HttpResponse(null, { status: 204 })),
  // Owner-shaped by default — the signed-in teacher owns their center, so the
  // layout's center card resolves everywhere; member-shaped tests override.
  http.get(`${API_URL}/centers/me`, () =>
    HttpResponse.json(
      ok({
        center: {
          id: "30000000-0000-4000-8000-000000000001",
          name: "Trung Tâm Bình Minh",
          is_owner: true,
        },
        members: [],
        permissions: ALL_PERMISSION_KEYS,
      }),
    ),
  ),
  http.post(`${API_URL}/centers/me/invitations`, () =>
    HttpResponse.json(
      ok({
        id: "40000000-0000-4000-8000-000000000001",
        phone: "+84901234567",
        expires_at: "2026-08-19T10:00:00Z",
        link: "https://app.teka.dev/invite/test-invite-token",
        dm_status: "sent",
      }),
      { status: 201 },
    ),
  ),
  http.get(`${API_URL}/centers/me/invitations`, () => HttpResponse.json(ok([]))),
  http.delete(
    `${API_URL}/centers/me/invitations/:id`,
    () => new HttpResponse(null, { status: 204 }),
  ),
  // Anti-enumeration: every rejection reason (unknown/expired/revoked/used
  // token) collapses to the same generic 404 on the real API; tests override
  // this default with a fixture keyed to a specific token.
  http.post(`${API_URL}/invitations/preview`, () =>
    HttpResponse.json(ok({ center_name: "Trung Tâm Bình Minh", phone_masked: "+84******567" })),
  ),
  http.post(`${API_URL}/invitations/accept`, () => new HttpResponse(null, { status: 204 })),
  // Every failure mode (unknown, malformed, revoked, expired, already-paid,
  // soft-deleted) collapses to a plain 404 — the real API has no other error
  // code for this endpoint.
  http.get(`${PUBLIC_API_URL}/public/statements/:token`, ({ params }) => {
    const fixture = publicStatementFixturesByToken[params.token as string];
    if (!fixture) {
      return HttpResponse.json(fail("NOT_FOUND", "statement not found"), { status: 404 });
    }
    return HttpResponse.json(ok(fixture));
  }),
];
