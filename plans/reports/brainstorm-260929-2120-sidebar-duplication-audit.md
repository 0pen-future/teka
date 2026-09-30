# Brainstorm (--ultra): rà trùng lặp menu sidebar

Ngày: 2026-09-29 (Asia/Saigon). Yêu cầu gốc: "remove all sidebar menu and then find out duplication feature --ultra", đính kèm `plans/reports/brainstorm-260929-1711-unified-students-page-option-a.md`. Bản thắng được giữ nguyên văn bên dưới, không trộn với ứng viên khác.

# Brainstorm contract: rà trùng lặp sidebar (ứng viên #3)

Tôi hiểu yêu cầu giống bộ điều phối: đi qua từng mục menu, tìm các cặp mục hoặc tính năng trùng nhau, rồi gộp hoặc bỏ. Cách hiểu "xoá toàn bộ sidebar" được để ở phần Ẩn số.

## 1. Chỗ packet hoặc lead sai

1. **Comment và test nói chủ trung tâm đã tới được mọi kỳ học phí qua nhóm Học phí. Điều này sai.**
   - Comment ở `dashboard-layout.tsx:160-161` và test `dashboard-layout.test.tsx:543` giả định như vậy.
   - Thực tế, `useCurrentPeriod` gọi `POST /billing-periods` (`billing/api/*.ts:27-33`). Service mở kỳ theo `sc.Self()` (`apps/api/internal/features/billing/service.go:57`). Kỳ là **của từng giáo viên**: `Period.TeacherID` (service.go:85-88), và `ComputePeriod` tính theo `AnchorTo(period.TeacherID)` (`billing/preview.go:40`).
   - Vì vậy "Chốt sổ", "Gửi thông báo", "Thu tiền" (dashboard-layout.tsx:126-145) chỉ mở **kỳ của chính người đang đăng nhập**.
   - `PeriodSwitcher` lấy `per_page=2` (`billing/api:46-51`). Với chủ trung tâm, danh sách này lọc theo toàn trung tâm (`repository.go:109-115, 466`), nên đó là 2 kỳ mới nhất của bất kỳ ai, không phải "tháng này + tháng trước".
   - Kết luận: `/reports` là **chỗ duy nhất** liệt kê kỳ của mọi giáo viên (`send-reports-page.tsx:36-41`). Nó không phải bản sao của "Gửi thông báo".
2. **Lead 4 đúng một nửa.** Phần trùng thật chỉ có ở vai trò không phải chủ trung tâm nhưng giữ `reports.send`:
   - Người này thấy cả "Gửi thông báo" (dashboard-layout.tsx:134-139, perm `reports.send`) lẫn "Gửi báo cáo" (:162-164). Hai mục cùng icon `HvSendIcon` và cùng dẫn tới `/notifications/:periodId`.
   - `/reports` bao trùm kỳ của chính họ, vì `ListPeriodsRead` lọc theo toàn trung tâm qua `billing.view_all`, key được `reports.send` kéo theo (`authctx/catalog.go:387`).
   - Chủ trung tâm không thấy "Gửi báo cáo" (:162, `!isOwner`), nên với họ không có trùng.
3. **Lead 1 chưa đủ.** `/classes/recruiting` cho ra đúng tập dòng của chip "Cần tuyển sinh" trên `/classes`: cùng gọi API `recruiting=true` (class-list-page.tsx:73-75). Nhưng trang riêng còn **kết hợp được giai đoạn lớp × cần tuyển sinh** (stats `recruiting: true` ở :87, chip giai đoạn ở :70-72). Trên `/classes`, `view` chỉ nhận một giá trị (`use-class-list-url-state.ts:7,20`), nên không kết hợp được.
   - Mục này sinh ra từ prototype v5 trong một plan đã được chấp nhận (`plans/260925-0009-lop-can-tuyen-sinh/plan.md`).
