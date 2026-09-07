# Code Review — Phase 1: view_all không nới quyền ghi

- Ngày: 2026-09-06
- Phạm vi: `plans/260906-0627-authz-write-scope-root-cause/phase-01-view-all-write-scope-hotfix.md`
- Nhánh: `master` @ `57986c5` (35 file chưa commit)

## Kết luận

- **Điểm: 8/10**
- **Critical: không**
- Trục ngữ nghĩa `CenterWideFor` (đọc) vs `WriteWide` (ghi) được áp dụng nhất quán qua 10 package. Không tìm thấy hồi quy luồng owner hay luồng "member đọc/ghi hàng của chính mình".

## Số liệu kiểm chứng

| Hạng mục | Kết quả |
|---|---|
| `go vet -tags integration ./...` | sạch |
| `make lint-api` | 0 issues |
| `go test ./internal/features -run TestRepositories` | ok, 0.028s |
| Test pin mới `TestViewAllWidens<Resource>ReadsNotWrites` | 8/8 tài nguyên |
| Findings | 0 critical, 2 major, 8 minor |

## Major

### M1 — `Record` trả về dữ liệu hoá đơn của teacher khác cho member chỉ có `payments.create`

- `apps/api/internal/features/payments/repository.go:402-421` (`ResolveContactScope` bỏ hoàn toàn bộ lọc teacher, theo D9)
- `apps/api/internal/features/payments/service.go:120` (`s.repo.ListAllocations(ctx, ownerScope, payment.ID)`)

`ResolveContactScope` giờ tra cứu theo center. Trong production contact luôn neo vào owner vì `contacts/service.go:38-41` chỉ cho owner tạo, nên một member giữ `payments.create` (key mặc định của role) có thể POST thanh toán cho **mọi** contact trong center. Đó là phần D9 đã chấp nhận. Phần chưa được cân nhắc là response: `Record` đọc allocation bằng `ownerScope` (scope của teacher sở hữu contact) và trả về `student_name`, `total_due`, `paid_amount` của hoá đơn thuộc owner. Member không đọc được các trường này ở bất kỳ endpoint nào khác — `payments.Get` dùng scope người gọi và trả 404.

Đề xuất: hoặc owner xác nhận cả payload trả về (không chỉ tầm với ghi) khi chốt Open Question 1, hoặc lọc `allocRows` qua scope đọc của người gọi trước khi trả, ví dụ gọi `ListAllocations(ctx, sc, payment.ID)` và chỉ dùng `ownerScope` cho nhánh recalc trong transaction.

### M2 — Xoá `TestGenerateMasksPhoneForCenterWideReader` là mất coverage thật

- `apps/api/internal/features/statements/phone_privacy_integration_test.go` (xoá 66 dòng)

Test bị xoá là chỗ duy nhất khẳng định `phone_visible` được suy ra theo **người xem**, không theo teacher của kỳ. Sự phân đôi đó vẫn còn sống: `GenerateForSend` và `GenerateForSendClass` truyền cả hai scope vào `TargetContacts(ctx, periodScope, viewer, periodID)` (`statements/repository.go:91`) và `TargetContactsClass` (`:104`). Execution Note nói ý định này đã được `TestViewAllWidensStatementReadsNotWrites` giữ, nhưng test đó chỉ khẳng định tầm với 404, không chạm tới việc suy ra số điện thoại.

Đề xuất: thêm lại một test tương đương chạy qua `GenerateForSendClass`, với người gọi giữ stint gửi báo cáo trên kỳ của teacher khác, và khẳng định `phone_visible` theo người gọi.

## Minor

1. `apps/api/internal/features/scoping_guard_test.go` — guard chỉ quét `*/repository.go`, nên `CenterWideFor` ở tầng service không được canh: `sessions/service.go:211` (ngoại lệ D3), `enrollments/service.go:114`, `classstaff/service.go:49`. Guard cũng chỉ bắt lời gọi `CenterWideFor` trực tiếp; một helper ghi gọi helper đọc (`readNarrow`) sẽ lọt. Đã grep toàn bộ `internal/features/*/repository.go`: hiện không có hàm ghi nào gọi helper đọc, nên đây là lỗ hình dạng chứ chưa phải lỗ thật.
2. `apps/api/internal/features/grading/service.go:534-536` — `resolveClass` dùng `classes.Get` (write port), nên member có `classes.view_all` mất quyền ghi grading component. Đúng theo D1 nhưng không nằm trong danh sách file của phase và không có test pin.
3. `apps/api/internal/features/sessions/service.go:196-198` — `validateRange` chạy trước `GetReadableWithRoles`, nên khoảng ngày sai trên lớp không đọc được giờ trả 422 thay vì 404/403. Rò rỉ thông tin không đáng kể nhưng là đổi hình dạng lỗi không ghi trong Execution Notes.
4. `plans/260830-2310-*/inventory.md:117-127` — blockquote "Superseded on 2026-09-06" chèn giữa dòng phân cách header và dòng dữ liệu đầu tiên, làm vỡ bảng markdown.
5. `apps/api/internal/features/sessions/integration_test.go` — trong `TestViewAllMaterialisesSessionsExactlyAsOwnerWould`, hai biến `byMember` và `byOwner` đều là lớp do `owner.ID` sở hữu; tên gây hiểu nhầm là lớp của member.
6. Fixture test tạo contact neo vào member (`testutil.Contact(t, db, member.ID)`) — production không sinh ra hình dạng này vì `contacts.Create` chỉ owner. Pin vẫn đúng nhưng lệch thực tế.
7. `PermNotificationsViewAll` sau thay đổi không còn được tham chiếu ở đâu ngoài `catalog.go:125,229`. Đối chiếu `git show HEAD` xác nhận đây **không** phải hồi quy đọc (`scoped` cũ chỉ phục vụ `MarkSent`), nhưng key đang được quảng cáo mà không cấp gì cho tới phase 3.
8. `notifications.MarkSent` khi bị từ chối là no-op im lặng (trả nil, update 0 dòng) trên `POST /api/v1/notifications/mark-sent`, thay vì lỗi.

