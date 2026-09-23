# Review — Phase 7: Chi tiết lớp (Chương trình học, Bài tập, Tài liệu, Chat, Lịch sử lớp)

Ngày: 2026-09-24 · Branch `feat/giang-day-menu` · Phạm vi: `0f5c174` (API) và `952f0e5` (web), đọc qua `git show` từng commit.

## Verdict tóm tắt

Có một lỗi High, không có Critical hay Medium. Phần lõi đúng:
- **Tenancy.** Mọi truy vấn của `classprogram` và `classchat` gắn `center_id`. FK composite `(class_id, center_id)`, `(template_version_id, center_id)`, `(applied_by|author_id, center_id)` và `(parent_class_id, center_id)` chặn nối chéo trung tâm ngay ở DB. Id của trung tâm khác đọc thành 404, có test.
- **Authorization.** PUT/DELETE program là `KindOwnerOnly` ở manifest và service kiểm lại `sc.IsOwner`. Chat đọc và ghi đều đi qua `GetReadable` rồi tới `HasActiveAssignment` (`ended_at IS NULL`). POST cần `class_messages.post` ở cả middleware lẫn service. Xoá tin chỉ cho tác giả hoặc owner.
- **Luồng áp dụng.** Chỉ version `published`. 409 `CURRICULUM_DIFFERS` chỉ khi curriculum khác rỗng, khác tiêu đề mẫu và thiếu `confirm`. Upsert `class_programs` và `PutCurriculum` chạy chung một transaction. Gỡ chỉ xoá `class_programs`. Xoá template đang được lớp dùng trả 409 `TEMPLATE_IN_USE`.
- **Hợp đồng.** Swagger sinh lại khớp file đã commit. Snapshot +7 và audit +4 khớp 7 route mới.

Lỗi High: lưu trữ (archive) một phiên bản mà lớp đang áp dụng làm ba tab Buổi học, Bài tập và Tài liệu của lớp đó hỏng. Thư viện có sẵn nút "Lưu trữ" cho thao tác này.

## Verification đã chạy

| Lệnh | Kết quả |
|---|---|
| `go build ./...`, `go vet ./...` (api) | OK |
| `go vet -tags integration` classprogram, classchat, migrations | OK (chỉ compile) |
| `gofmt -l` classprogram, classchat, classes, library, audit | sạch |
| `go test` library, audit, classes, shared/..., server | tất cả `ok` |
| `go tool swag init … -o <scratchpad>` rồi so với `apps/api/docs/swagger.json` | trùng khớp |
| `npx tsc -b --noEmit` (web) | OK |
| `npx vitest run` `class-detail-program.test.tsx` và `class-detail-page.test.tsx` | 2 files, 38 passed |

Lead đã chạy trước: `make lint`, `make test-web`, integration tests API chạy serial, và các e2e trên stack `teka-e2e`. Tôi không chạy lại.

## Đối chiếu yêu cầu