4. **Có một trùng lặp ngoài sidebar mà các lead bỏ sót: hai hệ "Bộ điểm".**
   - Hệ 1: bộ điểm của trung tâm ở `/center/class-config`, API `/score-sets`, chỉ chủ trung tâm (routespec.go:222-227). Bộ này được chép vào `class_score_components` khi gán cho lớp (`grading/service.go:194-198`).
   - Hệ 2: tab "Bộ điểm" của mẫu chương trình (`library/pages/template-detail-page.tsx:64,457`), lưu vào `program_template_versions.score_set` (`library/model.go:77-87`).
   - Tôi grep `ScoreSet|score_set` trong `classprogram/`, `teaching/` và `grading/service.go` thì không thấy chỗ nào đọc bộ điểm của mẫu. Hai định nghĩa song song, không nối với nhau.
5. **Lối tắt tới "Phân quyền vai trò" đã có trong trang Cài đặt trung tâm**: thẻ link tới `/center/permissions` ở `center/pages/center-page.tsx:94-108`, thuộc nhánh chỉ chủ trung tâm (:34 trả sớm cho thành viên). Packet không nhắc.
6. **Rail có một link trùng "Chốt sổ" và không gate quyền.** `CurrentPeriodDisc` (dashboard-layout.tsx:478-493) dẫn tới `/billing/:id` mà không kiểm `billing.read`, trong khi rail đã render mục "Chốt sổ" (:651-653).
7. **"Hồ sơ học sinh" gate lệch quyền.** Mục gate bằng `students.list` (:88), nhưng `records-page.tsx` không gọi `/students`. Trang dùng `classes.list` (:52), `enrollments.list` (:64), `sessions.list`, `teaching.read` qua marks (routespec.go:255, 296, 305, 327).
8. **Tài liệu đã cũ.**
   - `docs/frontend-guidelines.md:161-164` nói "Kho học liệu" có ba mục trỏ `/library`, `/library/materials`, `/library/exercises`. Sidebar chỉ có `/library` (dashboard-layout.tsx:117-120).
   - `docs/api-guidelines.md:447` vẫn gọi nhóm là "Giảng dạy".
   - Comment `class-overview-cards.tsx` ("roster page is owner-only") sai từ khi `/students` gate bằng `students.list` (students-page.tsx:74-76).

## 2. Bảng trùng lặp

Ghi chú ai thấy được: chủ trung tâm thấy mọi mục. "GV mặc định" là giáo viên với các key baseline `DefaultGrant: true` (catalog.go:167, 394-409). Baseline không gồm `reports.send`, `audit.read`, `imports.run`, `teaching.review_queue`, `*.view_all` (catalog.go:366-375).

