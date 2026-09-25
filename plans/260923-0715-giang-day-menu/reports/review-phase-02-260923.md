# Review — Phase 2: Lời mời nhận lớp (class invitations)

Ngày: 2026-09-23 · Branch `feat/giang-day-menu` · Phạm vi: diff chưa commit (bỏ qua `docs/diagrams/` và hunk `docs/architecture.md` theo yêu cầu).

## Verdict: **SHIP WITH FIXES**

Không có lỗi Critical/High. Tenancy, phân quyền, state machine và transaction của confirm đều đúng với spec và được test tích hợp chứng minh. Có hai lỗi Medium đã xác nhận nên sửa trước khi merge: pre-check khi gửi quá hẹp nên có thể tạo lời mời không bao giờ confirm được, và confirm giao_vien không invalidate cache buổi học. Phần còn lại là Low/Nit.

## Verification đã chạy

| Lệnh | Kết quả |
|---|---|
| `go build ./...` | OK |
| `go vet` classinvites, server, centers, migrations (+ `-tags integration` cho classinvites) | OK |
| `go test ./internal/features/classinvites/ ./internal/server/ ./internal/features/centers/ ./internal/features/audit/ ./internal/shared/routespec/` | tất cả `ok` |
| `golangci-lint run` classinvites + centers | 0 issues |
| `gofmt -l` các package đụng tới | sạch |
| `go tool swag init` ra thư mục tạm rồi diff với `apps/api/docs` | `swagger.json`, `swagger.yaml` giống hệt; `docs.go` chỉ khác tên package do thư mục output → swagger được sinh lại, không sửa tay; diff chỉ có dòng thêm |
| `npx vitest run src/features/roster` | 24 files, 189 passed, 3 skipped |
| `npx tsc -b --noEmit` (web) | OK |
| `npx eslint src/features/roster src/layouts/dashboard-layout.tsx e2e/class-invitations.spec.ts` | 0 error, 4 warning đều ở file có sẵn (class-dialog, class-ops-card, student-dialog, class-settings-page), không thuộc diff |

Không chạy: integration suite (`make test-api`), Playwright — theo ràng buộc. Các kết luận về integration dựa trên đọc `integration_test.go`.

## Findings

### Critical
Không có.

### High
Không có.

### Medium

**M1 — Send chỉ chặn "đã giữ cùng vai", nhưng `class_staff` chỉ cho một stint active mỗi người mỗi lớp → lời mời chết không confirm được.**
`apps/api/internal/features/classinvites/service.go:113-121`
- `uq_class_staff_active` là `(class_id, teacher_id) WHERE ended_at IS NULL` (000015). `classstaff.Service.Assign` từ chối mọi người đang có *bất kỳ* vai active (`HasActiveAssignment` → 409).
- Pre-check của Send chỉ so `role == req.RoleKey`. Ví dụ: người đang là `tro_giang` được mời làm `hoc_vu`, hoặc GV chính hiện tại được mời làm `tro_giang`. Send trả 201, member accept được, nhưng confirm luôn 409 "người này đã có vai trò đang hoạt động trong lớp". Lời mời kẹt ở pending/accepted cho tới khi owner hủy tay.
- `TestConfirmRollsBackWhenStintWriteFails` chứng minh đúng đường này ở confirm, nhưng chỉ vì stint được gán *sau* khi gửi.
- UI giảm nhẹ vì `invite-teacher-dialog.tsx:58-59` lọc bỏ mọi người đang có stint, nhưng API là ranh giới hợp đồng.
- Fix: với `tro_giang`/`hoc_vu`, trả 409 khi `len(roles[classID]) > 0`. Với `giao_vien`, chỉ chặn khi người đó đã là `giao_vien` (SyncPrimaryTeacher tự đóng vai khác của họ, `classstaff/repository.go:216-244`). Thêm unit test cho cả hai nhánh.

