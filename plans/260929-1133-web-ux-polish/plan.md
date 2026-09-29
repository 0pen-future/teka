---
title: "Web UX polish — responsive, contrast, crawl blocking, content, structure"
description: "Đưa apps/web từ 'chưa đạt' lên 'polished' theo report enhance-ux-ax-260929-1051-web: hết tràn ngang, mọi nút đạt WCAG AA, chặn index toàn app, bỏ nội dung giả, mỗi route có h1 và tiêu đề tab riêng."
status: pending
priority: P1
effort: 5.25d
branch: feat/web-ux-polish
tags: [frontend, infra, docs]
blockedBy: []
blocks: []
created: 2026-09-29
source: plans/reports/enhance-ux-ax-260929-1051-web.md
---

# Web UX polish

## Outcome

`apps/web` đạt toàn bộ "Project DONE contract" của report
[enhance-ux-ax-260929-1051-web](../reports/enhance-ux-ax-260929-1051-web.md),
với các quyết định sản phẩm đã chốt ở bảng dưới. Mọi check được mã hoá thành
test chạy lại được (e2e `ux-audit` + unit test contrast), không chỉ đo tay một lần.

## Quyết định đã chốt (2026-09-29)

| Câu hỏi trong report | Quyết định |
|---|---|
| Màu nút | Giữ nền thương hiệu, **chữ `ink-900`** trên mọi nền pastel (mint/sky/coral/sun). Ghi quy tắc vào design system trong `docs/frontend-guidelines.md`. |
| Trang sao kê phụ huynh `/s/:token` | **Gỡ khỏi `apps/web`** (feature `statement`, `PublicLayout`, `AppFooter`, public client, e2e). **API giữ nguyên**: link `/s/{token}` đã/sẽ gửi đi sẽ mở trang 404 — đã chấp nhận. |
| Sidebar IA | **Đổi tên cho rõ, không gộp route.** |
| Link preview (S2) | Meta description + OG đầy đủ; **ảnh OG 1200×630 tạo từ `public/app-icon-1024.svg`**, `og:image` tuyệt đối qua build-arg mới `VITE_PUBLIC_ORIGIN`. |
| Nhóm sidebar "Trung tâm" (9 mục) | **Không tách nhóm.** Chỉ đổi tên 3 nhãn lớp học. |
| `DESIGN.md` gốc / `REVIEW.md` (S7) | Không tạo file gốc mới. Quy tắc thiết kế và checklist review UX đặt trong `docs/frontend-guidelines.md`, link từ `apps/web/AGENTS.md`/`CLAUDE.md`. |

## Phát hiện thêm khi scout (so với report)

- Không chỉ primary: `secondary` (trắng/sky-300 **1.85:1**), `danger`
  (trắng/coral-400 **2.55:1**) và `reward` (sun-600/sun-400 **2.27:1**) đều trượt AA.
  Chữ `ink-900` đạt: mint 6.10, sky 6.67, coral 4.84, sun 7.99.
  (Report ghi ink-900/mint-400 = 6.96; đo lại là **6.10**, vẫn đạt.)
- Có **7 bản sao tay** style `HvButton` cho `<Link>` (billing, dashboard alert,
  sessions, students, teaching). Chúng phải đổi cùng lúc, nên cần export class builder dùng chung.
- ~19 chỗ `bg-mint-400 text-white` ngoài `HvButton` (segmented active, period
  switcher, attendance header, chips…).
- `text-mint-600` (142 chỗ) chỉ đạt 4.08 trên trắng và 3.80 trên cream. Đổi
  giá trị token `--mint-600` thành `#24775c` (5.06 trên cream-100, 5.43 trên trắng) sửa được tất cả ở một nơi.
- `PublicLayout` và `AppFooter` chỉ bọc route sao kê, nên gỡ trang sao kê là gỡ luôn M4.
- Lỗi 401 console ở `/login`, `/forgot-password`, `/reset-password/*`, `/invite/*`.

## Phases

| # | Phase | Priority | Effort | Depends on | Status |
|---|---|---|---|---|---|
| 1 | [UX guard harness](./phase-01-ux-guard-harness.md) | P1 | 0.5d | — | Pending |
| 2 | [Crawl blocking (robots, X-Robots-Tag, meta)](./phase-02-crawl-blocking.md) | P1 | 0.25d | — | Pending |
| 3 | [Color tokens and button contrast](./phase-03-color-contrast.md) | P1 | 1d | 1 | Pending |
| 4 | [Responsive overflow](./phase-04-responsive-overflow.md) | P1 | 1d | 1, 3 | Pending |
| 5 | [Public surfaces: remove statement page, 404, login, link preview](./phase-05-public-surfaces.md) | P1 | 1d | 3 | Pending |
| 6 | [Structure: titles, h1, billing, sidebar, touch targets](./phase-06-page-structure.md) | P1/P2 | 1d | 3 | Pending |
| 7 | [Design-system docs and final verification](./phase-07-docs-and-verification.md) | P1 | 0.5d | 1–6 | Pending |

