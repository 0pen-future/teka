---
phase: 5
title: "Web — buổi học mẫu (Thông tin / Bài tập)"
status: completed
priority: P1
effort: "2d"
dependencies: [2]
---

# Phase 5: Web — buổi học mẫu

## Context Links
- [plan.md](./plan.md) · D2, D3, D6, D7, D12 · v5 màn `ts`.
- Hiện trạng: `pages/template-lesson-page.tsx` (`LessonFields` + `LessonAttachments` + `LessonPrepPanel`, `useLessonForm`,
  `useUpdateLesson`), `components/lesson-attachments.tsx` (2 checklist picker, `useSearch` debounce), `material-dialog.tsx`,
  `exercise-dialog.tsx`.
- Hooks: `useLesson`, `useUpdateLesson`, `useSetLessonMaterials`, `useSetLessonExercises`, `useMaterialsList`, `useExercisesList`,
  `useCreateMaterial`, `useCreateExercise`, `useUpdateMaterial`, `useExerciseGroups` (Phase 2).

## Goal
Trang `/library/templates/:id/lessons/:lessonId` theo v5: hai tab Thông tin / Bài tập, khối thông tin view↔edit với
Hình thức và thời lượng bước 15, danh sách nội dung theo loại có toggle chia sẻ và sắp xếp, menu thêm nội dung (tạo mới 7
loại hoặc chọn từ ngân hàng), tab bài tập có tự soạn / có sẵn / gán nhóm. Giữ `LessonPrepPanel` (chuẩn bị) dưới tab Thông tin.

## Layout (v5 `ts`)
- Header: badge "Buổi {position}", tiêu đề, `versionLabel`, crumb "Kho học liệu › {template} › v{n}".
- Banner khoá khi `status !== "draft"` (`HvNotice`): *"Phiên bản đã phát hành ({class_count} lớp đang gắn) — chỉ xem. Muốn
  sửa, tạo bản nháp mới ở màn chương trình mẫu."* (archived: "đã ngừng"). Khoá = ẩn mọi nút ghi.
- Tabs `HvSegmented`: **Thông tin** · **Bài tập** (`?tab=info|exercises`).

## Tab Thông tin
### Khối "Thông tin chung" (`components/lesson-info-card.tsx`)
- Header: tiêu đề khối + pill `lessonModeLabel` + nút "Sửa"/"Huỷ".
- View: Hình thức · Thời lượng (`formatDuration`) · Mô tả ngắn (`objectives`) · Bài tập về nhà (`homework_note`, giữ field cũ) · Đơn vị (`unit`).
- Edit (`LessonFields` mở rộng, `useLessonForm`): Tiêu đề; Hình thức = 2 nút toggle "Buổi học có lịch" / "Không lịch"
  (`HvSegmented`); Thời lượng `HvScoreInput`-style number step 15 (min 15); Đơn vị; Mô tả ngắn textarea placeholder
  "Mục tiêu buổi học, lưu ý cho giáo viên…"; BTVN. Ghi chú: *"'Không lịch' = buổi tự học, không chiếm slot thời khóa biểu
  khi sinh lịch cho lớp."* Lưu → `useUpdateLesson`; lỗi API qua `useApiFormErrors`.
### Khối "Nội dung buổi học" (`components/lesson-contents.tsx` thay `LessonAttachments` phần materials)
- Header: tiêu đề + mô tả *"video, audio, hình ảnh, tài liệu, ghi chú, buổi học trực tuyến, liên kết ngoài"* + toggle
  "Sắp xếp thứ tự" + menu **"+ Thêm nội dung"** (`DropdownMenu`): nhóm "TẠO MỚI" 7 loại (mở `MaterialDialog` với `kind`
  prefill; `onCreated` → append vào danh sách rồi `useSetLessonMaterials` wholesale) và mục "Chọn từ ngân hàng nội dung"
  (`HvModal` picker: search, filter loại, chỉ `active=true`, checkbox nhiều, "Thêm {n} nội dung").
