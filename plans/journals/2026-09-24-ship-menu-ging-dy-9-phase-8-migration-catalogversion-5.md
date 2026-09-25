---
title: "Ship menu Giảng dạy: 9 phase, 8 migration, CatalogVersion 5"
date: 2026-09-24
summary: "Hoàn tất plan Giảng dạy (lớp học, lời mời, kho học liệu, khóa học, lộ trình, chuẩn bị tài liệu) trên feat/giang-day-menu; còn push nhánh, mở PR và backup DB production chờ người dùng duyệt."
---

# Ship menu Giảng dạy: 9 phase, 8 migration, CatalogVersion 5

**Date**: 2026-09-24 11:00
**Severity**: Low
**Component**: apps/api (classes, classinvites, library, courses, paths, classprogram, classchat, prep), apps/web (roster, library, courses features), migrations 000025–000032
**Status**: Resolved

## What Happened

Plan `plans/260923-0715-giang-day-menu/plan.md` xong cả 9 phase trên nhánh `feat/giang-day-menu`:
nav "Giảng dạy" + danh sách/chi tiết lớp học, lời mời nhận lớp, kho học liệu (chương trình mẫu, học
liệu, bài tập), danh mục khóa học, lộ trình học, chương trình học của lớp + chat + lịch sử, chuẩn bị
tài liệu (bảng chuẩn bị trên bản nháp, web-only), và Phase 9 seed/e2e/docs/ship. Migration
`000025`–`000032` up/down/up sạch trên DB rỗng và DB đã seed. `CatalogVersion` bump đúng một lần
4→5, khớp `catalog_test.go:316-318` và MSW `handlers.ts:17`. Toàn bộ suite xanh: `go test -tags=integration
-p 1` (coverage 78.5%, floor 60%), `make lint`, `make build-api`, `make build-web`, `make test-web`
(1092 pass), `make api-docs` không tạo diff. Playwright trên stack `teka-e2e` (seed mới): 48/48 pass.

## The Brutal Truth

15.5 ngày effort, 9 phase deep-mode, mỗi phase tự scout lại trước khi làm — nặng nhưng đúng: hai lỗi
thật (GORM Scan, lost update trên PATCH) đều bị bắt bởi chính kỷ luật test/review đó, không phải bởi
may mắn. Không có gì kịch tính để kể — đúng nghĩa "nó chạy được" sau khi làm đủ các bước, không tắt bớt
bước nào. Cái đáng nhớ nhất không phải bug lớn, mà là hai cái nhỏ, âm thầm, kiểu sẽ lọt qua review hời
hợt: một cách scan UUID sai kiểu, và một PATCH ghi đè field không được gửi lên.

## Technical Details

- `apps/api/seeds/seed_test.go`: scan `uuid.UUID` qua `db.Raw(...).Scan(&demoTeacherID)` (GORM) coi
  mảng byte là `[]uint8`, sai kiểu — không lỗi biên dịch, sai lặng lẽ ở runtime. Sửa bằng
  `.Row().Scan(&demoTeacherID)` (commit `b1123bd`). Đường `database/sql` chuẩn, không qua layer struct-map
  của GORM.
- Phase 8 review (`c1781f3`, `c90e4f9`) phát hiện: (1) `PATCH /library/lessons/:lid/prep` ghi đè cả
  field không có trong request — lost update khi hai người sửa checklist/status gần nhau; sửa thành chỉ
  ghi field có mặt trong payload. (2) Thiếu endpoint cho người có `prep.assign` nhưng không có
  `members.list` chọn assignee — sửa bằng cách gate `GET /library/assignees` riêng theo `prep.assign`
  thay vì bắt buộc quyền directory.
- E2E: chạy Playwright trên stack `teka-e2e`, seed mới — statement spec cần seed tươi. Lượt chạy đầu bị
  dừng giữa chừng (không phải lỗi test), lượt hai tiếp tục trên cùng stack, cùng thứ tự seed, kết quả
  48/48 pass — không phải chạy lại từ đầu, môi trường isolated giữ được state đủ để nối tiếp an toàn.

## What We Tried

- GORM `.Scan()` cho UUID → sai kiểu ngầm, không crash, chỉ fail assertion `NotEqual(uuid.Nil, ...)`
  khi giá trị đọc ra khác `uuid.UUID` thật. Đổi sang `Row().Scan()` là fix, không phải workaround.
- PATCH ghi đè full struct cho prep fields → lost update giữa hai request đồng thời hoặc thiếu field.
  Fix cause-aligned: đổi contract để chỉ ghi field có mặt trong request thay vì thêm lock hay optimistic
  concurrency (D5/D9 không yêu cầu, và bảng chỉ có 1 hàng chỉnh sửa nhỏ nên overkill).

## Root Cause Analysis

- UUID scan: GORM `Scan()` trên `db.Raw(...)` map đúng khi đích là field của struct có tag `type:uuid`,
  nhưng scan trực tiếp vào một biến `uuid.UUID` rời thì không đi qua converter đó, rơi về coi cột UUID
  như `[]uint8` — edge case ít được test, dễ đoán nhầm là tương đương `database/sql` thuần.
- Lost update PATCH: thiết kế ban đầu ở phase-08 dùng "PATCH ghi cả object" theo thói quen REST đơn giản,
  không tính đến việc client luôn phải gửi lại field muốn giữ khi có nhiều actor sửa cùng bản nháp.
  Review sau Phase 8 bắt được trước khi ship — đúng lý do tồn tại bước review riêng thay vì tin implement
  ban đầu.

## Lessons Learned

- Không dùng `gorm.DB.Raw(...).Scan(&singleUUIDVar)` cho một biến rời — luôn `.Row().Scan(...)` khi scan
  vào biến đơn (đặc biệt `uuid.UUID`), để đi qua `database/sql` driver thay vì GORM struct mapper.
- PATCH endpoint mặc định phải "chỉ ghi field có trong request", không phải "ghi đè toàn bộ struct" —
  đưa quy tắc này vào review checklist cho mọi PATCH mới thay vì phát hiện lại mỗi lần.
- Chuẩn bị tài liệu (Phase 8) là nơi dễ thiếu endpoint phụ trợ (như `assignees`) khi tách quyền
  `prep.assign` ra khỏi `members.list` — kiểm tra "quyền X có đủ tự vận hành không, không cần quyền Y
  đi kèm" khi thiết kế optIn key mới.

## Next Steps

- Push nhánh `feat/giang-day-menu` và mở PR lên `master` — cần người dùng duyệt (remote SSH ghi
  `cesc1802`, không tự push).
- Sau khi merge: backup DB production (`teka-*` containers) trước khi chạy `migrate-up` 000025–000032;
  rollback sau khi production đã có dữ liệu là forward migration, không chạy down (D10).

> Historical work record — not durable authority. Prefer docs/specs/ADRs for current decisions.
