# PM — đóng plan Web UX polish (2026-09-29)

Plan `260929-1133-web-ux-polish` đã hoàn tất cả 7 phase trên nhánh
`feat/web-ux-polish`. Trạng thái plan và phase là `completed`, và
`ak plan validate` báo hợp lệ. Bằng chứng DONE contract nằm ở
[round-2/summary.md](../260929-1133-web-ux-polish/reports/round-2/summary.md).

## Đối chiếu docs

| Docs | Trạng thái |
|---|---|
| `docs/frontend-guidelines.md` | Đã cập nhật: hv kit (`hvButtonVariants`, `HvTableScroll`), mục Color and contrast (các cặp token có kèm tỉ lệ, được khoá bằng `contrast.test.ts`), Accessibility baseline (một h1, `handle.title`, target, vùng cuộn), UX review checklist, quy tắc không làm mờ chữ bằng opacity |
| `apps/web/AGENTS.md`, `CLAUDE.md` | Có một dòng trỏ tới các mục trên; đã gỡ public client khỏi danh sách |
| `docs/deployment.md` | Có build-arg `VITE_PUBLIC_ORIGIN`, hành vi noindex/robots |
| Docs nhắc route `/public/*` của API | Giữ nguyên, vì API không đổi (thuộc non-goal) |

## Còn mở

- Các bước kiểm tra trên prod của phase 7 (header, discovery scan, chụp 23 route, preview Zalo) sẽ làm sau khi deploy local.
- Push và merge nhánh vào `master` cần người dùng duyệt.