**M2 — Confirm giao_vien là bàn giao lớp, nhưng cache buổi học và stats không bị invalidate.**
`apps/web/src/features/roster/hooks/use-class-invitations.ts:67-78`
- Server chuyển các buổi planned tương lai sang GV mới. `useReassignTeacher` (`hooks/use-classes.ts:78-87`) vì vậy invalidate `classesKeys.all` và `sessionsKeys.all`.
- Confirm chỉ invalidate `classesKeys.lists()` và `detail(id)`. Nó bỏ sót `sessionsKeys.all`, là key mà tab buổi học của chi tiết lớp dùng (`components/class-sessions-tab.tsx:46`). Nó cũng bỏ sót `classesKeys.stats()`.
- Sau khi bấm "GV nhận lớp", tab Buổi học và các màn attendance còn hiển thị GV cũ cho tới khi hết staleTime.
- Fix: ở nhánh `confirmed.role_key === "giao_vien"`, invalidate `classesKeys.all` và `sessionsKeys.all` giống `useReassignTeacher`. Nhánh còn lại giữ `classStaffKeys.list` và `classesKeys.detail`. Thêm assertion trong `class-invitations-page.test.tsx`.

**M3 — Dialog confirm bàn giao cho phép bấm "Xác nhận" trước khi biết, hoặc khi không tải được, GV bị thay.**
`apps/web/src/features/roster/components/confirm-invitation-dialog.tsx:71,78-90`
- Spec (Risks) yêu cầu dialog phải nêu tên GV bị thay trước khi gọi API. Nút "Xác nhận" chỉ disable theo `confirm.isPending`.
- Khi `staff.isPending`, owner vẫn bấm được. Khi `staff.isError`, dialog rơi vào nhánh "sẽ làm giáo viên chính của lớp" mà không nêu ai bị thay. Như vậy một cuộc bàn giao thật có thể trông như gán lớp trống.
- Tên GV hiện tại lấy từ stint `class_staff` đang active, còn `Reassign` dùng `classes.teacher_id`. Khi hai nguồn lệch nhau, tên hiển thị có thể sai. Nguồn chuẩn nên là `klass.teacher_name` từ class detail.
- Fix: disable "Xác nhận" khi `isHandoff && (staff.isPending || staff.isError)`, và hiện lỗi kèm "Thử lại". Tốt hơn nữa là đọc GV hiện tại từ `useClass(invitation.class_id)`.

### Low

**L1 — Lời mời của lớp đã xoá mềm vẫn hiển thị và member vẫn accept được.**
`apps/api/internal/features/classinvites/repository.go` (`rowSelect`, `JOIN classes c` không lọc `c.deleted_at IS NULL`)
- `classes.Delete` chỉ soft-delete và không đụng tới `class_invitations`. Dòng pending/accepted vẫn nằm trong list của owner và member. Member accept được. Confirm trả 404 "class" vì `classes.Get` và `readAccess` loại lớp đã xoá, nên dòng đó chỉ còn cách hủy tay.
- Fix: thêm `AND c.deleted_at IS NULL` vào `rowSelect`, hoặc để `Send`, `respond` và `Confirm` trả 404 khi lớp đã xoá. Nên cân nhắc thêm việc từ chối Send vào lớp `archived`.

**L2 — `GET /class-invitations` của owner không phân trang và không giới hạn.**
`apps/api/internal/features/classinvites/repository.go` (`List`), `service.go:142-169`
- `docs/api-guidelines.md` §Pagination yêu cầu list endpoint dùng `page/per_page`. Các dòng terminal (declined/cancelled/assigned) tích luỹ mãi. Trang web tải toàn bộ rồi lọc chip ở client.
- Ở quy mô hiện tại rủi ro thấp, và `classstaff` List cũng không phân trang. Fix nhẹ nhất: thêm cửa sổ thời gian hoặc `LIMIT`, hoặc mặc định chỉ lấy dòng open cộng N ngày gần nhất. Nếu giữ nguyên thì ghi rõ lý do trong docs.