| Cặp / nhóm | Kết luận | Bằng chứng | Ai thấy hôm nay |
|---|---|---|---|
| Danh mục lớp ↔ Lớp cần tuyển sinh | **Trùng thật.** Cùng component, cùng filter API. Chỉ khác ở chỗ kết hợp giai đoạn × cần tuyển sinh | class-list-page.tsx:54-75, 195-198; class-status-chips.tsx:36-37; routes.tsx:47-59 | Cả hai gate `classes.list`: chủ TT và GV mặc định |
| Gửi thông báo (Học phí) ↔ Gửi báo cáo (Trung tâm) | **Trùng thật** với người không phải chủ TT nhưng có `reports.send`. **Không trùng** với chủ TT (họ không thấy /reports; /reports là danh mục kỳ duy nhất cho mọi giáo viên) | dashboard-layout.tsx:134-139, 160-164; send-reports-page.tsx:36-41, 68-69; billing/service.go:57; repository.go:109-115 | "Gửi thông báo": ai có `reports.send` (chủ TT ngầm có). "Gửi báo cáo": chỉ người không phải chủ TT có `reports.send` (không nằm trong baseline) |
| Phân quyền vai trò (sidebar) ↔ thẻ trong Cài đặt trung tâm | **Trùng lối vào** (cùng một trang) | dashboard-layout.tsx:166-168; center-page.tsx:94-108 | Chỉ chủ TT, ở cả hai lối |
| Chốt sổ ↔ đĩa "T{tháng}" trên rail | **Trùng lối vào** (rail, md–lg) | dashboard-layout.tsx:127-131, 478-493, 675 | Mục: `billing.read`. Đĩa: mọi thành viên |
| Hồ sơ học sinh (/records) ↔ Học sinh (/students, tab Theo lớp) | **Trùng một phần.** Cả hai là danh sách học sinh theo lớp, nhưng cột khác (điểm TB, xu hướng, vắng so với ngày nhập học, số buổi, người liên hệ) và trang chi tiết khác (/records/:id: điểm và nhận xét; /students/:id: liên hệ và ghi danh). Hai trang không link sang nhau | records-page.tsx:70-82, 201; students-page.tsx:145-230; roster-table.tsx:91-96; student-detail-page.tsx:16-138; student-record-page.tsx:24-29 | `students.list`: chủ TT và GV mặc định |
| Điểm danh ↔ Sổ lớp ↔ tab "Buổi học" của lớp | **Không trùng.** Điểm danh là nơi ghi điểm danh (trợ giảng cũng ghi được). Sổ lớp là điểm, nhận xét, giáo án (chỉ chủ TT hoặc `giao_vien` ghi). Tab Buổi học chỉ đọc và link sang Sổ lớp | attendance-page.tsx:95-125; session-expand-row.tsx:28-37, 61-67; class-sessions-tab.tsx:40-47, 99-110; classbook-page.tsx:60-66 | `sessions.list` / `classes.list`: chủ TT và GV mặc định |
| Duyệt giáo án ↔ giáo án trong Sổ lớp | **Không trùng.** Hai đầu của một luồng: Sổ lớp để nộp, Duyệt giáo án để duyệt hoặc trả lại | lesson-plans-page.tsx:19-27; course-view.tsx (submit, :79-85); routespec.go:216-221, 331, 335 | Duyệt: `teaching.review_queue` (không có trong baseline). Nộp: `teaching.edit` |
| Lộ trình học ↔ Khóa học ↔ Kho học liệu | **Không trùng.** Ba thực thể nối nhau: lộ trình gồm các chặng khóa học, khóa học trỏ tới phiên bản mẫu mặc định, kho học liệu chứa mẫu, học liệu, bài tập. Riêng **nhãn trùng**: tên nhóm "Kho học liệu" trùng tên mục "Kho học liệu" | learning-paths-page.tsx:56-60, 384; courses-schemas.ts:38-39; library-page.tsx:26-33; dashboard-layout.tsx:115, 120 | `paths.read` / `courses.read` / `library.read` (baseline) |
| Cấu hình lớp học ↔ tab Thông tin / `/classes/:id/settings` | **Không trùng.** `/settings` chuyển sang dialog sửa lớp. Cấu hình lớp học là CRUD bộ điểm và gán bộ điểm; lối vào khác duy nhất là shortcut trong từng lớp | class-settings-redirect.tsx; class-config-page.tsx; class-info-tab.tsx:122-126 | Chỉ chủ TT |
| Bộ điểm trung tâm ↔ "Bộ điểm" của mẫu chương trình | **Trùng khái niệm** (ngoài sidebar). Bộ điểm của mẫu không được dùng khi chấm điểm lớp | xem mục 1.4 | Chủ TT / `library.edit` (opt-in) |
| Lưới lớp ở Tổng quan ↔ Danh mục lớp; banner chờ điểm danh ↔ chấm đỏ ở Điểm danh | **Không trùng.** Lưới lớp là tiến độ điểm danh và dẫn tới `/sessions?class_id` (không phải danh mục). Banner và chấm đỏ cùng nguồn dữ liệu nhưng là tín hiệu, không phải mục menu | class-overview-cards.tsx (`to=/sessions?class_id`); pending-attendance-alert.tsx:82; dashboard-layout.tsx:69-73 | Mọi thành viên / `sessions.list` |
| Các tab Học viên, Tài liệu, Bài tập của lớp ↔ Học sinh, Kho học liệu | **Trùng một phần theo ngữ cảnh.** Học viên là danh sách lớp và link ra `/students?class_id`. Tài liệu và Bài tập là dữ liệu của mẫu đã áp dụng cho lớp. Không có mục sidebar riêng | class-students-tab.tsx:16-19, 41, 51, 87; class-documents-tab.tsx:15-22; class-homework-tab.tsx:11-14 | Ai mở được lớp |
| Lời mời nhận lớp, Nhật ký hoạt động, Công việc, Tổng quan, Cài đặt trung tâm | **Duy nhất** | class-invitations-page.tsx:58-66; routespec.go:205, 241, 395 | Theo perm tương ứng |