Phase 2 độc lập hoàn toàn và có thể làm song song với bất kỳ phase nào.
Phase 3 đi trước 4–6 vì nó sửa `HvSegmented`, `HvButton` và các link-button mà 4–6 cũng chạm tới.

## Mapping report → phase

| Report | Phase |
|---|---|
| M1 tràn ngang | 4 |
| M2 contrast nút | 3 |
| M3 chặn index | 2 |
| M4 footer | 5 (gỡ cùng trang sao kê) |
| M5 404 | 5 |
| M6 billing h1 + tường nút | 6 |
| S1 tiêu đề tab | 6 |
| S2 link preview | 5 |
| S3 login branding | 5 |
| S4 IA sidebar | 6 (đổi tên) |
| S5 touch target permissions | 6 |
| S6 token chữ phụ | 3 |
| S7 tài liệu thiết kế | 7 |
| C1 lỗi 401 console | 5 |
| C2 llms/sitemap/JSON-LD | Không làm (chấp nhận theo report) |

## Non-goals

- Không SSR, không `llms.txt`, sitemap, JSON-LD, canonical (report đã chấp nhận thiếu).
- Không gộp route lớp học. Không đổi API. Không sửa tay `src/components/ui`.
- Không đổi `apps/api`: endpoint `/public/statements/*` và việc sinh link `/s/{token}` giữ nguyên (xử lý ở plan khác nếu cần).
- Không tách nhóm sidebar "Trung tâm".
- Không redesign dashboard hay các trang đã ổn; chỉ sửa những gì report và phase liệt kê.

## Acceptance (DONE contract, rút gọn — chi tiết ở phase 7)

1. `scrollWidth - clientWidth === 0` trên 23 route đã đăng nhập cùng login, forgot, 404, ở 1440/768/375; thêm 320 cho các route M1.
2. axe `color-contrast` = 0 vi phạm trên login, dashboard, 404, billing; mọi biến thể `HvButton` ≥ 4.5:1 (unit test).
3. `robots.txt` là `text/plain` + `Disallow: /`; `X-Robots-Tag: noindex, nofollow` trên mọi response HTML; `sitemap.xml`/`llms.txt` trả 404; meta robots trong HTML.
4. `apps/web` không còn trang sao kê, `PublicLayout`, `AppFooter`, public client; `/s/*` ra 404. 404 hoàn toàn tiếng Việt; mọi route có `document.title` riêng dạng `<Trang> · Teka`.
5. Mỗi route đúng 1 h1; `/center/permissions` không có target < 24px.
6. Discovery scan hết `[meta-description]`, `[open-graph]`; `og:image` là URL tuyệt đối trỏ tới PNG 1200×630.
7. `npm run lint && npm run typecheck && npm test && npm run build` xanh trong `apps/web`.
8. Ảnh chụp round-2 so với round-1: không lệch thương hiệu, không phần tử bị cắt/chồng.
9. Ảnh có dữ liệu thật chỉ ở thư mục git-ignored.

## Risks

| Rủi ro | Giảm thiểu |
|---|---|
| Chữ tối trên nút làm "mất cảm giác" thương hiệu | So ảnh trước/sau ở phase 3; token hoá `--text-on-primary` để đổi một nơi nếu cần. |
| Đổi `--mint-600` lan sang dark mode / chart | Chỉ đổi hex của token; kiểm tra dark mode và chart trong phase 3. |
| Link sao kê đã gửi phụ huynh thành 404 | Người dùng đã chấp nhận; ghi rõ trong mô tả PR. |
| Import/test sót sau khi xoá feature statement | Phase 5 bước 1 chạy typecheck + test ngay; grep sạch trong verification. |
| `billing`/`collections` e2e từng hỏng sẵn trên master | Phân biệt bằng fresh seed; không coi là hồi quy của plan nếu fail giống baseline. |
| X-Robots-Tag lặp header ở nhiều location | Gom vào một snippet include duy nhất (phase 2). |

## Rollout

Một branch `feat/web-ux-polish`, commit theo phase (conventional commits, không AI refs).
Deploy prod cần người dùng duyệt; verify prod (curl header, capture) chạy sau deploy trong phase 7.
