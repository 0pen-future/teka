# Scouting Report: Loại bỏ "Hồ sơ học sinh" Feature

## 1. Files dành riêng cho /records (có thể xoá)

### Pages và Components:
- `/home/cesc/Documents/personal-workspace/teka/apps/web/src/features/teaching/pages/records-page.tsx` — trang danh sách hồ sơ (line 44-221)
- `/home/cesc/Documents/personal-workspace/teka/apps/web/src/features/teaching/pages/student-record-page.tsx` — trang chi tiết một học sinh (line 30-196)
- `/home/cesc/Documents/personal-workspace/teka/apps/web/src/features/teaching/components/records-toolbar.tsx` — thanh công cụ lọc/chọn lớp cho records page
- `/home/cesc/Documents/personal-workspace/teka/apps/web/src/features/teaching/components/student-records-table.tsx` — bảng danh sách học sinh

### Tests:
- `/home/cesc/Documents/personal-workspace/teka/apps/web/src/features/teaching/__tests__/records-pages.test.tsx` — tests cho cả hai trang records
- `/home/cesc/Documents/personal-workspace/teka/apps/web/src/features/teaching/__tests__/records-toolbar.test.tsx` — tests toolbar
- `/home/cesc/Documents/personal-workspace/teka/apps/web/src/features/teaching/__tests__/student-records-table.test.tsx` — tests bảng

### E2E:
- `/home/cesc/Documents/personal-workspace/teka/apps/web/e2e/records-search.spec.ts` — test tìm kiếm học sinh trên mobile (lines 21-68)
- `/home/cesc/Documents/personal-workspace/teka/apps/web/e2e/helpers/ux-routes.ts` — entry tại line 43: `{ name: "records", path: "/records", auth: "owner" }`

### Routing:
- `/home/cesc/Documents/personal-workspace/teka/apps/web/src/features/teaching/routes.tsx` — routes registration (lines 17-27)

## 2. Inbound Links đến /records hoặc /records/:id

### Sidebar Navigation:
- `/home/cesc/Documents/personal-workspace/teka/apps/web/src/layouts/dashboard-layout.tsx` line 88: `{ label: "Hồ sơ học sinh", to: "/records", Icon: IdCardIcon, perm: "students.list" }`
- `/home/cesc/Documents/personal-workspace/teka/apps/web/src/layouts/dashboard-layout.tsx` line 201: `"Hồ sơ học sinh"` trong `OVERFLOW_LABELS` (mobile menu)
- `/home/cesc/Documents/personal-workspace/teka/apps/web/src/layouts/dashboard-layout.tsx` line 232: `/records` trong `OVERFLOW_PATH_PREFIXES` (mobile route matching)

### Class Info Tab:
- `/home/cesc/Documents/personal-workspace/teka/apps/web/src/features/roster/components/class-info-tab.tsx` line 106: `<ShortcutLink to={`/records?class_id=${klass.id}`}>Hồ sơ học sinh</ShortcutLink>`

### Tests:
- `/home/cesc/Documents/personal-workspace/teka/apps/web/src/layouts/__tests__/dashboard-layout.test.tsx` lines 131, 263, 315, 320, 471 — tests cho sidebar entry
- `/home/cesc/Documents/personal-workspace/teka/apps/web/src/features/roster/__tests__/class-detail-page.test.tsx` line 172 — test cho ShortcutLink

## 3. Existing Redirect Patterns

Ba mẫu hiện tại, có thể tái sử dụng:

1. **Contacts Redirect** (`/home/cesc/Documents/personal-workspace/teka/apps/web/src/features/roster/components/contacts-redirect.tsx`):
   - Pattern: Chuyển hướng `/contacts` → `/students?tab=contacts`, giữ lại query params (line 8)

2. **Class Settings Redirect** (`/home/cesc/Documents/personal-workspace/teka/apps/web/src/features/roster/components/class-settings-redirect.tsx`):
   - Pattern: Chuyển hướng `/classes/:id/settings` → `/classes/:id?edit=1` (line 6)

3. **Billing Index Redirect** (tương tự cho period-scoped routes):
   - Pattern: Navigate + useSearchParams/useParams để giữ context

**Đăng ký trong routes.tsx**: Sử dụng `lazy: async () => ({ Component: ... })`

**Cho /records cần tạo**:
- `/records` → `/students` (không có query; nó là index)
- `/records/:studentId` → `/students/:studentId` (giữ lại path param)

## 4. Sidebar Permission Gate Audit

Sidebar entries có `perm` field được kiểm tra bởi `useCenterContext().has(perm)` (dashboard-layout.tsx line 188). Đối chiếu với API routespec.go:

