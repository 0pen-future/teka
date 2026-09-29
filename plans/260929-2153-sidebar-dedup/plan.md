---
title: "Gỡ trùng lặp sidebar và dọn các tính năng thừa"
description: "Bỏ các mục sidebar trùng (Lớp cần tuyển sinh, Hồ sơ học sinh, Gửi báo cáo, Cấu hình lớp học), gộp Gửi thông báo, lấy bộ điểm của mẫu chương trình làm nguồn duy nhất, sửa perm và lỗi nhỏ."
status: pending
priority: P2
effort: 22h
branch: feat/unified-students-page
tags: [frontend, backend, api, database, navigation, cleanup]
blockedBy: []
blocks: []
created: 2026-09-29
---

# Gỡ trùng lặp sidebar

## Overview

Sidebar vẫn giữ nguyên, kế hoạch chỉ gỡ những chỗ trùng lặp. Nguồn là [brainstorm](../reports/brainstorm-260929-2120-sidebar-duplication-audit.md); mục "Quyết định của người dùng" trong đó là ràng buộc cao nhất. Bằng chứng nằm ở các scout: [nav/reports](../reports/scout-260929-2153-nav-reports.md), [records](../reports/scout-260929-2153-records.md) và [score sets](../reports/scout-260929-2153-score-sets.md).

Ngoài phạm vi: không xoá sidebar, không đổi quyền mặc định, không thiết kế lại trang, không sửa PeriodSwitcher.

## Phases

| # | Phase | Status | Effort | Phụ thuộc |
|---|-------|--------|--------|-----------|
| 1 | [Nav, redirect, đổi tên Gửi thông báo](./phase-01-nav-redirects-and-reports-rename.md) | Completed | 4h | — |
| 2 | [Gỡ Hồ sơ học sinh](./phase-02-remove-student-records.md) | Completed | 2h | — |
| 3 | [Bộ điểm: backend chép khi áp dụng mẫu](./phase-03-score-set-backend.md) | Completed | 6h | — |
| 4 | [Bộ điểm: gỡ trang web và redirect](./phase-04-score-set-web-removal.md) | Completed | 2h | 3 |
| 5 | [Gate và lỗi nhỏ](./phase-05-gates-and-small-bugs.md) | Completed | 3h | 1 |
| 6 | [E2E và docs](./phase-06-e2e-and-docs.md) | Completed | 3h | 1, 2, 3, 4, 5 |
| 7 | [Xoá bảng bộ điểm cũ](./phase-07-drop-legacy-score-set-tables.md) | Pending | 2h | 3, 4 đã lên prod |

## Dependencies

- Phase 1, 2 và 3 sửa các tập file rời nhau nên chạy song song được. Phase 4 cần API của phase 3. Phase 5 sửa lại `class-list-page.tsx`, vốn đã được phase 1 chỉnh, nên phải đi sau phase 1.
- Phase 3 và 4 phải lên prod cùng một bản build. Nếu web cũ gọi `/score-sets` khi API đã bỏ route thì chỉ nhận 404 trên trang sắp bị gỡ, nên đây là rủi ro thấp.
- Phase 7 chỉ bắt đầu khi phase 3 và 4 đã chạy ổn trên prod, để lùi code phase 3 vẫn còn bảng mà dùng. Trước `migrate up` bắt buộc phải backup DB (docs/deployment.md:104-110).
- Mỗi phase là một commit theo conventional commit, không nhắc AI và không có dòng Co-Authored-By. Không push lên master khi chưa được duyệt.

## Acceptance criteria

1. Sidebar không còn "Lớp cần tuyển sinh", "Hồ sơ học sinh", "Gửi báo cáo" và "Cấu hình lớp học". "Phân quyền vai trò" vẫn còn cho chủ trung tâm.
2. Có đúng một mục "Gửi thông báo" (perm `reports.send`, nhóm Học phí) trỏ tới `/reports` cho mọi vai trò, kể cả chủ trung tâm. Mục này sáng khi đang ở `/reports` và ở `/notifications/:periodId`. Tiêu đề route và h1 của `/reports` là "Gửi thông báo".
3. Các redirect hoạt động và giữ query:
   - `/classes/recruiting` → `/classes?view=recruiting`, giữ `q`, `weekday`, `shift`;
   - `/records` → `/students`;
   - `/records/:studentId` → `/students/:studentId`;
   - `/center/class-config` → `/center`.
4. Nhóm "Kho học liệu" được đổi tên thành "Học liệu"; mục "Kho học liệu" bên trong giữ nguyên. Đĩa kỳ hiện tại trên rail chỉ hiện với `billing.read`.
5. Perm của từng mục nav bằng đúng key mà API của trang đích kiểm tra. "Sổ lớp" chuyển sang `teaching.read`.
6. Áp dụng một phiên bản mẫu (PUT `/classes/:id/program`) sẽ chép `score_set` vào `class_score_components` theo quy tắc làm phẳng ở phase 3. Lớp đã có điểm thì giữ nguyên snapshot và điểm. Không còn route `/score-sets` hay `/classes/:id/score-set`.
7. "Thêm học viên" chỉ hiện cho chủ trung tâm.
   - Nút "Tạo lớp" của Sổ lớp mở hộp tạo lớp tại `/classes?create=1`.
   - "Điểm danh & nhận xét" trỏ tới `/sessions?class_id=`.
   - Các comment và docs cũ đã được sửa.
8. Các lệnh sau đều xanh:
   - `make lint-web`, `make test-web`;
   - `make lint-api`, `make test-api-unit`, rồi `make test-api` chạy riêng;
   - `make e2e-isolated` trên stack `teka-e2e`.
9. Không còn chuỗi "Gửi báo cáo" (trừ nhãn quyền "Quyền Gửi báo cáo học phí"), "Lớp cần tuyển sinh", "Cấu hình lớp học" hay "Hồ sơ học sinh" trong `apps/web/src` và `apps/web/e2e`. Kiểm bằng `rg` ở phase 6.

## Quyết định cần người dùng xác nhận

Xem mục "Open decisions" ở cuối từng phase. Các điểm chính:

- nhãn nhóm "Học liệu";
- giữ snapshot khi lớp đã có điểm;
- quy tắc làm phẳng;
- xoá bảng ở phase 7;
- gate "Thêm học viên" theo `isOwner`;
- perm của "Sổ lớp".