| Mục kiểm | Kết quả | Bằng chứng |
|---|---|---|
| Repository scope theo `center_id` | Đạt | `classprogram/repository.go:46,72`; `classchat/repository.go:46,64,82`; `library/repository.go` `TemplateInUse` (`p.center_id = ?`) |
| PUT/DELETE program chỉ owner | Đạt | `routespec.go` (`KindOwnerOnly`), `classprogram/service.go:103,148`; test `TestOnlyTheOwnerAppliesOrRemovesWhileStaffRead` |
| GET program theo read port lớp | Đạt | `classprogram/service.go:61,77` qua `GetReadable` |
| Chat chỉ owner hoặc stint mở | Đạt | `classchat/service.go:126-148`; `classstaff/repository.go:130-139` (`ended_at IS NULL`); test `TestOwnerAndActiveStaffChatWhileAClosedStintIsForbidden` |
| Post cần `class_messages.post` | Đạt | manifest `perm(...)` và `classchat/service.go:78`; test deny qua `center_member_permissions` |
| Xoá tin: tác giả hoặc owner | Đạt | `classchat/service.go:110`; test `TestDeleteByAuthorOrOwnerOnly` |
| Lineage: cùng center, không tự tham chiếu, nil giữ, "" xoá | Đạt. **Không chặn vòng** A↔B (L3) | `classes/service.go:441-459`; `TestUpdateSetsAndClearsParentClass` |
| Chỉ áp dụng version published | Đạt khi áp dụng. **Cũng chặn khi đọc**, gây lỗi High H1 | `library/service.go:138-158` |
| 409 `CURRICULUM_DIFFERS` đúng điều kiện | Đạt | `classprogram/service.go:118`; test kiểm cả `fields` |
| Áp dụng ghi `class_programs` và `class_curricula` cùng tx | Đạt | `classprogram/service.go:121-135` |
| Gỡ chỉ xoá `class_programs` | Đạt | `classprogram/service.go:144-159`; test kiểm curriculum và plans còn nguyên |
| Template đang dùng → 409 khi xoá | Đạt, nhưng không khoá (L5) | `library/service.go:118-129` |
| Migration 000031 up/down đối xứng | Đạt | down gỡ theo `rbac_backfill_rows`, bỏ index, constraint, cột, bảng theo thứ tự ngược. `ON DELETE SET NULL (parent_class_id)` cần PG15+, repo dùng `postgres:16-alpine` |
| Index audit theo entity | Đạt, thêm `id DESC` để khớp keyset | `000031_class_programs.up.sql:77-78` |
| Web gate `isOwner`, `has("class_messages.post")`, `has("audit.read")` | Đạt | `class-detail-page.tsx` truyền đủ ba cờ |
| Envelope không có khoá `data` khi chưa có chương trình | Đạt | `class-program-api.ts:21` dùng `nullish() ?? null`; MSW trả `{ success: true }` đúng shape |
| Tài liệu mặc định chỉ hiện tài liệu chia sẻ | Đạt | `class-documents-tab.tsx:113-115`; switch có `role="switch"` và `aria-labelledby` |

## Findings

### Critical
Không có.

### High

**H1 — Lưu trữ phiên bản mà lớp đang áp dụng làm hỏng tab Buổi học, Bài tập và Tài liệu của lớp.** *(CONFIRMED qua đọc code)*
`apps/api/internal/features/classprogram/service.go:87` gọi `library.PublishedVersion`. Hàm đó ở `apps/api/internal/features/library/service.go:143-145` trả 409 `VERSION_NOT_PUBLISHED` khi `row.Status != StatusPublished`. `Archive` ở `library/service.go:309-321` chuyển `published → archived` mà không kiểm `class_programs`. Web có nút lưu trữ (`template-detail-page.tsx:119`, `useArchiveVersion`).
- Kịch bản:
  1. Owner áp dụng "TOAN-6 v1" cho lớp 6A.
  2. Owner phát hành v2 rồi bấm "Lưu trữ" trên v1. Đây là cách thư viện dự kiến dùng để cho v1 nghỉ.
  3. `GET /classes/6A/program` vẫn trả v1 (`repository.Get` không lọc trạng thái), nên card Chương trình học hiện bình thường.
  4. `GET /classes/6A/program/lessons` trả 409. Tab Bài tập và Tài liệu hiện "Không tải được chương trình của lớp" (`program-tab-state.tsx:279-281`). Tab Buổi học mất cột Buổi mẫu và cảnh báo lệch mà không báo lỗi nào.
  5. Lớp chỉ phục hồi được khi owner áp dụng sang v2, việc này có thể ghi đè curriculum.
- Tác động: mọi lớp theo một phiên bản đã lưu trữ mất ba tab. Không mất dữ liệu.
- Nguyên nhân: một port phục vụ hai việc có luật khác nhau. Áp dụng cần `published`. Đọc lại phiên bản lớp đã áp dụng thì phải chấp nhận cả `archived`, vì phiên bản đã phát hành là bất biến.
- Fix (khuyến nghị): tách port đọc. `Lessons` gọi một hàm `ReleasedVersion` (hoặc thêm tham số) chấp nhận `published|archived`. `Apply` giữ nguyên kiểm tra `published`. Nên hiện badge "Đã lưu trữ" trên card để owner biết nên đổi phiên bản. Cách khác là chặn `Archive` khi còn lớp dùng (409 giống `TEMPLATE_IN_USE`), nhưng cách đó buộc owner gỡ khỏi từng lớp trước khi cho v1 nghỉ, nặng tay hơn.
- Test cần thêm: integration "áp dụng v1, lưu trữ v1, `Lessons` vẫn trả bài của v1, còn `Apply` v1 trả 409".