**L3 — `respond` (accept/decline) không kiểm tra `IsActiveMember`. Spec ghi "accept 404", thực tế là 401 hoặc 409.**
`apps/api/internal/features/classinvites/service.go:182-200`
- Qua HTTP, không thể khai thác: `RemoveMember` là đường duy nhất set `left_at`, nó disable account, và `ResolveScope` trả 401. Hook còn đã hủy các lời mời open.
- `TestRemovedMemberLosesOpenInvitations` dùng scope cũ và nhận 409, không phải 404 như spec mục (6).
- Fix: hoặc thêm `requireActiveMember` vào `respond` (giữ 404 cho người lạ), hoặc sửa verification trong phase file thành "401 qua HTTP / 409 với scope cũ". Hiện code và spec đang lệch nhau.

**L4 — Fixture MSW list không scope theo caller.**
`apps/web/src/features/roster/__tests__/roster-handlers.ts:836`
- API thật thu hẹp member về dòng của chính họ. MSW trả mọi dòng, nên test "as the invited member" render lời mời của Cô Hương *và* Thầy Nam cho một member, kèm nút Chấp nhận trên dòng của người khác. Đây là trạng thái production không có.
- Kiểm tra SELF_INVITE so với `klass.teacher_id` thay vì caller.
- Fix: lọc theo `useAuthStore` user id khi caller không phải owner, và so SELF_INVITE với caller.

**L5 — Link tên lớp trên trang lời mời trỏ tới lớp mà invitee chưa có quyền đọc.**
`apps/web/src/features/roster/pages/class-invitations-page.tsx:143`
- Trước khi confirm, invitee không có stint, nên `/classes/:id` trả 404 (own-rows). Member bấm link sẽ gặp trang lỗi.
- Fix: chỉ render `Link` khi `isOwner`, hoặc khi `status === "assigned"`. Các trường hợp khác render text.

**L6 — Confirm hoặc cancel lỗi (409 "vừa thay đổi trạng thái") không refetch list.**
`use-class-invitations.ts` (mọi mutation chỉ invalidate ở `onSuccess`)
- Trên màn hình còn dòng cũ với nút vẫn bấm được. Fix: đưa invalidate `classInvitationsKeys.lists()` vào `onSettled`.

### Nit

- `invite-teacher-dialog.tsx:54`: vai mặc định là `giao_vien`, tức là bàn giao lớp. Mặc định `tro_giang` an toàn hơn, vì lỡ tay mời sai vai sẽ đưa tới một cuộc bàn giao.
- `class-team-section.tsx:71`: `aria-label` trên `<li>` ghi đè nội dung, và trình đọc màn hình hỗ trợ không đều. E2E đang dựa vào nó (`getByRole("listitem", { name })`). Chấp nhận được, nhưng nên dùng text ẩn `sr-only` nếu sau này đổi.
- `repository.go` `rowSelect`: `INNER JOIN teachers ib ON ib.id = ci.invited_by` và `invited_by` không có FK. Nếu tài khoản người mời bị xoá cứng, dòng biến mất khỏi list một cách im lặng. Nên dùng `LEFT JOIN` với `COALESCE`, hoặc thêm FK.
- `SendRequest.Message`: chuỗi rỗng hoặc toàn khoảng trắng được lưu thành `""` thay vì NULL. Web đã trim nên chỉ ảnh hưởng client khác.
- Audit của `send` ghi `EntityType: "class"` và id lớp, nên không truy ra được id lời mời vừa tạo từ audit. Chấp nhận được vì route không có id lời mời.

## Kiểm tra theo focus points

