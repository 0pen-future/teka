# Review — Phase 8: Chuẩn bị tài liệu (bảng chuẩn bị trên bản nháp chương trình)

Ngày: 2026-09-24 · Branch `feat/giang-day-menu` · Phạm vi: `d0c27ce` (API) và `d578cb9` (web), đọc bằng `git show` từng commit.
Spec: `phase-08-chuan-bi-tai-lieu.md`, D5/D8/D9/D10 trong `plan.md`, `docs/adding-permissions.md`.

## Verdict tóm tắt

Có một lỗi High, ba Medium, không có Critical. Phần lõi đúng:
- **Tenancy.** Mọi truy vấn mới đều đi qua `r.lessons(ctx, sc)` (`template_lessons.center_id = ?`) hoặc `liveTemplates`. `IsLiveMember` lọc theo `center_id = sc.CenterID AND left_at IS NULL`. FK composite `(assignee_id, center_id) → center_members (teacher_id, center_id)` chặn gán chéo trung tâm ngay ở DB. `ON DELETE SET NULL (assignee_id)` là dạng có danh sách cột (PG15+, đã dùng ở 000007 và 000022). Dạng này chỉ đặt NULL cho `assignee_id` và không động tới `center_id NOT NULL`. Test migration xoá cứng account và xác nhận lesson vẫn còn.
- **Authorization.** PATCH prep cần `library.edit`, PATCH assignment cần `prep.assign`. Cả hai được kiểm ở middleware (routespec) và kiểm lại ở service. `prep.assign` là `optIn` và implied `library.read`. Owner đi qua bypass. Member không có key bị 403. Tất cả đều có test HTTP.
- **Khoá phiên bản.** Cả hai PATCH đi qua `lessonWrite`, dùng `LockVersion FOR UPDATE` rồi trả 409 `VERSION_LOCKED` nếu không phải draft. Test integration có đủ hai nhánh.
- **Hợp đồng.** `CatalogVersion` 4→5 đúng một lần, khớp `catalog_test`, MSW `handlers.ts:17`, `RESOURCE_LABELS` và test tab. Snapshot route +3, audit action +2, cả hai route mutating đều khai `req(...)`. Swagger sinh lại khớp file đã commit.

Lỗi High: người chỉ có `prep.assign` (không phải owner, không có `members.list`) không chọn được người phụ trách. Lý do là trang phân công lấy danh sách thành viên từ `GET /centers/me/members/directory`, route này cần `members.list` (opt-in). Nghĩa là key mới chỉ dùng được với owner hoặc với người có thêm một key khác không liên quan.

## Verification đã chạy

| Lệnh | Kết quả |
|---|---|
| `go build ./...` (api) | OK |
| `go vet ./internal/features/library/ ./internal/shared/authctx/` | OK |
| `go tool swag init -g cmd/api/main.go --parseInternal -o <scratchpad>` rồi so với `apps/api/docs` | `swagger.json`, `swagger.yaml` trùng khớp. `docs.go` chỉ khác tên package, do thư mục output khác |
| Chạy thử validator gin (`go run` trong scratchpad) với `PrepRequest`/`TemplateRequest` | checklist 51 mục → lỗi `max`. Nhãn 200 ký tự có dấu → OK. Nhãn `"   "` → **được chấp nhận** (L2). `lesson_count=0` → lỗi `min` |
| `npx tsc -b --noEmit` (web) | OK |

Lead đã chạy trước: `make test-api-unit`, scopelint, `lint-api`, `make test-web` (122 files, 1077 passed), `lint-web`. Tôi không chạy lại, và cũng không chạy integration hay docker theo yêu cầu.

## Đối chiếu yêu cầu

