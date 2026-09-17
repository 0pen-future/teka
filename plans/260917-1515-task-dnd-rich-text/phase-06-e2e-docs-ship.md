---
phase: 6
title: "E2E, tài liệu, hợp đồng v2 và ship"
status: pending
priority: P2
effort: "1d"
dependencies: [4, 5]
---

# Phase 6: E2E, tài liệu, hợp đồng v2 và ship

## Overview

Chứng minh toàn luồng trên stack thật bằng Playwright (kéo-thả thật, rich text
thật), chốt tài liệu kiến trúc/guidelines/hợp đồng API v2, và ship với sao lưu
DB trước migration 000023.

## Requirements

- Functional (e2e, stack `teka-e2e` cô lập, seed mới — `make e2e` hiện chỉ chạy `npm run e2e` vào stack `make dev`, **không** dựng stack cô lập; phase này thêm target `make e2e-isolated` gói lệnh thật):
  ```
  POSTGRES_PORT=55432 API_HTTP_PORT=58080 WEB_PORT=55173 ADMINER_PORT=58081 \
  API_STATEMENTS_PUBLIC_BASE_URL=http://localhost:55173 \
  API_CORS_ORIGINS=http://localhost:55173,http://127.0.0.1:55173 \
  docker compose -p teka-e2e up -d --build
  (cd apps/api && API_DATABASE_URL=postgres://teka:teka_dev_password@localhost:55432/teka?sslmode=disable go run ./cmd/api seed)
  (cd apps/web && E2E_BASE_URL=http://localhost:55173 npx playwright test)
  docker compose -p teka-e2e down -v
  ```
  Suite cũ `billing`/`collections`/`statement` đã đỏ sẵn trên master (ghi nhận từ 01/09) → gate của plan này là 4 spec `tasks-board*`; nêu rõ trong bằng chứng.
  - `tasks-board-dnd.spec.ts`: tạo 3 việc trong cột A; kéo việc 3 lên giữa 1 và 2 bằng `mouse.down` → nhiều `mouse.move` → `mouse.up`; reload; assert thứ tự A = [1,3,2]. Kéo việc 1 sang cột B thả vào vùng trống; reload; assert B = [1], A = [3,2]. Assert card vẫn `role="option"` sau khi thả.
  - `tasks-board-rich-text.spec.ts`: mở "Thêm việc", gõ mô tả, nhấn Đậm, thêm list, thêm link; lưu; mở lại: `strong`, `ul`, `a[href][rel~="noopener"]` tồn tại; gửi payload XSS trực tiếp qua `page.request.post` với phiên đăng nhập → GET trả về không chứa `script`.
  - `tasks-board-mobile.spec.ts` (Pixel 7 emulate): nhấn giữ 300ms rồi kéo trong cột → thứ tự đổi; kéo nhanh (không giữ) → trang cuộn, thứ tự không đổi; sau khi thả **không** có dialog mở. Spec này là **gate** cho cặp sensor `MouseSensor`+`TouchSensor` của phase 4 (CDP touch cũng sinh pointer events; nếu còn `PointerSensor` thì nhấn giữ sẽ không kích hoạt).
- Documentation:
  - `docs/architecture.md`: đoạn `apps/web/src/lib/kanban` thêm "pointer DnD là adapter opt-in ở `features/tasks` (dnd-kit); lib chỉ có helper vị trí"; đoạn `apps/api/pkg/kanban` thêm "vị trí = float midpoint + renormalize trong tx"; mục Tasks thêm "mô tả là HTML subset sanitize hai lớp".
  - `docs/frontend-guidelines.md`, `docs/api-guidelines.md`: các đoạn đã nêu ở phase 2/4/5 — rà lại một lần cho nhất quán.
  - `plans/260917-1515-task-dnd-rich-text/reports/api-contract-v2.md`: hợp đồng `POST /tasks/:id/move` v2 (body, ví dụ, 422), `description` HTML subset + giới hạn, ghi rõ body v1 vẫn hợp lệ.
  - README hai lib (đã sửa ở phase 1/3) — rà lại link.
- Ship:
  - Thứ tự: **backup DB** (`pg_dump` container prod theo topology `teka-*`) → `compose up` prod (service `migrate` trong `docker-compose.prod.yml` tự chạy 000023 trước `api`, nên backup phải xong **trước** lệnh compose up) → web deploy trong cùng cửa sổ.
  - Kiểm sau deploy: mở bảng, kéo 1 việc, mở 1 việc cũ (mô tả legacy hiển thị đúng xuống dòng), tạo 1 việc có link.
  - Rollback: web/API về tag trước; DB giữ migration (HTML vô hại với client cũ, chỉ hiển thị thô) hoặc `migrate down --steps 1` (lossy) nếu bắt buộc; restore từ backup là đường cuối.

## Architecture

```
make e2e (compose -p teka-e2e, seed mới)
 ├─ e2e/tasks-board-dnd.spec.ts        ── Playwright mouse.* theo bước (dnd-kit cần pointermove liên tiếp)
 ├─ e2e/tasks-board-rich-text.spec.ts  ── UI + page.request XSS
 └─ e2e/tasks-board-mobile.spec.ts     ── devices["Pixel 7"], hasTouch:true, CDP Input.dispatchTouchEvent
```

Helper `e2e/helpers/drag.ts`:

