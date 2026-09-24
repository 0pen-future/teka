---
title: "Kho học liệu v5 — ngân hàng nội dung, ngân hàng bài tập, chương trình mẫu"
description: "Nâng menu Kho học liệu lên đúng prototype v5: ba ngân hàng, thẻ chương trình mẫu có đếm lớp/phiên bản, 7 tab chi tiết phiên bản, nhóm bài tập, nhiều bộ điểm, buổi học có hình thức và cây đơn vị."
status: completed
priority: P1
effort: "11d"
issue:
branch: feat/giang-day-menu
tags: [feature, frontend, backend, database, api, library, navigation]
blockedBy: [260923-0715-giang-day-menu]
blocks: []
created: 2026-09-24
---

# Kho học liệu v5

## Overview

Menu **Kho học liệu** hiện đã có nền tảng dữ liệu (plan `260923-0715-giang-day-menu`, Phase 3/4/8):
`program_templates → program_template_versions → template_lessons`, ngân hàng `library_materials` /
`library_exercises`, bảng nối `template_lesson_materials` / `template_lesson_exercises`, `template_log_fields`,
`score_set` JSONB trên phiên bản, và `class_programs` gắn lớp với phiên bản. Web đang render hub 4 tab, trang chi tiết
chương trình mẫu (chọn phiên bản bằng `HvSelect`, bảng buổi, editor Nhật ký & Điểm) và trang buổi học mẫu (form + checklist
đính kèm + panel chuẩn bị).

Prototype **So Lop v5** (`claude.ai/design` project `4a7e6c77…`, file `So Lop - Prototype v5.dc.html`, ba màn `lib`,
`td`, `ts`) tái cấu trúc menu này quanh **ba cấp**: *ngân hàng nội dung* + *ngân hàng bài tập* + *chương trình mẫu chỉ tham
chiếu, không giữ bản sao*. Plan này đưa API + web lên đúng v5, giữ nguyên các bất biến đã chốt (phiên bản published bất
biến, một draft/template, wholesale replace cho danh sách đính kèm) và không mở permission key mới.

Nguồn hiện trạng: [`../reports/scout-260924-1140-library-current-state.md`](../reports/scout-260924-1140-library-current-state.md).
Design system "Học Vui Mỗi Ngày" (Baloo 2 + Nunito, token cream/ink/mint/sky/sun/coral) đã được ánh xạ sang bộ `Hv*`
trong `apps/web/src/components/hv/` — không thêm token mới.

## Gap v5 ↔ hiện trạng

| Màn v5 | v5 yêu cầu | Hiện có | Việc cần làm |
|---|---|---|---|
| Sidebar | Nhóm riêng **KHO HỌC LIỆU** (header uppercase như các nhóm khác) | "Kho học liệu" là 1 mục trong nhóm "Giảng dạy" (`dashboard-layout.tsx:92-111`) | Nhóm mới `{ header: "Kho học liệu" }` sau "Giảng dạy", 4 mục (D13); đăng ký kép `OVERFLOW_LABELS`/`OVERFLOW_PATH_PREFIXES`; test nhóm |
| Hub `lib` | 3 tab: Chương trình mẫu · Ngân hàng nội dung · Ngân hàng bài tập | 4 tab (có "Buổi học mẫu" duyệt read-only) qua `?tab=` | Bỏ tab Buổi học mẫu, đổi nhãn; tab thành route con `/library` · `/library/materials` · `/library/exercises` (D13), `?tab=` cũ redirect |
| Hub · thẻ chương trình | Card: tên, pill trạng thái, Sửa, N buổi mẫu, N phiên bản, N lớp đang gắn, chip phiên bản | Bảng 5 cột, không có số lớp | `TemplateResponse` thêm `class_count`, `lesson_count`, `versions[]`; card UI |
| Hub · ngân hàng nội dung | LOẠI (7 loại) · TIÊU ĐỀ · ĐỊNH DẠNG · DÙNG TRONG · TRẠNG THÁI, nút ngừng/kích hoạt | kind 4 giá trị, không active, không đếm dùng | Migration mở rộng `kind`, thêm `active`, `lesson_count`/`template_count`, route `PATCH .../status` |
| Hub · ngân hàng bài tập | MÃ (copy) · BÀI TẬP · KỸ NĂNG · CẤP ĐỘ · DÙNG TRONG · TRẠNG THÁI | title/description/difficulty/tags | Thêm `code` (unique/center, tự sinh), `skill`, `level`, `active`, đếm dùng |
| Chi tiết `td` | Chip phiên bản + banner ngữ cảnh + 7 tab | `HvSelect` + 2 khối editor | Tabs Buổi học · Bài tập · Tài liệu · Nhóm bài tập · Bộ điểm · Nhật ký · Phiên bản |
| `td` · Buổi học | Bảng/Cây, HÌNH THỨC, PHÚT, đếm BT/nội dung, Nhân bản, Xoá tất cả, menu "+ Buổi học ▾" | Bảng STT/Tên/Thời lượng/BTVN, ↑↓, Xoá | `mode`, `unit`, count fields; route duplicate + clear-all |
| `td` · Nhóm bài tập | Nhóm theo phiên bản; mỗi bài tập trong buổi gán 1 nhóm | Không có | Bảng `template_exercise_groups` + `group_id` trên bảng nối |
| `td` · Bộ điểm | Nhiều bộ điểm/phiên bản, mỗi bộ có tiêu đề + tổng trọng số | 1 `score_set` JSONB/phiên bản | Reshape JSONB thành mảng bộ; editor nhiều bộ |
| `td` · Nhật ký | Loại: Văn bản · Đoạn dài · Tick · Chọn học sinh | text/number/select/checkbox | Thêm `long_text`, `student` (giữ 2 loại cũ) |
| `td` · Phiên bản | Lịch sử + chip lớp đang gắn + Kích hoạt/Ngừng | Danh sách qua `HvSelect`; publish/archive nút rời | `VersionResponse.classes[]`, tab lịch sử |
| Buổi học `ts` | Tab Thông tin/Bài tập, view↔edit, Hình thức "Có lịch/Không lịch", bước 15', banner khoá, nội dung có icon theo loại + toggle chia sẻ + sắp xếp, menu "+ Thêm nội dung" (tạo mới 7 loại / chọn từ ngân hàng), tab Bài tập với nhóm | Form + 2 checklist picker + prep panel | Viết lại 2 tab, giữ `LessonPrepPanel` |

