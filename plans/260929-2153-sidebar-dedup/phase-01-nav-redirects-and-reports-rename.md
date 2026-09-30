---
phase: 1
title: "Nav, redirect và đổi tên Gửi thông báo"
status: completed
priority: P2
effort: "4h"
dependencies: []
---

# Phase 1: Nav, redirect và đổi tên Gửi thông báo

## Goal

Phase này làm mọi thay đổi trong `dashboard-layout.tsx` trong một lần, để các phase sau không phải đụng lại file này. Cụ thể:

- gỡ bốn mục trùng;
- gộp hai mục gửi thành "Gửi thông báo" trỏ tới `/reports`;
- đổi tên nhóm "Kho học liệu";
- gate đĩa kỳ hiện tại trên rail;
- sửa perm của "Sổ lớp".

Ngoài ra phase này gỡ trang "Lớp cần tuyển sinh" (thay bằng redirect) và đổi tên trang `/reports`.

## Context

- `apps/web/src/layouts/dashboard-layout.tsx`:
  - `NavEntry` ở :37-49 và `useNavGroups` ở :67-191.
  - Các mục cần sửa:
    - "Sổ lớp" ở :87, đang dùng `perm: "classes.list"`;
    - "Hồ sơ học sinh" ở :88;
    - "Lớp cần tuyển sinh" ở :103-108;
    - header "Kho học liệu" ở :115;
    - "Gửi thông báo" ở :134-139, đang trỏ tới `/notifications/${periodId}`;
    - "Gửi báo cáo" ở :160-164;
    - "Cấu hình lớp học" ở :169-178.
  - `OVERFLOW_LABELS` ở :199-218, `OVERFLOW_PATH_PREFIXES` ở :225-242, `useNavActive` ở :258-272.
  - Ba component gọi `useNavActive(to)`: `SidebarNavItem` :278-285, `RailNavItem` :316-317 và `BottomTabItem` :346-347.
  - `CurrentPeriodDisc` ở :478-493, render ở :644.
- `apps/web/src/layouts/__tests__/dashboard-layout.test.tsx`, dài 724 dòng.
- `apps/web/src/features/reports/routes.tsx`: comment ở :5-11 và `title: "Gửi báo cáo"` ở :15.
- `apps/web/src/features/reports/pages/send-reports-page.tsx`: doc comment ở :35-41, h1 ở :47, aria-label ở :70. Test đi kèm là `apps/web/src/features/reports/__tests__/send-reports-page.test.tsx:87`.
- `apps/web/src/features/collections/pages/notifications-page.tsx`:
  - `EXPIRED_SESSION_409` ở :37-40;
  - `onError` của lệnh gửi ở :226-238 hiện đổi mọi lỗi 409 khác thành "Đang có lượt gửi chạy, đợi xong đã".
- Roster:
  - `apps/web/src/features/roster/routes.tsx:52-59` là route `classes/recruiting`;
  - `apps/web/src/features/roster/pages/class-list-page.tsx`: biến thể ở :21-47, :62-65, :73, :138, :165-169, :172-176 và :195-198;
  - `apps/web/src/features/roster/components/class-status-chips.tsx`: prop `includeRecruiting` ở :11-12, :26 và :36-38;
  - `apps/web/src/features/roster/components/class-detail-header.tsx`: hàm `backTarget` ở :17-32;
  - `apps/web/src/features/roster/hooks/use-class-list-url-state.ts`: `view`, `q`, `weekday` và `shift` đều nằm trong URL;
  - test `apps/web/src/features/roster/__tests__/class-list-page.test.tsx`: import ở :13, khối `describe("RecruitingClassListPage")` ở :368-459, `DetailStub` ở :462 (chỉ dùng ở :384).
- Mẫu redirect có sẵn là `apps/web/src/features/roster/components/contacts-redirect.tsx` và `class-settings-redirect.tsx`.

## Key insights

- Đã kiểm tra rằng chủ trung tâm lấy được mọi kỳ trên `/reports`:
  - `GET /billing-periods` dùng `PermBillingList` (routespec.go:340).
  - Đường gọi là `ListPeriods` → `ListPeriodsRead` → `readScoped` (repository.go:330) → `readNarrow` (:350) → `CenterWideFor(PermBillingViewAll)`.
  - Với chủ trung tâm, hàm cuối luôn trả về true, và web không có guard route nào. Vì vậy không có gì chặn chủ trung tâm.
- Có một chỗ vướng thật cho chủ trung tâm:
  - Khi chủ trung tâm gửi `zalo_personal` cho kỳ của giáo viên khác, API trả 409 "this period belongs to another teacher" (notifications/service.go:212 và :530), vì `delegatedSender` chỉ xét `Perms.HasKey(reports.send)`.
  - Kênh `zalo_manual` vẫn chạy được. Đây là thiết kế có chủ đích nên không đổi API.
  - Web lại báo sai thành "Đang có lượt gửi chạy". Phase này sửa câu toast đó.
