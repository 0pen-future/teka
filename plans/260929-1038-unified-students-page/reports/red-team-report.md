# Red-team review: plan "Trang Học sinh hợp nhất" (260929-1038)

Phạm vi: plan.md, phase-01..04, brainstorm Option A (quyết định người dùng là chuẩn). Mọi bằng chứng đã được đối chiếu với source hiện tại (chỉ đọc, không sửa code/plan).

## Tóm tắt

Bảng kê của phase 1 bỏ sót một chỗ lộ SĐT, và có một bước sẽ làm hỏng luồng gửi mà plan tự nói là "giữ nguyên". Phase 3-4 không nhắc tới e2e, trong khi ít nhất 5 spec sẽ đỏ. Các claim kỹ thuật còn lại (dòng code, view, route gate, implied keys) phần lớn đúng.

## Findings

| # | Mức | Persona | Finding | Bằng chứng | Đề xuất sửa plan |
|---|---|---|---|---|---|
| 1 | Critical | Security | `BulkSendRow.Phone` trả SĐT thô, không qua quy tắc SĐT. Học vụ gửi theo lớp (capability `statement.send`, không có `contacts.view_all`) gọi `POST /billing-periods/:id/notifications/bulk` với `class_id` + `zalo_manual` sẽ nhận SĐT của mọi gia đình trong lớp. Bảng kê phase 1 không có dòng này nên AC6 ("không API nào") trượt. | `notifications/dto.go:48` (`Phone string`), `notifications/service.go:328` (`Phone: target.ContactPhone`), gate lớp `service.go:122-127` (`AuthorizeClassSend`, không cần `reports.send`) | Thêm vào bảng kê: đổi thành `Phone *string`, gán khi `sc.PhoneVisible()`. Web: `bulkSendRowSchema.phone` đổi thành `.nullable()` (`collections-schemas.ts:210`), `message-card.tsx:41` ẩn khi null. Viết test tích hợp: học vụ gửi theo lớp nhận `phone: null`. |
| 2 | High | Failure | Bước 5 phase 1 đổi cả `ZaloMappingsClass` sang "không wide thì map rỗng". Hàm này không dùng fragment SĐT: nó giới hạn theo ghi danh của lớp, và là cơ sở để học vụ gửi `zalo_personal` theo lớp. Nếu đổi, mọi lần gửi theo lớp của học vụ sẽ rơi hết về copy thủ công, preview báo 0 người đã ghép. Plan cũng tự mâu thuẫn với câu "luồng gửi giữ nguyên". | `notifications/repository.go:630-660` (không có `PhoneVisibleViaContact`); các caller `service.go:250, 546, 759`; `docs/api-guidelines.md:304-306` ("their send path depends on mapping") | Không đổi `ZaloMappingsClass`: mapping là `zalo_user_id`, không phải SĐT. Chỉ `ZaloMappings` bỏ nhánh `hoc_vu`, và nhánh này vốn đã chết ở luồng gửi gia đình vì đã gate `ReportsOversight` (implies `contacts.view_all`). Nếu người dùng muốn chặn cả luồng này thì phải hỏi rõ, đừng làm ngầm. |
| 3 | High | Assumption / user decision concern | Người dùng quyết định "liên kết Zalo người liên hệ chỉ chủ trung tâm", và brainstorm ghi là "khớp API hiện tại". Thực tế thì sai: `scopedMappingWrite` đang cho `reports.send` và `hoc_vu` ghi. Plan giữ `ReportsOversight()` và dẫn tới "câu hỏi mở 1/2", nhưng hai câu hỏi này **không có trong file nào**. | `contacts/repository.go:105-118`; `grep "câu hỏi mở"` chỉ thấy ở `phase-01:35,40` | Thêm mục Unresolved Questions. Mặc định theo quyết định người dùng: `scopedMappingWrite` dùng `sc.WriteWide()` (owner). Nếu giữ `reports.send` thì cần người dùng xác nhận lại vì đó là đảo ngược quyết định. |
| 4 | High | Failure | Nút "Tự động ghép Zalo" hiện không gate owner. Khi dời vào tab mới, vai trò chỉ có `contacts.view_all` sẽ ghép được (MatchFriends cho qua), nhưng lúc lưu mapping sẽ bị 404 vì `scopedMappingWrite` loại trừ view_all. Điều này vi phạm ràng buộc "không hiện hành động API từ chối". Bước 6 phase 4 chỉ gate "Thêm người liên hệ". | `roster/pages/contacts-page.tsx:62-72` (chỉ gate theo `zaloReady`), `contacts/repository.go:111-118` | Gate "Tự động ghép Zalo" và nút liên kết/bỏ liên kết ở `contact-detail-page` theo đúng predicate của backend (owner, theo #3). Thêm test cho `contacts-tab`: member view_all không thấy nút ghép. |
| 5 | High | Failure | Plan không nhắc e2e, trong khi các spec sau sẽ đỏ: `class-staff-read.spec.ts:30-33` (chờ `/students` chuyển về `/`); `roster.spec.ts:29-36` và `audit.spec.ts:36-44` (`/contacts` + "Thêm người liên hệ"); `roster.spec.ts:50-53` và `courses.spec.ts:107-108` ("+ Tạo lớp mới" trên `/students`, mà ClassesTab bị xoá); `courses.spec.ts:133` (tab "Lớp học"); `helpers/ux-routes.ts:42,53`. | các file:dòng nêu bên trái (`apps/web/e2e/`) | Thêm bước "cập nhật e2e" vào phase 4: tạo lớp chuyển sang `/classes`; contacts chuyển sang `/students?tab=contacts`; staff-read chờ danh sách chỉ-xem + HvNotice. Verification thêm chạy e2e trên stack cô lập. |
| 6 | Medium | Assumption | Danh sách test phase 1 thiếu `notifications/zalo_mappings_scope_integration_test.go:46` (`TestZaloMappingsFollowsCenterWideOrStint`, khẳng định học vụ thấy mapping). Plan cũng không nói tới đổi chữ ký: `statements.TargetContacts/TargetContactsClass(…, viewer Scope, …)` sẽ thừa `viewer` (`statements/repository.go:377,395`), `students.withContact(q, teacherID, centerID)` thừa hai tham số (`students/repository.go:131`), kéo theo fake repo trong test. | các file:dòng bên trái | Bổ sung test đó vào bước 8. Ghi rõ việc bỏ tham số hoặc đổi interface cùng với fake repo, tránh để tham số chết. |
| 7 | Medium | Scope | Không có cơ chế phân trang có sẵn: contacts `per_page: 50`, students `per_page: 50`, và không có UI trang. Tab "Tất cả" cho chủ trung tâm hơn 50 học sinh sẽ hiện số `total` khác với số dòng thấy được. Plan chỉ ghi "dùng cơ chế có sẵn nếu có", trong khi cơ chế đó không tồn tại. | `contacts-page.tsx:56`, `students-page.tsx:156` | Chốt một trong hai: (a) thêm `?page=` + nút trang/"Xem thêm" (vẫn trong phạm vi AC2), hoặc (b) ghi rõ giới hạn và để người dùng quyết. |
| 8 | Medium | Assumption | Công nợ theo tháng cộng cả hoá đơn `draft` trong kỳ `open`, vì view chỉ loại `void`. Số nợ "tháng này" có thể là số tạm tính chưa chốt sổ. | `migrations/000007_centers.up.sql:295-307` (`WHERE i.status <> 'void'`); trạng thái `000001:295` | Ghi quy tắc vào Contract (khớp màn Thu tiền, hoặc chỉ kỳ `closed`) và thêm test cho trường hợp kỳ open/draft. |
| 9 | Medium | Failure | Phase 3 có `dependencies: []`. Nếu merge riêng, giáo viên thấy "Học sinh" nhưng bị chuyển về `/`, còn `/contacts` chuyển tới một tab chưa tồn tại. Plan có ghi ở mục rủi ro nhưng không ràng buộc bằng dependency. | `phase-03:6`, `phase-03:57-60` | Đặt phase 3 `dependencies: [4]` hoặc gộp 3+4 thành một PR/merge unit. |
| 10 | Low | Assumption | Có thể đóng rủi ro "`/centers/me` có implied keys không": `/centers/me` trả `EffectiveKeys()`, hàm này duyệt `Has()` trên PermSet đã gộp implied. | `authctx/permissions.go:92-99,127-135`; `centers/service.go:285,294` | Xoá mục rủi ro này ở phase 4, thay bằng một test web: member chỉ có `reports.send` thấy tab Người liên hệ. |
| 11 | Low | Assumption | Phương án dự phòng ở phase 2 ("nếu routespec không cho view_all làm route key") là thừa. Key `*.view_all` là grantable (`def` → `Grantable: true`) và test chỉ đòi hỏi key grantable. Nhóm route của collections là `/billing-periods`, không phải `/collections`. | `authctx/catalog.go:160-174`; `routespec_test.go:53-66`; `collections/routes.go:7-10` | Gate thẳng bằng `PermBillingViewAll`. Ghi rõ là tạo group mới `/collections` trong `RegisterRoutes`. Query binding dùng tag `form:` chứ không dùng `json:` của `EnsurePeriodRequest`. |
| 12 | Low | Assumption | Tài liệu đang có claim sai: "a sender who cannot see a phone can still send" nhưng thực tế vẫn nhận SĐT (#1). Phase 1 bước 10 chỉ nói bỏ câu về Zalo. | `docs/api-guidelines.md:301-306` | Viết lại cả đoạn: sender nhận `phone: null` trong `BulkSendRow`; mapping theo lớp vẫn mở cho người gửi theo lớp (theo #2). |
| 13 | Low | Scope | Đổi icon "Lớp cần tuyển sinh" sang `UserPlusIcon` là thay đổi không ai yêu cầu. | `phase-03:30-31`; `dashboard-layout.tsx:106` | Bỏ, hoặc tách thành đề xuất riêng. |
| 14 | Low | Scope / user decision concern | Nhóm "Trung tâm" luôn có "Cài đặt trung tâm" (không gate perm), nên bộ lọc nhóm rỗng không bao giờ ẩn được nó. Sidebar còn giữ "Hồ sơ học sinh" (`/records`, cũng `students.list`), thành ra có hai mục học sinh cho giáo viên. | `dashboard-layout.tsx:90,189` | Chỉ ghi nhận. Hỏi người dùng có muốn gộp hoặc đổi tên "Hồ sơ học sinh" hay không, không tự làm. |
| 15 | Low | Security | `imports.run` dry-run là oracle kiểm tra SĐT có trong danh bạ hay chưa (`FindIDByPhone` toàn trung tâm, tách đếm reuse/create). Không mới và nằm ngoài phạm vi. | `imports/apply.go:188-204` | Chỉ ghi vào mục Security Considerations. `imports.run` là opt-in của owner. |

## Security sweep (persona 4)

- Các SELECT có `contacts.phone`/`c.phone`: students `repository.go:134`, statements `:311,386,404`, collections `:152`, notifications `:294`, zalo `ReachableContactPhones :172`. Sau phase 1, tất cả đều được che bằng `sc.PhoneVisible()` hoặc bị xoá.
- **Ngoại lệ duy nhất:** `BulkSendRow.Phone` (#1).
- `ContactResponse.Phone` (`contacts/dto.go:35`) an toàn sau khi `scopedRead` thành `WHERE false` với người không wide.
- Audit chỉ ghi `phone_masked` (`audit/subscriber.go:200`); middleware bỏ query string (`request_events.go:114`).
- Friend request dùng `user_id`, không dùng SĐT.
- Endpoint nợ mới chỉ trả `contact_id` + `outstanding`, gate bằng `billing.view_all`. Không lộ SĐT hay tên, không lộ dữ liệu trung tâm khác (lọc `vcb.center_id`). Không phát hiện rò rỉ.

## Claims đã xác minh đúng

- Số dòng trong bảng kê phase 1.
- Cột `teacher_id, center_id, period_id, contact_id, outstanding` của `v_contact_balance`.
- `billing_periods(year, month, deleted_at, center_id)`.
- Kỳ tính theo giáo viên: `billing/service.go:45-58`, `uq_billing_periods(teacher_id, year, month)`.
- Collections chỉ đọc một kỳ: `repository.go:110-117`.
- `has()`: `use-center-context.ts:55`.
- Tạo lớp có ở `class-list-page.tsx:179`.
- "Ghi danh vào lớp" không gate: `student-detail-page.tsx:78-80`.

## Unresolved questions

1. Liên kết Zalo: chỉ owner (theo quyết định người dùng), hay giữ cả `reports.send`?
2. Học vụ gửi theo lớp (`statement.send`, hiện chỉ gọi qua API, không có UI — `class-staff-write.spec.ts:130-134`): giữ mapping Zalo theo lớp, hay chặn luôn cho nhất quán với "gửi chỉ qua quyền"?
3. Công nợ tháng: tính cả kỳ open/draft không?

Status: DONE_WITH_CONCERNS
Summary: Plan phần lớn khớp code nhưng còn thiếu. Cần sửa trước khi làm: rò SĐT qua `BulkSendRow.Phone`, bước 5 phase 1 làm hỏng luồng gửi Zalo theo lớp, "câu hỏi mở" bị dẫn nhưng không tồn tại, gate ghép Zalo mâu thuẫn quyết định người dùng, và 5+ e2e spec sẽ đỏ mà plan không nhắc.
