# Scout report — menu "Giảng dạy" (prototype v5)

Ngày: 2026-09-23 · Nguồn: DesignSync `So Lop - Prototype v5.dc.html` (bị cắt 256 KiB, mất
script nav) + v3 (đủ, nav builder) + repo `teka` @ `95bb7f4`.

## 1. Nguồn thiết kế

| File | Tình trạng | Dùng cho |
|---|---|---|
| `prototype-v5.stripped.html` | Markup đủ 29 màn, **mất script** sau `</x-dc>` | Binding từng màn (cl/inv/cdt/lib/cd/td/ts/prep/...) |
| `prototype-v4.stripped.html` | Cắt, 26 màn | Diff v4→v5: v5 thêm Danh sách lớp học, Lời mời nhận lớp, Chi tiết lớp học |
| `prototype-v3.stripped.html` | Đủ, có `navGroupsDef` | Khuôn nav: `['DẠY HỌC',[...]] + HỌC PHÍ (role!=teacher) + TRUNG TÂM (owner)` |

Kết luận: nhóm "Giảng dạy" **không có trong v3**; v5 thêm nhóm này nhưng script không lấy
được. Giả định thành viên nhóm ghi ở `plan.md` §Giả định, xác nhận ở cổng validation.

## 2. Màn v5 thuộc giả định "Giảng dạy" và binding chính

| Màn (`data-screen-label`) | Prefix | Điểm chính |
|---|---|---|
| Danh sách lớp học | `cl` | filter ngày/ca/q, chips đếm, bảng STT·LỚP(+khóa)·LỊCH·THẺ·TRẠNG THÁI·BĐ·KT, nút Sửa/Mở, `+ Lớp học` → `modalClass` |
| Lời mời nhận lớp | `inv` | chips, bảng LỚP·GV·GỬI LÚC·TRẠNG THÁI; pending: `GV nhận lớp`/`Nhắc lại`/`Hủy` |
| Chi tiết lớp học | `cdt` | header (khóa, mã lớp + Sao chép, lịch sử thay đổi, actions), tabs Thông tin/Học viên/Buổi học/Chat/Bài tập/Tài liệu; Thông tin: vận hành (chips, Cần tuyển sinh, BĐ/KT dự kiến, note), lịch tuần, nhân viên phụ trách, Học tập/Báo cáo/Thiết lập cards, Lịch sử lớp (lineage), Đội ngũ giảng dạy (+mời), Chương trình học (áp dụng/đổi phiên bản/gỡ) |
| Lộ trình học | `lib.paths` | path → stage → course |
| Danh mục khóa học / Chi tiết khóa học | `lib.courses` / `cd` | tabs info/template/setup/ops, gói học phí, kế thừa phiên bản template → course default → class override |
| Kho học liệu / Chương trình mẫu / Buổi học mẫu | `lib` / `td` / `ts` | ngân hàng học liệu, bài tập, template + phiên bản, nhóm, bộ điểm, log fields |
| Chuẩn bị tài liệu / Bảng chuẩn bị / Phân công & tiến độ / Chi tiết buổi chuẩn bị / Tạo chương trình | `prep`/`board`/`team`/`sess`/`nform` | Quyết định user: **chỉ tạo trên web, không import** — bỏ màn "Nguồn dữ liệu" (`imp`) |

Modal `modalClass` ("Tạo lớp mới": tên, khung giờ tuần, đơn giá) **đã có** trong web:
`apps/web/src/features/roster/components/class-dialog.tsx`.

## 3. Repo — điểm neo

### API (Go)
- `internal/features/classes/{model,dto,handler,repository,service,routes}.go` — `Class{ID,TeacherID,CenterID,Name,StartDate,EndDate,DefaultUnitPrice,Status,Schedules}`; list chỉ lọc `status` (`handler.go:102-135`), sort whitelist `listSorts`; read port `ListReadable/GetReadableByID`, write port `GetWritableByID(roles)`.
- `internal/features/classstaff` — `class_staff(role_key giao_vien|hoc_vu|tro_giang, started_at, ended_at)`; `uq_class_staff_one_gv`; assign/remove **owner-only** (`routespec.go:179-182`); giao_vien chỉ đổi qua `handoff.Service.Reassign` (`handoff/service.go:110`).
- `internal/features/invitations` — lời mời **vào trung tâm** (public token). Khác hoàn toàn "Lời mời nhận lớp" (cấp lớp, thành viên đã trong trung tâm).
- `internal/features/teaching` — `class_curricula.lessons JSONB StringList` + `CurrentIndex`; `lesson_plans` state machine theo `lesson index`; marks/notes theo session.
- `internal/features/sessions` — `class_sessions{SessionDate,StartTime,Status planned|held|cancelled}` sinh on-demand từ schedules; **không có** cột nguồn.
- `internal/features/audit` — list lọc `actor_id/action/from/to/cursor/limit`, **chưa** lọc theo entity.
- `shared/authctx/catalog.go` — `def/viewAll/optIn`, `CatalogVersion = 4` (mirror web `CATALOG_VERSION = 4` tại `src/test/msw/handlers.ts:17`); `shared/routespec/routespec.go` — `Kind{Public,PublicToken,Self,OwnerOnly,Permission,Service}`, mọi route phải có Spec.
- Migration mới nhất `000024_task_column_color`; mẫu backfill 2 bảng `000022_task_board`; `classes` DDL ở `000001` (`code/tags/course_id` chưa có).
- Seeds `seeds/seed.go` (`seedClasses` :128).

### Web (React)
- Nav: `src/layouts/dashboard-layout.tsx#useNavGroups` (nhóm Tổng quan / Dạy học / Học phí / Trung tâm) + `OVERFLOW_LABELS` (:172) cho thanh mobile.
- Router: `src/app/router.tsx` gom `*Routes` từ mỗi feature; roster có `classes/:id/settings` (chưa có `/classes`, `/classes/:id`).
- Roster: `api/classes-api.ts` (`listClasses{status,page,per_page,sort}`), `hooks/use-classes.ts`, `components/class-dialog.tsx` (= modalClass), `classes-tab.tsx` (bảng lớp cũ ở /students), `schemas/roster-schemas.ts#classSchema`.
- Center: `class-config-page.tsx` — mẫu deep-link guard owner-only; `use-score-sets`.
- Teaching: `use-center-context.ts#has(key)`; `teaching-api.ts`.
- Test: Vitest + MSW (`handlers.ts` có `PERMISSION_CATALOG` mirror, fixture `GET /classes` :969), Playwright `e2e/*.spec.ts` (`roster.spec.ts`, `class-staff-*.spec.ts`), helpers `auth.ts` (owner `0901000001`, member `0901000002`).

## 4. Rủi ro phát hiện khi scout
1. Script v5 mất → thành viên menu là giả định (validation Q1).
2. `classes.teacher_id NOT NULL` + `uq_class_staff_one_gv`: lời mời vai `giao_vien` khi chấp nhận phải đi qua handoff, không insert thẳng `class_staff`.
3. Kế thừa phiên bản template → course → class phải không phá `class_curricula` (classbook, giáo án dùng lesson index).
4. "Kênh chat lớp — đồng bộ Zalo OA": repo không có tích hợp Zalo OA (chỉ link Zalo trên contact) → non-goal, lưu tin nhắn nội bộ.
5. Mỗi phase thêm key permission → bump `CatalogVersion` và mirror MSW; quên → test `permission-catalog` đỏ.
