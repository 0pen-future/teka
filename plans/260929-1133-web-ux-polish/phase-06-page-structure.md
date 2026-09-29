---
phase: 6
title: "Structure: titles, h1, billing, sidebar, touch targets"
status: completed
priority: P1
effort: "1d"
dependencies: [3]
---

# Phase 6: Page structure

## Goal

Mỗi route có tiêu đề tab riêng và đúng một h1. `/billing/:periodId` không còn
tường nút coral. Sidebar không còn ba nhãn lớp học trùng nghĩa. Ma trận phân
quyền có target ≥ 24px. Gộp S1, M6, S4, S5.

## Files

- Create: `apps/web/src/components/shared/document-title.tsx` (component `RouteDocumentTitle` + type `RouteHandle`)
- Modify: `apps/web/src/layouts/root-layout.tsx` (mount `RouteDocumentTitle`)
- Modify: `apps/web/src/app/router.tsx` và mọi `features/*/routes.tsx` (thêm `handle: { title }`)
- Modify: `apps/web/src/features/billing/pages/billing-review-page.tsx`
- Modify: `apps/web/src/features/billing/components/blocking-sessions-panel.tsx`
- Modify: `apps/web/src/layouts/dashboard-layout.tsx` (nhãn nav + `OVERFLOW_LABELS`)
- Modify: `apps/web/src/layouts/__tests__/dashboard-layout.test.tsx`
- Modify: `apps/web/src/features/center/components/permission-matrix.tsx`
- Modify: các trang thiếu/thừa h1 do `ux-audit` phát hiện
- Tests: `components/shared/__tests__/document-title.test.tsx`, `features/billing/__tests__/billing-review-page.test.tsx`, e2e dùng nhãn sidebar cũ (grep)

## Design

### S1: Tiêu đề tab qua `handle.title`

Dùng `handle` có sẵn của React Router data router thay vì gọi hook trong từng page.
Tiêu đề nằm cạnh khai báo route (một nơi), và route lazy vẫn có tiêu đề ngay trước khi chunk tải xong.

```tsx
export interface RouteHandle { title?: string }

/** Sets document.title from the deepest matched route's `handle.title`. */
export function RouteDocumentTitle() {
  const matches = useMatches();
  const title = [...matches].reverse()
    .map((m) => (m.handle as RouteHandle | undefined)?.title)
    .find(Boolean);
  useEffect(() => {
    document.title = title ? `${title} · Teka` : "Teka";
  }, [title]);
  return null;
}
```

- Tiêu đề tĩnh theo trang, trùng nhãn sidebar sau khi đổi tên ("Chốt sổ", "Điểm danh", "Sổ lớp", …).
- Không đưa tên học sinh, phụ huynh hay số tiền vào title, vì title lộ ra lịch sử trình duyệt và thanh tab khi chia sẻ màn hình.
- 404: "Không tìm thấy trang" (route `*` cũng có handle).

### M6: `/billing/:periodId`

- `<p>Chốt sổ tháng {m}/{y}</p>` → `<h1>` (cùng class hiển thị).
- `BlockingSessionsPanel`: thay danh sách N nút "Điểm danh" bằng:
  - Một câu tóm tắt + **một** CTA chính: "Điểm danh {N} buổi còn thiếu" → `/sessions?pending=1`
    (kiểm tra `sessions-page` đã hỗ trợ lọc pending chưa; nếu chưa thì trỏ `/sessions`, vì trang này đã ưu tiên buổi chờ).
  - Nhóm theo `class_name` bằng `<details>`/`<summary>` (mặc định đóng): mỗi nhóm hiện "{Lớp} — {k} buổi". Bên trong, mỗi buổi là một **text link** (không phải nút coral) tới `/sessions/:id/attendance`.
- Kết quả: ≤ 1 CTA coral trong màn đầu ở 375. Mọi buổi thiếu vẫn tới được sau tối đa 2 lần bấm (mở nhóm → bấm buổi).
- Dùng `<details>` native thay vì Radix Collapsible: không thêm JS, có sẵn bàn phím và a11y.

### S4: Đổi tên sidebar (không gộp route)

