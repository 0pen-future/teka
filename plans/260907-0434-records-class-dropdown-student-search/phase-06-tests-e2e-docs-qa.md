---
phase: 6
title: "Test tích hợp, e2e, docs, QA thị giác"
status: completed
priority: P1
effort: "4h"
dependencies: [1, 2, 3, 4, 5]
---

# Phase 6: Test tích hợp, e2e, docs, QA thị giác

## Overview

Chốt chất lượng toàn luồng: test tích hợp trang, cập nhật 2 spec e2e đang click `role="tab"` trên `/records`, thêm spec mobile 375px, cập nhật docs tối thiểu, chạy toàn bộ gate và so khớp thị giác với mockup ở 5 trạng thái.

## Requirements

- Functional (test tích hợp `records-pages.test.tsx`, chỉ **thêm** case):
  - Seed thêm lớp thứ 2 (`getRosterStore().classes.push(...)`, kèm 1 ghi danh) trong case cần đổi lớp; gõ `q` trước, mở trigger "Lớp …", chọn option → URL `class_id` đổi, `q` **biến mất** khỏi URL, ô tìm trống và bộ đếm về `N / N` (D2).
  - Gõ "nguyen" → hàng "Nguyễn Văn An" còn, `mark` bọc "Nguyễn", bộ đếm `1 / N học sinh`, URL `?q=nguyen`.
  - Vào thẳng `/records?q=nguyen` → ô tìm có sẵn giá trị và bảng đã lọc.
  - Gõ "Trương" → trạng thái rỗng, bấm "Xoá tìm kiếm" → bảng đủ, ô tìm trống, URL không còn `q`.
  - Phím `/` từ body focus ô tìm; khi đang focus ô tìm gõ "/" không bị chặn.
  - `mockViewport(375)`: toolbar có select full-width, không có cột "NGÀY SINH", có nút "CSV" dưới bảng xuất cùng file `HocSinh_Toán_6A.csv`.
- e2e (Playwright, stack e2e cách ly `compose -p teka-e2e`, xem memory `teka-e2e-isolated-stack`):
  - `class-staff-read.spec.ts` dòng 38–41 và `class-staff-write.spec.ts` `classIdFromRecordsTab` (dòng 33–40): thay `getByRole("tab", { name })` bằng `getByRole("button", { name: /^Lớp/ }).click()` → `getByRole("option", { name: new RegExp(className) }).click()`; assertion URL/visible giữ nguyên. Đổi tên helper `classIdFromRecordsTab` → `classIdFromRecordsPicker` cho đúng nghĩa (chỉ nội bộ spec).
  - Spec mới `e2e/records-search.spec.ts` với `test.use({ viewport: { width: 375, height: 812 } })`, đăng nhập `HOC_VU` (Cô Thu, chỉ đọc, đã có trong spec read): mở `/records`, bấm trigger → `role="dialog"` "Chọn lớp" → chọn `STAFF_CLASS`; gõ "an" → thấy "Bé An", không thấy "Bé Bình" (xác nhận tên seed trong `apps/api` seeder trước khi viết); gõ "zzz" → thấy `Không tìm thấy học sinh nào khớp “zzz”` → "Xoá tìm kiếm"; kiểm tra `document.documentElement.scrollWidth <= 375` (không cuộn ngang); nút "Xem" mở trang chi tiết.
- Docs: đọc `docs/frontend-guidelines.md`; nếu có mục liệt kê hook/util dùng chung thì thêm 1 dòng cho `useMediaQuery` và `foldVietnamese`/`findFoldedMatch`; nếu không có mục sở hữu, **không** tạo tài liệu mới. `docs/api-guidelines.md`/`architecture.md` không đổi (trường additive, không đổi contract).
- QA gate: `npm run lint && npm run typecheck && npm run test && npm run build` trong `apps/web`; `go build ./... && go test ./...` + scopelint trong `apps/api` (nếu Phase 2); e2e 3 spec liên quan.
- QA thị giác: script Playwright trong scratchpad chụp `/records` ở 1280×800 và 375×812 cho 5 trạng thái (mặc định, dropdown mở, gõ "ng", "Trương", đang tải — throttle network) và so cạnh mockup (mở brief, bấm cùng trạng thái); ghi chênh lệch (nếu có) vào report `plans/reports/qa-260907-HHmm-records-class-dropdown.md`. Chụp thêm `/sessions`, `/students`, `/classbook` trước/sau để chứng minh không đổi.