- `/reports` gọi `per_page=100` (reports-api.ts). Nếu trung tâm có nhiều hơn 100 kỳ thì các kỳ cũ bị cắt. Hành vi này đã có từ trước và không nằm trong phạm vi phase này.
- `useClassListUrlState` chỉ giữ được một `view`. Vì vậy redirect sẽ bỏ `view`, `status` và `phase` cũ của `/classes/recruiting` rồi đặt `view=recruiting`. Trang cũ vốn đã bỏ chip "recruiting" (:62-65), nên không mất thông tin nào đáng kể.
- "Most specific wins" của `useNavActive` dựa trên `NavPathsContext`. Thêm một danh sách tiền tố phụ tuỳ chọn là thay đổi nhỏ nhất: các mục khác không truyền thì hành vi giữ nguyên.

## Requirements

- Mục "Gửi thông báo" có `to: "/reports"`, `perm: "reports.send"` và `activePrefixes: ["/notifications"]`. Mục này không phụ thuộc `periodId` nên không bao giờ bị disable.
- Không còn các mục "Gửi báo cáo", "Hồ sơ học sinh", "Lớp cần tuyển sinh" và "Cấu hình lớp học". "Phân quyền vai trò" giữ nguyên.
- Header nhóm đổi thành "Học liệu". Nhãn mục `/library` vẫn là "Kho học liệu".
- "Sổ lớp" dùng `perm: "teaching.read"` (routespec.go:325-327 cho curriculum, lesson-plans và marks). Mọi mục nav còn lại đã khớp key API.
- `CurrentPeriodDisc` chỉ render khi `has("billing.read")`.
- `/classes/recruiting` redirect tới `/classes?view=recruiting` và giữ `q`, `weekday`, `shift`.
- Route và h1 của `/reports` là "Gửi thông báo". Aria-label của từng dòng là `Gửi thông báo tháng m/y của X`.

## Related files

**Modify**

- `apps/web/src/layouts/dashboard-layout.tsx`
- `apps/web/src/layouts/__tests__/dashboard-layout.test.tsx`
- `apps/web/src/features/reports/routes.tsx`
- `apps/web/src/features/reports/pages/send-reports-page.tsx`
- `apps/web/src/features/reports/__tests__/send-reports-page.test.tsx`
- `apps/web/src/features/collections/pages/notifications-page.tsx`
- `apps/web/src/features/collections/__tests__/notifications-page.test.tsx`
- `apps/web/src/features/roster/routes.tsx`
- `apps/web/src/features/roster/pages/class-list-page.tsx`
- `apps/web/src/features/roster/components/class-status-chips.tsx`
- `apps/web/src/features/roster/components/class-detail-header.tsx`
- `apps/web/src/features/roster/__tests__/class-list-page.test.tsx`

**Create**

- `apps/web/src/features/roster/components/recruiting-redirect.tsx`
- Test redirect: thêm một ca vào `class-list-page.test.tsx`, không tạo file test mới.

**Delete**: không có. Code biến thể được gỡ ngay trong file.

## Implementation steps

1. Sửa `NavEntry` trong `dashboard-layout.tsx`.
   - Thêm trường `activePrefixes?: readonly string[]` kèm doc comment: đây là các route ngoài `to` mà mục vẫn sáng.
   - Đổi chữ ký thành `useNavActive(to, activePrefixes?)`. Sau khi xử lý nhánh `!to` và nhánh `"/"`, nếu `activePrefixes?.some(covers)` thì trả true; còn lại giữ nguyên logic cũ.
   - Truyền `activePrefixes` vào `useNavActive` ở cả `SidebarNavItem`, `RailNavItem` và `BottomTabItem`.
2. Sửa `useNavGroups`.
   - Gỡ "Hồ sơ học sinh", "Lớp cần tuyển sinh", "Gửi báo cáo" và "Cấu hình lớp học".
   - Đổi "Gửi thông báo" thành `{ label: "Gửi thông báo", to: "/reports", Icon: HvSendIcon, perm: "reports.send", activePrefixes: ["/notifications"] }`. Viết lại comment ở :132-133: mục này mở danh sách kỳ, còn trang gửi của từng kỳ vẫn sáng mục này.
   - Đổi header "Kho học liệu" thành "Học liệu".
   - Đổi `perm` của "Sổ lớp" sang `"teaching.read"`.
   - Sửa doc comment ở :57-66: tên nhóm mới, và chỉ còn hai route theo kỳ là Chốt sổ và Thu tiền.
   - Gỡ các import không còn dùng: `IdCardIcon`, `SlidersHorizontalIcon`, và icon của "Lớp cần tuyển sinh" nếu không còn chỗ nào dùng. Để `tsc` và eslint xác nhận.
   - Nếu biến `isOwner` trong `useNavGroups` không còn dùng thì gỡ khỏi destructuring. "Phân quyền vai trò" vẫn dùng nó.
