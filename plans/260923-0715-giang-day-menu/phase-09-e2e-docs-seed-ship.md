---
phase: 9
title: "Seed, e2e, docs & ship"
status: in-progress
priority: P1
effort: "1.5d"
dependencies: [1, 2, 3, 4, 5, 6, 7, 8]
---

# Phase 9: Seed, e2e, docs & ship

> Outline (deep mode). Chạy sau khi các phase chức năng merge vào `feat/giang-day-menu`. Phase 8 đã xác nhận trong scope
> (Validation Session 1, Q2). <!-- Updated: Validation Session 1 - Q2 -->

## Goal
Dữ liệu seed đủ để demo toàn menu Giảng dạy; bộ e2e phủ luồng chính; tài liệu kiến trúc/API/frontend cập nhật;
PR lên `master`.

## Tasks
1. **Seeds** (`apps/api/seeds/seed.go`): 2 chương trình mẫu (1 published, 1 draft), 3 khóa học, 1 lộ trình 3 giai
   đoạn, lớp hiện có gán `code` (qua `classcode`)/`tags`/`course_id`, 1 lớp đã áp dụng chương trình, 2 lời mời
   (`pending`/`accepted`), tài khoản GV demo được cấp `library.edit`, `courses.edit`, `prep.assign` (optIn), 1 template có draft
   đang chuẩn bị. `make seed` idempotent (kiểm tra theo `code`).
2. **e2e** (`apps/web/e2e/`): `class-list.spec.ts` (lọc, chip, mở chi tiết, tạo lớp), `class-invitations.spec.ts`
   (mời → accept → GV nhận lớp), `library.spec.ts`, `courses.spec.ts`, `paths.spec.ts`, `prep.spec.ts`; chạy `make e2e-isolated`
   (stack riêng `teka-e2e`, seed mới — statement specs cần seed tươi).
3. **Docs** (`docs/`): `docs/api-guidelines.md` mục feature modules mới (`classinvites`, `library`, `courses`, `paths`,
   `classprogram`, `classchat`) + hợp đồng lọc `/classes` + quy ước D8 (FK composite `center_id`); `docs/frontend-guidelines.md`
   nav group "Giảng dạy", 2 feature web mới, quy ước import chéo `roster`↔`courses`↔`library`; `docs/architecture.md`
   mục feature điều phối (`classprogram`/`classchat` đứng trên `classes`+`teaching`+`library`, cùng khuôn `handoff`);
   `docs/adding-permissions.md` không đổi (chỉ tham chiếu). Cập nhật `apps/api/CLAUDE.md` nếu liệt kê key quyền/migration mới nhất.
4. **Kiểm tra catalog một lần**: `CatalogVersion` = 5, `catalog_test.go:316-318` và `handlers.ts:17` khớp; trang phân quyền
   hiển thị đủ nhóm mới nhờ `RESOURCE_LABELS`. <!-- Red Team S1 F7 -->
5. `make api-docs` (swagger), `make lint`, `make test-web`, `make test-api-unit`, `make test-api` (serial), `make build-api`, `make build-web`;
   `make migrate-down` toàn bộ 000032→000025 rồi `migrate-up` lại trên DB dev đã seed (down migration là hợp đồng — D10). <!-- Red Team S1 F8 -->
6. **Ship**: branch `feat/giang-day-menu`, commit conventional không AI reference, PR qua `gh` (remote SSH cesc1802 để push —
   **push cần user duyệt**), mô tả PR liệt kê migration 000025–000032 và key quyền mới (CatalogVersion 5).
7. Sau merge: ghi chú backup DB production trước `migrate-up`; rollback sau khi đã có dữ liệu production = **forward migration**,
   không chạy down (D10). Production `teka-*` containers — không tự chạy.

## Verification
- Toàn bộ suite xanh; e2e isolated xanh; `ak plan status` mọi phase completed; `/ak:journal`.

## Risks
- e2e phụ thuộc seed tươi và thứ tự serial; tách spec theo dữ liệu riêng (prefix `E2E-`).
- Tổng 8 migration: viết `docs/` phần "nâng cấp" hướng dẫn `make migrate-status` trước/sau.

## Verification notes (2026-09-24)

- `go test -tags=integration -p 1` toàn bộ API: pass, coverage 78.5% (floor 60%). Test seed mới từng lỗi do scan `uuid.UUID` qua GORM; sửa ở `b1123bd`.
- `make lint`, `make build-api`, `make build-web`, `make test-web` (1092 pass): pass. `make api-docs` không tạo diff.
- `CatalogVersion` = 5, khớp `CATALOG_VERSION` trong MSW mirror.
- Playwright trên stack `teka-e2e` với seed mới: 48/48 pass (chạy thành hai lượt do lượt đầu bị dừng giữa chừng; lượt hai tiếp tục trên cùng stack, cùng thứ tự).
- Migration trên DB `teka-e2e` đã seed (có backup trước): down 32→24, up lại 32, không dirty; reseed idempotent.
- Còn lại: push nhánh + mở PR (cần người dùng duyệt), và bước sau merge (backup DB production do người dùng chạy).
