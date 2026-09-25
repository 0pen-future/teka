---
phase: 4
title: "Web — chi tiết chương trình mẫu (7 tab)"
status: completed
priority: P1
effort: "2.5d"
dependencies: [2]
---

# Phase 4: Web — chi tiết chương trình mẫu

## Context Links
- [plan.md](./plan.md) · D2, D6, D7, D8, D9, D11 · v5 màn `td`.
- Hiện trạng: `pages/template-detail-page.tsx` (`HvSelect` phiên bản, `LessonsTable`, `LogFieldsEditor`, `ScoreSetEditor`,
  nút publish/archive/new draft), `components/lessons-table.tsx`, `log-fields-editor.tsx`, `score-set-editor.tsx`.
- Hooks có sẵn: `useTemplate`, `useVersions`, `useVersionDetail`, `useLessons`, `useCreateLesson`, `useDeleteLesson`,
  `useReorderLessons`, `usePublishVersion`, `useArchiveVersion`, `useCreateVersion`, `useSetLogFields`, `useSetScoreSet`;
  Phase 2 thêm `useDuplicateLesson`, `useClearLessons`, `useExerciseGroups`, `useCreateExerciseGroup`, `useDeleteExerciseGroup`.

## Goal
Trang `/library/templates/:id` theo v5: chọn phiên bản bằng chip, banner ngữ cảnh, 7 tab đồng bộ `?tab=`, tab Buổi học có
Bảng/Cây + menu thêm + nhân bản + xoá tất cả, hai tab aggregate, tab Nhóm bài tập, Bộ điểm nhiều bộ, Nhật ký 4 loại,
Phiên bản có lịch sử + lớp đang gắn.

## Layout (v5 `td`)
- Back link "← Kho học liệu". Header: tên + `StatusPill` (status phiên bản đang chọn) + "Sửa" (`TemplateDialog`) + "Xoá"
  (`HvConfirmDialog`, disabled khi `class_count > 0`).
- Hàng "PHIÊN BẢN": `HvChip` mỗi phiên bản (`versionLabel`), active = chọn; `?v=:vid` giữ trong URL; mặc định `defaultVersion`.
  Nút "+ Bản nháp mới" (`useCreateVersion`) khi không có draft.
- Banner ngữ cảnh (`HvNotice`):
  - draft: *"Bản nháp — chỉnh sửa tự do. Kích hoạt để lớp có thể gắn."* + action "Kích hoạt" (`has("library.publish")`).
  - published: *"Phiên bản đã phát hành ({class_count} lớp đang gắn) — chỉ xem. Muốn sửa, tạo bản nháp mới."* + action
    "Tạo bản nháp" (ẩn nếu đã có draft) và "Ngừng" (`useArchiveVersion`, confirm).
  - archived: *"Phiên bản đã ngừng — chỉ xem."*
- Tabs (`HvSegmented variant="tabs"`, `?tab=`, `tabSchema.catch("lessons")`):
  `lessons | exercises | materials | groups | scores | logs | versions`.

## Tab Buổi học (`components/lessons-tab.tsx`)
- Toolbar: `HvSegmented` **Bảng / Cây**; đếm "{n} buổi · {m} phút"; search "Tìm buổi theo tiêu đề…" (client-side filter);
  "Xoá tất cả" (`HvConfirmDialog`, `useClearLessons`, chỉ draft, ẩn khi 0 buổi); menu **"+ Buổi học ▾"** (`DropdownMenu`
  shadcn) với 2 mục (v5 có 3, mục "Nhập từ file" là non-goal D12):
  1. "Thêm một buổi" — sub "Mở form tiêu đề, hình thức, thời lượng" → `LessonDialog` (bổ sung `mode`, `unit`).
  2. "Thêm nhiều buổi" — sub "Nhập số buổi, tạo tiêu đề Buổi N" → `HvModal` số lượng 1–50, gọi `useCreateLesson` tuần tự
     (`for … await`) với title "Buổi {position}", `mode = scheduled`; invalidate một lần khi xong.
- **Bảng** (`LessonsTable` viết lại): STT · BUỔI HỌC (title link → lesson page, dòng phụ `unit`) · HÌNH THỨC
  (`lessonModeLabel` pill) · PHÚT · BÀI TẬP (`exercise_count`) · NỘI DUNG (`material_count`) · thao tác ↑↓ (giữ),
  **Nhân bản** (`useDuplicateLesson`), **Xoá** (confirm). Empty: *"Chưa có buổi mẫu nào — thêm buổi hoặc nhập từ file."*
  (giữ nguyên câu v5, mục nhập từ file chưa có).
- **Cây** (`components/lessons-tree.tsx`): group theo `unit` (null → "Chưa phân đơn vị"), caret mở/đóng (state cục bộ),
  meta "{k} buổi · {phút} phút", mỗi buổi hiện "{material_count} nội dung · {exercise_count} bài". Chỉ đọc; thao tác ở bảng.
- Thao tác ghi ẩn khi phiên bản khoá (`status !== "draft"`) hoặc `!has("library.edit")`.

## Tab Bài tập / Tài liệu (aggregate, `components/version-exercises-tab.tsx`, `version-materials-tab.tsx`)
- Nguồn: `useVersionDetail(vid)` → flatten `lessons[].exercises` / `lessons[].materials`, dedupe theo id, đếm số buổi dùng.
- Bài tập: STT · MÃ (copy) · TIÊU ĐỀ BÀI TẬP · KỸ NĂNG · CẤP ĐỘ · DÙNG TRONG ("{k} buổi" + tooltip danh sách tiêu đề buổi).
  Search client-side theo tên/mã; empty *"Không có bài tập nào khớp."*