## Architecture

Không có kiến trúc mới; phase này chỉ tiêu thụ helper `mockViewport` (Phase 3) và fixtures roster hiện có.

## Related Code Files

- Modify: `apps/web/src/features/teaching/__tests__/records-pages.test.tsx`
- Modify: `apps/web/e2e/class-staff-read.spec.ts`, `apps/web/e2e/class-staff-write.spec.ts`
- Create: `apps/web/e2e/records-search.spec.ts`
- Modify (có điều kiện): `docs/frontend-guidelines.md`
- Create: `plans/reports/qa-260907-HHmm-records-class-dropdown.md` (report QA, đường dẫn theo quy ước reports)
- Read only: `apps/api/cmd/**/seed*` hoặc nơi seeder tạo "Toán 8 - Tối Thứ Ba" (tên học sinh seed cho e2e), `apps/web/playwright.config.ts`, `apps/web/src/test/utils.tsx`

## Implementation Steps

1. Thêm case vào `records-pages.test.tsx` (không đổi 3 case cũ); chạy `npx vitest run src/features/teaching`.
2. Cập nhật 2 spec e2e hiện có; viết `records-search.spec.ts`; dựng stack e2e cách ly và chạy `npx playwright test class-staff-read class-staff-write records-search`.
3. Docs theo điều kiện trên; kiểm tra link.
4. Chạy toàn bộ gate web + api; sửa regression thay vì nới test.
5. QA thị giác + report; nếu lệch mockup, quay lại phase sở hữu (3/4/5) sửa rồi chụp lại.
6. Commit theo conventional commits, mỗi phase một commit, không tham chiếu AI (memory `teka-no-ai-refs-in-commits`); push cần người dùng duyệt (memory `teka-github-auth-split`).

## Todo

- [x] Test tích hợp bổ sung xanh, case cũ nguyên vẹn
- [x] 2 spec e2e cập nhật + spec mobile mới xanh trên stack cách ly
- [x] Docs (có điều kiện) cập nhật, link kiểm tra
- [x] Gate web/api xanh
- [x] Report QA thị giác 5 trạng thái × 2 viewport + 3 trang không đổi

## Success Criteria

- [x] `npm run test` xanh, không assertion cũ nào bị sửa; coverage cho file mới ≥ mức hiện tại của feature teaching.
- [x] 3 spec e2e xanh; không còn `getByRole("tab")` nào trỏ tới `/records`.
- [x] Report QA liệt kê từng mục Success Criteria của `plan.md` với bằng chứng (ảnh/lệnh) và không có mục "lệch mockup" chưa xử lý.
- [x] Ảnh chụp `/sessions`, `/students`, `/classbook` trước/sau trùng nhau.

## Risk Assessment

- **Tên học sinh seed e2e khác giả định** ("Bé An"/"Bé Bình" lấy từ spec read hiện có) → đọc seeder trước khi viết assertion; tín hiệu vỡ: spec đỏ ở `toBeVisible` → sửa fixture tên, không sửa logic.
- **Stack e2e đang chạy dở** từ phiên trước chiếm port → theo `process-management.md`: kiểm tra `docker compose -p teka-e2e ps`, tái dùng hoặc dừng, không tăng port.
- **Playwright chưa cài browser** trên máy → `npx --no-install playwright --version` và `~/.cache/ms-playwright` đã xác nhận ở phiên brainstorm; nếu thiếu, `npx playwright install chromium`.

## Kết quả

Commit `6776e0a`. 7 case tích hợp mới (kể cả re-pick cùng lớp), 3 spec e2e xanh trên `docker compose -p teka-e2e` (đã `down -v`). Assertion classbook trong `class-staff-read` đổi từ `tab` sang combobox vì classbook đã đổi selector ở `05d3f50` (vốn đỏ trên master). Docs: 2 dòng trong `docs/frontend-guidelines.md` (lib/hooks, `mockViewport`). QA: `plans/reports/qa-260907-1425-records-class-dropdown.md` — ảnh 3 trang không đụng chỉ có bản "sau", bằng chứng không đổi là diff rỗng trên các file đó. Tester: `plans/reports/tester-260907-1428-records-class-dropdown.md` (185 test teaching+lib pass, file mới 94–100% statements, ngang/bằng mức thư mục teaching 91–100%).