## Quyết định thiết kế

| # | Quyết định | Lý do |
|---|---|---|
| D1 | **Không thêm permission key**; mọi route mới dùng `library.read` / `library.edit` / `library.publish` hiện có. Không bump `CatalogVersion`. | v5 không thêm vai trò mới; giữ `authctx/catalog.go` và MSW mirror ổn định. |
| D2 | **Phiên bản published vẫn bất biến** (409 `VERSION_LOCKED`), khoá theo trạng thái chứ không theo "có lớp gắn". Banner khoá đổi nội dung để hiển thị số lớp. | Kế thừa D3 plan trước; khoá theo lớp sẽ tạo đường sửa lén nội dung lớp đang chạy. |
| D3 | Ngân hàng dùng **`active` flag** thay cho xoá cứng khi còn tham chiếu; `DELETE` giữ 409 `MATERIAL_IN_USE`/`EXERCISE_IN_USE`. Item ngừng hoạt động ẩn khỏi picker nhưng vẫn hiển thị trong bảng ngân hàng và trong buổi đã gắn. | Đúng ghi chú v5 "chỉ ngừng hoạt động". |
| D4 | `library_materials.kind` mở rộng thành `video · audio · image · doc · note · live · link`, **giữ `other`** cho dữ liệu cũ (nhãn "Khác", không cho chọn mới). Cột ĐỊNH DẠNG suy từ URL phía client (đuôi file / host). | Không cần cột mới; migration additive, down chỉ thu hẹp CHECK sau khi map ngược. |
| D5 | `library_exercises.code` NOT NULL, unique per center (live rows), backfill `BT-0001…` theo `created_at`; service tự sinh mã kế tiếp khi request để trống. `skill`, `level` là text tự do nullable; **giữ `difficulty`** làm cột riêng. | CẤP ĐỘ trong v5 là nhãn ("A2", "Cơ bản"), khác thang 1–5; không phá dữ liệu cũ. |
| D6 | `template_lessons.mode` (`scheduled` \| `self_study`, default `scheduled`) và `unit` (text, nullable) — cây ở tab Buổi học nhóm theo `unit`, buổi không có unit vào nhóm "Chưa phân đơn vị". `classprogram.Apply` **vẫn copy mọi buổi** vào `class_curricula`; việc bỏ qua `self_study` khi sinh lịch là scope của menu Lớp, không phải plan này. | Giữ hợp đồng `PutCurriculum`; ghi rõ non-goal. |
| D7 | Nhóm bài tập = bảng `template_exercise_groups(version_id, name, position)` + `template_lesson_exercises.group_id` nullable `ON DELETE SET NULL`. Tạo draft mới copy nhóm và ánh xạ lại `group_id`. | Nhóm "gắn theo phiên bản" đúng v5; SET NULL khiến xoá nhóm không phá bài tập. |
| D8 | `score_set` JSONB đổi hình dạng thành **mảng bộ điểm** `[{key,title,components:[{key,label,max,weight}]}]`; migration bọc mảng cũ vào một bộ `main` "Bộ điểm"; down lấy bộ đầu tiên. Không liên quan đến `grading.score_sets` (bộ điểm cấp trung tâm gán cho lớp). | Chỉ `library` đọc cột này (đã grep); giữ tách biệt với `grading` để không gộp hai khái niệm. |
| D9 | Log-field kind thêm `long_text`, `student`; giữ `number`, `select` cho dữ liệu cũ (UI vẫn hiển thị nhãn nhưng select thêm-mới chỉ 4 loại v5). | Additive, không cần backfill. |
| D10 | Mọi route mutating mới khai `req(action, entity, idParam)` và cập nhật cả `route_policy_snapshot_test.go` lẫn `audit/action_test.go`; migration có down đầy đủ; FK composite `(id, center_id)`. | Kế thừa D8/D9/D10 plan trước. |
| D11 | Aggregate tab **Bài tập** / **Tài liệu** ở chi tiết phiên bản tính **client-side** từ `GET /library/versions/:vid` (đã trả lessons kèm materials/exercises). | Tránh 2 endpoint đọc mới; payload phiên bản hiện đã đủ. |
| D12 | **Non-goal**: upload file ("+ Đính kèm file"), "Nhập từ file" trong menu "+ Buổi học ▾", drag-and-drop. Nội dung vẫn là URL. | Repo không có object storage; multipart chỉ tồn tại ở `imports`. |
| D13 | Sidebar có nhóm riêng `{ header: "Kho học liệu" }` (render uppercase → "KHO HỌC LIỆU") đặt ngay sau "Giảng dạy", gồm 4 mục: **Chương trình mẫu** → `/library`, **Ngân hàng nội dung** → `/library/materials`, **Ngân hàng bài tập** → `/library/exercises`, **Chuẩn bị tài liệu** → `/prep` (3 mục đầu `perm: "library.read"`, mục cuối giữ perm hiện có). Hai mục "Kho học liệu"/"Chuẩn bị tài liệu" rời khỏi "Giảng dạy"; "Danh mục khóa học"/"Lộ trình học" ở lại. Tab hub đổi từ `?tab=` sang **route con** (`library/materials`, `library/exercises` trong `features/library/routes.tsx`), `LibraryPage` nhận `tab` prop; `?tab=materials|exercises` cũ → `<Navigate replace>` sang route con, `?tab=lessons`/không tab → templates. Cả 4 mục vào `OVERFLOW_LABELS`; `OVERFLOW_PATH_PREFIXES` giữ `/library`, `/prep`. | Yêu cầu user 2026-09-24 (nhóm riêng "KHO HỌC LIỆU"); nhóm 1 mục trùng tên header là vô nghĩa nên mỗi tab hub thành 1 mục. `useNavActive` (`dashboard-layout.tsx:258`) chỉ so `pathname` theo tiền tố dài nhất qua `NavPathsContext` → route con cho active-state đúng mà không thêm logic query; `/library/templates/:id` vẫn sáng mục Chương trình mẫu. Bottom bar <md không đổi (cả 4 đều overflow, `Thêm` active nhờ prefix). Không thêm permission key (D1). Script nav của v5.html bị cắt ở 256 KiB nên danh sách mục là quyết định của plan, không phải bằng chứng từ design. |

