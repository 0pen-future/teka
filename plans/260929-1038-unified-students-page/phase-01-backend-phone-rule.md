---
phase: 1
title: "Quy tắc SĐT chỉ theo quyền (backend)"
status: completed
priority: P1
effort: "1.25d"
dependencies: []
---

# Phase 1: Quy tắc SĐT chỉ theo quyền

## Goal

SĐT phụ huynh (hiển thị, ghép Zalo, liên kết Zalo) chỉ mở cho chủ trung tâm hoặc
vai trò có `contacts.view_all` (kể cả qua `reports.send` implied); bỏ hẳn nhánh
ngầm "đang làm `hoc_vu` của lớp có học sinh đang học".

## Context

- Quy tắc hiện tại: `apps/api/internal/shared/authctx/authctx.go:67-76` — `PhoneVisible(rowVisible) = CenterWideFor(contacts.view_all) || rowVisible`.
- Fragment SQL nhánh `hoc_vu`: `apps/api/internal/shared/classscope/classscope.go:74-120` (`PhoneVisibleViaStudent`, `PhoneVisibleViaContact`).
- Tài liệu hành vi: `docs/api-guidelines.md:290-305` (mục "Phone privacy").
- Đây là thay đổi hành vi có chủ đích (quyết định người dùng 2026-09-29). Test cũ khẳng định `hoc_vu` thấy SĐT phải được viết lại theo quy tắc mới, không được xoá assertion mà không thay bằng assertion ngược lại.

## Inventory — mọi đường dựa vào nhánh `hoc_vu`

| Vị trí | Gate gì | Sau thay đổi |
|---|---|---|
| `students/repository.go:126-136` + `students/service.go:103-110` | Che `contact_phone` ở list/get/update | Bỏ cột `phone_visible`; `maskPhone` dùng `sc.PhoneVisible()` |
| `statements/repository.go:309, 382, 396` + `statements/service.go:186, 255, 344` | Che SĐT trong danh sách sao kê, preview/resolve target | Bỏ cột và field `PhoneVisible` ở row; mask theo scope |
| `collections/repository.go:147-156, 183` + `collections/service.go:50-54`, `dto.go:14-18` | Che SĐT ở Thu tiền (theo phụ huynh) | Như trên |
| `notifications/repository.go:294-310` + `notifications/dto.go:164` | Che SĐT trong sổ thông báo | Như trên |
| `notifications/repository.go:600-625` `ZaloMappings` | Phạm vi đọc mapping Zalo (luồng gửi trung tâm) | Bỏ nhánh fragment; không wide thì trả map rỗng |
| `notifications/repository.go:630-660` `ZaloMappingsClass` | Mapping Zalo cho gửi theo lớp, lọc theo **ghi danh** (không qua fragment SĐT) | **Giữ nguyên** (quyết định 2026-09-29): học vụ vẫn gửi `zalo_personal` theo lớp qua Zalo ID; không lộ SĐT |
| `notifications/dto.go:48` `BulkSendRow.Phone` + `service.go:328` | **Lỗ hổng hiện có**: trả SĐT thô cho người gửi theo lớp (`AuthorizeClassSend`, `service.go:122-127`) | Che: `Phone *string`, `nil` khi `!sc.PhoneVisible()` |
| `contacts/repository.go:86-103` `scopedRead` | **Phạm vi hàng** danh bạ | Không wide → không có hàng (`WHERE false`) |
| `contacts/repository.go:105-118` `scopedMappingWrite` | Ghi mapping Zalo | Chỉ `ReportsOversight()` (owner + `reports.send`, quyết định 2026-09-29); bỏ nhánh `hoc_vu` |
| `zalo/service.go:483-530` `MatchFriendsScoped` + `zalo/repository.go:152-181` | Ghép bạn Zalo (gửi SĐT ra bên thứ ba) | Không wide → 403; xoá `HasActiveHocVu`, `ReachableContactPhones` |

Luồng gửi (sao kê, thông báo, `zalo_personal`) tự đọc SĐT phía server và gate
bằng quyền gửi (`reports.send`, gửi theo lớp qua `AuthorizeClassSend`), không qua
fragment — giữ nguyên quyền gửi (quyết định 2026-09-29), nhưng **mọi SĐT trả về
trong response gửi** phải che theo `sc.PhoneVisible()` (xem `BulkSendRow`).

## Implementation Steps