| Mục kiểm | Kết quả | Bằng chứng |
|---|---|---|
| Không có bảng `prep_*`, 4 cột trên `template_lessons` | Đạt | `000032_template_lesson_prep.up.sql:7-15` |
| FK composite vào `center_members (teacher_id, center_id)` | Đạt. **Lệch spec có chủ đích**: spec ghi `ON DELETE CASCADE`, code dùng `SET NULL (assignee_id)` theo khuôn 000022. Cách này đúng hơn vì CASCADE sẽ xoá cả nội dung buổi mẫu. Plan chưa được cập nhật (L1) | `up.sql:14`; `000022_task_board.up.sql:46`; `migrations_test.go` `TestTemplateLessonPrepSchemaInvariants` |
| Index `assignee_id` partial | Đạt | `up.sql:16` |
| Down đối xứng | Đạt: drop index → constraint → 4 cột, đều `IF EXISTS`. Số bước `MigrateDown` 27→28 đã cập nhật | `000032_…down.sql:2-8`; `migrations_test.go:332` |
| `center_id` trên mọi truy vấn mới | Đạt | `repository.go:391-395` (`lessons`), `:481-491`, `:493-500`; subquery summary gắn theo `program_templates.id` đã scope |
| Assignee phải là thành viên đang hoạt động | Đạt ở thời điểm ghi (`left_at IS NULL`). Chỉ có test bằng fake, không có test integration cho nhánh `left_at` (L5). Kiểm tra nằm ngoài tx (L1) | `service.go:509-518`; `repository.go:493-500` |
| Assignee khác trung tâm → 422 | Đạt | `integration_test.go` `TestPrepBoardAndAssignment` (outsider → 422 `assignee_id`) |
| PATCH prep cần `library.edit`, assignment cần `prep.assign` | Đạt | `routespec.go:460-464`; `service.go:473,497`; `handler_test.go` `TestPrepOverHTTP` (reader 403, editor 403 khi assign) |
| `prep.assign` optIn + implied `library.read` | Đạt | `catalog.go:284,392`; `catalog_test.go:48`; `permissions_test.go:91,105`; test HTTP assigner đọc được board |
| 409 `VERSION_LOCKED` khi không phải draft | Đạt | `service.go:527-553` → `lessonWrite:637-655` |
| Checklist JSONB: validate và giới hạn | Đạt: tối đa 50 mục, nhãn 1–200. Nhãn toàn khoảng trắng lọt qua (L2) | `dto.go:116`; `model.go:132`; `Checklist.Value` ghi `[]` thay cho nil |
| `lesson_count` 1..100, seed `Buổi 1..N` cùng tx | Đạt | `dto.go:21`; `service.go:58-71`; test HTTP `lesson_count:0` → 422 |
| Tạo draft mới từ bản cũ thì reset prep | Đạt | `service.go:234-242` (tạo `Lesson` mới); integration test kiểm v2 |
| `has_draft` filter | Đạt | `handler.go:81`; `repository.go:240-243`; test integration và HTTP |
| `CatalogVersion` 4→5 một lần + mirror | Đạt | `catalog.go:361`; `catalog_test.go:320-323`; `apps/web/src/test/msw/handlers.ts:17,330-336`; `permission-schemas.ts:86` |
| Snapshot route +3, audit action +2, `req()` trên route mutating | Đạt | `route_policy_snapshot_test.go:179-181`; `audit/action_test.go:144-145`; `routespec.go:460-464` |
| Swagger | Đạt | regen vào scratchpad trùng khớp |
| Web: board dùng `src/lib/kanban` | Đạt: `useKanban`, `kanbanReducer`, `KanbanDataSource`, phím `[`/`]`, live region | `use-prep.ts:118-176`; `prep-board-page.tsx:55-67` |
| Invalidate cache sau mutation | Đạt: board, lesson list, lesson details, template lists | `use-prep.ts:100-108` |
| Gate `has("prep.assign")` ở trang assign + deep-link | Đạt: redirect về board. **Nhưng thiếu `members.list`**, xem H1 | `prep-assign-page.tsx:35-43` |
| Nav `/prep` perm `library.read`, `OVERFLOW_LABELS`, `OVERFLOW_PATH_PREFIXES` | Đạt | `dashboard-layout.tsx:107,202,229` |
| Wizard 3 bước → board của draft | Đạt ở luồng chính. Luồng lỗi còn yếu (L3) | `template-create-wizard-page.tsx:103-122` |
| a11y cơ bản | Đạt: `aria-current="step"`, label cho select/date, hai live region, checkbox có label | các page tương ứng |

## Findings

### Critical
Không có.

### High