## Phases

| # | Phase | Effort | Phụ thuộc | Status |
|---|---|---|---|---|
| 1 | [API — ngân hàng nội dung & bài tập](./phase-01-api-ngan-hang-noi-dung-bai-tap.md) | 1.5d | — | Completed |
| 2 | [API — chương trình mẫu, phiên bản, buổi học](./phase-02-api-chuong-trinh-mau-phien-ban-buoi-hoc.md) | 2d | 1 | Completed |
| 3 | [Web — hub Kho học liệu + nhóm sidebar](./phase-03-web-hub-kho-hoc-lieu.md) | 2d | 1, 2 | Completed |
| 4 | [Web — chi tiết chương trình mẫu](./phase-04-web-chi-tiet-chuong-trinh-mau.md) | 2.5d | 2 | Completed |
| 5 | [Web — buổi học mẫu](./phase-05-web-buoi-hoc-mau.md) | 2d | 2 | Completed |
| 6 | [Test e2e, seed, docs, ship](./phase-06-test-e2e-seed-docs-ship.md) | 1d | 3, 4, 5 | Completed |

Phase 3, 4, 5 sở hữu file tách biệt (hub / template detail / lesson page) nên có thể chạy song song sau Phase 2;
`schemas/library-schemas.ts`, `api/library-api.ts`, `hooks/use-library.ts` và MSW handlers được Phase 2 mở rộng
trước (phần web contract) để ba phase web không sửa chung một file.

