---
phase: 5
title: "Public surfaces: remove statement page, 404, login, link preview"
status: completed
priority: P1
effort: "1d"
dependencies: [3]
---

# Phase 5: Public surfaces

## Goal

Gỡ trang sao kê phụ huynh `/s/:token` khỏi `apps/web` (người dùng chốt 2026-09-29:
chỉ web, API giữ nguyên). Những trang người ngoài còn thấy (login, link mời, 404)
phải có thương hiệu, hoàn toàn tiếng Việt, không có thông tin giả, và tạo preview
có logo khi dán link vào Zalo. Gộp M4, M5, S2, S3, C1.

## Files

Gỡ trang sao kê (kéo theo M4, vì `PublicLayout`/`AppFooter` chỉ bọc route này):

- Delete: `apps/web/src/features/statement/` (toàn bộ: api, components, hooks, lib, pages, routes, schemas, types, `__tests__`)
- Delete: `apps/web/src/layouts/public-layout.tsx`, `apps/web/src/components/shared/app-footer.tsx`
- Delete: `apps/web/src/lib/api/public-client.ts` (chỉ feature statement dùng `publicApiClient`)
- Delete: `apps/web/e2e/statement.spec.ts`
- Modify: `apps/web/src/app/router.tsx` (bỏ import `statementRoutes`, `PublicLayout` và khối route public)
- Modify: `apps/web/src/test/msw/handlers.ts` (bỏ `PUBLIC_API_URL`, fixture và handler `GET /public/statements/:token`; **giữ** các quyền `statements.*` trong catalog vì API vẫn có)
- Modify: `apps/web/e2e/auth.spec.ts` (bỏ test "the public statement route never attempts a session refresh")
- Modify: comment nhắc tới feature statement trong `lib/hooks/use-no-index.ts` và `features/invitation/hooks/use-invitation.ts` (giữ hook, reset/invite vẫn dùng)

Phần còn lại:

- Modify: `apps/web/src/components/shared/not-found.tsx`
- Modify: `apps/web/src/features/auth/pages/login-page.tsx` (+ forgot/reset/invite header)
- Modify: `apps/web/src/layouts/auth-layout.tsx` (nền cream thay `bg-muted/40` nếu cần cho thương hiệu)
- Modify: `apps/web/index.html` (meta description, OG, twitter)
- Create: `apps/web/public/og-image.svg` (nguồn) và `apps/web/public/og-image.png` (1200×630)
- Modify: `apps/web/Dockerfile`, compose prod, `docs/deployment.md` (build-arg `VITE_PUBLIC_ORIGIN`)
- Modify: `apps/web/src/features/auth/components/session-restore.tsx` và `features/auth/stores/auth-store.ts` (C1)
- Tests: `components/shared/__tests__/not-found.test.tsx` (mới), `features/auth/__tests__/session-restore.test.tsx`

**Không đụng:** `apps/api` (endpoint `/public/statements/*`, `PublicBaseURL`, việc sinh link `/s/{token}` trong `statements/service.go` giữ nguyên), `location /public/` trong `nginx.conf` (API vẫn phục vụ path này), trang Thông báo học phí.

## Design

### Gỡ trang sao kê

- Hệ quả đã chấp nhận: link `/s/{token}` API đã và sẽ gửi cho phụ huynh mở ra **trang 404** của web. Xử lý phía API (bỏ link khỏi tin nhắn, bỏ endpoint public) là việc của plan khác.
- Sau khi gỡ, `router.tsx` không còn layout public; 404 (`*`) tự đứng như hiện tại.
- Dọn triệt để: `grep -rn "features/statement\|public-client\|publicApiClient\|PublicLayout\|AppFooter\|/public/statements" apps/web/src apps/web/e2e` phải rỗng (trừ comment nginx).
- `SessionRestore` bỏ hằng `PUBLIC_STATEMENT_PATH_PREFIX` và nhánh bỏ qua `/s/*` (gộp với C1 bên dưới).
- `docs/`: chỉ `docs/api-guidelines.md:166` nhắc "public statement token" và nói về API, nên giữ nguyên.

### M5: 404

```tsx
<main id="main-content" className="flex min-h-svh flex-col items-center justify-center gap-4 bg-cream-100 p-4 text-center">
  <p aria-hidden className="font-display text-[56px] font-extrabold leading-none text-mint-600">404</p>
  <h1 className="font-display text-[24px] font-extrabold text-ink-900">Không tìm thấy trang</h1>
  <p className="text-[15px] text-ink-500">Đường dẫn có thể đã đổi hoặc không còn tồn tại.</p>
  <Link to="/" className={hvButtonVariants({ variant: "primary", size: "md" })}>Về trang chủ</Link>
</main>
```

`/` đã tự điều hướng về login khi chưa đăng nhập (qua `ProtectedRoute`), nên một CTA là đủ; không cần đọc trạng thái auth trong 404.
Phụ huynh mở link sao kê cũ cũng rơi vào trang này; không thêm thông điệp riêng cho `/s/*`.

### S3: Login branding

- Logo mark: import `/app-icon-1024.svg` dạng `<img>` 56px, `alt=""` (tên thương hiệu đã có ở chữ).
- Tên "Teka" chuyển thành `<h1>` (hiện là `<p>`: login đang không có h1).
- Tagline: "Quản lý lớp, điểm danh và thu tiền trong một nơi" (`text-ink-500`).
- "Quên mật khẩu?" dùng `text-mint-600` với hex mới (5.43:1). Subtitle `ink-500`.
- Áp dụng cùng header logo cho forgot/reset/invite để các trang auth đồng bộ; mỗi trang có đúng một h1.

