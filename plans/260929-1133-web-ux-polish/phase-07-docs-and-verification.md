---
phase: 7
title: "Design-system docs and final verification"
status: pending
priority: P1
effort: "0.5d"
dependencies: [1, 2, 3, 4, 5, 6]
---

# Phase 7: Design-system docs and final verification

## Goal

Ghi các quy tắc mới vào design system trong `docs/` để agent và reviewer không
tái phạm, rồi chứng minh toàn bộ DONE contract bằng evidence round-2 (local, và prod sau khi deploy được duyệt).

## Files

- Modify: `docs/frontend-guidelines.md` (mục `## hv kit` và `## Accessibility baseline`, thêm mục `## Color and contrast` và `## UX review checklist`)
- Modify: `apps/web/AGENTS.md`, `apps/web/CLAUDE.md` (một dòng trỏ tới các mục mới; không chép nội dung)
- Modify: `docs/deployment.md` (nếu phase 2/5 chưa cập nhật: headers noindex, robots.txt, build-arg `VITE_PUBLIC_ORIGIN`)
- Create: `plans/260929-1133-web-ux-polish/reports/round-2/` (ảnh non-PII + kết quả) và `round-2/app-prod/.gitignore` = `*`

Không tạo `DESIGN.md`/`REVIEW.md` ở gốc (đã chốt). `docs/frontend-guidelines.md` đang 243 dòng, dưới giới hạn 800.

## Nội dung docs (tóm tắt, viết đầy đủ khi làm)

**`## Color and contrast`** (design system, theo yêu cầu người dùng)
- Bảng token chữ/nền đã được kiểm chứng kèm ratio: `ink-900` trên mint-400/sky-300/coral-400/sun-400; `ink-500` cho chữ phụ; `mint-600` (#24775c) cho link/chữ nhấn; `ink-400` chỉ cho placeholder/trang trí.
- Quy tắc: nền pastel thương hiệu luôn đi với `--text-on-brand` (ink-900), không dùng `text-white`.
- Link trông như nút dùng `hvButtonVariants(...)`, không chép class.
- `src/styles/tokens/__tests__/contrast.test.ts` là nguồn khoá; thêm cặp mới vào đó khi thêm token.

**`## hv kit`**: thêm `HvTableScroll` (bảng rộng luôn bọc nó; lý do containing block của `sr-only`) và `hvButtonVariants`.

**`## Accessibility baseline`**: mỗi route một h1; `handle.title` bắt buộc cho route mới; target ≥ 24px (khuyến nghị 44); vùng cuộn có `role="region"` + `aria-label`.

**`## UX review checklist`**: dạng câu hỏi cho reviewer:
- Có bảng/hàng ngang mới không? Nó có bọc `HvTableScroll` không, và `ux-audit` ở 320/375 còn xanh không?
- Có `text-white` trên nền pastel không?
- Route mới có `handle.title` và đúng một h1 không?
- Có chuỗi placeholder (SĐT, email giả, link chết) không?
- Có surface public mới nào cần `X-Robots-Tag` hay có response HTML mới đi ngoài nginx không?

## Final verification (DONE contract)

Chạy trên stack isolated `teka-e2e` (fresh seed):

1. `cd apps/web && npm run lint && npm run typecheck && npm test && npm run build`.
2. `E2E_BASE_URL=http://localhost:55173 npx playwright test ux-audit` → xanh toàn bộ (overflow 1440/768/375 + 320 cho route M1; h1; title duy nhất; axe contrast/target-size).
3. `npx playwright test` toàn suite; so fail (nếu có) với baseline đã biết (billing/collections trước đây; `statement.spec` đã bị xoá ở phase 5). Mọi fail mới phải được sửa.
4. `git diff master --stat -- apps/web/src/components/ui` → rỗng.
5. Copy ảnh `test-results/ux-audit/` của trang public vào `reports/round-2/`, so với `round-1/` bằng vision: không lệch thương hiệu, không phần tử bị cắt/chồng. Ghi kết luận vào `reports/round-2/summary.md`.
6. `docker compose -p teka-e2e down -v`; dừng container kiểm tra nginx nếu còn.

**Sau khi người dùng duyệt merge + deploy prod** (theo memory `teka-prod-deploy-topology`; deploy là hành động cần xác nhận):

7. `curl -sI https://teka-web.cauchuyenlaptrinh.com/robots.txt` → `text/plain`; `/`, `/login`, `/s/x`, `/invite/x`, `/reset-password/x` có `x-robots-tag: noindex`; `/sitemap.xml`, `/llms.txt` → 404.
8. Discovery scan lại: không còn ERROR `[robots]`, `[meta-description]`, `[open-graph]`.
9. Capture read-only 23 route prod ở 4 viewport vào `round-2/app-prod/` (git-ignored), đo `scrollWidth - clientWidth`.
10. Dán link `/login` vào Zalo, xác nhận preview có logo (người dùng làm và báo lại).

## Acceptance

Tất cả 9 điều kiện của DONE contract trong `plan.md` đúng. Điều kiện nào không
chạy được phải ghi rõ lý do trong `round-2/summary.md` và **không** tính là pass.

## Privacy

Ảnh chứa dữ liệu thật chỉ nằm trong `round-2/app-prod/` (git-ignored). Summary và
commit không chứa tên học sinh/phụ huynh, số tiền, token hay thông tin đăng nhập.