## Success Criteria

- [x] Sidebar có nhóm "Kho học liệu" (`role="group"`, `aria-label="Kho học liệu"`) đứng sau "Giảng dạy" với 4 mục theo D13; mục ẩn khi thiếu `library.read`; `dashboard-layout.test.tsx` phủ nhóm mới; tab "Thêm" (<md) vẫn liệt kê đủ 4 mục.
- [x] Hub có đúng 3 tab v5 tại `/library`, `/library/materials`, `/library/exercises`; `?tab=materials|exercises|lessons` cũ redirect đúng; thẻ chương trình mẫu hiển thị số buổi, số phiên bản, số lớp đang gắn và chip phiên bản.
- [x] Ngân hàng nội dung và ngân hàng bài tập có cột DÙNG TRONG và TRẠNG THÁI; ngừng hoạt động ẩn item khỏi picker, không ẩn khỏi buổi đã gắn.
- [x] Bài tập có mã duy nhất trong trung tâm, tự sinh khi bỏ trống, copy được từ bảng.
- [x] Chi tiết chương trình mẫu có 7 tab; Buổi học có chế độ Bảng/Cây, Nhân bản, Xoá tất cả, menu thêm buổi.
- [x] Nhóm bài tập tạo/xoá theo phiên bản và gán được cho từng bài tập trong buổi; xoá nhóm không xoá bài tập.
- [x] Phiên bản có nhiều bộ điểm; tổng trọng số hiển thị; dữ liệu cũ vẫn đọc được sau migration và sau `migrate down`.
- [x] Buổi học mẫu có Hình thức (có lịch / không lịch), thời lượng bước 15', nội dung theo loại có icon, toggle chia sẻ, sắp xếp; tab Bài tập có gán nhóm.
- [x] Phiên bản published vẫn từ chối mọi ghi với 409 `VERSION_LOCKED`; banner khoá hiển thị số lớp đang gắn.
- [x] `make test-api-unit`, `make scopelint`, `make test-web`, `make lint` xanh; `make test-api` (serial) và `make e2e-isolated` xanh ở Phase 6.
- [ ] Không thêm permission key, `CatalogVersion` giữ 5; snapshot route policy và audit action test cập nhật đủ route mới.

## Rủi ro & rollback

- **Reshape `score_set`** là thay đổi hình dạng JSONB trên dữ liệu thật → backup DB trước khi chạy `000034`; down migration khôi phục mảng phẳng từ bộ đầu tiên (mất các bộ thêm sau — chấp nhận, ghi trong changelog).
- **Backfill `code` bài tập** chạy trong một transaction; unique index partial (`deleted_at IS NULL`) để không đụng dòng đã xoá mềm.
- Rollback ở production = forward migration (quy ước repo), không chạy `migrate down` trên prod.

## Validation Log

**2026-09-24 — validation gate (Full tier, 6 phases).**