**✓ MATCHED:**
- "Điểm danh" (perm: `sessions.list`) ← `GET /api/v1/sessions/pending` (line 306: PermSessionsList)
- "Sổ lớp" (perm: `classes.list`) ← `GET /api/v1/classes/:id/marks` (line 327: PermTeachingRead) — **GHI CHÚ:** classbook đọc marks nên cần `teaching.read`, nhưng sidebar chỉ yêu cầu `classes.list`. Cần kiểm tra classbook page.
- "Hồ sơ học sinh" (perm: `students.list`) ← `GET /api/v1/enrollments` (line 296: PermEnrollmentsList) — **GHI CHÚ:** records dùng enrollments.list, nhưng sidebar yêu cầu `students.list`, khác.
- "Học sinh" (perm: `students.list`) ← `POST /api/v1/enrollments` (line 294: PermEnrollmentsCreate) — **GHI CHÚ:** tạo ghi danh cần `enrollments.create` chứ không phải `students.list`.
- "Danh mục lớp" (perm: `classes.list`) ← `GET /api/v1/classes` (line 255: PermClassesList)
- "Lớp cần tuyển sinh" (perm: `classes.list`) ← dùng cùng endpoint như "Danh mục lớp"
- "Lộ trình học" (perm: `paths.read`) ← `GET /api/v1/paths` (line 504: PermPathsRead)
- "Khóa học" (perm: `courses.read`) ← `GET /api/v1/courses` (line 491: PermCoursesRead)
- "Kho học liệu" (perm: `library.read`) ← `GET /api/v1/library/materials` (line 471: PermLibraryRead)
- "Chốt sổ" (perm: `billing.read`) ← `GET /api/v1/billing-periods/:id` (line 341: PermBillingRead)
- "Gửi thông báo" (perm: `reports.send`) ← `POST /api/v1/billing-periods/:id/notifications/bulk` (line 379: KindService, nhưng kiểm tra trong service)
- "Thu tiền" (perm: `billing.read`) ← `GET /api/v1/billing-periods/:id/collections` (line 343: PermBillingRead)
- "Duyệt giáo án" (perm: `teaching.review_queue`) ← `GET /api/v1/teaching/review-queue` (line 335: PermTeachingReviewQueue)
- "Nhật ký hoạt động" (perm: `audit.read`) ← `GET /api/v1/audit-logs` (line 241: PermAuditRead)
- "Công việc" (perm: `tasks.list`) ← `GET /api/v1/tasks/board` (line 395: PermTasksList)
- "Gửi báo cáo" (perm: `reports.send`) ← `GET /api/v1/billing-periods/:id/notifications` (line 381: KindService)

**MISMATCHES (sai lệch permission gates):**
- **"Hồ sơ học sinh"** (perm: `students.list`) nhưng đọc `/api/v1/enrollments` cần `enrollments.list` — **SỬA:** thay `students.list` → `enrollments.list`? Hoặc records page cần kiểm tra cả hai?
- **"Sổ lớp"** (perm: `classes.list`) nhưng classbook page đọc `/api/v1/classes/:id/marks` cần `teaching.read` — cần kiểm tra classbook page đầy đủ.

## 5. Small Bugs - Vị trí chính xác

### a. "Thêm học viên" permission gate
- **File:** `/home/cesc/Documents/personal-workspace/teka/apps/web/src/features/roster/components/class-students-tab.tsx`
- **Lines 40-45:** Link "Thêm học viên" luôn hiển thị (không có permission check)
- **Issue:** Tạo ghi danh cần API permission `enrollments.create`, nhưng link không kiểm tra `useCenterContext().has("enrollments.create")`
- **Fix:** Thêm conditional `canWrite || has("enrollments.create")` hoặc tương tự (xem component đầy đủ để biết `canWrite` từ đâu)

### b. Classbook page link to non-existent route
- **File:** `/home/cesc/Documents/personal-workspace/teka/apps/web/src/features/teaching/pages/classbook-page.tsx`
- **Line 372:** `navigate("/center/classes")` — route này **không tồn tại**
- **Context:** Nút "Tạo lớp" trong trạng thái "Chưa có lớp đang hoạt động"
- **Expected:** Có thể muốn dẫn đến `/center/class-config` (cấu hình lớp owner-only) hoặc `/classes` (danh sách lớp)? Cần xác nhận ý định.

### c. Class sessions tab - hai links với cùng href
- **File:** `/home/cesc/Documents/personal-workspace/teka/apps/web/src/features/roster/components/class-sessions-tab.tsx`
- **Lines 99-110:** 
  - Link 1 (line 99-103): `to={/classbook?class_id=${klass.id}}`, label "Sổ đầu bài"
  - Link 2 (line 105-109): `to={/classbook?class_id=${klass.id}}`, label "Điểm danh & nhận xét →"
- **Issue:** Cả hai cùng href, chỉ label khác → Link 2 redundant
- **Fix:** Link 2 nên dẫn đến `/sessions` hoặc route sessions-related khác?

### d. Stale comment in class-overview-cards
- **File:** `/home/cesc/Documents/personal-workspace/teka/apps/web/src/features/dashboard/components/class-overview-cards.tsx`
- **Line 21:** `/** The roster page behind the `Lớp mới` link is owner-only. */`
- **Fact:** Line 45 thực tế link đến `/students?class_id=...`, **không phải owner-only** (từ sidebar: `perm: "students.list"` dành cho mọi teacher)
- **Fix:** Xoá hoặc cập nhật comment

## 6. Stale Documentation

**Không tìm thấy** mention của `/records`, `/classes/recruiting`, `/center/class-config`, "Gửi báo cáo", hoặc "Lớp cần tuyển sinh" trong `docs/`.

Chỉ tìm thấy trong `docs/frontend-guidelines.md` section **Navigation** (lines 152-173) kể về cấu trúc sidebar, nhưng không liệt kê cụ thể /records — có thể cần cập nhật khi xoá feature nếu docs nhắc đến nó ở đâu khác.

---

## Summary

**Files cần xoá:** records-page.tsx, student-record-page.tsx, records-toolbar.tsx, student-records-table.tsx, 3 test files, records-search.spec.ts

**Inbound links cần xử lý:** 3 chỗ trong dashboard-layout (sidebar + OVERFLOW), 1 chỗ trong class-info-tab

**Redirect pattern:** Tạo RecordsRedirect component, tương tự ContactsRedirect; đăng ký 2 routes `/records` và `/records/:studentId` trong teaching/routes.tsx

**Permission bugs:** Classbook vs students.list mismatch cần xem xét; "Thêm học viên" link cần gate; /center/classes không tồn tại.

**Duplicate links:** class-sessions-tab.tsx cần fix một trong hai links.

**Status: DONE**
Summary: Scouted all records-related files, inbound links, redirect patterns, sidebar permission mismatches, and 4 specific bugs with exact line numbers.