## 3. Kết quả mong muốn, ràng buộc, ngoài phạm vi, tiêu chí nghiệm thu

**Kết quả:** mỗi vai trò chỉ thấy một mục sidebar cho mỗi việc. Bỏ ba lối vào trùng ("Lớp cần tuyển sinh", "Phân quyền vai trò", và một trong hai mục gửi thông báo cho người không phải chủ TT có `reports.send`). Không vai trò nào mất đường tới tính năng nào, link cũ vẫn chạy.

**Ràng buộc**
- Giữ nguyên các quyết định của người dùng trong báo cáo đính kèm: giáo viên chỉ xem học sinh; không cho giáo viên vào Người liên hệ; SĐT chỉ hiện qua `contacts.view_all`; cột công nợ gate bằng `billing.view_all`; nhóm "Lớp học" với "Học sinh" đứng đầu; ẩn nhóm rỗng.
- Mục sidebar gate cùng key mà API kiểm (`NavEntry.perm`, dashboard-layout.tsx:43-48).
- URL cũ phải redirect, không được 404.
- Dùng bộ hv; copy tiếng Việt; không đổi backend.
- Không push lên master khi chưa được duyệt; commit không nhắc tới AI.

**Ngoài phạm vi**
- Gộp hai hệ Bộ điểm; gộp /records vào /students; sửa `PeriodSwitcher`; thêm danh mục kỳ cho chủ trung tâm; đổi quyền mặc định. Các việc này đều là tính năng mới hoặc thay đổi mô hình dữ liệu.

**Tiêu chí nghiệm thu**
1. Sidebar, rail và sheet "Thêm" không còn "Lớp cần tuyển sinh" với bất kỳ vai trò nào. `/classes/recruiting` chuyển sang `/classes?view=recruiting`, giữ `q`, `weekday`, `shift`. Chip "Cần tuyển sinh" vẫn liệt kê cùng tập lớp.
2. Chủ trung tâm không còn mục "Phân quyền vai trò" trong sidebar. `/center/permissions` vẫn mở được, và khi đang ở đó thì "Cài đặt trung tâm" sáng. Thẻ trong `/center` vẫn dẫn tới trang.
3. Người không phải chủ TT có `reports.send` thấy **đúng một** mục gửi thông báo, nằm trong nhóm Học phí, trỏ `/reports`. Chủ trung tâm vẫn thấy "Gửi thông báo", trỏ `/notifications/:periodId`. Người không có `reports.send` không thấy mục nào.
4. Các mục còn lại giữ đúng perm như hôm nay. Test theo vai trò (owner, GV mặc định, member có `reports.send`) khẳng định không mục nào biến mất ngoài 3 mục trên.
5. `OVERFLOW_LABELS` không còn nhãn đã bỏ; thanh dưới vẫn "ba tab chính + Thêm" (test :167).
6. Nút quay lại trong chi tiết lớp không còn nhánh `/classes/recruiting`. Mở lớp từ `/classes?view=recruiting` rồi bấm quay lại thì về lại danh mục lớp.
7. Vitest pass cho `dashboard-layout.test.tsx`, `class-list-page.test.tsx`, `class-detail-page.test.tsx`. Typecheck và lint sạch.
8. `docs/frontend-guidelines.md` phần Navigation khớp với sidebar mới, bỏ luôn câu cũ về ba mục của Kho học liệu.

## 4. Các hướng đi và đánh đổi