### Medium
Không có.

### Low

**L1 — Kiểm tra `CURRICULUM_DIFFERS` đọc curriculum ngoài transaction, nên sửa đồng thời có thể bị ghi đè mà không hỏi.** *(CONFIRMED qua đọc code)*
`apps/api/internal/features/classprogram/service.go:114-135`.
- Kịch bản: giáo viên lưu curriculum mới đúng lúc owner bấm áp dụng lên lớp đang có curriculum rỗng hoặc trùng tiêu đề mẫu. Owner đã qua bước kiểm tra, nên `PutCurriculum` trong tx ghi đè danh sách giáo viên vừa lưu mà không có 409.
- Rủi ro thấp: cửa sổ hẹp, chỉ owner áp dụng được, và giáo án theo index vẫn còn.
- Fix: trong `WithinTx`, khoá dòng `class_curricula` bằng `FOR UPDATE` (hoặc khoá lớp) rồi đọc và so sánh lại, và đọc `current_index` trong cùng tx.

**L2 — Áp dụng chương trình vượt qua giới hạn của curriculum.** *(CONFIRMED qua đọc code)*
`apps/api/internal/features/teaching/dto.go:27` giới hạn `lessons` ở `max=100` khi binding HTTP. `classprogram/service.go:130` gọi `PutCurriculum` trực tiếp nên không qua binding. Thư viện không giới hạn số buổi mỗi phiên bản (không tìm thấy trần nào trong `library/service.go`).
- Kịch bản: mẫu có 120 buổi được áp dụng, và curriculum nhận 120 dòng. Sau đó giáo viên sửa một tên buổi trong Sổ đầu bài, web gửi lại cả danh sách, và API trả 422 cho tới khi họ tự xoá bớt 20 buổi.
- Fix (chọn một): đặt trần 100 buổi mỗi phiên bản ở `CreateLesson`, hoặc kiểm `len(titles) > 100` trong `Apply` và trả 422 rõ ràng. Tiêu đề thì đã khớp: cả hai phía đều tối đa 200 ký tự.

**L3 — Lineage không chặn vòng.** *(CONFIRMED qua đọc code)*
`apps/api/internal/features/classes/service.go:441-459` chỉ chặn tự tham chiếu (`parentID == selfID`).
- Kịch bản: đặt cha của A là B, rồi cha của B là A. Cả hai lệnh đều 200. Card hiện "B → A" trên A và "A → B" trên B, mỗi lớp liệt kê lớp kia là "Lớp tách ra".
- Web không duyệt chuỗi nên không có vòng lặp vô hạn. Tuy vậy phase spec hứa "UI hiện chuỗi cha → con". Nếu sau này dựng chuỗi nhiều cấp, dữ liệu vòng sẽ làm treo trang.
- Fix: trong `resolveParentClass`, dùng một `WITH RECURSIVE` đi ngược từ `parentID` theo `parent_class_id` (có trần độ sâu) và trả 422 nếu gặp `selfID`.

**L4 — "Lịch sử thay đổi" trộn request thất bại và tin chat vào lịch sử lớp.** *(CONFIRMED qua đọc code)*
`apps/api/internal/middleware/request_events.go:66-86` ghi một dòng audit cho mọi request ghi, "success or failure alike". `routespec.go` gắn `class_message.post` vào `("class", id)`, còn `class_message.delete` gắn vào `("class_message", mid)`. `class-lineage-card.tsx:205-243` hiện `actor_name · action · thời gian` mà không có `status_code`.
- Kịch bản: owner áp dụng một chương trình, nhận 409 và xác nhận. Lịch sử hiện hai dòng `class_program.apply` giống hệt nhau. Giáo viên gửi 30 tin chat thì lịch sử lớp có 30 dòng `class_message.post`, còn các lần xoá tin thì không xuất hiện.
- Hai chi tiết phụ: `actor_name` rỗng (tài khoản đã xoá) hiện thành khoảng trống, trong khi `audit-table.tsx:36-41` đã có `actorLabel` xử lý trường hợp này. Mã action hiện thô, nhưng trang `/audit-logs` cũng vậy, nên đây là parity.
- Fix:
  - Ẩn các dòng `status_code >= 400` hoặc hiện badge lỗi.
  - Đổi entity của `class_message.post` sang `class_message` để đồng bộ với delete, hoặc lọc tiền tố `class_message.` ở web.
  - Dùng lại `actorLabel` (export từ feature `audit`).