| Route | Nhãn cũ | Nhãn mới (đề xuất) | Lý do |
|---|---|---|---|
| `/classbook` | Quản lý lớp học | **Sổ lớp** | Trang làm việc hằng ngày (điểm, buổi, học sinh của lớp mình dạy) |
| `/classes` | Danh sách lớp học | **Danh mục lớp** | Danh sách/tạo lớp ở nhóm Giảng dạy |
| `/students` | Lớp & học sinh | **Quản trị học sinh** | Owner-only, quản trị dữ liệu trung tâm |

Trước khi chốt nhãn, đọc 3 page (`classbook-page.tsx`, `roster` classes page, `students-page.tsx`) để nhãn khớp nội dung thật. Nếu nội dung khác bảng trên, chỉnh nhãn và ghi lý do vào phase report.
Cập nhật `OVERFLOW_LABELS`, text dòng 602 (`dashboard-layout.tsx`), test layout và mọi e2e `getByRole("link", { name: ... })` dùng nhãn cũ.
**Không tách nhóm "Trung tâm"** (người dùng chốt 2026-09-29): giữ nguyên nhóm và thứ tự hiện tại, chỉ đổi 3 nhãn trên. Không đổi route hay quyền.

### S5: Touch target ma trận phân quyền

Checkbox `size-4` (16px) → bọc trong `<label>` phủ kín ô `td` với `min-h-11 min-w-11 flex items-center justify-center cursor-pointer`, nên vùng bấm thành 44×44 và checkbox hiển thị `size-5`.
Giữ `aria-label` hiện có trên input. Ma trận nằm trong `HvTableScroll` (phase 4), nên cột rộng hơn không làm tràn trang.

## Steps

1. `RouteDocumentTitle` + test (title theo match sâu nhất, fallback "Teka").
2. Thêm `handle.title` cho mọi route (auth, invitation, dashboard children, `*`).
3. Billing h1 + panel mới + test (1 CTA, nhóm theo lớp, link từng buổi).
4. Đổi nhãn sidebar + test.
5. Permission matrix target.
6. Chạy `ux-audit` → sửa các trang còn 0 hoặc >1 h1 (thường do `PageHeader`/`HvCard` title lồng nhau); h1 của layout (nếu có) phải bị hạ cấp.

## Verification

- `npx playwright test ux-audit -g "title|h1|target-size"` pass trên mọi route; test "unique titles" pass.
- `npx vitest run src/layouts src/features/billing src/components/shared` xanh.
- `/billing/:periodId` với seed có buổi thiếu, ở 375: đếm phần tử `.bg-coral-400` trong viewport đầu ≤ 1.
- `npm run lint && npm run typecheck && npm test`.

## Risks

- Đổi nhãn sidebar làm fail e2e hiện có: grep `apps/web/e2e` cho 3 nhãn cũ và sửa cùng commit.
- `handle.title` bị quên ở route mới trong tương lai: test "unique titles" trong `ux-audit` và quy tắc ở guidelines (phase 7) bắt được.

## Completion notes

- Billing CTA: nút duy nhất dẫn tới trang điểm danh của buổi quá hạn lâu nhất,
  không phải `/sessions`, vì trang buổi học không có bộ lọc "còn thiếu" và chỉ
  xem theo từng lớp. Danh sách theo lớp nằm trong `<details>` đóng mặc định.
- `ux-audit` round cuối lộ thêm vi phạm contrast ngoài phạm vi phase 3: chữ nhóm
  sidebar (`ink-300`), `sun-600` trên `sun-100` (3.21), `ink-500` trên
  `cream-200` (4.34), `sky-500` trên `sky-50` (4.30), nút huỷ lời mời
  `coral-500`; sau khi suite khác tạo dữ liệu còn lộ avatar giao việc (chữ trắng), số đếm chip lọc `opacity-75` và ô "Chưa có buổi" (`ink-300`). Sửa tại token: `--sun-600` #8f6000, `--ink-500` #536c63,
  `--sky-500` #29739a; nhóm sidebar dùng `ink-500`; nút huỷ dùng `coral-600`.
  `contrast.test.ts` khoá thêm các cặp này.
  Avatar dùng nền pastel thương hiệu + `text-on-brand`; số đếm bỏ opacity;
  ô trống dùng `ink-500`.
