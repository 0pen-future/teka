# Code review — restyle "Lớp & học sinh" + "Điểm danh"

Reviewer: `code-reviewer` subagent · Scope: 6 file UI trong `apps/web` (~252+/175−), không đổi backend/schema/route/props export. Đã chạy lint (sạch), `tsc -b --noEmit` (sạch), unit suites attendance (7/7 pass), đối chiếu mọi token với `@theme inline` trong `globals.css`.

Kết luận: **DONE_WITH_CONCERNS** — restyle đúng chức năng, an toàn contract; lỗi thật nằm ở accessibility.

## Findings và xử lý

### High

- **H1 — Mất focus ring bàn phím trên tab pill lớp** (`sessions-page.tsx`, và pre-existing ở `students-page.tsx`). Ring toàn cục khai báo qua `:focus-visible { outline:none; box-shadow: var(--ring) }` ở `@layer base`; utility `shadow-soft-sm`/`shadow-press-mint` nằm ở layer `utilities` nên đè mất `box-shadow` → người dùng bàn phím không còn chỉ báo focus. `hv-button.tsx` đã né đúng bẫy này bằng `focus-visible:outline-none focus-visible:ring-4`.
  → **ĐÃ FIX**: thêm `focus-visible:outline-none focus-visible:ring-4` vào tab pill của cả 2 màn.

- **H2 — Tương phản chữ dưới WCAG AA (4.5:1)**: title hàng buổi học tint theo trạng thái (`ink-400` ~3.01:1 cho "Sắp diễn ra"/huỷ, `mint-600` ~4.08:1 cho đã điểm danh); SĐT `ink-400` 12.5px (~3.01:1); header bảng `ink-500` trên `cream-200` (~4.34:1); nhãn "Có mặt"/"Vắng" `ink-400` trên `cream-50` (~2.95:1).
  → **GIỮ NGUYÊN, chờ user quyết**: đối chiếu source prototype xác nhận tint cả hàng (`color:col` trên row div), SĐT `ink-400`, header `ink-500`/`cream-200` đều là spec prototype mà user đã chốt "100% design system". Đổi màu = lệch prototype → trình user lựa chọn thay vì tự đảo quyết định.

### Medium

- **M1 — Offset viewport hard-code** (`sm:h-[calc(100svh-158px)]`…) lặp lại số đo chrome của `DashboardLayout`, sẽ trôi nếu layout đổi. → **Ghi comment cảnh báo** ngay tại chỗ (refactor CSS-var đụng layout, ngoài scope).
- **M2 — Card bảng có thể bị bóp về ~0** khi hàng tab pill wrap nhiều dòng trên viewport thấp (card là flex child `min-h-0` duy nhất). → **ĐÃ FIX**: `min-h-0` → `min-h-[240px]`.
- **M3 — Cuộn lồng trên mobile**: `max-h-[430px] overflow-auto` tạo scroller trong document đang cuộn dưới `lg`. → **ĐÃ FIX**: gate sau `lg:` (`lg:max-h-[430px] lg:overflow-auto`); mobile giữ 1 trục cuộn + confirm bar sticky.
- **M4 — Header panel mint bị copy 2 lần** (nhánh live vs nhánh huỷ). → **ĐÃ FIX**: hoist thành `PanelHeader({ session, subtitle })` dùng chung.
- **M5 — Card tự chế thay vì `HvCard`** (radius 20/28 vs `--radius-xl`). → **Chấp nhận**: radius prototype khác token HvCard; đổi HvCard API ngoài scope, ghi nhận nợ nhỏ.

### Low

- Ô search thiếu accessible label → **ĐÃ FIX**: thêm `aria-label`.
- Điều kiện `unconfirmedPast.length > 0 || remainingGroups.length > 0` ≡ `allSessions.length > 0` → **ĐÃ FIX**: đơn giản hoá.
- `text-ink-600` (attendance-page dialog, billing panel) không phải token định nghĩa — pre-existing, ngoài scope.
- ARIA tabs pattern thiếu `aria-controls`/roving tabindex — pre-existing cả 2 màn, ngoài scope.

## Edge cases đã soát

- Sticky + `border-collapse` an toàn vì head cell không mang border (border nằm ở `td` dạng `border-t`).
- `min-w-[640px]` của bảng được nuốt bởi `overflow-auto` trong card `overflow-hidden` → không có h-scroll cấp trang.
- Nhãn `aria-hidden` "Vắng"/"Có mặt" giữ accessible name = tên học sinh → selector `name: /Học sinh 1$/` và nút confirm `/vắng/` không bị nhiễu (nếu bỏ aria-hidden, 30 hàng sẽ match `/vắng/`).
- e2e selectors (`button[aria-pressed]`, link "Sắp diễn ra", "Buổi học đã huỷ", "Không tính tiền cho học sinh nào.", link "Điểm danh" trong billing spec) đều còn resolve; h1 "Điểm danh" là heading nên không đụng strict-mode với nav link.

## Acceptance criteria

1. Sticky header + thân cuộn + viewport-fit — **đạt** (kèm fix M2). Mobile stacked cards nguyên vẹn.
2. Chức năng Điểm danh nguyên vẹn — **đạt**.
3. Public contracts — **đạt** (`statusText` đổi kiểu trả về nhưng là hàm module-private).
4. Test contracts — **đạt** (grep + chạy suite).
5. Lint/type — **đạt**.

## Sau vòng fix

`tsc` sạch, eslint sạch, vitest 24 files / 104 tests pass, visual re-check không đổi ngoài ý muốn.

## Câu hỏi mở

1. Offset 158/94/102 đo thực nghiệm (dư ~2.5px so với số học) — chấp nhận, có comment.
2. H2: giữ màu prototype hay nâng tương phản — chờ user (xem plan.md).