**H1 — Người chỉ có `prep.assign` không chọn được người phụ trách.** *(CONFIRMED qua đọc code)*
`apps/web/src/features/library/pages/prep-assign-page.tsx:37` gọi `useMemberDirectory(isResolved && canAssign)`, hook này gọi `GET /api/v1/centers/me/members/directory`. Route đó là `perm(..., authctx.PermMembersList, none())` ở `apps/api/internal/shared/routespec/routespec.go:235`, mà `members.list` là key opt-in. Trang assign chỉ kiểm `prep.assign`.
- Kịch bản: owner cấp `prep.assign` cho tổ trưởng chuyên môn, đúng mục đích của key. Tổ trưởng mở `/prep/:vid/assign`, directory trả 403, trang hiện "Không tải được danh sách thành viên.", dropdown chỉ còn "Chưa phân công". Tổ trưởng chỉ đặt được hạn. Nếu buổi đã có người phụ trách, select nhận value không có trong options, và khi đổi hạn thì request vẫn gửi assignee cũ nên không mất dữ liệu, nhưng tổ trưởng không đổi được người.
- Không test nào bắt được lỗi này: e2e `prep.spec.ts` đăng nhập bằng owner, còn test `prep-assign-page.test.tsx` mock directory luôn trả 200.
- Hướng sửa, cần lead hoặc user chọn vì có chạm catalog:
  1. Trả danh sách người có thể được phân công (thành viên đang hoạt động: id và tên) ngay trong `GET /library/versions/:vid/board` khi caller có `prep.assign`, hoặc làm một route `GET /library/assignees` gắn `prep.assign`. Cách này không nới quyền `members.list`.
  2. Cho `prep.assign` implied thêm `members.list`. Đơn giản hơn, nhưng làm rộng tầm đọc danh bạ, nên phải ghi rõ trong mô tả key và `catalog.go:382-395`.
  3. Tối thiểu: gate trang theo `has("prep.assign") && has("members.list")` và ghi rõ trong mô tả key là cần cả hai. Cách này làm UX kém đi.

### Medium

**M1 — PATCH prep ghi đè bằng giá trị đọc trước khi khoá, gây lost update giữa hai request đồng thời.** *(CONFIRMED qua đọc code, race phụ thuộc thời điểm)*
`apps/api/internal/features/library/service.go:530` đọc lesson (`GetLesson`) **trước** `LockVersion` (`service.go:535` → `:639`). Sau đó `UpdateLessonPrep` luôn ghi **cả** `prep_status` và `checklist` (`repository.go:467-471`), kể cả khi request chỉ gửi một trong hai trường.
- Kịch bản (READ COMMITTED): A kéo card sang "Đang làm" (`{prep_status:"doing"}`). Cùng lúc B tick một mục checklist trên trang buổi học (`{checklist:[…]}`). T_B đọc lesson lúc `prep_status='todo'`, rồi chờ lock trong khi T_A commit. Khi có lock, T_B ghi lại `prep_status='todo'` cùng checklist mới. Thay đổi của A bị mất mà không có lỗi nào. Assignment không dính lỗi này vì luôn thay cả khối.
- Sửa: đọc lesson bên trong `lessonWrite`, tức là sau khi đã có lock. Cách này cần `version_id`, lấy bằng một lần đọc nhẹ trước rồi đọc lại sau lock. Hoặc làm gọn hơn: dựng map update chỉ gồm các trường có trong request (`if req.PrepStatus != nil { fields["prep_status"] = … }`), để một PATCH từng phần không bao giờ ghi lại trường nó không gửi.

**M2 — Khối chuẩn bị trên trang buổi học mất chỉnh sửa chưa lưu và ghi đè trạng thái do người khác đặt.** *(CONFIRMED qua đọc code)*
- `apps/web/src/features/library/pages/template-lesson-page.tsx:112` đặt key `prep-${lesson.updated_at}`. Khi lưu nội dung buổi học (form phía trên), `useUpdateLesson` (`hooks/use-library.ts:202,209`) cập nhật detail với `updated_at` mới, `PrepEditor` bị remount, và checklist đang sửa dở bị mất (state local ở `lesson-prep-panel.tsx:109-110`).
- `lesson-prep-panel.tsx:140` luôn gửi `{ prep_status: status, checklist }`. `status` là giá trị lúc mở trang, nên nếu trong lúc đó ai đó đã kéo card trên board, bấm "Lưu chuẩn bị" sẽ đưa trạng thái về giá trị cũ. Lỗi này là lost update ở tầng UI, xảy ra cả khi M1 đã được sửa.
- Sửa: bỏ `updated_at` khỏi key, chỉ dùng `lesson.id`. Chỉ gửi `prep_status` khi người dùng thực sự đổi segmented (so với `lesson.prep_status` ban đầu), tương tự với checklist.