1. `authctx.go`: đổi thành `func (s Scope) PhoneVisible() bool { return s.CenterWideFor(PermContactsViewAll) }`; viết lại comment (một quy tắc, không còn row arm). Trình biên dịch sẽ liệt kê mọi caller cần sửa.
2. `classscope.go`: xoá `PhoneVisibleViaStudent`, `PhoneVisibleViaContact` và test fragment tương ứng trong `classscope/integration_test.go` (`TestPhoneVisibleFragmentsAgainstDatabase`).
3. Students/statements/collections/notifications: bỏ `frag AS phone_visible` khỏi SELECT và bind args (`teacherID, centerID` đi kèm fragment — kiểm tra kỹ số `?` còn khớp), bỏ field `PhoneVisible` ở row struct, mask bằng `sc.PhoneVisible()`.
4. Contacts: `scopedRead` — không wide thì `q.Where("false")`; viết lại comment. `scopedMappingWrite` — `if !sc.ReportsOversight() { q = q.Where("false") }`.
5. Notifications:
   - `ZaloMappings`: không wide thì trả map rỗng sớm (không query). **Không** sửa `ZaloMappingsClass`.
   - `BulkSendRow.Phone` → `*string` (`json:"phone"`), gán `nil` khi `!sc.PhoneVisible()` ở `service.go:~328`. Thêm test: học vụ gửi theo lớp → `phone` null; owner/`contacts.view_all` → có SĐT.
   - Dọn tham số thừa: `TargetContacts(… viewer …)` và bind args của `students.withContact` không còn dùng sau khi bỏ fragment — xoá khỏi chữ ký, đừng để `_`.
6. Zalo: `MatchFriendsScoped` → `if !sc.CenterWideFor(PermContactsViewAll) { return nil, apperror.Forbidden("matching phones requires contacts.view_all") }`; xoá `HasActiveHocVu`, `ReachableContactPhones` khỏi interface, repo, fake repo trong test.
7. Swagger: sửa mô tả `phone` (collections DTO) và MatchFriends (403 text) trong swag annotations; chạy `make api-docs`.
8. Tests — viết lại theo quy tắc mới (mỗi file: `hoc_vu` có lớp đang học → SĐT `null`/403/0 hàng; `contacts.view_all` → thấy; owner → thấy):
   - `authctx/phone_visibility_test.go`
   - `students/phone_privacy_integration_test.go`, `students/staff_read_integration_test.go`
   - `statements/phone_privacy_integration_test.go`, `statements/service_test.go`
   - `collections/phone_privacy_integration_test.go`
   - `notifications/phone_privacy_integration_test.go`, `notifications/zalo_mappings_scope_integration_test.go` (giữ assertion `ZaloMappingsClass` cho học vụ; `ZaloMappings` rỗng khi không wide)
   - `contacts/phone_privacy_integration_test.go` (`TestContactAccessAcrossRoles`), `contacts/service_test.go`
   - `zalo/phone_privacy_integration_test.go` (`TestMatchFriendsScopedFollowsTheOnePhoneRule`)
9. `grep -rn "hoc_vu" apps/api/internal --include='*.go'` sau khi sửa: mọi comment còn nói `hoc_vu` thấy SĐT phải được cập nhật.
10. Web: `bulkSendRowSchema.phone` (`apps/web/src/features/collections/schemas/collections-schemas.ts:210`) → `z.string().nullable()`; `message-card.tsx:41` chỉ render `<p>` khi `row.phone` có giá trị. Cập nhật test/MSW fixture liên quan.
11. Docs: viết lại **cả đoạn** "Phone privacy" trong `docs/api-guidelines.md:~285-335` (quy tắc một nhánh; ghép bạn Zalo cần `contacts.view_all`; ghi mapping cần owner/`reports.send`; gửi theo lớp giữ Zalo ID nhưng response che SĐT), thêm release note ngắn về thay đổi hành vi.

## Todo

- [x] Rút gọn `Scope.PhoneVisible` và xoá hai fragment
- [x] Sửa students, statements, collections, notifications
- [x] Sửa contacts `scopedRead` / `scopedMappingWrite`
- [x] Sửa zalo `MatchFriendsScoped`, xoá hai method repo
- [x] Che `BulkSendRow.Phone` (API + schema web + message-card)
- [x] Viết lại các test phone-privacy
- [x] `make api-docs`, cập nhật `docs/api-guidelines.md`

## Verification

- `cd apps/api && go build ./... && go vet ./...`
- `make test-api-unit` rồi `make test-api` (chạy riêng, không song song)
- `make lint-api` (gồm `scopelint`)
- `grep -rn "PhoneVisibleVia\|HasActiveHocVu\|ReachableContactPhones" apps/api` → rỗng

## Risk Assessment

- **Học vụ mất khả năng gọi phụ huynh ngay khi deploy.** Đây là ý định; release note phải nói chủ trung tâm cấp `contacts.view_all` cho vai trò học vụ trong Phân quyền vai trò nếu cần.
- **Lệch số bind arg** khi bỏ fragment khỏi SELECT → lỗi SQL lúc chạy, không phải lúc biên dịch. Test tích hợp của từng feature phải chạy.
- **Trang Thu tiền / Sao kê của giáo viên**: `phone` đã nullable ở DTO và UI đã kiểm tra null (`contact-collection-row.tsx:50`), nên không vỡ giao diện.

## Security Considerations

Thay đổi thu hẹp quyền (không mở rộng). Không thêm key, không migration. Các ghi
vẫn giữ gate cũ; mapping Zalo chỉ còn `reports.send`/owner. Đóng thêm một lỗ
hổng có sẵn: `BulkSendRow.Phone` trả SĐT thô cho người gửi theo lớp.

## Rollback

Revert commit của phase; không có thay đổi schema.
