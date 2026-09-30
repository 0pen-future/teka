# Brainstorm (--ultra): Option A — trang "Học sinh" hợp nhất

Ngày: 2026-09-29 (Asia/Saigon). Người thắng được giữ nguyên văn bên dưới, không trộn với ứng viên khác.

## Brainstorm contract — Option A "Học sinh" page

### Evidence corrections
1. **No "Lớp học" sidebar group exists today.** Groups: Dạy học, Giảng dạy, Kho học liệu, Học phí, Trung tâm (apps/web/src/layouts/dashboard-layout.tsx:79-193). "Phụ huynh" is under Dạy học (:91), not Trung tâm. Option A must add a group or rename Giảng dạy — still an open decision.
2. **"API already member-safe" holds for reads only — teachers cannot create students.** Contact create/update/delete are owner-only in the service regardless of permission key: `if !sc.IsOwner { Forbidden }` (apps/api/internal/features/contacts/service.go:56-61, 90, 113). Student create requires `contact_id` (students/dto.go:15), which must be visible via `readNarrowContacts` — `teacher_id = caller` unless contacts.view_all (students/repository.go:245-265) — and a composite FK then rejects a foreign contact (students/service.go:59-63). Contacts are always owner-anchored, so a teacher cannot attach a new student to any contact. The "+ Thêm học sinh" wizard would fail for teachers even though they hold students.create.
3. **Student writes are own-rows only.** `writeScoped` restricts to `teacher_id = caller` (students/repository.go:96-108). StudentResponse exposes no `teacher_id`/`can_edit` (dto.go:31-39), so the UI cannot tell which rows a teacher may edit.
4. **Teachers already hold students.create/edit/delete and contacts.create/edit/delete by default** (DefaultGrant baseline, migrations/000018…up.sql:61-65, 134-138; catalog.go:167, 403-406); students.delete is RiskHigh (catalog.go:213). imports.run and *.view_all are not baseline. Gating buttons on `has("students.create")` alone would show buttons that 403/422.
5. **Contacts list has no children names and no debt** — ContactResponse carries only `student_count` (contacts/dto.go:33-39). Debt is billing data (billing.read / statements.view_all); showing it in a contacts tab to teachers would leak data they cannot see today.
6. **Confirmed:** students-page owner-gated (students-page.tsx:63-69); tabs Lớp học / Học sinh / Chưa ghi danh (:25-29); list endpoint supports `query`, `class_id`, `unenrolled` (students/handler.go:110-137); read scoping creator OR enrolled in caller's class (repository.go:113-122); contacts-page and student-detail-page hide writes with `isOwner` (contacts-page.tsx:72; student-detail-page.tsx:65).

### Outcome
One "Học sinh" page usable by owners and teachers, with Tất cả / Theo lớp / Chưa vào lớp / Người liên hệ tabs, replacing the "Quản trị học sinh", "Phụ huynh" and "Nhập từ Excel" sidebar entries, where every write action appears only when the API would actually accept it.

### Constraints
- Nghị định 13/2023 minimal data (docs/prd.md R1): no new student fields.
- hv kit; one primary action per screen.
- Keep old links: `?tab=` / `?class_id=none` (students-page.tsx:34-50), /contacts, /contacts/:id, /students/import.
- No push to master without approval; no AI references in commits.

### Non-goals
Option B, Option C, changing default role grants, debt column (see correction 5).

### Acceptance criteria
1. Member with students.list sees "Học sinh" in the sidebar and opens /students; without students.list sees neither.
2. Teacher sees only API-scoped students; counts on "Tất cả"/"Chưa vào lớp" equal the API `total`.
3. "+ Thêm học sinh", Sửa, Xoá appear only when the API would accept them — with the recommended path, owner only; otherwise an HvNotice pointing to Hệ thống → Phân quyền vai trò.
4. "Nhập từ Excel" button only with imports.run; /students/import still renders.
5. Người liên hệ tab shows name, phone (masked per API), number of children; /contacts redirects to `/students?tab=contacts`.
6. Sidebar no longer contains the three old entries; OVERFLOW_LABELS and dashboard-layout.test.tsx updated.
7. Vitest passes for teacher and owner fixtures; `make test-api` passes if backend is touched.

### Trade-offs
**A1: frontend-only, teacher read-only (recommended).** Load-bearing assumption: teachers mainly need to look up students, not register them. First failure: a center expects teachers to register walk-ins. Worst case: a follow-up request for create access.