```ts
export async function dragTo(page: Page, source: Locator, target: Locator, steps = 12) {
  const from = await source.boundingBox();
  const to = await target.boundingBox();
  if (!from || !to) throw new Error("drag: element not visible");
  const sx = from.x + from.width / 2, sy = from.y + from.height / 2;
  const tx = to.x + to.width / 2, ty = to.y + to.height / 2;
  await page.mouse.move(sx, sy);
  await page.mouse.down();
  await page.mouse.move(sx + 8, sy); // vượt activation distance
  for (let i = 1; i <= steps; i++) {
    await page.mouse.move(sx + ((tx - sx) * i) / steps, sy + ((ty - sy) * i) / steps);
  }
  await page.mouse.up();
}
```

## Related Code Files

- Create: `apps/web/e2e/helpers/drag.ts`
- Create: `apps/web/e2e/tasks-board-dnd.spec.ts`, `apps/web/e2e/tasks-board-rich-text.spec.ts`, `apps/web/e2e/tasks-board-mobile.spec.ts`
- Modify: `apps/web/e2e/tasks-board.spec.ts` (bước "move" qua menu giữ nguyên; thêm assert việc nằm đầu cột sau "Chuyển")
- Modify: `apps/web/playwright.config.ts` (chưa có `projects`: thêm project `desktop` với `testIgnore: /-mobile\.spec\.ts$/` và project `mobile` với `devices["Pixel 7"]`, `testMatch: /-mobile\.spec\.ts$/` — thiếu `testIgnore` thì spec mobile chạy 2 lần; giữ `workers: 1`)
- Modify: `Makefile` (target `e2e-isolated` gói compose `-p teka-e2e` + seed + playwright + `down -v`; `e2e` cũ giữ nguyên)
- Modify: `docs/architecture.md`, `docs/frontend-guidelines.md`, `docs/api-guidelines.md`
- Create: `plans/260917-1515-task-dnd-rich-text/reports/api-contract-v2.md`
- Modify: `plans/260917-1515-task-dnd-rich-text/plan.md` (mục "Test evidence"; trạng thái phase qua `ak plan`)

## Implementation Steps

1. Thêm `make e2e-isolated`; viết `helpers/drag.ts` + `tasks-board-dnd.spec.ts`; chạy `make e2e-isolated` (spec có state cần seed mới). Nếu drag không kích hoạt: tăng `steps`, thêm `waitForTimeout(30)` giữa bước.
2. Viết `tasks-board-rich-text.spec.ts`; XSS qua `page.request` (tái dùng login helper theo mẫu các spec sẵn có).
3. Project mobile trong Playwright config + `tasks-board-mobile.spec.ts` (touch qua CDP `Input.dispatchTouchEvent`: `touchStart` → đợi 300ms → `touchMove` nhiều bước → `touchEnd`).
4. Cập nhật 3 file docs + rà README hai lib; viết `reports/api-contract-v2.md`.
5. Chạy đủ gate: `make test-api` (đơn lẻ), `make lint-api`, `make api-docs` (diff sạch), `make test-web`, `make lint-web`, `make e2e-isolated`; dán bằng chứng vào `plan.md` mục "Test evidence" (lệnh + số test pass; ghi rõ 3 suite cũ đỏ sẵn nếu vẫn còn).
6. Commit theo phase (conventional, không tham chiếu AI), ví dụ: `feat(api): place moved tasks after a neighbour and renormalize positions`, `feat(api): store task descriptions as a sanitized html subset`, `feat(web): add position helpers to the kanban lib`, `feat(web): drag and drop tasks on the board`, `feat(web): rich text task descriptions`, `test(e2e): cover board drag and rich text`, `docs: describe board positioning and rich text boundaries`.
7. Ship: backup DB trước (lệnh `pg_dump -Fc` vào container postgres prod, ghi file có timestamp, xác minh kích thước > 0); `compose up` prod (migrate tự chạy rồi api lên); kiểm `count(*)` mô tả chưa bọc `<p>` = 0; deploy web; checklist sau deploy; cập nhật trạng thái plan bằng `ak plan`.

## Success Criteria

- [ ] AC12: 3 spec e2e mới xanh trên `teka-e2e` (desktop + mobile project); `tasks-board.spec.ts` cũ vẫn xanh; spec mobile chỉ chạy ở project mobile.
- [ ] Docs: 3 file docs + 2 README cập nhật, link kiểm tra được; `reports/api-contract-v2.md` khớp swagger.
- [ ] Bằng chứng gate đầy đủ trong `plan.md`.
- [ ] Prod: file backup tồn tại trước migrate; checklist sau deploy hoàn tất; không lỗi 5xx ở log API trong 30 phút đầu.

## Risk Assessment

- **Playwright drag flaky**: nguyên nhân thường là thiếu `pointermove` trung gian hoặc auto-scroll; ứng phó: `steps ≥ 12`, `expect.poll` trên thứ tự sau reload; nếu vẫn flaky sau 3 lần → sửa helper/selector, không dùng `test.fixme` (quy tắc không làm yếu test).
- **Touch emulation Chromium headless** không kích hoạt `TouchSensor` → CDP thô; nếu vẫn không được, ghi rõ giới hạn trong spec và kiểm tay trên thiết bị thật trước ship.
- **Migration prod**: nhỏ, idempotent; rủi ro thật là quên backup → bước 7 bắt đầu bằng backup và assert kích thước file.
- **Deploy lệch API/web**: web cũ hiển thị `<p>` thô trong vài phút → deploy web ngay sau API.
