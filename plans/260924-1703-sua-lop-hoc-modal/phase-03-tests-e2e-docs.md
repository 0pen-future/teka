---
phase: 3
title: "Test, e2e, docs"
status: done
effort: "0.5d"
dependsOn: [2]
---

# Phase 3 — Test, e2e, docs

Tất cả đường dẫn dưới đây tương đối với `apps/web/`.

## Unit và integration (vitest + MSW)

| File | Việc |
|---|---|
| `src/features/roster/__tests__/class-dialog.test.tsx` | Thêm `describe("edit mode")`. Port **toàn bộ** ca của `class-settings-page.test.tsx`: prefill; chọn ngày rỗng bị từ chối; cảnh báo đổi đơn giá; lưu tên/đơn giá + diff lịch (dialog đóng, toast); thêm khung giờ thứ hai; trùng ngày giữa hai khung giờ; đổi giờ thì **đóng** lịch cũ chứ không xoá; quyền (owner / `giao_vien` bật nút lưu, chỉ `hoc_vu` thì disable + notice). Thêm ca mới: lưu một phần giữ dialog mở kèm lỗi root |
| `src/features/roster/__tests__/teacher-handoff-card.test.tsx` | Đổi tên / port từ `class-settings-handoff.test.tsx`: render `ClassInfoTab` (hoặc `ClassDetailPage`) thay cho màn settings. Giữ 6 ca: target list, member không thấy, two-click confirm, re-pick giữ trạng thái arm, đổi target thì un-arm, lỗi 422 inline |
| `src/features/roster/__tests__/class-list-page.test.tsx` | Dòng ~102: thay assert `href …/settings` bằng: bấm "Sửa" → dialog "Sửa lớp học" mở, URL vẫn `/classes`. Thêm: hàng của lớp không có quyền ghi thì không có nút |
| `src/features/roster/__tests__/class-detail-page.test.tsx` | Dòng ~54-65 và ~152: bỏ stub `/classes/:id/settings`. Assert nút "Sửa lớp" mở dialog và set `?edit=1`. Route `?edit=1` mở sẵn dialog, đóng thì xoá param |
| `src/features/roster/__tests__/students-page.test.tsx` | Dòng ~69-101: "⚙ Cài đặt" là button mở dialog (cả mobile card lẫn bảng) |
| `src/features/roster/__tests__/class-routes.test.tsx` (mới, hoặc gộp vào detail test) | `/classes/:id/settings` redirect sang `/classes/:id?edit=1` |
| `class-settings-page.test.tsx`, `class-settings-handoff.test.tsx` | Xoá **sau khi** các ca đã port và pass |

Lệnh:

```bash
cd apps/web && npx vitest run src/features/roster
```

## E2E (Playwright, isolated stack theo memory `teka-e2e-isolated-stack`)

| Spec | Việc |
|---|---|
| `e2e/class-list.spec.ts` (~112-128) | Không đọc `href` của link "Sửa" nữa. Bấm nút "Sửa" → `getByRole("dialog", { name: "Sửa lớp học" })` → đổi tên → Lưu → hàng hiện tên mới, URL vẫn `/classes` |
| `e2e/class-staff-write.spec.ts` (~137-206) | `page.goto(\`/classes/${classId}\`)`, locator `#teacher-handoff` giữ nguyên. Bước "settings save reserved for new teacher" đổi thành: mở dialog qua `?edit=1` rồi kiểm tra nút lưu |
| `e2e/class-invitations.spec.ts` (~52-63) | `goto` trang chi tiết thay cho `/settings` |

```bash
cd apps/web && npx playwright test e2e/class-list.spec.ts e2e/class-staff-write.spec.ts e2e/class-invitations.spec.ts
```

## Docs

- `rg -n "classes/:id/settings|Cài đặt lớp|ClassSettingsPage" docs apps/web/README.md apps/web/AGENTS.md`. Sửa đúng
  những chỗ mô tả luồng sửa lớp (màn thành modal, redirect). Ở thời điểm lập plan, grep `docs/*.md` chưa thấy tham
  chiếu nào, nên có thể không phải sửa gì.
- Không động vào `docs/architecture.md` (đang có thay đổi chưa commit, không thuộc plan này), trừ khi grep chỉ ra
  đúng tham chiếu.

## Hoàn tất

- `npm run lint && npm run typecheck && npx vitest run` trong `apps/web`.
- Đối chiếu lại 9 tiêu chí nghiệm thu trong `plan.md`.
- Commit theo conventional commits, không có dòng tham chiếu AI (memory `teka-no-ai-refs-in-commits`). Gợi ý tách:
  `feat(web): edit a class in the Sửa lớp học dialog`, `refactor(web): drop the class settings screen`,
  `test(e2e): cover class editing through the dialog`.
