# Review — Kho học liệu v5 (chưa commit, nhánh feat/giang-day-menu)

Kết luận: nền tảng tenancy, khoá version, route policy vững; 3 lỗi High và nhiều cache react-query cũ.

## High
- H1 `library/repository.go` NextExerciseCode: `max(code)` so chuỗi, mã tự nhập "BT-ABC" làm generator kẹt ở BT-0001 → 409 vĩnh viễn; BT-10000 < BT-9999.
- H2 `library/dto.go:326` bỏ `other` khỏi oneof → sửa học liệu cũ kind `other` luôn 422 (plan muốn giữ hiển thị item cũ).
- H3 `dto.go:128` ListVersions trả `classes: []` → chip lớp tab Phiên bản luôn rỗng; MSW che lỗi.

## Medium
- M4 `repository.go:254-281` lesson_count cộng dồn mọi version published.
- M5 `service.go:767` DuplicateLesson tràn VARCHAR(200) → 500.
- M6 `exercise-dialog.tsx` giới hạn code/skill/level lệch DTO (20/50).
- M7 `use-library.ts` xoá nhóm bài tập không invalidate lesson detail → group_id cũ → 422.
- M8 `use-library.ts` nhiều mutation không invalidate lessons list / version detail / templates / banks.
- M9 `service.go:98-116` N+1 ListVersions trong ListTemplates/GetBoard.
- M10 `service.go:1184-1218` race sinh mã, chỉ retry 1 lần.

## Low
- L1 class_count/ListVersionClasses không lọc lớp đã xoá (nhất quán với TemplateInUse — giữ nguyên).
- L2 comment SetMaterialStatus nói sai về lọc mặc định.
- L3 seed v5 không trong transaction.
- L4 redirect `?tab=` làm rơi query param khác.
- L5 picker giới hạn 100 item (chấp nhận).
- L6 `unit` không trim/rỗng→NULL.

## Câu hỏi mở
- Nhiều version published cùng tồn tại là hành vi có sẵn (Publish không auto-archive) — không đổi trong plan này; M4 sửa bằng cách đếm version published mới nhất.