- Item row: icon theo `materialKindIcon` · title (+ badge "Ngừng" nếu `active=false`) · toggle "Chia sẻ cho học viên"
  (`shared_with_students`, lưu ngay qua wholesale PUT) · meta (`materialFormatLabel`) · caret mở rộng (Mô tả, link mở
  ngoài) · "Sửa" (mở `MaterialDialog` sửa item ngân hàng, `HvNotice` cảnh báo *"Sửa nội dung này sẽ ảnh hưởng mọi buổi đang
  dùng"*) · "Gỡ khỏi buổi". "+ Đính kèm file" **không làm** (D12) — thay bằng "Mở liên kết".
- Sắp xếp: khi bật, mỗi hàng có ▲▼ đổi `position` cục bộ; tắt → PUT một lần. Empty: *"Chưa thêm nội dung cho buổi học mẫu."*
### `LessonPrepPanel` giữ nguyên, đặt dưới cùng tab Thông tin.

## Tab Bài tập (`components/lesson-exercises.tsx`)
- Toolbar: "+ Bài tập tự soạn" (mở `ExerciseDialog`, tạo xong tự gắn) · primary "+ Bài tập có sẵn" (`HvModal` picker: search
  "Tìm kiếm theo tên hoặc mã bài tập", chỉ `active=true`, chọn nhiều) · search cục bộ.
- Bảng: STT · TIÊU ĐỀ BÀI TẬP (title; dòng phụ "{skill} · {level}"; nút copy mã) · NHÓM BÀI TẬP (`HvSelect` từ
  `useExerciseGroups(versionId)`, option "Chưa phân nhóm" = null; đổi → wholesale PUT với `group_id`) · thao tác
  "Ngân hàng" (link `/library?tab=exercises&q={code}`) · "Gỡ".
- Empty: *"Chưa có bài tập nào trong buổi."* Khoá: select disabled, nút ẩn.

## Tests (`__tests__/template-lesson-page.test.tsx` viết lại)
- Banner khoá + ẩn nút ghi khi published.
- Info: view↔edit, chọn "Không lịch" gửi `mode: "self_study"`, thời lượng step 15.
- Nội dung: tạo mới từ menu → POST material rồi PUT materials chứa id mới; picker lọc `active=true`; toggle chia sẻ PUT
  đúng `shared_with_students`; sắp xếp ▲▼ rồi tắt → PUT thứ tự mới; Gỡ.
- Bài tập: tự soạn → POST rồi PUT; đổi nhóm → PUT có `group_id`; Gỡ.

## Files
- Create: `components/lesson-info-card.tsx`, `components/lesson-contents.tsx`, `components/lesson-exercises.tsx`,
  `components/bank-picker-modal.tsx` (dùng chung cho nội dung và bài tập: generic theo `items`, `renderRow`, `search`).
- Modify: `pages/template-lesson-page.tsx`, `components/lesson-form.tsx` (`LessonFields` + mode/unit), `hooks/use-lesson-form.ts`,
  `components/material-dialog.tsx` (prop `defaultKind`, `onCreated`), `components/exercise-dialog.tsx` (`onCreated`),
  `__tests__/template-lesson-page.test.tsx`.
- Delete: `components/lesson-attachments.tsx`.

## Verification
```bash
cd apps/web && npx vitest run src/features/library/__tests__/template-lesson-page.test.tsx && npx tsc --noEmit
make lint-web
```

## Success Criteria
- [x] Hai tab, view↔edit, Hình thức và thời lượng bước 15 lưu đúng qua `PUT /library/lessons/:lid`.
- [x] Thêm nội dung bằng tạo mới hoặc chọn từ ngân hàng; item ngừng hoạt động không xuất hiện trong picker nhưng vẫn hiển thị nếu đã gắn.
- [x] Toggle chia sẻ, sắp xếp, gỡ đều dùng wholesale PUT hiện có; không route mới.
- [x] Bài tập gán nhóm theo phiên bản; "Ngân hàng" điều hướng đúng.
- [x] Phiên bản khoá: chỉ xem, banner nêu số lớp.