**H1: Chỉ bỏ các lối vào trùng thật (3 mục). Được đề xuất.**
- Giả định chịu lực: không ai cần lọc kết hợp giai đoạn × cần tuyển sinh; chủ trung tâm chấp nhận vào Phân quyền qua một cú bấm từ Cài đặt trung tâm.
- Điều kiện hỏng đầu tiên: người tuyển sinh cần xem "lớp sắp khai giảng đang tuyển" thì phải lọc thủ công.
- Tệ nhất: mất một tổ hợp lọc (không mất dữ liệu). Hoàn tác được bằng revert vài file frontend.

**H2: H1 cộng gộp "Hồ sơ học sinh" vào "Học sinh".** Hoặc thêm cột điểm TB, xu hướng, vắng vào tab Theo lớp; hoặc đưa hồ sơ điểm thành tab trong `/students/:id`, rồi redirect `/records`.
- Giả định chịu lực: một màn hình học sinh là đủ cho cả tra cứu hành chính lẫn theo dõi học tập.
- Điều kiện hỏng đầu tiên: tab Theo lớp phải tải thêm sessions, rosters, marks cho mỗi lớp (records-page.tsx:58-67). Trang Học sinh chậm hơn, và bảng vốn ưu tiên thẻ ở mobile trở nên chật.
- Tệ nhất: đây là tính năng mới, thêm độ phức tạp vào một trang vừa được làm lại trên nhánh này. Nó cũng đè lên thiết kế prototype "Hồ sơ học sinh" (bằng chứng để trao đổi với phụ huynh, records-page.tsx:159-160).

**H3: Làm "Gửi thông báo" trỏ `/reports` cho mọi người có `reports.send`, kể cả chủ TT, và bỏ "Gửi báo cáo".**
- Giả định chịu lực: chủ trung tâm muốn chọn kỳ của mọi giáo viên.
- Điều kiện hỏng đầu tiên: trung tâm một giáo viên phải bấm thêm một lần mới tới kỳ của mình.
- Tệ nhất: đổi luồng hằng ngày của chủ trung tâm mà chưa hỏi. Được cái vá luôn lỗ hổng ở mục 1.1.

So theo trường hợp xấu nhất: H1 chỉ mất một tổ hợp lọc; H2 là tính năng mới cộng rủi ro hiệu năng trên trang vừa giao; H3 đổi hành vi của chủ trung tâm. **Chọn H1.** H3 để thành câu hỏi.

## 5. Cách tốt hơn

Không có cách nào thay được H1. Các trùng lặp còn lại hoặc là hai đầu của một luồng (nộp/duyệt giáo án, ghi điểm danh/ghi điểm), hoặc là mô hình dữ liệu khác nhau (lộ trình/khóa học/mẫu). Hai trùng lặp thật ngoài sidebar là hai hệ Bộ điểm và danh mục kỳ của chủ trung tâm; cả hai cần quyết định về sản phẩm, không phải sửa menu (mục 1.1 và 1.4).

## 6. Hướng đề xuất: sidebar cuối cùng

- *(không nhóm)* **Tổng quan** → `/` (mọi thành viên)
- **Dạy học**
  - Điểm danh → `/sessions` (`sessions.list`)
  - Sổ lớp → `/classbook` (`classes.list`)
  - Hồ sơ học sinh → `/records` (`students.list`, giữ nguyên)
- **Lớp học**
  - Học sinh → `/students` (`students.list`)
  - Danh mục lớp → `/classes` (`classes.list`). Chip "Cần tuyển sinh" thay cho mục đã bỏ.
  - Lời mời nhận lớp → `/class-invitations` (không perm)
- **Kho học liệu**
  - Lộ trình học → `/paths` (`paths.read`)
  - Khóa học → `/courses` (`courses.read`)
  - Kho học liệu → `/library` (`library.read`)
- **Học phí**
  - Chốt sổ → `/billing/:periodId` (`billing.read`)
  - Gửi thông báo → chủ TT: `/notifications/:periodId`; người không phải chủ TT có `reports.send`: `/reports`. Chỉ một mục, perm `reports.send`.
  - Thu tiền → `/collections/:periodId` (`billing.read`)
