---
phase: 1
title: "Nav Giảng dạy + Danh sách lớp học + Chi tiết lớp học"
status: completed
priority: P1
effort: "2d"
dependencies: []
---

# Phase 1: Nav "Giảng dạy" + Danh sách lớp học + Chi tiết lớp học

## Context Links
- Plan: [plan.md](./plan.md) · Scout: [reports/scout-260923-giang-day-menu.md](./reports/scout-260923-giang-day-menu.md)
- Red team: [reports/redteam-failure-260923.md](./reports/redteam-failure-260923.md), [reports/redteam-scope-260923.md](./reports/redteam-scope-260923.md)
- Prototype v5 màn `Danh sách lớp học` (prefix `cl`), `Chi tiết lớp học` (`cdt`), modal `modalClass`
- Mẫu quyền: `docs/adding-permissions.md`; mẫu feature web: `docs/frontend-guidelines.md`
- Mẫu deep-link guard: `apps/web/src/features/center/pages/class-config-page.tsx`

## Overview
Dựng nhóm sidebar **Giảng dạy** với mục đầu tiên **Danh sách lớp học** (`/classes`) và trang
**Chi tiết lớp học** (`/classes/:id`) với ba tab chạy được ngay bằng dữ liệu sẵn có (Thông tin,
Học viên, Buổi học). Mở rộng bảng `classes` với mã lớp, thẻ, cờ tuyển sinh, ghi chú vận hành;
mở rộng `GET /classes` với bộ lọc ngày/ca/từ khóa/thẻ và endpoint đếm cho dải chip. Các khối
phụ thuộc phase sau (Đội ngũ giảng dạy có lời mời, Chương trình học, Chat, Bài tập, Tài liệu,
Lịch sử lớp, khóa học) render dạng **placeholder có văn bản** đúng prototype ("Lớp chưa có
giáo viên.", "Lớp chưa có buổi học trong chương trình.") và được thay ở Phase 2/5/7.

## Key Insights
- `ClassDialog` (`roster/components/class-dialog.tsx`) đã là `modalClass` (tên, khung giờ tuần,
  đơn giá) → tái dùng nguyên, không viết modal mới.
- `GET /classes` hiện chỉ lọc `status` (`classes/handler.go:102-115`); prototype cần
  `day` (thứ), `shift` (ca sáng/chiều/tối), `q` (tên, mã), chip trạng thái có số đếm.
- Trạng thái hiển thị (Sắp khai giảng / Đang học / Đã kết thúc / Lưu trữ) **suy từ**
  `status + start_date + end_date` — không thêm enum vào DB. Quy tắc này sống ở **một nơi**:
  `classes/phase.go` (Go) và một predicate SQL tương đương trong repository (dùng cho filter và
  stats); web **chỉ render** trường `phase` do API trả, không tính lại. <!-- Red Team S1 F12 -->
- "Nhân viên phụ trách" = `class_staff` vai `hoc_vu` (đã có, owner-only assign) → tái dùng
  `GET/POST /classes/:id/staff`, `class-staff-section.tsx` (hành động ghi trong đó đã gate `isOwner`).
- Tab Học viên = `GET /enrollments?class_id=&active=true` (hook `useEnrollmentsList` có sẵn).
- Tab Buổi học = `GET /classes/:id/sessions?from=&to=&readonly=true`: đường read-only (thêm ở phase này)
  chỉ trả buổi đã có, **không materialise** và không cap 400 ngày; một request cho cả khoảng
  `start_date..(end_date ?? hôm nay+90d)`. Đường mặc định (không `readonly`) vẫn materialise với cap 400 ngày
  cho classbook/lịch. <!-- Red Team S1 F10 --> <!-- Updated: Validation Session 1 - cap 400 ngày xác minh từ sessions/service.go --> <!-- Updated: Phase 1 review H1/H2 - readonly -->
- `PUT /classes/:id` hiện là **full-replace** và gate bằng `repo.GetByID` (own-rows: owner hoặc GV
  có stint) — không có write port riêng. Trường mới (`code/tags/recruiting/note`) dùng pointer,
  `nil` = giữ nguyên, để ops card không xoá dữ liệu khi chỉ bật một toggle. <!-- Red Team S1 F9 -->
- `classes.code NOT NULL` chạm ba đường tạo lớp không đi qua `service.Create`:
  `classes.CreateAnchored` (dùng bởi `imports`), `testutil.Class` (`internal/testutil/fixtures.go:409`,
  chèn thẳng ở 35 file test) và seeds. Mã lớp phải được sinh ở **một helper chung** cả bốn nơi
  dùng. <!-- Red Team S1 F3 -->

## Requirements
- [x] R1 Migration `000025_class_catalog_fields`: `code VARCHAR(20) NOT NULL`, `tags JSONB NOT NULL DEFAULT '[]'`, `recruiting BOOLEAN NOT NULL DEFAULT false`, `note TEXT`; backfill `code` bằng `ROW_NUMBER()` theo trung tâm (không thể trùng); unique partial index `(center_id, code) WHERE deleted_at IS NULL`; down migration đầy đủ. <!-- Red Team S1 F3 -->
- [x] R2 `POST /classes` nhận `code` (tuỳ chọn, tự sinh qua helper chung nếu rỗng), `tags[]`, `note`; `PUT /classes/:id` nhận thêm `code`, `tags`, `recruiting`, `note` dạng **pointer, vắng = không đổi**; trùng `code` → 409 `CLASS_CODE_TAKEN`. `CreateAnchored` cũng sinh `code`. <!-- Red Team S1 F9 -->
- [x] R3 `GET /classes` thêm query `q`, `weekday` (0–6), `shift` (`morning|afternoon|evening`), `tag`, `phase` (`upcoming|running|ended`); `status` giữ hợp đồng cũ. `q` escape `%`, `_`, `\` trước ILIKE; `tag` bind tham số `?::jsonb`. <!-- Red Team S1 F14 -->
- [x] R4 `GET /classes/stats` trả `{all, upcoming, running, ended, archived, recruiting}` trong phạm vi đọc của caller; perm `classes.list`.
- [x] R5 `ClassResponse` thêm `code`, `tags`, `recruiting`, `note`, `phase`; web `classSchema` mirror với `.default()` an toàn.
- [x] R6 Web trang `/classes`: header + `+ Lớp học` (mở `ClassDialog`), bộ lọc ngày/ca/ô tìm, dải chip đếm, bảng 8 cột như prototype, hàng bấm mở chi tiết, nút `Sửa` → `/classes/:id/settings`; trạng thái loading/empty/error (`HvStateBlock`).
- [x] R7 Web trang `/classes/:id`: header (mã lớp + Sao chép, chip trạng thái, nút sửa), tabs Thông tin / Học viên / Buổi học chạy thật; tabs Chat / Bài tập / Tài liệu hiển thị nhưng **disabled** với tooltip "Có ở phase sau"; khối Đội ngũ giảng dạy đọc `class_staff` hiện có; khối Chương trình học và Lịch sử lớp là placeholder.
- [x] R8 Nav nhóm "Giảng dạy" (sau "Dạy học") với entry `Danh sách lớp học` perm `classes.list`; thêm nhãn vào `OVERFLOW_LABELS` (`dashboard-layout.tsx:172`) **và** prefix `/classes` vào `OVERFLOW_PATH_PREFIXES` (`:194`) để mobile overflow nhận diện route đang mở. <!-- Red Team S1 F14 -->
- [x] R9 Toggle "Cần tuyển sinh", sửa ghi chú vận hành và thẻ ngay trên tab Thông tin (mutation `PUT /classes/:id` chỉ gửi trường đổi), gate web `canWriteClass(cls)` (owner hoặc GV chính) khớp gate API `GetByID`. <!-- Red Team S1 F9 -->
- [x] R10 Test: Go unit + HTTP cho filter/stats/code; integration `imports` tạo lớp có `code`; toàn bộ test dùng `testutil.Class` vẫn xanh; Vitest cho hai trang (quartet loading/empty/error/data) và nav; MSW fixture cập nhật; `route_policy_snapshot_test.go` thêm entry `GET /classes/stats`. <!-- Red Team S1 F7 -->

## Architecture
```
Sidebar "Giảng dạy" ─▶ /classes (ClassListPage)
                         ├─ useClassesList({q, weekday, shift, tag, phase, status, page})
                         ├─ useClassStats()  ─▶ GET /classes/stats
                         └─ ClassDialog (POST /classes)
                      ─▶ /classes/:id (ClassDetailPage)
                         ├─ useClass(id) ─▶ GET /classes/:id
                         ├─ tab Thông tin: ClassOpsCard (PUT /classes/:id, pointer fields) · schedules · ClassStaffSection (hoc_vu) · cards
                         ├─ tab Học viên: useEnrollmentsList({class_id, active:true})
                         └─ tab Buổi học: GET /classes/:id/sessions?from&to (+ NGUỒN suy từ schedule hiệu lực tại ngày buổi)
API classes: handler.list parse filters → repo.ListReadable(filter) (EXISTS class_schedules khi có weekday/shift)
             handler.stats → repo.CountReadableByPhase (một query SUM(CASE WHEN phasePredicate))
             shared/classcode.Generate() ← service.Create · CreateAnchored · testutil.Class · seeds
```

## Related Code Files

### File inventory
| File | Action | Ước lượng | Test impact |
|---|---|---|---|
| `apps/api/migrations/000025_class_catalog_fields.up.sql` / `.down.sql` | Create | 45 dòng | `migrations_test.go` (`TestMigrationRoundTrip` + `TestClassCodeBackfill` mới; `backfill_parity_test.go` chỉ đóng băng 000018, không dùng) |
| `apps/api/internal/shared/dbtypes/string_list.go` | Create | 40 dòng | unit Scan/Value |
| `apps/api/internal/shared/classcode/classcode.go` | Create (`Generate() string` = `L` + 6 ký tự base32 Crockford; `Valid(code) bool`) — package thuần, không import feature để `testutil` dùng được | 40 dòng | `classcode_test.go` |
| `apps/api/internal/features/classes/model.go` | Modify (+Code, Tags dbtypes.StringList, Recruiting, Note) | +15 | — |
| `apps/api/internal/features/classes/dto.go` | Modify (Create/Update/Response + `Phase`, `ClassStatsResponse`; Update dùng pointer) | +60 | `dto_test.go` |
| `apps/api/internal/features/classes/errors.go` | Modify (`ErrCodeTaken` 409) | +8 | handler test |
| `apps/api/internal/features/classes/repository.go` | Modify (`ListFilter{Q,Weekday,Shift,Tag,Phase}`, `CountReadableByPhase` qua `readScoped`, `CodeExists`) | +120 | `repository_integration_test.go` |
| `apps/api/internal/features/classes/service.go` | Modify (`Create`/`CreateAnchored` sinh `code` qua `classcode`, validate unique, `Update` merge pointer, `Stats`) | +80 | `service_test.go` (fake repo) |
| `apps/api/internal/features/classes/handler.go` | Modify (`list` parse filter, `stats`) | +70 | `handler_test.go` |
| `apps/api/internal/features/classes/routes.go` | Modify (`GET /classes/stats` **trước** `/:id`) | +1 | manifest test |
| `apps/api/internal/features/classes/phase.go` | Create (`PhaseOf(class, today)`, `ShiftOf(start_time)`, `phasePredicate` SQL dùng chung filter/stats) | 60 | `phase_test.go` |
| `apps/api/internal/features/imports/*` | Verify/Modify: đường `CreateAnchored` truyền hoặc nhận `code` sinh tự động (scout xem imports dựng model ở đâu) | +5 | `imports` integration |
| `apps/api/internal/testutil/fixtures.go` | Modify (`Class` fixture: `Code` mặc định `classcode.Generate()` khi rỗng) | +4 | 35 file test dùng fixture |
| `apps/api/internal/shared/routespec/routespec.go` | Modify (spec `GET /api/v1/classes/stats` perm `classes.list`, `none()`) | +1 | `TestRoutePolicyCoversEveryRegisteredRoute` |
| `apps/api/internal/server/route_policy_snapshot_test.go` | Modify (+1 entry) | +1 | snapshot |
| `apps/api/seeds/seed.go` | Modify (code/tags cho `seedClasses`) | +10 | `make seed` |
| `apps/api/docs/*` | Regenerate `make api-docs` | — | — |
| `apps/web/src/features/roster/schemas/roster-schemas.ts` | Modify (`classSchema` + `classStatsSchema`, input schemas) | +40 | `roster-schemas.test.ts` |
| `apps/web/src/features/roster/api/classes-api.ts` | Modify (`ListClassesParams` mở rộng, `getClassStats`) | +25 | — |
| `apps/web/src/features/roster/hooks/roster-keys.ts` | Modify (`classesKeys.stats`) | +3 | — |
| `apps/web/src/features/roster/hooks/use-classes.ts` | Modify (`useClassStats`; `useCreateClass`/`useUpdateClass` invalidate `classesKeys.all`) | +14 | — |
| `apps/web/src/features/roster/lib/class-labels.ts` | Create (`phaseLabel`, `weekdayOptions`, `shiftOptions` — **chỉ nhãn/option**, không tính phase) | 40 | `class-labels.test.ts` |
| `apps/web/src/features/roster/components/class-filter-bar.tsx` | Create (select ngày, ca, ô tìm — `HvSelect`) | 90 | trang test |
| `apps/web/src/features/roster/components/class-status-chips.tsx` | Create (chip + số đếm, `HvChip`) | 60 | trang test |
| `apps/web/src/features/roster/components/class-table.tsx` | Create (bảng 8 cột, hàng bấm được, nút Sửa/Mở) | 140 | trang test |
| `apps/web/src/features/roster/components/class-detail-header.tsx` | Create (mã lớp + Sao chép `navigator.clipboard`, chip trạng thái, actions) | 90 | trang test |
| `apps/web/src/features/roster/components/class-ops-card.tsx` | Create (chips vận hành, toggle Cần tuyển sinh, BĐ/KT, note; form RHF+zod; gửi partial) | 150 | `class-ops-card.test.tsx` |
| `apps/web/src/features/roster/components/class-info-tab.tsx` | Create (lưới trái/phải như prototype; dùng `ClassStaffSection`, placeholders) | 160 | trang test |
| `apps/web/src/features/roster/components/class-students-tab.tsx` | Create (bảng học viên từ enrollments; CTA "Thêm học viên" → `/students?class=`) | 90 | trang test |
| `apps/web/src/features/roster/components/class-sessions-tab.tsx` | Create (bảng buổi theo khoảng ngày của lớp, cột NGUỒN, CTA "Điểm danh & nhận xét →" `/classbook`) | 110 | trang test |
| `apps/web/src/features/roster/pages/class-list-page.tsx` | Create | 120 | `class-list-page.test.tsx` |
| `apps/web/src/features/roster/pages/class-detail-page.tsx` | Create (tabs `HvSegmented variant="tabs"`, guard 404/403) | 140 | `class-detail-page.test.tsx` |
| `apps/web/src/features/roster/routes.tsx` | Modify (`classes`, `classes/:id`) | +12 | router smoke |
| `apps/web/src/features/roster/index.ts` | Modify (export `useClassStats`, `phaseLabel`) | +2 | — |
| `apps/web/src/layouts/dashboard-layout.tsx` | Modify (nhóm "Giảng dạy", `OVERFLOW_LABELS` :172, `OVERFLOW_PATH_PREFIXES` :194) | +14 | `dashboard-layout.test.tsx` (nếu có) |
| `apps/web/src/test/msw/handlers.ts` | Modify (fixture class có `code/tags/recruiting/note/phase`, handler `GET /classes/stats` đăng ký **trước** `GET /classes/:id`) | +25 | toàn bộ suite |
| `apps/web/src/features/roster/__tests__/roster-handlers.ts` | Modify (fixture mở rộng; `/classes/stats` trước `/classes/:id` tại :401) | +15 | roster tests |
| `apps/web/src/features/roster/__tests__/class-list-page.test.tsx` | Create | 150 | — |
| `apps/web/src/features/roster/__tests__/class-detail-page.test.tsx` | Create | 180 | — |
| `apps/web/src/features/roster/__tests__/class-labels.test.ts` | Create | 30 | — |

### Dependency map
```
000025 migration ──▶ classes.model ──▶ dto/repository/service/handler ──▶ routespec ──▶ snapshot test ──▶ swagger
                          ▲                                   │
      shared/classcode ───┴── testutil.Class · imports · seeds ◀┘
web: roster-schemas ──▶ classes-api ──▶ use-classes ──▶ pages ──▶ routes.tsx ──▶ dashboard-layout nav
     msw handlers (mirror ClassResponse) ──▶ mọi test roster/teaching dùng fixture class
```

## Implementation Steps

### A. Migration (backup DB trước khi chạy trên máy có dữ liệu)
1. `000025_class_catalog_fields.up.sql`: <!-- Red Team S1 F3 -->
   ```sql
   ALTER TABLE classes
     ADD COLUMN code       VARCHAR(20) NOT NULL DEFAULT '',
     ADD COLUMN tags       JSONB       NOT NULL DEFAULT '[]'::jsonb,
     ADD COLUMN recruiting BOOLEAN     NOT NULL DEFAULT false,
     ADD COLUMN note       TEXT;
   -- Backfill: số thứ tự theo trung tâm, ổn định theo created_at rồi id → không thể trùng.
   -- (6 hex đầu của UUIDv7 là timestamp nên KHÔNG dùng: trùng trong cùng cửa sổ thời gian.)
   WITH numbered AS (
     SELECT id, ROW_NUMBER() OVER (PARTITION BY center_id ORDER BY created_at, id) AS rn FROM classes
   )
   UPDATE classes c SET code = 'L' || lpad(n.rn::text, 4, '0') FROM numbered n WHERE c.id = n.id AND c.code = '';
   ALTER TABLE classes ALTER COLUMN code DROP DEFAULT;
   CREATE UNIQUE INDEX uq_classes_center_code ON classes (center_id, code) WHERE deleted_at IS NULL;
   CREATE INDEX idx_classes_center_recruiting ON classes (center_id) WHERE recruiting AND deleted_at IS NULL;
   ```
   `.down.sql`: `DROP INDEX` 2 index, `ALTER TABLE classes DROP COLUMN note, DROP COLUMN recruiting, DROP COLUMN tags, DROP COLUMN code;`.
   Header comment giải thích bất biến (tiếng Việt như 000015).
2. Chạy `make migrate-up` trên dev, `make migrate-down` một bước rồi up lại; thêm case vào
   `migrations_test.go` (`domainTables`/danh sách migration nếu file liệt kê từng cái).

### B. API
3. `shared/dbtypes/string_list.go`: **di chuyển** `teaching.StringList` (`teaching/model.go:39-60`) sang đây và để
   `teaching` alias `type StringList = dbtypes.StringList` — một bản duy nhất, không sao chép; hành vi `Scan/Value` giữ nguyên,
   test hiện có của teaching phải xanh. <!-- Updated: Red Team S1 scope audit follow-up - StringList trùng lặp -->
4. `shared/classcode/classcode.go`: `Generate()` = `L` + 6 ký tự base32 Crockford từ `crypto/rand`
   (khác dạng `L0001` của backfill nên không đụng); `Valid()` regex `^[A-Z0-9-]{2,20}$`. <!-- Red Team S1 F3 -->
5. `classes/model.go`: thêm `Code string`, `Tags dbtypes.StringList`, `Recruiting bool`, `Note *string`.
6. `classes/phase.go`: `PhaseOf(c *Class, today time.Time) string` → `archived` nếu status archived;
   `upcoming` nếu `start_date > today`; `ended` nếu `end_date != nil && end_date < today`; else `running`.
   `ShiftOf(t TimeOfDay)`: `< 12:00` morning, `< 17:30` afternoon, else evening. `phasePredicate(phase, today)`
   trả mảnh SQL + args dùng chung cho `ListReadable` và `CountReadableByPhase` (một nguồn sự thật với `PhaseOf`,
   có test đối chiếu hai đường). <!-- Red Team S1 F12 -->
7. `classes/dto.go`: `CreateClassRequest` + `Code *string` (validate bằng `classcode.Valid`), `Tags []string binding:"max=10,dive,max=30"`,
   `Note *string binding:"omitempty,max=1000"`; `UpdateClassRequest` thêm `Code *string`, `Tags *[]string`, `Recruiting *bool`,
   `Note *string` — **nil = giữ nguyên**; `ClassResponse` thêm `Code, Tags, Recruiting, Note, Phase`;
   `ClassStatsResponse{All, Upcoming, Running, Ended, Archived, Recruiting int64}`. <!-- Red Team S1 F9 -->
8. `classes/repository.go`: `ListFilter{Status, Phase, Q, Weekday *int16, Shift, Tag string}`; hai consumer hiện có
   của `ListFilter` (`centers/dashboard.go:22`, `centers/handler.go:422`) chỉ đặt `Status` → thêm trường là tương thích.
   `ListReadable` áp: `Q` → escape `\ % _` (khuôn `enrollments.SearchEnrollableStudents`) rồi
   `name ILIKE ? ESCAPE '\' OR code ILIKE ? ESCAPE '\'`; `Tag` → `tags @> ?::jsonb` với JSON đã marshal;
   `Weekday/Shift` → `EXISTS (SELECT 1 FROM class_schedules s WHERE s.class_id = classes.id AND s.deleted_at IS NULL AND (s.effective_to IS NULL OR s.effective_to >= CURRENT_DATE) AND s.weekday = ? [AND s.start_time >= ? AND s.start_time < ?])`;
   `Phase` → `phasePredicate`. `CountReadableByPhase(ctx, sc, today)` một query `SUM(CASE ...)` trên cùng predicate,
   đi qua helper `readScoped(ctx, sc)` (`classes/repository.go:124`) — đây là điểm scopelint đã cho phép `CenterWideFor`,
   **không** dựa vào tên hàm. `CodeExists(ctx, a Anchor, code, exceptID)`. <!-- Red Team S1 F14 -->
9. `classes/service.go`: `Create` và `CreateAnchored` — nếu `Code` rỗng gọi `classcode.Generate()`, thử lại tối đa 3 lần
   khi `CodeExists`; nếu có `Code` và trùng → `ErrCodeTaken` (409). `Update`: load qua `repo.GetByID` (gate hiện có),
   merge từng pointer khác nil rồi save. `Stats(ctx, sc)` gọi repo. <!-- Red Team S1 F3, F9 -->
10. `testutil/fixtures.go#Class`: nếu `Code == ""` gán `classcode.Generate()` trước khi insert. Chạy `make test-api`
    (serial) để xác nhận 35 file test dùng fixture vẫn xanh. <!-- Red Team S1 F3 -->
11. `classes/handler.go`: `list` parse `q`, `weekday` (0–6 else 422), `shift` (enum else 422), `tag`, `phase` (enum else 422);
    `stats` handler + swag annotations. `routes.go`: `g.GET("/stats", h.stats)` đặt **trước** `g.GET("/:id")`.
12. `routespec.go`: `perm("GET", "/api/v1/classes/stats", authctx.PermClassesList, none())`; thêm dòng tương ứng vào
    `server/route_policy_snapshot_test.go`. Không thêm key mới → **không** bump `CatalogVersion`. <!-- Red Team S1 F7 -->
13. `seeds/seed.go`: thêm `Code`, `Tags` cho `seedClasses` (vd `TOAN9C`, `["Toán","Khối 9"]`).
14. `make api-docs`; `make test-api-unit`; `make scopelint`.

### C. Web
15. `roster-schemas.ts`: `classSchema` + `code: z.string().default("")`, `tags: z.array(z.string()).default([])`,
    `recruiting: z.boolean().default(false)`, `note: z.string().nullable().default(null)`,
    `phase: z.enum(["upcoming","running","ended","archived"]).default("running")`; `classStatsSchema`;
    `classOpsInputSchema` (recruiting, note, tags) → `toClassUpdateInput` chỉ gồm trường đổi.
16. `classes-api.ts`: mở rộng `ListClassesParams` (`q, weekday, shift, tag, phase`), thêm `getClassStats()`;
    `roster-keys.ts` thêm `stats`; `use-classes.ts` thêm `useClassStats()`; `useCreateClass` và `useUpdateClass`
    invalidate `classesKeys.all` (hiện chỉ `lists()` → chip đếm sẽ lệch). <!-- Red Team S1 F14 -->
17. `lib/class-labels.ts`: nhãn tiếng Việt `{upcoming:"Sắp khai giảng", running:"Đang học", ended:"Đã kết thúc", archived:"Lưu trữ"}`,
    `weekdayOptions` (Tất cả các ngày, Thứ 2…Chủ nhật, dùng `formatWeekday`), `shiftOptions` (Tất cả ca, Sáng, Chiều, Tối).
    Không có hàm tính phase ở web. <!-- Red Team S1 F12 -->
18. Components theo inventory; bảng dùng lớp `tableHeadCellClassName` như `classes-tab.tsx`; chip trạng thái và chip đếm
    dùng `HvChip`/`HvBadge` của hv kit (`StatusPill` là widget riêng của `collections`, không dùng). Select dùng `HvSelect`.
    Debounce ô tìm 300 ms; filter đồng bộ URL `useSearchParams` (khuôn `useBoardUrlState`). <!-- Red Team S1 F14 -->
19. `class-list-page.tsx`: đọc `useClassStats` cho chip; `useClassesList` với filter; chip đang chọn = `phase`/`status`;
    hàng bấm → `navigate('/classes/:id')`.
20. `class-detail-page.tsx`: `useClass(id)`; 404 → `HvStateBlock` lỗi "Không tìm thấy lớp"; tabs qua `HvSegmented variant="tabs"`
    + `?tab=`; Chat/Bài tập/Tài liệu disabled. Tab Thông tin: `ClassOpsCard` (gate `canWriteClass(cls)`), lịch tuần
    (`formatScheduleLabel`, nút sửa → settings), `ClassStaffSection` lọc vai `hoc_vu` cho "Nhân viên phụ trách", cards Học tập
    (→ `/classbook?class=id`, `/records`), Báo cáo (→ `/sessions`, `/billing/:periodId` nếu có `billing.read`), Thiết lập
    (→ `/classes/:id/settings`, `/center/class-config` owner). Cột phải: Lịch sử lớp placeholder, Đội ngũ giảng dạy (danh sách
    `class_staff` hiện có, nút mời disabled "Phase 2"), Chương trình học placeholder đúng câu prototype.
    Tab Buổi học: gọi `GET /classes/:id/sessions?from&to` theo Key Insights; cột NGUỒN = "Lịch tuần" nếu buổi khớp
    `weekday + start_time` của một schedule **hiệu lực tại `session_date`** (`effective_from ≤ date ≤ effective_to`),
    còn lại "Thêm tay". <!-- Red Team S1 F10 -->
21. `routes.tsx`: thêm `classes` và `classes/:id` (react-router ưu tiên `classes/:id/settings` vì dài hơn).
22. `dashboard-layout.tsx`: chèn nhóm `{ header: "Giảng dạy", entries: [{ label: "Danh sách lớp học", to: "/classes", Icon: <lucide phù hợp>, perm: "classes.list" }] }`
    sau nhóm "Dạy học"; thêm nhãn vào `OVERFLOW_LABELS` và `/classes` vào `OVERFLOW_PATH_PREFIXES`. <!-- Red Team S1 F14 -->
23. MSW: fixture class thêm trường mới; `http.get(`${API_URL}/classes/stats`)` đăng ký **trước** handler `/classes/:id`
    trong cả `handlers.ts` và `roster-handlers.ts` (MSW khớp theo thứ tự đăng ký). <!-- Red Team S1 F14 -->
24. Tests theo ma trận dưới; `make test-web`, `make lint-web`.

## Todo List
- [x] A1 Viết migration 000025 up/down (backfill ROW_NUMBER) + chạy up/down/up trên dev (đã backup)
- [x] B1 `dbtypes.StringList`, `classcode` + unit test
- [x] B2 Model/DTO/phase.go + unit test `PhaseOf`, `ShiftOf`, `phasePredicate` khớp `PhaseOf`
- [x] B3 Repository filter (escape ILIKE, jsonb bind) + stats qua `readScoped` + integration test
- [x] B4 Service code sinh/unique cho `Create` + `CreateAnchored`, `Update` merge pointer + test fake repo
- [x] B5 `testutil.Class` Code mặc định; `make test-api` xanh toàn bộ
- [x] B6 Handler list/stats + HTTP test 422/409
- [x] B7 routespec + snapshot test + `make test-api-unit` + `make scopelint` + `make api-docs`
- [x] B8 Seeds
- [x] C1 Schemas/api/hooks/keys (invalidate `classesKeys.all`)
- [x] C2 `class-labels.ts` + test
- [x] C3 Components bảng/lọc/chip/header/ops/tabs (`HvChip`)
- [x] C4 Hai trang + routes
- [x] C5 Nav "Giảng dạy" + `OVERFLOW_LABELS` + `OVERFLOW_PATH_PREFIXES`
- [x] C6 MSW fixture (thứ tự `/stats` trước `/:id`) + tests hai trang + nav
- [x] C7 `make test-web`, `make lint`

## Test scenario matrix
| Mức | Kịch bản | Loại |
|---|---|---|
| Critical | `GET /classes?weekday=1&shift=evening` chỉ trả lớp có schedule thứ 2 ≥17:30 còn hiệu lực | API integration |
| Critical | `GET /classes/stats` của member không `view_all` chỉ đếm lớp mình đọc được; owner đếm toàn trung tâm | API integration |
| Critical | `POST /classes` với `code` trùng trong cùng trung tâm → 409; cùng code ở trung tâm khác → 201 | API HTTP |
| Critical | Migration 000025 backfill: mọi lớp cũ có `code` ≠ '' và unique theo trung tâm, kể cả 2 lớp tạo cùng mili-giây | migrations test |
| Critical | `imports` tạo lớp qua `CreateAnchored` → có `code` hợp lệ, không vi phạm NOT NULL/unique | API integration |
| High | `testutil.Class` không truyền `Code` → fixture insert được; toàn bộ `make test-api` xanh | API integration |
| High | `PUT /classes/:id` body `{recruiting:true}` không xoá `code/tags/note`; GV không có stint → 404 như hiện tại | API HTTP |
| High | `q="100%"` và `q="a_b"` không bị hiểu là wildcard | API integration |
| High | `PhaseOf`: hôm nay = start_date → running; end_date = hôm qua → ended; archived thắng mọi ngày; `phasePredicate` cho cùng kết quả trên fixture | Go unit + integration |
| High | Trang `/classes`: quartet loading/empty/error/data; chip đếm đúng fixture; bấm hàng điều hướng `/classes/:id` | Vitest |
| High | Trang `/classes/:id`: 404 → thông báo; tab Học viên rỗng hiện câu "Lớp chưa có học viên…"; tab Buổi học gọi `?from&to` đúng khoảng và rỗng hiện câu prototype | Vitest |
| High | Toggle "Cần tuyển sinh" gọi `PUT /classes/:id {recruiting:true}` (chỉ trường đó) và invalidate stats | Vitest |
| High | Nav: member không `classes.list` không thấy "Danh sách lớp học"; mobile overflow chứa nhãn và nhận `/classes/abc` là đang mở | Vitest |
| Medium | Ô tìm debounce, filter lên URL `?q=&weekday=&shift=` và khôi phục khi reload | Vitest |
| Medium | Sao chép mã lớp gọi `navigator.clipboard.writeText` + toast | Vitest |
| Medium | Swagger regenerate không diff ngoài phần classes | CI |

## Success Criteria
- [x] Sidebar hiện nhóm "Giảng dạy" → "Danh sách lớp học"; `/classes` liệt kê đúng lớp với lịch, thẻ, trạng thái suy diễn (từ API), ngày BĐ/KT; tạo lớp từ `+ Lớp học` xuất hiện ngay trong bảng và chip đếm cập nhật.
- [x] `/classes/:id` hiển thị mã lớp, chip trạng thái, thông tin vận hành sửa được (partial update), lịch tuần, nhân viên phụ trách, học viên, buổi học.
- [x] `make test-api-unit`, `make scopelint`, `make test-web`, `make lint` xanh; `make test-api` (serial, toàn bộ) xanh — không chỉ package `classes`.
- [x] Không thay đổi hành vi `/students`, `/classbook`, `/classes/:id/settings`, `imports` hiện có (test cũ xanh).

## Risk Assessment
| Rủi ro | Mức | Giảm thiểu |
|---|---|---|
| Backfill `code` trùng trong một trung tâm | Đã loại | `ROW_NUMBER() OVER (PARTITION BY center_id)` — không phụ thuộc UUID; test parity <!-- Red Team S1 F3 --> |
| `code NOT NULL` làm đỏ 35 file test / luồng import | Cao nếu bỏ qua | Helper `classcode` dùng chung + `testutil.Class` default; chạy toàn bộ `make test-api` trong phase <!-- Red Team S1 F3 --> |
| Filter `weekday/shift` EXISTS schedules làm chậm list | Thấp | EXISTS + index `class_schedules(class_id)` sẵn có; `per_page ≤ 100` |
| Fixture MSW đổi shape làm đỏ test teaching/dashboard | Trung bình | `.default()` trong zod; chạy toàn bộ `make test-web` |
| Route `/classes/stats` bị `/:id` nuốt (Gin và MSW) | Trung bình | Đăng ký `/stats` trước `/:id` ở cả API và hai file MSW + HTTP test <!-- Red Team S1 F14 --> |
| Tab Buổi học gọi thiếu `from/to` → 422; mở tab sinh buổi `planned` ngoài ý muốn | Trung bình | Luôn gửi `from/to`; dùng `readonly=true` nên không có cap 400 ngày và không ghi DB khi duyệt <!-- Red Team S1 F10 --> <!-- Updated: Phase 1 review H1/H2 --> |
| `useSearchParams` xung đột `?tab=` chi tiết | Thấp | Tách key `tab` riêng trang chi tiết |

## Security Considerations
- Không thêm key quyền; `stats` dùng `classes.list` và cùng read port (`readScoped`) → không lộ số lớp ngoài phạm vi.
- `code`, `note`, `tags` là văn bản thuần; escape mặc định của React; giới hạn độ dài bằng `binding`; `q` escape trước ILIKE, `tag` bind tham số.
- Sửa `recruiting/note/tags` đi qua `repo.GetByID` (own-rows: owner hoặc GV có stint) đúng như `Update` hiện tại — không mở rộng quyền ghi; web gate `canWriteClass` khớp. <!-- Red Team S1 F9 -->

## Next Steps
Phase 2 (Lời mời nhận lớp) thay khối "Đội ngũ giảng dạy" placeholder bằng danh sách có trạng thái
chờ/đã nhận; Phase 5 điền chip khóa học (`course_id`); Phase 7 mở ba tab còn lại.