1. **Tenancy/authz: đạt.** Mọi query repo có `center_id = sc.CenterID`. List của member bị ép `teacher_id = caller` trước khi gọi repo, và filter không nới rộng được. Lời mời của người khác hoặc trung tâm khác trả 404 (`visible`, `respond`). Routespec phân loại send/cancel/remind/confirm là `KindOwnerOnly`, list/accept/decline là `KindService`. Snapshot có +7 route, audit có +6 action. Self-invite trả 422 `SELF_INVITE`, gửi cho member không active trả 422 `MEMBER_INACTIVE`. Path id sai định dạng trả 404. `catalog.go` không bị đụng.
2. **Transaction/state machine: đạt.** `Confirm` chạy `Transition` và `Reassign`/`Assign` trong một `WithinTx`. `withCenterLock` của handoff dùng lại tx ngoài (`database/tx.go:23-26`), nên TryLock bận → 409 rollback cả lời mời. Test tích hợp đã chứng minh rollback. Các trạng thái terminal được chặn bằng `status IN (from)`. Hai lần send đồng thời thua ở `uq_class_invitations_pending`, được map thành 409.
   - Tương tác giữa `RemoveMember` và `Confirm`: cả hai khoá cùng dòng `class_invitations` trước mọi khoá khác. Confirm chỉ *try-lock* advisory lock và không chờ. `RemoveMember` không lấy advisory lock. Vì vậy không có vòng deadlock. Nếu `RemoveMember` chạy trước, confirm thấy 0 dòng → 409. Nếu `Confirm` chạy trước, `CancelOpenForMember` chờ, rồi đánh giá lại và bỏ qua dòng `assigned`. Member bị gỡ khi đó vẫn giữ stint, nhưng đó là hành vi có sẵn của `RemoveMember`, vốn không đóng `class_staff`.
   - Hook không bỏ qua im lặng khi đã wire. Nhánh nil chỉ log Warn, và `task_handover_wiring_test.go` khẳng định router có wire.
3. **Contract: đạt.** Không route hay response cũ nào thay đổi. Migration down là `DROP TABLE IF EXISTS`. `TestMigrationRoundTrip` phủ up/down, và số bước `MigrateDown` được cập nhật lên 22. Swagger được sinh lại (đã xác minh bằng diff).
4. **Web: phần lớn đạt.** Không đụng `src/components/ui/`. Chip dùng `role="radio"` trong `radiogroup` đúng quy ước của `HvChip` (`aria-checked`). Lỗi `ApiError` hiện trong dialog và toast. Các thiếu sót là M2, M3, L4, L5, L6.
5. **Test gaps, xem phần Test gaps bên dưới.**

## Test gaps

Nên thêm cùng các fix:
- Unit Send: người đang giữ vai khác (tro_giang ↔ hoc_vu, giao_vien → tro_giang) trả 409. Người đang tro_giang được mời giao_vien thì được phép (M1).
- Vitest confirm giao_vien invalidate `sessionsKeys` (M2). Dialog disable "Xác nhận" khi staff đang tải hoặc lỗi (M3).
- MSW list scope theo caller (L4).

Nice-to-have:
- Integration: hai `Send` đồng thời cùng (class, teacher) → một 201 và một 409 (index partial).
- Integration: lớp soft-delete và lời mời open (L1).
- Integration: confirm giao_vien cho người đang là tro_giang của lớp. Vai cũ phải đóng và `uq_class_staff_active` phải giữ.

## Decisions not reversed

- **Unique index chỉ trên `status = 'pending'`** (spec, migration sketch). Hệ quả là một người có thể đồng thời có lời mời `accepted` và một lời mời `pending` mới cho cùng lớp, và khối Đội ngũ hiện cả hai dòng. Trade-off: mở rộng index sang `IN ('pending','accepted')` đóng được lỗ này nhưng đổi hợp đồng đã chốt. Các lựa chọn: (a) giữ nguyên; (b) mở rộng index; (c) chỉ mở rộng pre-check `HasPending` trong service. Việc này cần người dùng quyết định.
- **Owner được confirm thẳng từ `pending`** mà không cần invitee chấp nhận (spec: "pending | accepted → assigned", phục vụ đồng ý offline). Giữ nguyên. UI đã có dòng cảnh báo "bỏ qua bước chấp nhận".
- **Accept của member đã rời trả 404** (spec). Không đảo quyết định, chỉ ghi nhận ở L3 rằng thực tế là 401 hoặc 409. Chọn sửa code hay sửa spec là quyết định của lead.
- Theo memory đã chốt về class-staff, việc dùng 404 thay 403 khi lookup chéo là cố ý. Review không raise lại.

## Unresolved questions

- Có muốn chặn Send vào lớp `archived` không? Spec không nói.
- L3: sửa code để trả 404, hay cập nhật verification (6) trong phase file?