**A2: A1 + backend member create via the owner anchor** (reuse roster-import CreateAnchored / OwnerAnchor, contacts/service.go:64-66; teacher with students.create creates contact+student on the owner's behalf; add `can_edit` to StudentResponse). Assumption: owners accept teachers writing to the shared contact book. First failure: every default teacher instantly gains contact-book write because students.create is baseline. Worst case: a silent privilege widening across all existing centers plus RBAC/phone-privacy integration test churn.

**A3: A1 + children chips and debt on contacts.** Needs new contact DTO fields and a billing join. Worst case: teachers see billing data they cannot see today (privacy regression).

Worst cases compared: A1 = missing capability; A2/A3 = security/privacy. A1 first.

**Better approaches:** none that replaces Option A — evidence (corrections 2-5) only changes the first delivery to A1 (teachers can't create yet; no debt column).

### Recommended direction — A1, phased
1. **Frontend page:** rework students-page.tsx — remove owner redirect, gate on `has("students.list")`; tabs all/by-class/unenrolled/contacts, keep legacy `tab` values mapped; pull contacts-page list into a component reused in the tab; `canManageStudents = isOwner` for now with one comment stating the API constraint; HvNotice for others. "Ghi danh vào lớp" also requires `canWriteClass` (lib/class-permissions.ts:10-12).
2. **Routes:** features/roster/routes.tsx — /contacts → redirect `/students?tab=contacts`; keep /contacts/:id, /students/import.
3. **Sidebar:** dashboard-layout.tsx — add/rename group, "Học sinh" (perm students.list), remove three entries, update OVERFLOW_LABELS.
4. **Tests:** layouts/__tests__/dashboard-layout.test.tsx; new roster page tests for owner/teacher.
5. **Backend/migration:** none for A1.
6. **Docs:** only if docs/ describes this navigation (check docs/frontend-guidelines.md, user guide if any).

### Unknowns — questions (recommended default)
1. **Teachers add/edit students?** Today the API effectively blocks it (corrections 2-3). [View-only now; plan A2 as a separate change if wanted.]
2. **Debt in "Người liên hệ"?** [No; show child count only, add chips later if desired.]
3. **"Lớp học" group source?** [Rename "Giảng dạy" → "Lớp học", Học sinh first.]
4. **Hide "Hệ thống" from non-owners?** [Yes when all entries are owner-only; notice link text should target owners only or say "liên hệ chủ trung tâm" to teachers.]
5. **Ship with the sidebar split/renames?** [Separate commits, same branch; sidebar split first since both touch dashboard-layout.tsx.]


---

## Phụ lục xếp hạng (verifier: kongming, thang 1-20/tiêu chí)

| Ứng viên | Bám yêu cầu | Bằng chứng | Tiêu chí nghiệm thu | Trung thực về ẩn số | Trường điều kiện | Tổng |
|---|---|---|---|---|---|---|
| **B (thắng)** | 16 | 18 | 15 | 17 | 17 | **83** |
| D | 10 | 11 | 15 | 12 | 15 | 63 |
| E | 9 | 11 | 14 | 12 | 15 | 61 |
| A | 8 | 5 | 14 | 10 | 15 | 52 |
| C | 8 | 6 | 14 | 10 | 14 | 52 |

Lý do: B là ứng viên duy nhất đọc đúng rằng `contacts/service.go:58,90,113` từ chối non-owner khi tạo/sửa/xoá người liên hệ, và `students/repository.go:240-263` (`ContactExists`) chặn giáo viên gắn học sinh vào người liên hệ của chủ trung tâm. A, C, D, E đều đề xuất hiện nút "+ Thêm học sinh"/Sửa cho giáo viên theo quyền key — API sẽ trả 403/422 (vi phạm ràng buộc cứng "UI không được hiện hành động API từ chối"). C còn đề xuất gate nợ theo `billing.read`, nhưng `billing.read` nằm trong grant mặc định (`000018:84`) nên nợ sẽ hiện cho mọi giáo viên.

Tiêu chí quyết định: Bằng chứng (+7 so với D) và Bám yêu cầu (+6). B hoà D ở tiêu chí nghiệm thu.

### Lưu ý của verifier về bản thắng (không sửa bản thắng)
1. Chốt chặn thực tế là `ContactExists`/`readNarrowContacts`, không phải FK (FK là `(contact_id, center_id)`, `000007:195,226`). Hệ quả: nếu chủ trung tâm cấp `contacts.view_all` cho một vai trò, giáo viên đó tạo được học sinh ngay mà không cần sửa backend — nhưng đổi lại thấy toàn bộ danh bạ và số điện thoại (rủi ro Nghị định 13).
2. Dưới A1, thông báo trỏ "Hệ thống → Phân quyền vai trò" gây hiểu lầm (cấp `students.create` không mở được gì). Nên chốt copy "Liên hệ chủ trung tâm" trước khi làm.
3. `contact-picker.tsx:20,38` có nút inline "Tạo người liên hệ" — trả 403 với giáo viên nếu wizard từng hiện cho họ.
4. Tiêu chí nghiệm thu thiếu `/students/:id` trong danh sách URL cũ và thiếu `?q=` cho ô tìm kiếm.
5. `canManageStudents = isOwner` đi ngược tinh thần "gate theo key" nhưng phản ánh đúng API hôm nay: đợt 1 là "giáo viên xem được", chưa phải "giáo viên quản lý được".

ultra: picked=4/5 margin=high unanimous=no rejected_all=no

---

## Quyết định của người dùng (sau brainstorm)

- **2026-09-29 — Giáo viên không được tương tác với "Người liên hệ".** Tab "Người liên hệ", trang `/contacts`, `/contacts/:id` và mọi hành động tạo/sửa/xoá/liên kết Zalo người liên hệ chỉ dành cho chủ trung tâm. Điều này khớp với API hiện tại (`contacts/service.go:58,90,113` từ chối non-owner), nên không cần thay đổi backend.
  - Hệ quả: vì học sinh bắt buộc gắn người liên hệ, giáo viên cũng không thêm được học sinh → đợt 1 giữ hướng A1 của bản thắng: **giáo viên chỉ xem** danh sách học sinh trong phạm vi của mình (Tất cả / Theo lớp / Chưa vào lớp), không thấy "+ Thêm học sinh", Sửa, Xoá, Ẩn danh, "Nhập từ Excel". Việc giáo viên có được "Ghi danh vào lớp" (không đụng tới người liên hệ) còn chờ quyết định.
  - Thông báo cho giáo viên dùng copy "Liên hệ chủ trung tâm để thêm hoặc sửa học sinh." thay cho pointer "Hệ thống → Phân quyền vai trò".
  - Nút inline "Tạo người liên hệ" trong `contact-picker.tsx` không bao giờ hiện cho giáo viên (wizard đã ẩn).
  - Câu hỏi A2 (giáo viên tạo học sinh qua owner anchor) và cấp `contacts.view_all` cho giáo viên: đóng, không làm.
- **2026-09-29 — Giáo viên không "Ghi danh vào lớp".** Ẩn hành động này với giáo viên; đợt 1 giáo viên chỉ xem học sinh.
- **2026-09-29 — Giáo viên không thấy và không tương tác với SĐT phụ huynh, trên toàn hệ thống.** Sửa quy tắc chung `authctx.Scope.PhoneVisible` (`apps/api/internal/shared/authctx/authctx.go:74`) để chỉ chủ trung tâm hoặc vai trò được cấp `contacts.view_all` (kể cả qua `reports.send` implied) mới thấy SĐT; bỏ nhánh `rowVisible` (học vụ `hoc_vu` của lớp). Ảnh hưởng mọi nơi dùng quy tắc này: `students/service.go:108`, `collections/service.go:52`, `statements/service.go:344`, `notifications/dto.go:164`; các fragment `classscope.PhoneVisibleViaStudent/ViaContact` trở thành thừa. Test phone-privacy hiện có phải cập nhật theo quy tắc mới (đây là thay đổi hành vi có chủ đích, không phải làm yếu test). Giao diện bỏ mọi link `tel:`/nút gọi khi SĐT bị ẩn.
- **2026-09-29 — Có cột công nợ trong tab Người liên hệ, uỷ quyền được qua Phân quyền vai trò, dùng quyền có sẵn.**
  - Tab "Người liên hệ" (chỉ xem, kèm SĐT) hiện cho chủ trung tâm và vai trò có `contacts.view_all`.
  - Cột công nợ hiện cho chủ trung tâm và vai trò có `billing.view_all` (không dùng `billing.read` vì key này có trong grant mặc định của giáo viên). Cần một nguồn dữ liệu backend mới: số nợ theo người liên hệ, gate bằng `CenterWideFor(billing.view_all)`.
  - Tạo/sửa/xoá/liên kết Zalo người liên hệ vẫn chỉ chủ trung tâm (`contacts/service.go`). Không thêm quyền mới, không migration quyền.
- **2026-09-29 — Nhóm "Giảng dạy" đổi tên thành "Lớp học"**, "Học sinh" đứng đầu nhóm.
- **2026-09-29 — Ẩn nhóm "Hệ thống" khi mọi mục trong nhóm bị lọc** (generic empty-group filter).

- **2026-09-29 — Công nợ tách theo tháng.** Tab Người liên hệ có bộ chọn tháng (kỳ thu tiền); cột công nợ là số còn nợ của từng gia đình trong tháng đã chọn. Kỳ thu tiền vốn theo tháng (`billing` Period: `year`, `month`), và API đã có số dư theo gia đình mỗi kỳ: `GET /api/v1/billing-periods/:id/collections?view=contact` → `ContactBalanceRow.outstanding` (`apps/api/internal/features/collections/dto.go:8-25`). Bước plan cần xác nhận có thể tái dùng endpoint này (route gate `billing.read`, `routespec.go:343`; phạm vi dòng theo `billing.view_all`) thay vì viết endpoint mới.
- **2026-09-29 — Mọi luồng dùng SĐT (hiển thị, gọi, gửi) chỉ mở qua quyền được cấp.** Chủ trung tâm cấp `contacts.view_all` trong Phân quyền vai trò cho bất kỳ vai trò nào — học vụ (`hoc_vu`), trợ giảng (`tro_giang`), kể cả giáo viên (`giao_vien`). Không còn quyền SĐT ngầm theo phân công học vụ của lớp. Bước plan phải liệt kê mọi luồng đang dựa vào nhánh `hoc_vu` (Thu tiền, Sao kê, Thông báo/Zalo) và chuyển chúng sang cùng quy tắc.
