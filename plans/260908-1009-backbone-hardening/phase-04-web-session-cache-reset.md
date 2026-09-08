---
phase: 4
title: "Web: xoá query cache tại ranh giới phiên"
status: pending
priority: P1
effort: "0.5d"
dependencies: []
---

# Phase 4: Web — xoá TanStack cache tại ranh giới phiên (finding 8)

## Overview

Một component `SessionCacheReset` (render null) mount trong `Providers` quan sát `useAuthStore` và gọi `queryClient.clear()` khi user id chuyển từ non-null sang null (logout, refresh chết) hoặc sang id khác (đổi tài khoản trên cùng tab). `useLogout` bỏ `queryClient.clear()` trùng lặp. Query key giữ nguyên.

## Requirements

- Functional:
  - Chuyển `A → null` hoặc `A → B` ⇒ cache rỗng (cả data lẫn query đang inflight bị huỷ theo `queryClient.clear()`).
  - `null → A` (đăng nhập, `SessionRestore` thành công) ⇒ không xoá (không có gì để xoá, tránh xoá prefetch nếu có).
  - `setAccessToken` (rotation) và `setUser` cùng id (đổi tên/avatar) ⇒ không xoá.
  - Đường refresh chết trong `interceptors.ts` (`markRefreshDead(); clearSession()`) không cần sửa — store đổi trạng thái là component xử lý.
  - `renderWithProviders` trong test utils mount `SessionCacheReset` để test phản ánh runtime thật và để test dùng `queryClient` riêng của từng test.
- Non-functional: không đổi query key; không thêm dependency; component không gây re-render (`useAuthStore` selector trả primitive `user?.id ?? null`); tuân `apps/web/CLAUDE.md` (feature barrel export, không import ngang giữa feature ngoài barrel).

## Architecture

```text
Providers
└─ QueryClientProvider(queryClient)
   ├─ SessionCacheReset            ← useAuthStore(s => s.user?.id ?? null) + useQueryClient()
   │     prevRef = useRef<string|null>(current)
   │     useEffect([id]): if (prevRef.current !== null && id !== prevRef.current) queryClient.clear()
   │                      prevRef.current = id
   └─ SessionRestore → RouterProvider …
```

Nguồn sự thật cho "ranh giới phiên" là store zustand đã có (`setSession`, `clearSession`, `setUser`), nên mọi call site hiện tại và tương lai đều được phủ mà không cần nhớ gọi `clear()`.

## Related Code Files

- Create: `apps/web/src/features/auth/session-cache-reset.tsx`, `apps/web/src/features/auth/__tests__/session-cache-reset.test.tsx`.
- Modify: `apps/web/src/features/auth/index.ts` — export `SessionCacheReset`.
- Modify: `apps/web/src/app/providers.tsx` — mount trong `QueryClientProvider`, trước `SessionRestore`.
- Modify: `apps/web/src/features/auth/hooks/use-auth.ts` — `useLogout.onSettled` chỉ `clearSession()`; bỏ `useQueryClient` nếu không còn dùng.
- Modify: `apps/web/src/test/utils.tsx` — `renderWithProviders` mount `SessionCacheReset` trong `QueryClientProvider`.
- Modify: `docs/frontend-guidelines.md` — mục server/client state (~dòng 60-80): cache xoá tại ranh giới phiên bởi `SessionCacheReset`; key không mang user id là chủ ý.
- Không đổi: `interceptors.ts`, `auth-bridge.ts`, `auth-store.ts`, key factories, `components/ui/*`.

## Implementation Steps

1. **Component.** Viết `SessionCacheReset` như sơ đồ; khởi tạo `prevRef` bằng giá trị hiện tại lúc mount (để mount khi đã đăng nhập không kích hoạt clear). Export qua barrel.
2. **Mount.** `providers.tsx`: `<SessionCacheReset />` là con trực tiếp của `QueryClientProvider`, đứng trước `SessionRestore`. `test/utils.tsx`: tương tự trong `renderWithProviders`.
3. **`useLogout`.** Bỏ `queryClient.clear()`; kiểm tra grep `queryClient.clear` toàn `apps/web/src` — chỉ còn trong component mới. Nếu có test hiện tại assert logout xoá cache (grep `getQueryCache` / `clear` trong `__tests__`), giữ test đó và đảm bảo vẫn pass nhờ component mount qua `renderWithProviders`.
4. **Test component** (`session-cache-reset.test.tsx`, dùng `renderWithProviders` và `queryClient` nó trả về; `signInAs` để seed user A):
   - A đăng nhập, `queryClient.setQueryData(["probe"], 1)`; `useAuthStore.getState().clearSession()` → `waitFor(getQueryCache().getAll().length === 0)`.
   - A → `setSession({ user: B, accessToken })` → cache rỗng.
   - A → `setUser({ ...A, name: "x" })` → cache còn.
   - A → `setAccessToken("t2")` → cache còn.
   - Mount khi đã có user (signInAs trước render) rồi seed data → không bị xoá.
   - End-to-end refresh chết: `server.use(http.get(\`${API_URL}/probe\`, () => 401), http.post(\`${API_URL}/auth/refresh\`, () => 401))`; component nhỏ gọi `useQuery` tới `/probe`; seed data key khác; sau khi query fail → `waitFor(cache rỗng)` và `useAuthStore.getState().user === null`.
5. **Docs.** Cập nhật `docs/frontend-guidelines.md` một đoạn ngắn; kiểm tra `apps/web/CLAUDE.md` có nhắc "queryClient.clear in useLogout" không (grep) — nếu có, sửa cho khớp.
6. **Verify.** `cd apps/web && npm run typecheck && npm run test`, `make lint-web`.

## Success Criteria

- [ ] Cache rỗng sau `clearSession()` (logout và refresh chết) và sau đổi user; không rỗng khi rotation token hoặc cập nhật profile cùng id.
- [ ] `queryClient.clear()` chỉ tồn tại ở một nơi trong `apps/web/src`.
- [ ] `renderWithProviders` mount component; toàn bộ test web pass; typecheck + lint xanh.
- [ ] `docs/frontend-guidelines.md` mô tả ranh giới cache.

## Risk Assessment

- **Effect chạy sau khi UI của người dùng B đã render một frame với data của A** (clear xảy ra trong `useEffect`, không đồng bộ). Trên thực tế đổi user luôn đi qua trang login (route không có query của A đang mount) nên không hiển thị; nếu review thấy cần, chuyển sang `useLayoutEffect` hoặc subscribe store ngoài React (`useAuthStore.subscribe`) trong `useEffect` mount một lần — chọn khi có bằng chứng flash.
- **Query inflight bị huỷ khi clear** → component đang mount có thể thấy `isError` thoáng qua; hành vi hiện tại của `useLogout` đã như vậy; không đổi.
- **Ai đó mount `useLogout` ngoài `Providers`** (không có component) → cache không xoá. Chỉ có một root; test utils cũng mount; ghi trong guideline.