Status: DONE_WITH_CONCERNS
Summary: Phase 2 đạt về tenancy, authz, transaction và contract, và mọi kiểm tra unit/lint/typecheck/vitest đều pass. Có hai lỗi Medium đã xác nhận (M1 pre-check Send quá hẹp, M2 thiếu invalidate sessions khi confirm giao_vien) và một Medium UX an toàn (M3) nên sửa trước khi merge.
Concerns/Blockers: Không chạy integration suite hay Playwright theo ràng buộc. Các kết luận integration dựa trên đọc code và test.

## Disposition (sau review, 2026-09-23)

| Finding | Quyết định | Thay đổi |
|---|---|---|
| M1 | Sửa | `service.go` `refuseHeldRole`: vai hỗ trợ (`tro_giang`/`hoc_vu`) bị 409 khi người đó đang giữ bất kỳ vai active nào; `giao_vien` chỉ bị chặn khi đã là `giao_vien`. Unit test trong `TestSendValidation` phủ ba nhánh. |
| M2 | Sửa | `useConfirmClassInvitation`: confirm `giao_vien` invalidate `classesKeys.all` + `sessionsKeys.all` như `useReassignTeacher`; vai khác giữ `detail` + `lists`. Vitest spy `invalidateQueries`. |
| M3 | Sửa một phần | Nút "Xác nhận" disable khi `isHandoff && (staff.isPending \|\| staff.isError)`; nhánh lỗi hiện thông báo kèm "Thử lại". Không đổi nguồn tên GV sang `useClass`: schema `Class` không có `teacher_name`, còn stint `giao_vien` trong `class_staff` được `SyncPrimaryTeacher` giữ khớp với `classes.teacher_id`. Vitest phủ trạng thái lỗi → retry → mở khoá. |
| L1 | Sửa | `rowSelect` thêm `AND c.deleted_at IS NULL`: lời mời của lớp đã xoá mềm biến mất khỏi list, accept/confirm trả 404. |
| L2 | Giữ nguyên | Cùng hình dạng với `GET /classes/:id/staff` (không phân trang). Chip lọc ở client là quyết định của phase file. Ghi nhận cho Phase 9 nếu số dòng terminal tăng. |
| L3 | Sửa code theo spec | `respond` kiểm `IsActiveMember(caller)` → 404. Integration `TestRemovedMemberLosesOpenInvitations` đổi kỳ vọng 409 → 404; unit test mới cho accept/decline của người đã rời. |
| L4 | Giữ nguyên | MSW không có danh tính caller (fixture `/centers/me` mới phân biệt owner/member); scope theo caller đã được chứng minh ở `integration_test.go` (`TestListScopesMemberToOwnInvitations`). Không rework fixture để tránh test phụ thuộc vào auth store. |
| L5 | Sửa | Tên lớp chỉ là `Link` khi `isOwner` hoặc `status === "assigned"`; member thấy text thuần. |
| L6 | Sửa | Mọi mutation lời mời invalidate `classInvitationsKeys.lists()` trong `onSettled`. |
| Nit vai mặc định `giao_vien` | Giữ nguyên | Nút là "+ Mời GV" và luồng chính của phase là "GV nhận lớp"; dialog confirm nêu tên GV bị thay trước khi gọi API. |
| Nit `INNER JOIN teachers ib` | Giữ nguyên | `teachers` không có đường xoá cứng trong API. |
| Nit message rỗng | Sửa | `Send` trim và lưu NULL khi rỗng; unit test `TestSendStoresBlankMessageAsNull`. |
| Câu hỏi: chặn Send vào lớp `archived`? | Chưa làm | Spec không yêu cầu; để mở cho user quyết. |

Xác minh sau khi sửa: `go test ./internal/features/classinvites/` ok; `go test -tags=integration -p 1 ./internal/features/classinvites/... ./internal/features/centers/...` ok; `make test-api-unit`, `make scopelint`, `make lint` xanh; `npx vitest run src/features/roster` 190 passed; e2e `class-invitations.spec.ts` chạy trên stack `teka-e2e` cô lập (lần 1 trước fix: 1 passed; lần 2 sau fix: xem journal).