**L5 — Guard `TEMPLATE_IN_USE` không khoá, nên xoá mẫu có thể chạy song song với áp dụng.** *(CONFIRMED qua đọc code)*
`apps/api/internal/features/library/service.go:118-129`: `TemplateInUse` rồi `SoftDeleteTemplate`, không có transaction và không có khoá. `classprogram.Apply` đọc version cũng không khoá template.
- Kịch bản: A xoá mẫu TOAN-6 trong khi B áp dụng TOAN-6 v1 cho một lớp. Cả hai thành công. Lớp trỏ tới một mẫu đã xoá mềm, và link "Mở chương trình mẫu" dẫn tới 404. Tab bài học vẫn đọc được, vì `versions()` không lọc mẫu đã xoá.
- Cùng khuôn với phase 6 (courses giờ đã khoá `FOR UPDATE` và `FOR SHARE`), nên nên làm cho nhất quán.
- Fix: `DeleteTemplate` chạy trong tx và khoá template `FOR UPDATE`. `Apply` khoá template của version bằng `FOR SHARE` trong tx của nó, và kiểm `deleted_at IS NULL`.

**L6 — Cột "Buổi mẫu" và cảnh báo lệch dựa trên các buổi đã sinh, không phải toàn bộ lịch.** *(CONFIRMED qua đọc code)*
`apps/web/src/features/roster/components/class-sessions-tab.tsx:48` (`readonly: true`), `:64-73` (zip và `mismatch`).
- Tab cố ý không sinh buổi. Vì vậy với lớp mới hay lớp chưa ai mở lịch hoặc sổ đầu bài tới cuối khóa, "N buổi thực" chỉ là số buổi đã có trên DB. Cảnh báo "Lớp có 4 buổi thực … chương trình mẫu có 24 buổi" sẽ hiện trên gần như mọi lớp đang chạy.
- Nếu các buổi được sinh không liền (lịch tháng 3 được mở trước tháng 2), buổi thứ i trên màn không phải buổi thứ i của khóa, nên tên buổi mẫu bị lệch.
- Phase spec nói lệch số buổi là bình thường, nhưng câu chữ hiện tại nói về "buổi thực" như thể đã đủ.
- Fix nhẹ: đổi câu thành "N buổi đã lên lịch", chỉ hiện cảnh báo khi lớp đã có `end_date` và phạm vi đã được sinh đủ, hoặc ghi rõ "tính trên các buổi đã tạo".

**L7 — Nhãn a11y và thao tác xoá tin chat.** *(CONFIRMED)*
`apps/web/src/features/roster/components/class-chat-panel.tsx:356-366`.
- Mọi nút xoá đều có `aria-label="Xoá tin nhắn"`. Phase 6 đã sửa đúng kiểu lỗi này (L5 của phase 6) bằng nhãn mang ngữ cảnh.
- Xoá chạy ngay sau một cú bấm, không hỏi xác nhận, và web không có cách hoàn tác.
- Fix: `aria-label={\`Xoá tin nhắn của ${message.author_name} lúc ${formatDateTime(message.created_at)}\`}`, và mở `HvConfirmDialog` trước khi gọi `remove.mutate`.

**L8 — Link "Mở chương trình mẫu" hiện với người không có `library.read`.** *(CONFIRMED)*
`apps/web/src/features/roster/components/class-program-card.tsx:111-116`.
- Giáo viên đọc được chương trình của lớp qua port của lớp, đúng thiết kế. Nhưng link dẫn tới `/library/templates/:id`, nơi họ nhận 403. Phase 6 đã gate link chip khóa học bằng `has("courses.read")` (disposition L7 của phase 6).
- Fix: chỉ render link khi `has("library.read")`, còn không thì hiện tên mẫu dạng text.