**M3 — Ô hạn hoàn thành lưu ở mỗi `onChange` và bị disable trong lúc lưu, nên gõ ngày bằng bàn phím bị ngắt.** *(PLAUSIBLE, tuỳ trình duyệt)*
`apps/web/src/features/library/pages/prep-assign-page.tsx:213-220`: mỗi `onChange` của `<input type="date">` gọi `save()` và PATCH ngay. `disabled={locked || mutation.isPending}` ở dòng 215 khoá ô trong lúc request đang chạy.
- Kịch bản: trên Chrome, khi gõ tay phần năm, mỗi phím đều tạo một ngày hợp lệ (`0002-…`, `0020-…`, `0202-…`, `2026-…`). Phím đầu gửi PATCH với năm 0002, ô bị disable và mất focus, các phím sau bị nuốt. Kết quả là hạn lưu thành năm 0002 cùng một toast "Đã lưu phân công". e2e dùng `fill()` nên không thấy lỗi này.
- Sửa: lưu ở `onBlur` (hoặc debounce), không disable ô trong lúc pending mà chỉ chặn gửi chồng request. Có thể thêm chặn năm < 2000 ở client.

### Low

**L1 — Ngữ nghĩa "gỡ thành viên" trong comment và commit message không khớp thực tế; plan còn ghi CASCADE.** *(CONFIRMED)*
`000032_…up.sql:4-6` và commit `d0c27ce` nói "gỡ thành viên chỉ bỏ phân công". Thực tế gỡ thành viên là soft-leave (`centers/repository.go:481` `SET left_at = now()`), nên FK không bao giờ chạy và `assignee_id` vẫn trỏ tới người đã rời. Board và `/prep` vẫn hiện tên người đó (`repository.go:198-205,404-412`). Cách làm này nhất quán với tasks (000022), nhưng khác với `class_staff` là loại được đóng stint cùng câu lệnh. `IsLiveMember` (`service.go:510`) cũng nằm ngoài tx, nên có TOCTOU với thao tác rời trung tâm chạy đồng thời. Sửa: chỉnh comment thành "chỉ khi xoá cứng". Nếu sản phẩm muốn thì bổ sung bước dọn phân công prep vào CTE gỡ thành viên, hoặc hiện "(đã rời)" trên card. Cập nhật sketch migration trong `phase-08-chuan-bi-tai-lieu.md` từ CASCADE sang `SET NULL (assignee_id)`.

**L2 — Nhãn checklist toàn khoảng trắng được API chấp nhận.** *(CONFIRMED bằng chạy validator)*
`apps/api/internal/features/library/model.go:132` chỉ có `required,min=1,max=200`, nên `"   "` lọt qua. Web có trim (`lesson-prep-panel.tsx:115`), nhưng client API khác thì không. Sửa: trim nhãn ở service rồi trả 422 nếu rỗng.

**L3 — Wizard đưa mọi lỗi tạo về bước 1, nơi không hiện root error.** *(CONFIRMED qua đọc code)*
`template-create-wizard-page.tsx:116` luôn gọi `setStep(0)`, nhưng `FieldError errors.root` chỉ render ở bước 3 (`:213`). Với 403 (quyền vừa bị thu), 422 `lesson_count` hay 5xx, người dùng bị đẩy về bước 1 mà không thấy thông báo nào. Sửa: chỉ quay về bước 0 khi lỗi là 409 hoặc lỗi trường thuộc form thông tin; các lỗi khác ở lại bước 3.

**L4 — `/prep` lấy tối đa 100 template, không phân trang.** *(CONFIRMED)*
`prep-page.tsx:32` dùng `per_page: 100`. Từ bản nháp thứ 101 trở đi sẽ âm thầm không hiện. Rủi ro thấp với trung tâm nhỏ. Sửa: phân trang, hoặc ít nhất hiện "còn N bản nháp" từ `total`.

