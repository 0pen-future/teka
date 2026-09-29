---
title: Plan web UX polish
date: 2026-09-29
summary: Lập plan 7 phase đưa apps/web đạt DONE contract của report enhance-ux-ax
---

# Plan web UX polish

## Chuyện gì đã xảy ra
Từ report `plans/reports/enhance-ux-ax-260929-1051-web.md`, đã lập plan `plans/260929-1133-web-ux-polish/` gồm 7 phase: guard harness, chặn index, contrast, tràn ngang, trang public, cấu trúc trang, docs + verify.

Khi scout phát hiện thêm vài điểm report chưa nêu:
- Không chỉ nút primary mà secondary (1.85:1), danger (2.55:1) và reward (2.27:1) cũng trượt AA.
- Có 7 file chép tay chuỗi class của `HvButton` cho `<Link>`.
- `text-mint-600` chỉ đạt 3.80 trên cream.
- API statement không trả thông tin liên hệ của trung tâm.

## Quyết định
- Dùng chữ `ink-900` trên nền pastel thương hiệu.
- Đổi `--mint-600` thành `#24775c`.
- Export `hvButtonVariants` để thay các bản chép tay.
- Gỡ trang sao kê `/s/:token` khỏi web (kèm `PublicLayout`, `AppFooter`); API giữ nguyên, link cũ ra 404.
- Đổi tên sidebar, không gộp route.
- Ảnh OG từ `app-icon-1024.svg`, `og:image` qua build-arg `VITE_PUBLIC_ORIGIN`. Không tách nhóm sidebar "Trung tâm".
- Thêm primitive `HvTableScroll` có `relative`.
- Dùng e2e `ux-audit` và unit test contrast làm cổng kiểm tra.

## Bước tiếp theo
- Chạy `/ak:cook plans/260929-1133-web-ux-polish/plan.md`.

> Historical work record — not durable authority. Prefer docs/specs/ADRs for current decisions.