## Xác nhận (a)–(e)

**(a) Success Criteria** — Đạt. Có đủ 8 test `TestViewAllWidens<Resource>ReadsNotWrites` (sessions, classes, enrollments, attendance, billing, payments, statements, notifications), thêm `TestViewAllMaterialisesSessionsExactlyAsOwnerWould` cho ngoại lệ D3, `TestMemberRecordsPaymentForCenterContactWithoutViewAll` cho D9, và `TestPolicyHTTPViewAllNeverWidensWrites` ở tầng HTTP (`internal/server/policy_integration_test.go`). Guard AST `TestRepositoriesWidenWritesThroughWriteWideOnly` chạy xanh.

**(b) Không hồi quy nghiệp vụ** — Đạt. Đã đi hết caller của các hàm đổi: billing `Close`/`Draft`/`AddAdjustment`/`VoidInvoice` qua `GetPeriodForWrite`/`GetInvoiceForWrite`; payments `Reallocate`/`Reverse`; sessions `ListRangeReadable` → `materialiseRange`; classes `ListEffectiveSchedules`; statements `Generate`/`GenerateForSend`/`Revoke`; notifications `MarkSent`. Điểm chốt: **mọi anchor scope tổng hợp trong repo đều là scope trần** (`authctx.Scope{TeacherID, CenterID}`, không `Perms`, không `IsOwner`), nên `CenterWideFor` và `WriteWide` cùng false — việc hoán đổi ở các call site đó trung tính về hành vi. Luồng owner giữ nguyên vì `WriteWide() = IsOwner`. Luồng member đọc/ghi hàng của chính mình giữ nguyên vì bộ lọc `teacher_id` không đổi. `statements.GetPeriodStatus` vẫn sống qua method value tại `statements/service.go:201`, nên siết WriteWide ở đó là thay đổi hành vi có chủ đích, đã có test pin. `classstaff.readAccess` nới theo `classes.view_all` không thành lỗ ghi vì `Assign`/`Remove` đều kiểm `!sc.IsOwner` ngay đầu.

**(c) Không đổi public contract** — Đạt. Không có thay đổi route, DTO, envelope response, migration, swagger (`make api-docs` không diff theo báo cáo của lead). `billing.Repository` thêm `GetPeriodForWrite` và `GetInvoiceForWrite`; chỉ có một implementation thật (`gormRepository`) và hai fake (`service_test.go`, `close_test.go`), cả hai đã cập nhật — `go vet -tags integration ./...` sạch xác nhận không sót implementation nào.

**(d) Tuân thủ pattern** — Đạt. Tên helper nhất quán theo trục: `readScoped`/`scopedRead` cho port đọc, `writeScoped` cho port ghi, `readNarrow(q, sc, col)` cho predicate nội tuyến, `GetXForWrite` cho getter ghi. Comment mô tả bất biến, không nhúng mã plan/phase/finding. Không thấy abstraction thừa; `readNarrow` và `materialiseRange` đều là trích xuất từ code trùng lặp sẵn có.

**(e) Không lỗi lint/type/build mới** — Đạt. `go vet -tags integration ./...` không output; `make lint-api` → 0 issues; guard test pass. Không chạy `make test-api` đầy đủ theo yêu cầu về tải máy.

## Mã lỗi trong test pin

Nhất quán với gate tương ứng:
- **403** khi lớp đọc được nhưng role thiếu capability — `classes.GetWritable` phân biệt được quan hệ.
- **404** khi không có quan hệ nào (write port lọc mất hàng, `Take` trả `ErrRecordNotFound`).
- **422** cho lỗi validate tham số, xảy ra trước khi phân giải tài nguyên.

Ngoại lệ duy nhất là mục Minor 3: thứ tự `validateRange` trước `GetReadableWithRoles` khiến 422 thắng 404 trên lớp không đọc được.

## Câu hỏi chưa giải quyết

1. Response của `Record` (M1) có nằm trong phạm vi owner đã duyệt ở D9 không, hay chỉ tầm với ghi? Cần chốt trước khi deploy, cùng Open Question 1 của plan.
2. Grading component write bị siết cho member có `classes.view_all` (Minor 2) là chủ ý hay tác dụng phụ? Nếu chủ ý, nên bổ sung vào danh sách file của phase và thêm test pin.
3. `notifications.view_all` sẽ được đấu lại ở phase 3, hay nên gỡ khỏi catalog để không quảng cáo key rỗng?
