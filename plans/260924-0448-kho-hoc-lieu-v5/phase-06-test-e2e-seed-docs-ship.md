---
phase: 6
title: "Test e2e, seed, docs, ship"
status: completed
priority: P1
effort: "1d"
dependencies: [3, 4, 5]
---

# Phase 6: Test e2e, seed, docs, ship

## Context Links
- [plan.md](./plan.md) · Success Criteria.
- e2e: `apps/web/e2e/library.spec.ts` (journey với hậu tố RUN_CODE, `afterEach` dọn dữ liệu, selector theo role),
  `make e2e-isolated` (compose `-p teka-e2e`, port/URL override — cần seed mới cho spec mới).
- Seed: `apps/api/seeds/teaching_menu.go` (`MAU-TOAN8` published, `MAU-VAN9` draft), `apps/api/seeds/seed_test.go`.
- Docs: `docs/api-guidelines.md` §feature tables (mục `library`) và câu `:435` "Six feature modules back the 'Giảng dạy'
  sidebar group", `docs/frontend-guidelines.md:112-120` (danh sách nhóm sidebar + quy tắc đăng ký kép),
  `docs/architecture.md` (đang có diff chưa commit về diagram — không thuộc plan này, không đụng).

## Goal
Chứng minh hành trình v5 chạy end-to-end trên stack cô lập, dữ liệu demo phản ánh field mới, docs nêu hợp đồng mới, và
đóng gói commit/PR đúng quy ước.

## Steps
1. **Seed** (`teaching_menu.go`): sau khi tạo template published, bổ sung
   - 3 material đủ loại mới (`video`, `doc`, `note`) + 1 material `active=false`;
   - 3 exercise có `skill`/`level`, mã tự sinh (kiểm tra `BT-0001…`);
   - 1 nhóm bài tập trên draft `MAU-VAN9`, gán 1 bài tập vào nhóm; 1 buổi `self_study` có `unit` "Unit 1";
   - `score_set` 2 bộ; log field `student`.
   Cập nhật `seed_test.go` assert mã bài tập, nhóm, `mode`. Seed phải idempotent (`find… skip` như hiện có).
2. **e2e `library.spec.ts`** mở rộng journey:
   - hub: 3 tab, card hiện đếm, toggle Ngừng một material rồi kiểm tra picker ở buổi không còn thấy;
   - tạo bài tập không mã → mã `BT-` xuất hiện, copy;
   - chi tiết: thêm nhiều buổi (3), Nhân bản, Cây hiện nhóm unit, thêm nhóm bài tập, gán bài tập vào nhóm ở trang buổi,
     thêm bộ điểm thứ hai, thêm trường nhật ký "Chọn học sinh", Kích hoạt → banner khoá, Tạo bản nháp → nhóm được copy;
   - dọn: xoá template RUN_CODE trong `afterEach` (đã có khuôn).
3. **Chạy toàn bộ**: `make test-api` (serial), `make test-web`, `make lint`, `make e2e-isolated`.
4. **Docs**: `docs/api-guidelines.md` mục `library` — mô tả `code` bài tập tự sinh, `active` flag và 409 in-use, nhóm bài
   tập theo phiên bản, `score_set` là mảng bộ; link tới `library/dto.go` thay vì chép schema; sửa câu `:435` vì hai
   module `library`/`prep` nay nằm dưới nhóm "Kho học liệu" (D13). `docs/frontend-guidelines.md:112-120`: thêm nhóm
   "Kho học liệu" (4 mục, sau "Giảng dạy") vào danh sách nhóm sidebar và ghi route con `/library/materials|exercises`.
5. **Plan bookkeeping**: đánh dấu phase `completed` (hand-edit nếu `ak plan phase close` báo "plan not found"), `ak plan reindex`.
6. **Ship**: commit theo conventional commits, tách theo phase (`feat(api): …`, `feat(web): …`, `test(e2e): …`,
   `docs(api): …`), **không** thêm AI reference; push nhánh `feat/giang-day-menu` qua SSH remote; PR/merge vào `master`
   cần user duyệt (memory rule).

## Files
- Modify: `apps/api/seeds/teaching_menu.go`, `apps/api/seeds/seed_test.go`, `apps/web/e2e/library.spec.ts`,
  `docs/api-guidelines.md`, `docs/frontend-guidelines.md`, plan status files.

## Verification
```bash
make test-api          # serial, chạy một mình
make test-web && make lint
make e2e-isolated
make seed && make seed   # idempotent: lần 2 không tạo trùng
```

## Success Criteria
- [x] `make e2e-isolated` xanh với spec mở rộng; `afterEach` không để lại template RUN_CODE.
- [x] Seed chạy 2 lần không nhân đôi dữ liệu; `seed_test.go` xanh.
- [x] Docs API nêu đúng hợp đồng mới, link tới file nguồn; frontend-guidelines liệt kê nhóm "Kho học liệu".
- [x] Commit không có AI reference; push nhánh feature xong, PR chờ user duyệt.