- **Trung tâm**
  - Duyệt giáo án → `/lesson-plans` (`teaching.review_queue`)
  - Nhật ký hoạt động → `/audit` (`audit.read`)
  - Công việc → `/tasks` (`tasks.list`)
  - Cấu hình lớp học → `/center/class-config` (chỉ chủ TT)
  - Cài đặt trung tâm → `/center` (mọi thành viên)

**Mục bị bỏ và nơi thay thế**

| Mục bỏ | Thay bằng | Link cũ |
|---|---|---|
| Lớp cần tuyển sinh | Chip "Cần tuyển sinh" trên `/classes` | `/classes/recruiting` → `/classes?view=recruiting` (giữ `q`, `weekday`, `shift`) |
| Phân quyền vai trò | Thẻ "Phân quyền vai trò" trong `/center` (center-page.tsx:94-108) | `/center/permissions` giữ nguyên |
| Gửi báo cáo (nhóm Trung tâm) | Mục "Gửi thông báo" trong Học phí, trỏ `/reports` cho người không phải chủ TT có `reports.send` | `/reports` giữ nguyên |

**File cần sửa**
- `apps/web/src/layouts/dashboard-layout.tsx`
  - bỏ entry tại :103-108 và :165-168;
  - gộp :134-139 với :160-164 thành một entry, `to = isOwner ? /notifications/${periodId} : "/reports"`;
  - cập nhật `OVERFLOW_LABELS` (:199-218) và các comment :57-66, :160-161, :193-198.
- `apps/web/src/layouts/__tests__/dashboard-layout.test.tsx`: các test :217, :504-549, :585-601; thêm case member có `reports.send` chỉ thấy một mục.
- `apps/web/src/features/roster/routes.tsx:52-59`: route `classes/recruiting` chuyển sang redirect.
- File mới `apps/web/src/features/roster/components/recruiting-classes-redirect.tsx`, theo mẫu `class-settings-redirect.tsx`.
- `apps/web/src/features/roster/pages/class-list-page.tsx`: bỏ `variant`, `variantCopy.recruiting`, `RecruitingClassListPage`.
- `apps/web/src/features/roster/components/class-status-chips.tsx`: bỏ prop `includeRecruiting`.
- `apps/web/src/features/roster/components/class-detail-header.tsx:18-32`: bỏ nhánh `/classes/recruiting`.
- `apps/web/src/features/roster/__tests__/class-list-page.test.tsx` (khối :380-420) và `class-detail-page.test.tsx`.
- `docs/frontend-guidelines.md:152-172`. Tuỳ chọn thêm: `docs/api-guidelines.md:447` (tên nhóm "Giảng dạy" đã cũ).
- Backend: không đổi.

## 7. Ẩn số (kèm mặc định đề xuất)

1. **Có đúng là "xoá toàn bộ sidebar" không?** Mặc định: không. Hiểu là rà trùng như trên, vì câu yêu cầu tiếp tục bằng "then find out duplication feature".
2. **Bỏ "Lớp cần tuyển sinh" có lật ngược plan prototype v5 đã chấp nhận** (`plans/260925-0009-lop-can-tuyen-sinh/plan.md`) **không?** Mặc định: bỏ, vì yêu cầu mới của chính người dùng là dọn trùng. Tổ hợp giai đoạn × cần tuyển sinh bị mất; chấp nhận.
3. **Có bỏ "Phân quyền vai trò" khỏi sidebar không?** Việc này thêm một cú bấm cho chủ trung tâm. Mặc định: bỏ.
4. **Chủ trung tâm có nên thấy danh mục kỳ của mọi giáo viên (H3) không?** Hôm nay họ không có đường tới kỳ của giáo viên khác (mục 1.1). Mặc định: tách thành một thay đổi riêng. Trong lần này chỉ sửa comment/test sai ở :160-161 và :543.
5. **Hai hệ "Bộ điểm"** (trung tâm và mẫu chương trình): hệ nào là nguồn chuẩn? Mặc định: ghi nhận, không làm lần này, vì cần quyết định về mô hình dữ liệu.
6. **Gộp "Hồ sơ học sinh" với "Học sinh" (H2), hoặc ít nhất thêm link chéo giữa `/students/:id` và `/records/:id`?** Mặc định: giữ hai mục, không thêm link trong lần này.
7. **Sửa gate của "Hồ sơ học sinh"** từ `students.list` sang key mà trang thật sự dùng (mục 1.7)? Mặc định: không, vì GV mặc định có đủ key; ghi nhận để sửa riêng.
8. **Đĩa "T{tháng}" trên rail** (link `/billing/:id` không gate): bỏ hay gate `billing.read`? Mặc định: giữ, vì là chi tiết của prototype; ghi nhận.
9. **Nhãn nhóm "Kho học liệu" trùng tên mục "Kho học liệu".** Có đổi tên không? Mặc định: không đổi, chỉ là vấn đề nhãn.