3. Sửa `OVERFLOW_LABELS`: bỏ "Hồ sơ học sinh", "Lớp cần tuyển sinh", "Gửi báo cáo" và "Cấu hình lớp học". Sửa `OVERFLOW_PATH_PREFIXES`: bỏ `"/records"`, giữ `"/notifications"` và `"/reports"`. Cập nhật comment ở :193-198.
4. Gate `CurrentPeriodDisc`: lấy `has` từ `useCenterContext()` và trả `null` khi `!has("billing.read")`. Đặt kiểm tra này trước khi đọc `period`. Không đụng `CurrentPeriodCard`.
5. Sửa `reports/routes.tsx`.
   - Đổi `title` thành "Gửi thông báo".
   - Viết lại comment: mục nav "Gửi thông báo" gate theo `reports.send` và là lối vào chung cho mọi vai trò, kể cả chủ trung tâm. API là nơi quyết định. Chủ trung tâm và người giữ `reports.send` thấy kỳ của cả trung tâm qua `billing.view_all` (ngầm có từ `reports.send`), còn thành viên thường chỉ thấy kỳ của mình.
6. Sửa `send-reports-page.tsx`: h1 thành "Gửi thông báo", aria-label thành `Gửi thông báo tháng ${m}/${y} của ${teacher}`, và cập nhật doc comment. Cập nhật `send-reports-page.test.tsx:87` theo nhãn mới.
7. Sửa `notifications-page.tsx`.
   - Thêm hằng `CROSS_TEACHER_409 = "belongs to another teacher"` cạnh `EXPIRED_SESSION_409`, giữ comment về việc phụ thuộc chuỗi thông báo của server.
   - Trong mọi nhánh `onError` 409 của lệnh gửi (:226-238), map lỗi này sang "Kỳ này của giáo viên khác — Zalo cá nhân chỉ gửi kỳ của bạn, hãy chọn Zalo thủ công".
   - Rà thêm các `onError` khác trong file đọc 409 (ví dụ `resume` ở :243-251) và chỉ thêm nhánh này ở nơi API có thể trả lỗi đó.
   - Thêm một ca vào `notifications-page.test.tsx` với MSW trả 409 kèm message này.
8. Gỡ "Lớp cần tuyển sinh" khỏi roster.
   - Tạo `recruiting-redirect.tsx` export `RecruitingRedirect`. Component đọc `useSearchParams`, chỉ chép `q`, `weekday` và `shift`, đặt `view=recruiting`, rồi `<Navigate to={`/classes?${params}`} replace />`. Làm theo mẫu `contacts-redirect.tsx`.
   - Trong `roster/routes.tsx`, route `classes/recruiting` dùng `lazy` tới `RecruitingRedirect`. Giữ comment về static segment, và giữ handle title "Danh mục lớp" để tiêu đề không nhấp nháy.
   - Trong `class-list-page.tsx`:
     - xoá `ClassListVariant`, `variantCopy`, `ClassListPageProps` và `RecruitingClassListPage`;
     - `ClassListPage()` không nhận prop, `view = url.view`;
     - `params.recruiting = true` chỉ khi `view === "recruiting"`;
     - tiêu đề, phụ đề và `emptyLabel` dùng chuỗi của biến thể `all`;
     - `navigate(`/classes/${klass.id}`)` không còn `state`;
     - `emptyLabel` dùng `filtered ? ... : ...` và bỏ `recruitingPage`.
   - Trong `class-status-chips.tsx`: gỡ prop `includeRecruiting` và luôn render chip "Cần tuyển sinh". Cập nhật doc comment.
   - Trong `class-detail-header.tsx`: gỡ `backTarget` và `useLocation`. Link quay lại cố định là `{ to: "/classes", label: "Danh mục lớp" }`.
9. Sửa `class-list-page.test.tsx`.
   - Xoá khối `describe("RecruitingClassListPage")` (:368-459), `DetailStub` và import `RecruitingClassListPage`.
   - Thêm ca kiểm redirect: render `/classes/recruiting?q=a&weekday=2&shift=morning&phase=x` qua `MemoryRouter` với route redirect. Kỳ vọng pathname là `/classes`, `view=recruiting`, còn `q`, `weekday` và `shift`, và không còn `phase`.