### S2: Link preview

`index.html` (giá trị chung, SPA, không chứa dữ liệu người dùng):

```html
<meta name="description" content="Teka — quản lý lớp dạy thêm: điểm danh 1 chạm, tính học phí từng buổi, gửi thông báo học phí cho phụ huynh." />
<meta property="og:type" content="website" />
<meta property="og:site_name" content="Teka" />
<meta property="og:title" content="Teka — quản lý lớp dạy thêm" />
<meta property="og:description" content="Điểm danh, học phí và thông báo phụ huynh trong một nơi." />
<meta property="og:image" content="%VITE_PUBLIC_ORIGIN%/og-image.png" />
<meta property="og:image:width" content="1200" />
<meta property="og:image:height" content="630" />
<meta name="twitter:card" content="summary_large_image" />
```

`og:image` phải là URL tuyệt đối. Vite thay `%VITE_*%` trong `index.html` lúc build.
`env.ts`/Dockerfile hiện chỉ có `VITE_API_URL`, nên thêm build-arg `VITE_PUBLIC_ORIGIN`
(prod: `https://teka-web.cauchuyenlaptrinh.com`) vào Dockerfile, compose prod và
`docs/deployment.md`. Không hardcode domain prod vào source. Build thiếu biến này
phải fail rõ ràng (kiểm tra trong Dockerfile như `VITE_API_URL`), không để lọt
`%VITE_PUBLIC_ORIGIN%` nguyên văn vào HTML.
Không đặt `og:url`: SPA không biết route lúc crawler đọc, và một `og:url` cố định sẽ làm Zalo gom mọi link về một.

Ảnh OG: viết `og-image.svg` 1200×630: nền `cream-100`, logo từ `app-icon-1024.svg`
(nhúng path), chữ "Teka" Baloo 2 `ink-900`, tagline `ink-500`. Chuyển PNG bằng
`npx @resvg/resvg-js-cli` hoặc `rsvg-convert` (dùng công cụ có sẵn trên máy; không
thêm dependency vào `package.json`). Commit cả SVG nguồn và PNG. Font phải được
chuyển thành path (hoặc render với font cài sẵn) để PNG không rơi về font mặc định.

### C1: Không gọi `/auth/refresh` vô ích trên trang auth public

- Thêm cờ `localStorage["teka.hasSession"] = "1"` khi `setSession` chạy, và xoá khi `clearSession` chạy (trong `auth-store.ts`, bọc try/catch).
- Trên `/login`, `/forgot-password`, `/reset-password/*`, `/invite/*`: **chỉ** thử refresh khi có cờ.
- Route được bảo vệ **luôn** thử refresh như hiện tại.
- Nhánh bỏ qua `/s/*` cũ bị xoá cùng trang sao kê.

**Đánh đổi:** lần đầu sau khi deploy, người đã đăng nhập (chưa có cờ) mở thẳng
`/login` sẽ thấy form thay vì được chuyển vào app. Họ vào `/` vẫn được khôi phục
phiên, và lần sau cờ đã có. Chấp nhận được, vì không ai mất phiên. Phương án đổi
API trả 204 khi không có cookie sạch hơn nhưng đổi hợp đồng API, nên nằm ngoài scope.

## Steps

1. Gỡ trang sao kê: xoá feature, layout, footer, public client, e2e; sửa router, MSW, `auth.spec`, comment. Chạy `npm run typecheck && npm test` ngay để bắt import sót.
2. Viết lại 404 + test (tiếng Việt, 1 h1, link tới `/`).
3. Login/auth pages: logo, h1, tagline, màu chữ.
4. OG image + meta + build-arg `VITE_PUBLIC_ORIGIN`.
5. C1 + test cho 3 trường hợp (public auth route không cờ → không gọi; có cờ → gọi; route bảo vệ → luôn gọi).

## Verification

- `grep -rn "features/statement\|publicApiClient\|PublicLayout\|AppFooter\|0900\|hotro@teka\|Page not found\|Back to dashboard" apps/web/src apps/web/e2e` → rỗng.
- `/s/bat-ky` trên stack e2e → trang 404 tiếng Việt, không có request tới `/public/statements`.
- `npx playwright test ux-audit -g "login|forgot|404"` → pass overflow/h1/contrast/target-size (404 CTA ≥ 44×44).
- Chụp `/login` 3 viewport: logo nằm trong màn đầu.
- Mở `/login` trên stack e2e với storage trống → tab Network không có request `/auth/refresh`, console 0 lỗi.
- Build image với `--build-arg VITE_PUBLIC_ORIGIN=http://localhost:58090`: `index.html` có `og:image` tuyệt đối, không còn `%VITE_`; `/og-image.png` trả `image/png`. Build thiếu biến → fail.
- `npm run lint && npm run typecheck && npm test && npm run build`.
- Sau deploy (phase 7): dán link login/invite vào Zalo thấy preview có logo; discovery scan hết `[meta-description]`, `[open-graph]`.

## Risks

- Link sao kê đã gửi cho phụ huynh thành 404: đã chấp nhận; ghi rõ trong mô tả PR để người dùng báo trước cho trung tâm nếu cần.
- Import sót hoặc knip/lint báo export chết sau khi xoá: bước 1 chạy typecheck/test ngay trước khi làm phần khác.