- Tài liệu: STT · LOẠI (icon+label) · TÀI LIỆU (title → link ngoài) · ĐỊNH DẠNG · DÙNG TRONG.

## Tab Nhóm bài tập (`components/exercise-groups-tab.tsx`)
- Câu dẫn: *"Mỗi bài tập trong buổi mẫu gán vào một nhóm. Nhóm gắn theo phiên bản."*
- Danh sách: tên · "Dùng trong {exercise_count} bài" · "Xóa" (confirm khi `exercise_count > 0`: *"Bài tập trong nhóm sẽ về
  'Chưa phân nhóm'"*). Hàng thêm: input "Tên nhóm mới…" + "+ Thêm nhóm" (Enter submit).

## Tab Bộ điểm (`components/score-sets-editor.tsx` thay `score-set-editor.tsx`)
- Nhiều bộ: mỗi bộ = `HvCard` với tiêu đề (input), dòng tổng trọng số `sumL` ("Tổng trọng số: {sum}%" — cảnh báo màu
  coral khi ≠ 100), hint, danh sách thành phần (label, max, weight %, Xóa), "+ Thêm điểm thành phần"; "+ Thêm bộ điểm";
  "Xóa bộ". Lưu wholesale bằng `useSetScoreSet` (body mảng bộ). Key sinh từ slug tiêu đề + suffix chống trùng (giữ key cũ khi sửa).
- Ghi chú: *"Hai lớp dùng hai phiên bản khác nhau có thể có cấu trúc điểm khác nhau — báo cáo join qua phiên bản, không qua khóa."*
- Read-only khi khoá (`ScoreSetsReadOnly`).

## Tab Nhật ký (`log-fields-editor.tsx` sửa)
- Hàng: label · `logFieldKindLabel` · toggle "Bắt buộc" · Xóa. Hàng thêm inline: input placeholder
  "Tên trường (VD: Học sinh cần lưu ý)" + select 4 loại v5 (`text` Văn bản, `long_text` Đoạn dài, `checkbox` Tick,
  `student` Chọn học sinh) + "+ Thêm trường". Field cũ `number`/`select` vẫn render (nhãn Số / Lựa chọn) nhưng không có trong select thêm mới; `select` giữ editor options hiện có.

## Tab Phiên bản (`components/versions-tab.tsx`)
- Mỗi hàng: "v{n}" · `StatusPill` · ngày (`published_at` ?? `created_at`) · changelog · chip lớp (`classes[]`, link
  `/classes/:id`) hoặc *"Chưa có lớp nào gắn"* · thao tác: draft → "Kích hoạt" (`usePublishVersion`, gate publish),
  published → "Ngừng" (`useArchiveVersion`, confirm nêu số lớp). Chọn hàng = chuyển `?v=`.

## Tests (`__tests__/template-detail-page.test.tsx` viết lại)
- Chip phiên bản đổi `?v=`; banner đúng theo status; 7 tab đổi `?tab=`.
- Buổi học: Bảng↔Cây; "Thêm nhiều buổi" gọi POST N lần; Nhân bản gọi `/duplicate`; Xoá tất cả gọi DELETE `/lessons` sau confirm;
  thao tác ẩn khi published.
- Aggregate: dedupe đúng, đếm "dùng trong".
- Nhóm: thêm/xoá gọi đúng route; xoá nhóm có bài tập yêu cầu confirm.
- Bộ điểm: thêm bộ, tổng trọng số cảnh báo khi ≠ 100, PUT body dạng mảng bộ.
- Nhật ký: thêm trường `student`; Phiên bản: chip lớp + Kích hoạt.

## Files
- Create: `components/lessons-tab.tsx`, `components/lessons-tree.tsx`, `components/version-exercises-tab.tsx`,
  `components/version-materials-tab.tsx`, `components/exercise-groups-tab.tsx`, `components/score-sets-editor.tsx`,
  `components/versions-tab.tsx`, `components/version-chips.tsx`, `components/version-banner.tsx`.
- Modify: `pages/template-detail-page.tsx`, `components/lessons-table.tsx`, `components/lesson-form.tsx` (`mode`, `unit`),
  `components/log-fields-editor.tsx`, `__tests__/template-detail-page.test.tsx`.
- Delete: `components/score-set-editor.tsx` (sau khi editor mới thay thế; Phase 2 giữ nó chạy tạm).

## Verification
```bash
cd apps/web && npx vitest run src/features/library/__tests__/template-detail-page.test.tsx && npx tsc --noEmit
make lint-web
```

## Success Criteria
- [x] 7 tab đúng thứ tự v5, URL `?v=&tab=` bookmark được.
- [x] Bảng/Cây, Nhân bản, Xoá tất cả, "+ Buổi học ▾" hoạt động và bị ẩn khi phiên bản khoá.
- [x] Tab Bài tập/Tài liệu là aggregate không gọi route mới.
- [x] Bộ điểm nhiều bộ lưu và đọc lại đúng; tổng trọng số hiển thị.
- [x] Tab Phiên bản hiện lớp đang gắn và cho Kích hoạt/Ngừng đúng gate quyền.