10. Cập nhật `dashboard-layout.test.tsx`.
    - Nhóm "Học liệu" (:106-117, :647-724, :668): query theo tên nhóm mới, còn mục "Kho học liệu" vẫn có.
    - Danh sách nhóm (:124-140), thứ tự Dạy học (:305-322) và thứ tự Trung tâm của chủ trung tâm (:326-341): không còn "Hồ sơ học sinh" và "Cấu hình lớp học"; "Phân quyền vai trò" vẫn còn.
    - "Gửi thông báo" trong sheet (:167-194): href là `/reports`.
    - Các mục theo kỳ bị disable (:284-299): chỉ còn Chốt sổ và Thu tiền; "Gửi thông báo" luôn bật.
    - Thay các test "Gửi báo cáo" (:504-549) bằng các ca sau:
      - chủ trung tâm, thành viên có `reports.send` và thành viên không có key đều thấy đúng một mục hoặc không thấy mục "Gửi thông báo";
      - không còn link "Gửi báo cáo";
      - trên `/notifications/p1` và `/reports`, mục "Gửi thông báo" có `aria-current="page"` (cách assert active mà file test đang dùng ở :214).
    - Gỡ test "Lớp cần tuyển sinh" (:585-601) và "Hồ sơ học sinh" trong sheet (:251-265), rồi thêm assert âm cho cả hai.
    - Thêm ca: "Sổ lớp" ẩn khi thiếu `teaching.read` và hiện khi có.
    - Thêm ca: đĩa kỳ hiện tại (aria-label "Kỳ hiện tại: tháng …") không render khi thiếu `billing.read`.

## Todo

- [x] `NavEntry.activePrefixes` và `useNavActive(to, activePrefixes)` ở cả ba item
- [x] Gỡ bốn mục trùng, gộp "Gửi thông báo" → `/reports`
- [x] Header "Học liệu", perm "Sổ lớp" = `teaching.read`
- [x] Dọn `OVERFLOW_LABELS`, `OVERFLOW_PATH_PREFIXES` và comment
- [x] Gate `CurrentPeriodDisc` theo `billing.read`
- [x] Đổi tiêu đề, h1 và aria-label của `/reports`, sửa comment route
- [x] Toast 409 kỳ của giáo viên khác trên trang gửi, kèm test
- [x] `RecruitingRedirect` và dọn biến thể, chips, header
- [x] Cập nhật test layout, class-list và send-reports

## Verification

Chạy trong `apps/web`:

- `npx vitest run src/layouts/__tests__/dashboard-layout.test.tsx src/features/reports src/features/collections/__tests__/notifications-page.test.tsx src/features/roster/__tests__/class-list-page.test.tsx src/features/roster/__tests__/class-detail-page.test.tsx`
- `npm run typecheck`, `npm run lint`, `npm run format:check`

Tiêu chí hoàn thành:

- `rg -n "Gửi báo cáo|Lớp cần tuyển sinh|RecruitingClassListPage|includeRecruiting" apps/web/src` chỉ còn khớp ở những test assert âm.
- Kiểm tay trên dev stack với tài khoản chủ trung tâm: mở `/reports` thấy mọi giáo viên; bấm một kỳ sang `/notifications/:id` thì "Gửi thông báo" vẫn sáng.

## Risk assessment

| Rủi ro | Khả năng × Tác động | Giảm thiểu |
|---|---|---|
| Chủ trung tâm mất lối tắt một cú bấm tới kỳ hiện tại | Cao × Thấp | Đây là quyết định đã chốt. Trang Chốt sổ vẫn còn link "Gửi thông báo →" |
| `activePrefixes` làm hai mục cùng sáng | Thấp × Thấp | Chỉ "Gửi thông báo" khai báo tiền tố, và không mục nào khác có `to` bắt đầu bằng `/notifications`. Có test active |
| Vai trò bị thu hẹp có `reports.send` nhưng thiếu `billing.list` sẽ gặp 403 trên `/reports` | Thấp × Trung bình | Hành vi này có từ trước và không đổi. Ghi vào open decisions |
| Bookmark cũ `/classes/recruiting?view=active` mất phase | Thấp × Thấp | Redirect đặt `view=recruiting` và giữ các bộ lọc còn lại |

## Security

Phase này không đổi quyền phía API. Ẩn mục nav chỉ là UX, API vẫn là nơi quyết định. Toast mới không để lộ thêm dữ liệu.

## Rollback

`git revert` commit của phase. Phase không có thay đổi dữ liệu. Các redirect biến mất cùng commit nên route cũ quay lại.

## Open decisions

- Nhãn nhóm "Học liệu" có thể đổi lúc validate.
- Câu toast 409 cho chủ trung tâm khi dùng Zalo cá nhân với kỳ của giáo viên khác.
- "Sổ lớp" dùng `teaching.read`. Trang này còn gọi `classes.list` và `sessions.list`, nhưng cả ba đều là quyền nền nên vai trò mặc định không đổi gì.
