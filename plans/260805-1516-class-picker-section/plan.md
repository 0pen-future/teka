# Section "CHỌN LỚP" theo prototype — Điểm danh + Lớp & học sinh

Status: done · Branch: master · Source: claude.ai/design project `4a7e6c77` (`So Lop - Prototype.dc.html`, spec tại `clsSearch`/`classTabs`)

## Contract

- **Outcome:** Hàng chọn lớp ("CHỌN LỚP") đúng prototype ở 2 màn Điểm danh và Lớp & học sinh: ô search "Tìm lớp…" dạng pill hiện khi >5 lớp, lọc tab lớp theo tên (trim + lowercase substring), note `Không có lớp nào khớp "<query>"` khi không khớp.
- **Constraints:** Giữ tab "Tất cả"/"Chưa ghi danh" và hành vi chọn lớp hiện có; không lọc mất 2 tab cố định này; giữ `role="tab"`/`tablist` aria; token DS 100% (`border-line-200`, `focus:border-mint-400`, 13.5px/700, w-150px; note 13px `ink-400` bold).
- **Non-goals:** Tab lớp màn Thu tiền; share query giữa 2 trang; số học sinh trên tab (user chốt bỏ — cần backend `student_count`, có thể thêm sau như thay đổi additive).
- **Acceptance:** (tất cả đạt)
  1. ≤5 lớp: không có ô "Tìm lớp…"; >5 lớp: có, đứng trước dãy tab. ✓
  2. Lọc chỉ thu hẹp tab lớp thật; "Tất cả"/"Chưa ghi danh" (màn học sinh) luôn còn. ✓
  3. Không khớp → note `Không có lớp nào khớp "<query>"` inline sau tabs. ✓
  4. Xoá query → đủ tab; lớp đang chọn không đổi bởi việc lọc. ✓
  5. Typecheck, lint, unit tests xanh; không đổi public contract (barrel chỉ thêm export). ✓

## Files (thực tế)

- `apps/web/src/features/roster/hooks/use-class-search.ts` (mới) — `useClassSearch`: ngưỡng >5, lọc trim+lowercase, emptyNote; bỏ qua query khi input ẩn (list tụt ≤5 không lọc ngầm). Tách khỏi component vì rule `react-refresh/only-export-components`.
- `apps/web/src/features/roster/components/class-search.tsx` (mới) — `ClassSearchInput` (style prototype, không mang shadow-* nên giữ nguyên ring `:focus-visible` nền + `focus:border-mint-400`) + `ClassSearchEmptyNote`.
- `apps/web/src/features/roster/index.ts` — export 3 API mới qua barrel.
- `apps/web/src/features/roster/pages/students-page.tsx` — tích hợp; fix review H1: `useClassesList` thêm `per_page: 100` (mặc định 20 → search sẽ khẳng định sai "không khớp" với lớp ở trang sau); comment ⚙ cập nhật (check theo full list, không theo pills đã lọc).
- `apps/web/src/features/attendance/pages/sessions-page.tsx` — tích hợp; fix review M2: tablist rời khỏi cây khi filter rỗng (tablist không được phép 0 tab theo ARIA); bỏ nút "Thêm buổi học" (quyết định user cho review M1, xem dưới).
- Gỡ chuỗi tạo buổi thủ công (user chốt khi xử lý M1): xoá `components/create-session-dialog.tsx`, `useCreateSession` (hooks/use-sessions.ts), `createAdHocSession` (api/attendance-api.ts), `createSessionInputSchema`/`CreateSessionInput` + helper `dateField`/`hhmmPattern` (schemas/attendance-schemas.ts), MSW handler POST `/classes/:classId/sessions` (\_\_tests\_\_/attendance-handlers.ts).
- `apps/web/src/features/attendance/__tests__/sessions-page.test.tsx` — +3 test (ẩn ≤5, lọc + giữ selection, empty note).
- `apps/web/src/features/roster/__tests__/students-page.test.tsx` (mới, review M3) — 3 test: ẩn ≤5, lọc giữ 2 tab cố định, empty note.

## Verification

- `tsc -b --noEmit` sạch; eslint sạch (3 warning react-compiler pre-existing ngoài diff); vitest 25 files / 110 tests pass.
- Visual check Playwright (DB dev 9 lớp): search hiện ở cả 2 màn, lọc đúng, empty note đúng, selection giữ nguyên khi lọc. Ảnh trong scratchpad session.
- Code review (`code-reviewer` subagent): DONE_WITH_CONCERNS → đã fix H1 (per_page), M2 (ARIA tablist rỗng), M3 (test students-page), M4 (comment ⚙ stale). Chi tiết: `reports/code-review.md`.
- Low giữ nguyên theo prototype: note nội suy query thô (prototype dùng `S.clsQuery` thô y hệt); không normalize NFC (prototype không normalize; dữ liệu nhập từ form web hiện đại đều NFC).
- M1 (lọc ẩn tab đang chọn nhưng "Thêm buổi học" vẫn tạo buổi cho lớp đó) — user chốt: **bỏ hẳn nút "Thêm buổi học"** để khớp 100% prototype (màn Điểm danh của prototype không có nút này). An toàn chức năng vì backend tự sinh buổi từ lịch lớp khi list (`sessions/generator.go`, doc tại `listClassSessions`). Hệ quả có chủ đích: UI không còn tạo được buổi học bù ad-hoc; endpoint `POST /classes/:id/sessions` phía API vẫn tồn tại nếu cần khôi phục.
- Sau vòng gỡ: tsc sạch, eslint sạch, vitest 25 files / 110 tests pass, visual re-check khớp prototype (h1 → subtitle → search + tabs).
- Follow-up 260805 (2 bước): màn Điểm danh — (1) wrapper `role="tablist"` đổi sang `className="contents"` để từng tab wrap độc lập cùng hàng với search pill + empty note (trước đó cả cụm tab là một flex item, rơi nguyên khối xuống dưới). (2) Header + section "CHỌN LỚP" tách khỏi cột list 360px, đưa lên block full-width phía trên hàng list/panel — đúng cấu trúc prototype (h1 → subtitle → picker full width → hàng flex chứa session card + panel); trong cột 360px tab không bao giờ đủ chỗ một hàng. Block này ẩn dưới `lg` khi panel mở, đồng bộ hành vi responsive của cột list. Hàng lọc ngày "Từ/Đến" cũng chuyển lên block header để hàng dưới chỉ còn card buổi học + panel — hai bên bắt đầu cùng đường ngang (`lg:items-start`) như hàng list/panel của prototype. Verify: 12 tests attendance pass, tsc + eslint sạch. Màn Lớp & học sinh áp dụng cùng pattern `contents` cho tablist (section vốn đã full-width) — 3 tests students-page pass. Bỏ tab "Tất cả" theo yêu cầu user: URL không có `class_id` giờ mặc định chọn lớp đầu tiên (`effectiveClassId`, cùng pattern màn Điểm danh); tab cố định còn lại chỉ "Chưa ghi danh"; pill ⚙ theo `effectiveClassId`. "⚙ Cài đặt lớp" chuyển từ pill Link cuối hàng CHỌN LỚP lên hàng header cạnh "+ Tạo lớp mới"/"+ Thêm học sinh", đổi thành `HvButton variant="secondary" size="sm"` + `useNavigate` (đúng ngữ nghĩa button của prototype; điều kiện hiện theo lớp thật đang chọn giữ nguyên). Full suite 25 files / 112 tests pass.