Status: DONE
Summary: Có 3 trùng lặp thật ở sidebar (Lớp cần tuyển sinh, Phân quyền vai trò, Gửi thông báo/Gửi báo cáo với người không phải chủ TT có `reports.send`). Đề xuất bỏ ba mục này, chỉ sửa frontend, kèm redirect. Tìm thêm hai vấn đề ngoài sidebar (hai hệ Bộ điểm; chủ trung tâm không có danh mục kỳ của giáo viên khác), đưa vào phần Ẩn số.
Concerns: Bỏ "Lớp cần tuyển sinh" đảo lại một plan prototype đã được chấp nhận, cần người dùng xác nhận (Ẩn số 2).

---

## Phụ lục xếp hạng (verifier: kongming, thang 1-20/tiêu chí)

| Ứng viên | Bám yêu cầu | Bằng chứng | Tiêu chí nghiệm thu | Trung thực về ẩn số | Trường điều kiện | Tổng |
|---|---|---|---|---|---|---|
| **B (thắng)** | 17 | 17 | 17 | 18 | 17 | **86** |
| C | 15 | 16 | 17 | 16 | 17 | 81 |
| A | 14 | 15 | 16 | 14 | 17 | 76 |
| D | 14 | 15 | 16 | 15 | 15 | 75 |
| E | 9 | 12 | 14 | 12 | 16 | 63 |

Lý do chọn:
- Cả năm ứng viên đều tìm ra ba cặp trùng giống nhau: "Lớp cần tuyển sinh", "Gửi báo cáo" và "Phân quyền vai trò".
- B hơn ở ba điểm. Thứ nhất, B phát hiện thêm hai chỗ trùng thật mà bản xếp thứ hai bỏ sót: đĩa kỳ hiện tại trên rail không gate quyền, và hai hệ "Bộ điểm". Thứ hai, B giữ nguyên luồng "Gửi thông báo" của chủ trung tâm và chỉ đưa việc đổi sang `/reports` thành câu hỏi. Thứ ba, B nêu rõ việc bỏ trang tuyển sinh sẽ đảo ngược một plan đã được chấp nhận.
- E bị phạt nặng vì đề xuất ẩn "Cài đặt trung tâm" với người không phải chủ. Lý do sai: `CenterCard` chỉ render ở breakpoint `lg` (`dashboard-layout.tsx:587`), nên trên rail và mobile, trang `/center` là lối duy nhất để thấy tên trung tâm.
- Verifier đã tự đối chiếu các khẳng định chịu lực: kỳ học phí neo theo người gọi (`billing/service.go:44-57`), biến thể recruiting so với chip lọc, thẻ Phân quyền trong trang `/center` (`center-page.tsx:100-115`) và đĩa rail (`dashboard-layout.tsx:478-493`). Bản thắng chỉ lệch vài số dòng, không sai bản chất.