**L5 — Thiếu test integration cho assignee đã rời trung tâm.** *(CONFIRMED)*
Nhánh `left_at IS NULL` của `IsLiveMember` (`repository.go:493-500`) chỉ được kiểm qua fake (`service_test.go` `TestAssignmentRequiresPrepAssignAndLiveMember`). Test integration mới chỉ thử outsider khác trung tâm. Sửa: trong `TestPrepBoardAndAssignment`, cho member rời trung tâm (`testutil` đã có helper `SET left_at = now()`, `testutil/fixtures.go:225`) rồi kiểm 422.

**L6 — `PATCH /assignment` thay cả khối, trong khi spec viết `{assignee_id?, due_date?}`.** *(Informational)*
`dto.go:119-125` quy định field bị bỏ hoặc null thì xoá, khác kiểu tri-state của tasks (`tasks/dto.go:202`). Hành vi này đã ghi trong swagger, và web luôn gửi đủ hai field, nên không có lỗi hiện tại. Rủi ro là một client khác chỉ gửi `due_date` sẽ xoá mất người phụ trách. Có thể giữ nguyên, nhưng nên ghi vào phase file để hợp đồng không bị hiểu nhầm.

## Edge cases đã soát

- `ChecklistItem` nil hoặc `null` JSON: `*[]ChecklistItem` nil thì giữ nguyên; `Value()` ghi `[]`; response luôn là `[]`.
- Board với status lạ: CHECK constraint chặn ở DB. Map `index` fallback về cột 0 không bao giờ chạy.
- `draft_assignees` dùng `DISTINCT full_name`: hai giáo viên trùng tên sẽ gộp thành một tên. Đây chỉ là hiển thị nên chấp nhận được.
- `formatDayMonth` tách chuỗi `YYYY-MM-DD`, không qua `Date`, nên không lệch múi giờ.
- Board của version published hoặc archived vẫn đọc được. Data source từ chối mọi move và lib công bố "Không chuyển được buổi."
- Truy vấn: summary thêm 4 subquery tương quan mỗi dòng, danh sách có phân trang, không có N+1. Trang assign gửi một request mỗi hàng khi người dùng thao tác.

## Recommended actions

1. H1: chọn một trong ba hướng (khuyến nghị hướng 1) rồi thêm test web với caller có `prep.assign` nhưng không có `members.list`.
2. M1: update từng phần theo field có trong request, hoặc đọc lesson sau lock. Thêm unit test "PATCH chỉ checklist không đổi prep_status".
3. M2: đổi key panel thành `lesson.id`, chỉ gửi field đã đổi.
4. M3: lưu hạn ở `onBlur`, bỏ `disabled` trong lúc pending.
5. L1–L6 khi tiện, trong đó L1 (sửa comment và phase file) nên làm cùng đợt vì chi phí thấp.

## Plan follow-up

Các mục của Phase 8 (migration, 3 route + `lesson_count` + `has_draft`, bump catalog, 5 trang web, nav/overflow, test, e2e) đã có code. Nên giữ phase ở trạng thái mở cho tới khi H1 và M1 được sửa. Phase file cần cập nhật sketch FK (`SET NULL (assignee_id)`) và ngữ nghĩa thay cả khối của assignment.

## Disposition

Áp dụng toàn bộ (TDD) trong `c1781f3` (API) và `c90e4f9` (web): H1 qua route `GET /library/assignees` gate `prep.assign`
(không cho `prep.assign` kéo theo `members.list`); M1 chỉ ghi field gửi lên; M2 key `lesson.id` + chỉ gửi field đổi;
M3 commit hạn khi blur; L1 sửa comment (chỉ comment, không đổi SQL); L2 trim + 422; L3 wizard giữ bước 3; L4 cảnh báo
vượt trang; L5 test assignee đã rời; L6 ghi hợp đồng thay cả hai field. `AssignmentSummary` trên trang buổi mẫu vẫn
đọc tên qua danh bạ khi có `members.list`, không có thì hiện "Đã phân công" — giữ nguyên.
