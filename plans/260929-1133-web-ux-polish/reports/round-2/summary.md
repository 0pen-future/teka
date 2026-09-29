# Round 2 — kết quả kiểm chứng (2026-09-29)

Stack: `teka-e2e` isolated (Vite dev server :55173, API và Postgres seed mới).

## DONE contract

| # | Điều kiện | Kết quả | Bằng chứng |
|---|---|---|---|
| 1 | Không tràn ngang 1440/768/375 (+320 cho route M1) | Đạt | `ux-audit` 85/85 xanh; sau khi toàn suite tạo dữ liệu, overflow vẫn 0 |
| 2 | axe contrast = 0; mọi biến thể `HvButton` ≥ 4.5:1 | Đạt | `ux-audit` (desktop + phone, 26 route); `contrast.test.ts` 24 test (5 biến thể nút + 19 cặp token) |
| 3 | robots, `X-Robots-Tag`, 404 cho sitemap/llms, meta robots | Đạt (local) | Kiểm bằng container nginx ở phase 2; prod kiểm lại sau deploy |
| 4 | Gỡ trang sao kê; 404 tiếng Việt; title `<Trang> · Teka` | Đạt | `ux-audit` title + "every audited route has a distinct title" |
| 5 | Đúng 1 h1; `/center/permissions` không có target < 24px | Đạt | `ux-audit` h1 + axe `target-size` |
| 6 | meta description, Open Graph, `og:image` tuyệt đối 1200×630 | Đạt (local) | Image web tự dựng: `og:image` tuyệt đối, `/og-image.png` là `image/png` |
| 7 | lint, typecheck, test, bundle | Đạt | lint 0 lỗi (11 cảnh báo có sẵn), typecheck sạch, vitest 1190 pass / 3 skip, Vite bundle xanh |
| 8 | Ảnh round-2 so với round-1 | Đạt | Ảnh public trong thư mục này; xem mục dưới |
| 9 | Ảnh có dữ liệu thật chỉ ở thư mục git-ignored | Đạt | Chỉ commit ảnh trang public; `app-prod/.gitignore` = `*` |

Toàn suite Playwright: 124/128 pass ở lần chạy đầu. 4 lỗi đều là axe contrast ở
`sessions` và `tasks`. Chúng chỉ xuất hiện khi các spec khác đã tạo dữ liệu
(avatar giao việc, số đếm chip lọc, ô "Chưa có buổi"). Đã sửa và chạy lại:
các route này đều xanh. Không có lỗi mới nào khác so với baseline.

`git diff master --stat -- apps/web/src/components/ui` rỗng.

## So sánh ảnh public (round-1 → round-2)

- Login: thêm icon app, tagline, "Đăng nhập để tiếp tục" đậm hơn. Nút
  "Đăng nhập" đổi chữ trắng sang `ink-900` (đạt AA). Không bị cắt hay chồng
  ở 1440 và 375.
- 404: tiếng Việt hoàn toàn, "404" màu `mint-600`, nút "Về trang chủ" dùng
  `hvButtonVariants`. Không bị cắt ở 375.
- Forgot password: tiêu đề là h1, cùng nền `cream-100` như login.

## Prod sau deploy local (2026-09-29 13:35)

Web image `teka-web:e4d262b-260929-1334` đã được dựng với các URL prod và chạy
qua `compose -p teka` (prod + homelab). API image giữ nguyên
`ab092bc-260925-1126` vì `apps/api` không đổi; `migrate` exit 0.

- `/readyz` của API trả 200.
- `/`, `/login`, `/s/x`, `/invite/x`, `/reset-password/x`: 200, có
  `x-robots-tag: noindex, nofollow`. `/s/x` giờ render trang 404 của SPA.
- `robots.txt` là `text/plain` với `Disallow: /`. `/sitemap.xml`, `/llms.txt`
  và `/llms-full.txt` trả 404.
- HTML có meta `description`, `robots`, `og:*` (gồm `og:image` tuyệt đối
  1200×630) và `twitter:card`. `/og-image.png` là `image/png`. Nhờ vậy ba lỗi
  `[robots]`, `[meta-description]` và `[open-graph]` của discovery scan đã
  được xử lý tận gốc.
- Login, forgot và 404 ở 1440/768/375/320: overflow 0, đúng một h1, title
  riêng. Ảnh nằm trong `app-prod/`, thư mục này bị git-ignore.

## Chưa chạy được ở round này

- Chưa chụp 23 route cần đăng nhập trên prod. Việc này cần thông tin đăng
  nhập prod, nên không làm. Các route này đã đạt trên stack e2e (xem bảng trên).
- Preview Zalo: người dùng cần dán link `/login` vào Zalo và báo lại. Chưa
  tính là đạt.