### Verification Results
| # | Claim | Source | Result |
|---|---|---|---|
| 1 | `kind` CHECK ở `library_materials`/`template_log_fields` là CHECK inline (tên do Postgres tự đặt) | `migrations/000028_library_items.up.sql:26,97` | Đúng — Phase 1/2 yêu cầu xác nhận tên bằng `\d` trước khi viết DROP CONSTRAINT |
| 2 | `score_set` là JSONB `NOT NULL DEFAULT '[]'`, chỉ `library` đọc | `000028:109`; grep `score_set` ngoài `library/` chỉ ra `grading` (bảng `score_sets` riêng) | Đúng — D8 tách biệt với `grading` |
| 3 | `copyVersionContent` chỉ copy nội dung, không copy prep fields | `library/service.go:247` | Đúng — duplicate làm cùng cách |
| 4 | `versionSelect` có subselect `lesson_count` làm khuôn | `library/repository.go:344` | Đúng |
| 5 | `classprogram.Apply` chỉ đọc `Title` từ lessons | `classprogram/service.go:97-134` | Đúng — field mới không ảnh hưởng (D6) |
| 6 | Route lớp `/classes/:id` tồn tại cho chip lớp ở tab Phiên bản | `features/roster/routes.tsx:42` | Đúng |
| 7 | `DropdownMenu` shadcn có sẵn | `components/ui/dropdown-menu.tsx` | Đúng |
| 8 | `useSearch` là hàm cục bộ trong `items-tabs.tsx` | `items-tabs.tsx:29` | Đúng — Phase 3 tách ra `hooks/use-search.ts` |
| 9 | Test roll-back đếm bước cứng `MigrateDown(m, 28)` | `migrations_test.go:332` | Đúng — Phase 1/2 phải tăng lên 29/30 |
| 10 | Seed chạy qua `make seed` (`cd apps/api && go run ./cmd/api seed`) và e2e-isolated seed cùng lệnh | `Makefile:101-105,141-142` | Đúng — sửa lệnh trong Phase 6 |
| 11 | Không cần permission key mới; `CatalogVersion = 5` | `authctx/catalog.go` | Đúng (D1) |
| 12 | Không có multipart/upload ngoài `imports` | grep `multipart` | Đúng (D12) |
| 13 | Nhóm sidebar khai báo tĩnh trong `useNavGroups`, render `role="group"` + `aria-label={header}`, header uppercase bằng CSS | `dashboard-layout.tsx:69-160, 596-620` | Đúng — thêm nhóm = thêm 1 object + đăng ký kép overflow (tiền lệ `260923-0715` phase 1) |
| 14 | Active-state nav so `pathname` theo tiền tố dài nhất, không đọc query | `useNavActive` `dashboard-layout.tsx:258-272` | Đúng — tab hub phải là route con để 3 mục không cùng sáng (D13) |
| 15 | Test/e2e hiện dùng `?tab=` cho hub | `library-page.test.tsx:206-387`, `e2e/library.spec.ts:36,171,244` | Đúng — Phase 3 đổi sang route con + giữ redirect |
| 16 | Script nav của design v5 (`navGroupsDef`) không có trong bản `get_file` 256 KiB | grep `KHO HỌC LIỆU` v5.html = 0 | Danh sách mục của nhóm là quyết định plan (D13), cần user xác nhận khi cook |

### Critical questions (đã hỏi, đã chốt)
| # | Câu hỏi | Quyết định |
|---|---|---|
| Q1 | Bộ điểm nhiều bộ, reshape JSONB? | **Nhiều bộ, reshape** (D8) |
| Q2 | CẤP ĐỘ bài tập: cột `level` mới hay tái dùng `difficulty`? | **Thêm `level`, giữ `difficulty`** (D5) |
| Q3 | Bỏ tab "Buổi học mẫu" ở hub? | **Bỏ, hub 3 tab** |
| Q4 | Upload file / nhập buổi từ file? | **Non-goal** (D12) |
| Q5 | Buổi self_study khi lớp Apply? | **Vẫn copy mọi buổi** (D6) |
| Q6 | Khoá phiên bản? | **Bất biến sau publish** (D2) |
| Q7 | (cập nhật 2026-09-24) Nhóm sidebar riêng "KHO HỌC LIỆU" gồm mục nào? | **Mặc định D13: 4 mục (3 tab hub + Chuẩn bị tài liệu), tab hub thành route con.** Phương án thay thế nếu user muốn gọn: nhóm 2 mục "Kho học liệu" (`/library`) + "Chuẩn bị tài liệu" (`/prep`), giữ `?tab=` — chỉ sửa `dashboard-layout.tsx` + test, bỏ bước routes ở Phase 3. |

Mọi câu trả lời trùng mặc định của plan → không sửa phase. Whole-plan sweep: `ak plan validate` OK, `ak plan reindex`
nhận diện plan, mọi link nội bộ (`./phase-0N-*.md`, `../reports/scout-…`) tồn tại.

### Cập nhật 2026-09-24 — nhóm sidebar riêng
Yêu cầu: tạo sidebar group riêng tên "KHO HỌC LIỆU". Thêm D13, hàng gap "Sidebar", tiêu chí thành công, claim 13–16, Q7;
Phase 3 nhận bước nav + route con (+0.5d → 2d, tổng 11d); Phase 6 thêm docs `frontend-guidelines.md` (danh sách nhóm) và
`api-guidelines.md:435` (câu "Six feature modules back the 'Giảng dạy' sidebar group"). Không đổi API/migration.

<!-- slug: kho-hoc-lieu-v5 -->