### Lưu ý của verifier về bản thắng (không sửa bản thắng)
1. Bản thắng thiếu phần cập nhật e2e. Hai chỗ sẽ vỡ sau khi gộp mục và thêm redirect: `apps/web/e2e/secretary-send.spec.ts:99-106,174` (đang tìm link "Gửi báo cáo") và `apps/web/e2e/helpers/ux-routes.ts:46` (đang đăng ký `/classes/recruiting`).
2. Chưa chốt tiêu đề trang `/reports`. Với người không phải chủ, mục menu "Gửi thông báo" mở một trang có h1 "Gửi báo cáo" (`send-reports-page.tsx:47`, `reports/routes.tsx:15`).
3. Với người không phải chủ, mục menu trỏ `/reports` sẽ không sáng khi đang ở `/notifications/:id`, vì `useNavActive` chỉ so theo đích của chính mục đó. Tab "Thêm" trên mobile vẫn sáng nếu còn prefix `/notifications`.
4. Đĩa rail là "UI mà API có thể từ chối" với vai trò bị thu hẹp quyền. Chỉ cần gate bằng `has("billing.read")`, sửa một dòng, nên cân nhắc làm luôn trong đợt này.
5. Không được ẩn "Cài đặt trung tâm" với người không phải chủ, vì `CenterCard` chỉ có ở `lg`.
6. Một số lỗi ngoài sidebar nên xử lý riêng:
   - Link "Thêm học viên" (`class-students-tab.tsx:40-45`) hiện cho giáo viên, trái quyết định 2026-09-29 rằng giáo viên không được ghi danh.
   - `classbook-page.tsx:372` dẫn tới `/center/classes`, route này không tồn tại nên ra 404.
   - Tab Buổi học có hai link cùng href (`class-sessions-tab.tsx:99-110`).
7. Nếu sau này chọn cho chủ trung tâm dùng `/reports`: nút ở chân trang Chốt sổ chỉ xuất hiện khi kỳ đã chốt. Đường luôn có sẵn là "Nhắc nợ" ở Thu tiền (`contact-collection-row.tsx:86-93`).

ultra: picked=3/5 margin=low unanimous=yes rejected_all=no

---

## Quyết định của người dùng (2026-09-29 21:40, ràng buộc cho plan)

Các quyết định dưới đây thay cho các giá trị mặc định ở phần Ẩn số của bản thắng.

1. Không xóa sidebar. Chỉ bỏ các mục trùng.
2. Bỏ "Lớp cần tuyển sinh". Chấp nhận đảo ngược `plans/260925-0009-lop-can-tuyen-sinh/plan.md`. `/classes/recruiting` chuyển hướng sang `/classes?view=recruiting`.
3. **Giữ** "Phân quyền vai trò" trong sidebar.
4. Đổi tên trang `/reports` từ "Gửi báo cáo" thành "Gửi thông báo".
5. Chủ trung tâm cũng dùng `/reports`, để thấy kỳ học phí của mọi giáo viên (hướng H3). Chỉ còn một mục "Gửi thông báo" (`reports.send`) trỏ `/reports` cho mọi vai trò. Mục "Gửi báo cáo" riêng bị bỏ.
6. Gate đĩa kỳ trên rail bằng `has("billing.read")`.
7. Làm luôn trong đợt này:
   - Sửa gate quyền còn lệch.
   - Đổi tên nhóm "Kho học liệu" để không trùng tên mục bên trong.
   - Sửa các lỗi ngoài sidebar: link "Thêm học viên" hiện cho giáo viên, link 404 `/center/classes`, hai link trùng href ở tab Buổi học, tài liệu lỗi thời.
8. **Bộ điểm:** bỏ `/center/class-config` (Cấu hình lớp học). Bộ điểm của mẫu chương trình (`program_template_versions.score_set`) trở thành nguồn chuẩn. Luồng chấm điểm lớp phải lấy thành phần điểm từ mẫu mà lớp áp dụng, thay cho việc gán bộ điểm trung tâm. `/center/class-config` và `/classes/:id/settings` cần chuyển hướng hợp lý. Mọi migration dữ liệu phải sao lưu trước.
9. **Hồ sơ học sinh:** bỏ `/records` và `/records/:studentId`, cùng mục sidebar. Deep link cũ chuyển sang `/students` và `/students/:id`.