**L9 — Chat không tự làm mới.** *(CONFIRMED)*
`apps/web/src/features/roster/hooks/use-class-chat.ts:222-231` không có `refetchInterval`. Tin của người khác chỉ hiện khi đổi focus cửa sổ, mount lại, hoặc sau khi chính mình gửi hay xoá.
- Đây là lựa chọn sản phẩm hợp lý cho "tin nhắn nội bộ" không realtime. Nên xác nhận với user. Nếu cần, `refetchInterval: 30_000` chỉ khi tab Chat đang mở là đủ.

### Nit
- `class-program-card.tsx:215` có comment "A template carries one published version at a time". Điều này sai: DB chỉ có unique cho bản nháp (`000027_program_templates.up.sql:65`), và `Publish` không lưu trữ bản cũ. Code bên dưới xử lý đúng trường hợp nhiều bản, chỉ comment gây hiểu lầm.
- `class-sessions-tab.tsx:81-93` có hai link "Sổ đầu bài" và "Điểm danh & nhận xét" trỏ cùng `/classbook?class_id=`. Nên gộp lại, hoặc cho link "Sổ đầu bài" mở thẳng phần giáo trình nếu classbook có tham số tương ứng.
- Owner với `LineageForm`: picker lớp gốc chỉ nạp 100 lớp đầu (`class-lineage-card.tsx:44`). Lớp gốc nằm ngoài 100 lớp này hiện chữ "Lớp gốc" thay vì tên. Cùng giới hạn 100 đã chấp nhận ở phase 5 và 6.

## Khoảng trống test đáng đóng

1. **Lưu trữ phiên bản đang được áp dụng** (H1). Chưa có test nào gọi `library.Archive` sau `Apply`.
2. **`LiveClassInCenter` với DB thật.** `TestUpdateSetsAndClearsParentClass` chạy trên fake repo, nên câu SQL (lọc `center_id` và `deleted_at IS NULL`) và FK `fk_classes_parent_center` chưa được kiểm ở tầng integration. Nên có ít nhất một case: lớp cha đã xoá mềm thì trả 422.
3. **HTTP binding của `PUT /classes/:id/program`.** Chưa có case thiếu `template_version_id` hoặc gửi uuid sai định dạng qua router. Chat đã có case HTTP cho `before` và `limit`. Đây là cùng bài học M1 của phase 5.
4. **Web, luồng lỗi:**
   - Toast khi áp dụng lỗi khác 409 (ví dụ `VERSION_NOT_PUBLISHED`) và khi gỡ lỗi.
   - Toast khi gửi tin lỗi.
   - Trạng thái lỗi của tab Bài tập và Tài liệu khi `lessons` lỗi. Đây chính là đường H1 đi qua, và hiện chỉ có case chưa áp dụng.
5. **Tab Buổi học khi `lessons` lỗi.** Hiện cột "—" im lặng. Nên có test khẳng định hành vi mong muốn sau khi sửa H1.

Không cần thêm: test quyền đọc chat (owner, stint mở, stint đóng, người ngoài), cursor lạ, clamp limit, và xoá theo tác giả đều đã có. Snapshot và manifest cũng đã phủ đủ 7 route.

## Tuân thủ phase spec và plan decisions

| Điểm | Đánh giá |
|---|---|
| Hai feature điều phối `classprogram`, `classchat` dựng sau `teaching` và `library` | Đúng. `router.go` dựng hai feature sau `teachingSvc`, và `librarySvc` được tách thành biến để tiêm vào |
| `PUT`/`DELETE` `KindOwnerOnly`, `GET` `KindService`, không dùng `classes.edit` | Đúng |
| Xác nhận thay vì preview, không có endpoint `apply-preview` | Đúng |
| `GET /program/lessons` trả "materials `shared_with_students`" | **Lệch có chủ đích.** API trả mọi tài liệu, web lọc mặc định và có switch "Hiện tất cả". Người đọc là nhân sự lớp nên không có rủi ro lộ dữ liệu. Nên sửa câu trong phase spec cho khớp |
| Chat nội bộ, key `class_messages.post` (`def`, low, backfill), `RESOURCE_LABELS` | Đúng. Backfill hai nhánh có step label, down theo sổ |
| Lineage FK composite cùng center, sửa qua `PUT /classes/:id` | Đúng. `ON DELETE SET NULL (parent_class_id)` giữ `center_id` NOT NULL, hợp lý hơn bản phác thảo |
| Audit filter `entity_type`/`entity_id` additive và index mới | Đúng. Index có thêm `id DESC` để khớp keyset |
| Không bump `CatalogVersion` | Đúng |
| Verification: "GV chính và trợ giảng gọi PUT/DELETE → 403" | Đạt. Test chạy cả `teacher` lẫn `tro_giang` |
| Verification: "Migration 000031 up/down/up sạch" | Có trong `migrations_test.go` theo brief của lead. Tôi không chạy lại |
| e2e cập nhật `class-list.spec.ts` | **Lệch và chấp nhận được.** File đó không tồn tại, và spec danh sách lớp thật là `roster.spec.ts`. Để một spec riêng `class-program.spec.ts` tự tạo template và lớp qua API rồi dọn ở `afterEach` là tốt hơn: không đổi tên sổ đầu bài của lớp seed, và không phụ thuộc thứ tự file. Nên sửa tên file trong phase spec khi đóng phase |
| Risk "đụng hợp đồng `teaching`" | Hợp đồng không đổi. `teaching` chỉ export thêm `teachingKeys` cho web. API gọi `PutCurriculum` như một consumer |

## Verdict

**SHIP sau khi sửa H1.** H1 là thay đổi nhỏ ở `library` (một port đọc chấp nhận `archived`) kèm một integration test. Nếu không sửa, lớp nào theo phiên bản bị lưu trữ sẽ mất ba tab, và thao tác lưu trữ đó là luồng thường ngày của thư viện.

Nên làm cùng lúc vì rẻ: L2 (trần 100 buổi), L7 (nhãn a11y và xác nhận khi xoá tin), L8 (gate link thư viện). L1, L3, L4, L5 và L6 có thể gom vào phase 9. L9 cần user quyết định.

## Disposition

Áp dụng ngay (TDD, test viết trước rồi mới sửa service/UI):

| Finding | Xử lý | Commit |
|---|---|---|
| H1 phiên bản lưu trữ làm trống ba tab | `library.ReleasedVersion` (published hoặc archived) cho đọc qua lớp; `Apply` vẫn chỉ nhận published; `ProgramResponse.version_status` mới; web hiện badge "Đã lưu trữ" và notice khuyên đổi phiên bản | `ae5a75b`, `a390f2d` |
| L2 áp dụng vượt trần 100 buổi | `teaching.MaxCurriculumLessons` dùng chung với binding; `Apply` trả 422 `TEMPLATE_TOO_LONG` trước khi mở tx | `ae5a75b` |
| L7 nút xoá tin nhắn | `aria-label` mang tên tác giả và thời điểm; `HvConfirmDialog` tone danger trước khi gọi API | `a390f2d` |
| L8 link "Mở chương trình mẫu" | Chỉ hiện khi `has("library.read")`, thread `canReadLibrary` từ trang chi tiết | `a390f2d` |
| Nit comment "một phiên bản published" | Giữ nguyên: bình luận trong `ProgramPicker` nói về preselect khi chỉ có một lựa chọn, không phải bất biến dữ liệu; sẽ đọc lại ở Phase 9 cùng các nit khác | — |

Chuyển sang Phase 9 (ghi vào `phase-09-e2e-docs-seed-ship.md` khi thực thi): L1 (TOCTOU curriculum khi áp dụng), L3 (chu trình lineage A↔B), L4 (change log hiện request lỗi và mọi tin chat, actor trống), L5 (guard `TEMPLATE_IN_USE` không khoá), L6 (cảnh báo lệch đếm cả buổi sinh tự động), các test còn thiếu (`LiveClassInCenter`, binding HTTP `PUT program`, luồng lỗi web, lỗi lessons ở tab Buổi học), và hai câu trong phase spec cần sửa cho khớp (materials trả đủ rồi web lọc; e2e nằm ở `class-program.spec.ts`).

Cần user quyết định: L9 (chat không tự làm mới; polling hay để người dùng tải lại). Không tự chọn trong `--auto`.

Kiểm chứng sau khi sửa: integration `classprogram` + `library` (`-p 1`) xanh, `make test-api-unit`, `make scopelint`, `make lint` xanh, `make api-docs` đã tái sinh, vitest 1057 pass, e2e `class-program.spec.ts` xanh trên stack cô lập.
