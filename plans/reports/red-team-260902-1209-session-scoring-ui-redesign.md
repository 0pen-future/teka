---
title: "Red-team: session scoring UI redesign plan"
date: 2026-09-02
target: plans/260902-1209-session-scoring-ui-redesign/
verdict: GO WITH FIXES
---

# Red-team — plan `260902-1209-session-scoring-ui-redesign`

## Summary verdict

**GO WITH FIXES.** Kiến trúc và phân pha hợp lý, ràng buộc web-only phần lớn
đứng vững, nhưng có 5 lỗi phải sửa trước khi code: một lỗi làm hỏng toàn bộ
test hiện có (media query), một phụ thuộc API ẩn (cột bộ điểm theo lớp trong
bảng Phase 6), một race/mất dữ liệu trong thiết kế autosave, một lỗi kiểu sai
im lặng (`HvButton.icon`), và một hồi quy accessibility (409 mất
`role="alert"`).

Đã kiểm tra: 7 file plan, report nguồn, 2 report research, và ~20 file mã
nguồn web + API/DB.

---

## Critical findings (phải sửa trước khi implement)

### C1 — `useMediaQuery("(min-width: 640px)")` sẽ biến MỌI test hiện có thành mobile

- **Phase**: 3
- **Bằng chứng**: `apps/web/src/test/setup.ts:32-44` — stub trả
  `matches: false` cho **mọi** query, cố định, không phụ thuộc nội dung query.
  Plan (phase-03, mục Architecture) viết
  `const isMobile = !useMediaQuery("(min-width: 640px)")` và khẳng định
  *"setup đã stub matchMedia trả `matches:false` → jsdom luôn là desktop"*.
- **Tại sao quan trọng**: với stub đó, `useMediaQuery("(min-width: 640px)")`
  trả `false`, nên `isMobile === true` trong **tất cả** test. Toàn bộ
  `classbook-page.test.tsx` và `component-score-grid.test.tsx` sẽ render panel
  bên trong `HvModal`, đổi cây DOM, focus trap Radix, và ít nhất
  `classbook-page.test.tsx:124` (`findByRole("heading", …)`) cùng các query
  `getByRole("tab")` sẽ phải viết lại. Kết luận trong plan là ngược dấu.
- **Fix**: đổi truy vấn sang dạng max-width để `false` = desktop:
  `const isMobile = useMediaQuery("(max-width: 639px)")`. Ghi thêm vào
  phase-03: *"Stub `matchMedia` trong `src/test/setup.ts:32` trả `matches:false`
  cho mọi query, nên truy vấn phải viết ở dạng `max-width` để mặc định trong
  jsdom là desktop; test mobile override `window.matchMedia` cục bộ và khôi
  phục trong `afterEach`."*

### C2 — Phase 6 hiển thị cột bộ điểm của từng lớp: không có API, sẽ thành N+1

- **Phase**: 6
- **Bằng chứng**:
  - `apps/web/src/features/center/pages/class-config-page.tsx:152` —
    `useClassesList({ per_page: 100 })`, bảng chỉ có 2 cột (Lớp, nút Gán), hôm
    nay **không** hiển thị cột bộ điểm.
  - `apps/web/src/features/center/hooks/use-score-sets.ts:49` —
    `useClassScoreComponents(classId)` là query **một lớp một lần**.
  - `apps/api/internal/features/grading/routes.go:19` — chỉ có
    `GET /classes/:id/score-components`; không có endpoint batch.
  - phase-06 Architecture: `class-score-set-table.tsx` với
    `rows: {classId, className, columns: string[]}`; Requirements: *"chip cột
    hoặc 'Chưa gán'"*. Report nguồn §5.3 cũng yêu cầu điều này.
- **Tại sao quan trọng**: `columns` không có nguồn. Hiện thực trung thành
  nghĩa là 100 request `GET /classes/:id/score-components` khi mở trang — N+1
  rõ ràng, và mâu thuẫn với ràng buộc "không đổi API" (đây chính là phụ thuộc
  API ẩn mà plan nói đã loại bỏ ở D5). Test MSW cũng sẽ phải seed handler cho
  mọi lớp.
- **Fix**: chọn một, ghi rõ vào phase-06:
  1. **Khuyến nghị** — bỏ cột bộ điểm khỏi bảng ở đợt này; giữ 2 cột + nút
     "Gán bộ điểm", thêm dòng non-goal *"Hiển thị bộ điểm đang gán cho từng lớp
     cần endpoint batch; chuyển sang follow-up API cùng `class_count` và
     `has_scores`."* Bản thẻ dưới `md` vẫn làm được.
  2. Hoặc: chỉ fetch `score-components` cho lớp **đang mở dialog** (đã làm rồi)
     và hiển thị cột trong dialog, không trong bảng.
  Nếu vẫn muốn trong bảng, phải mở plan API và bỏ ràng buộc web-only.

### C3 — Thiết kế autosave đè mất ô và vẫn ghi khi người dùng bấm "Bỏ thay đổi"

- **Phase**: 3 (kéo sang 4)
- **Bằng chứng**: `apps/web/src/features/teaching/hooks/use-debounced-save.ts:46-53`
  — `schedule(payload)` **thay thế** `pendingRef.current` bằng payload mới;
  `:37-44` — `flush()` gửi đúng payload đã lưu, không đọc lại state;
  `:60` — `useEffect(() => flush, [flush])` **flush khi unmount**.
  phase-03 chỉ viết *"`useDebouncedSave(save, 800)` schedule khi blur ô dirty"*.
- **Tại sao quan trọng**: hai lỗi thật.
  1. Nếu payload là ô vừa blur, gõ ô A rồi trong 800ms gõ ô B sẽ **mất ô A**
     (payload bị thay). Report research §4 giả định *"cùng một pending ref"*
     nhưng không nói rõ payload phải là toàn bộ tập dirty.
  2. Guard "Bỏ thay đổi" → `discard()` → unmount panel/modal → `flush()` ở
     dòng 60 vẫn gửi payload đang treo. Người dùng bấm bỏ mà dữ liệu vẫn được
     ghi.
- **Fix**: ghi rõ trong phase-03 Architecture:
  *"`schedule` luôn nhận **snapshot toàn bộ tập dirty hiện tại** (`buildPayload()`),
  không phải ô vừa blur, vì `useDebouncedSave.schedule` thay thế payload đang
  treo (`use-debounced-save.ts:46`). `discard()` phải gọi `cancel()` trước khi
  xoá draft, vì hook flush khi unmount (`use-debounced-save.ts:60`); nếu không,
  'Bỏ thay đổi' vẫn gửi PUT."*
  Thêm test bắt buộc: gõ ô A, gõ ô B trong <800ms, chờ → **một** PUT chứa cả
  hai ô; và: gõ → "Bỏ thay đổi" → unmount → **không** có PUT.

### C4 — `HvButton.icon` là `ReactNode`, không phải `HvIconName`: `icon="x"` typecheck vẫn xanh nhưng render sai

- **Phase**: 2, 4, 5, 6
- **Bằng chứng**: `apps/web/src/components/hv/hv-button.tsx:69`
  `icon?: React.ReactNode;` và `:96` render `<span>{icon}</span>`.
  phase-02 viết `HvButton variant="ghost" size="sm" icon="x"`;
  phase-04 viết `HvButton variant="secondary" size="sm" icon="table"`.
- **Tại sao quan trọng**: `"x"` là `ReactNode` hợp lệ → `npm run typecheck`
  **xanh**, lint xanh, nhưng nút hiển thị chữ cái "x"/"table" thay vì icon.
  Đây đúng kiểu lỗi qua được CI.
- **Fix**: sửa mọi chỗ trong plan thành `icon={<HvIcon name="x" />}`, và thêm
  một dòng vào phase-01 Interfaces: *"`HvButton.icon` nhận `ReactNode`
  (`hv-button.tsx:69`), không nhận tên icon — luôn truyền `<HvIcon name=… />`."*
  Nếu muốn nhận tên, phải đổi `HvButton` (thuộc phase-01, không phải phase-02).

### C5 — Thông điệp 409 mất `role="alert"` khi đổi sang `HvNotice tone="warning"`

- **Phase**: 2 và 6
- **Bằng chứng**:
  `apps/web/src/features/center/components/assign-score-set-dialog.tsx:132-138`
  hiện dùng `role="alert"`. phase-01 định nghĩa
  *"`tone=danger` → `role="alert"`, còn lại `role="note"`"*; phase-02 và
  phase-06 đều yêu cầu `HvNotice tone="warning"` cho `CONFLICT_MESSAGE`.
- **Tại sao quan trọng**: 409 là lỗi chặn hành động, xuất hiện **sau** thao
  tác của người dùng. Chuyển sang `role="note"` khiến trình đọc màn hình không
  thông báo, đồng thời làm hỏng bất kỳ assertion `getByRole("alert")` nào.
  phase-02 mô tả mơ hồ (*"assert `getByRole('note')`/alert … nếu markup đổi"*)
  — chưa quyết.
- **Fix**: dùng `HvNotice tone="danger"` cho 409 (giữ `role="alert"`), hoặc
  thêm prop `role?: "note" | "alert" | "status"` vào `HvNotice` ở phase-01 và
  đặt `role="alert"` cho 409. Chốt một cách và ghi vào cả phase-02, phase-06,
  và bảng test.

---

## Major findings

### M1 — Fixture MSW không tạo được trạng thái `late`/`excused`; D2 không test được trong ranh giới file đã khai báo

`apps/web/src/features/roster/__tests__/roster-handlers.ts:590` —
`status: session.status === "held" ? (absent ? "absent" : "present") : null`.
Chỉ có `present`/`absent`/`null`. phase-03 test scenario yêu cầu
*"3 học sinh present/late/absent"*. Muốn test D2 phải sửa
`features/roster/__tests__/roster-handlers.ts`, vi phạm ranh giới trong plan.md
(*"P3–P4 chỉ sửa `features/teaching/**` + `lib/hooks`"*), và file này dùng
chung với nhiều suite khác.

**Fix**: thêm vào phase-03 Related Code Files: `Modify
apps/web/src/features/roster/__tests__/roster-handlers.ts` — thêm store
`lateIds`/`excusedIds` và helper seed; cập nhật ranh giới file trong plan.md và
ghi rõ đây là file test dùng chung nên P3 và P5/P6 không được chạy song song
trên file này.

### M2 — Phase 2 làm lại `component-score-grid.tsx` và test của nó, Phase 3 xoá cả hai

phase-02 sửa `component-score-grid.tsx` (ô nhập, nút lưu, `invalidKeys`,
`HvStateBlock`) và thêm 3 case vào `component-score-grid.test.tsx`; phase-03
`Delete component-score-grid.tsx` và `Rename+Modify` chính file test đó. Ước
tính bỏ đi phần lớn 0.5d của Phase 2.

**Fix**: chuyển toàn bộ dòng `component-score-grid.tsx` và
`component-score-grid.test.tsx` từ phase-02 sang phase-03. Việc xoá
`save-button-styles.ts` và `parseScoreInput` cũ cũng chuyển sang phase-03
(caller cuối cùng biến mất ở đó). Phase 2 chỉ còn panel + 3 file center.

### M3 — Test debounce với fake timers chưa khả thi như mô tả

`component-score-grid.test.tsx:49` dùng
`vi.useFakeTimers({ toFake: ["Date"] })` — **không** fake `setTimeout`. Bảng
test phase-03 ghi *"sau 800ms (fake timers) PUT một lần"*. Muốn vậy phải fake
timer đầy đủ, và khi đó `userEvent.setup()` sẽ treo nếu không truyền
`advanceTimers`.

**Fix**: ghi vào phase-03 Test scenarios: *"Test debounce dùng
`vi.useFakeTimers()` đầy đủ và `userEvent.setup({ advanceTimers:
vi.advanceTimersByTime })`; các test khác giữ `toFake: ['Date']` như hiện tại
(`component-score-grid.test.tsx:49`)."* Cân nhắc tách file test riêng cho
`useScoreDraft` để không đụng cấu hình clock của suite UI.

### M4 — `toHaveValue` sẽ đổi kiểu khi bỏ `type="number"`

`component-score-grid.test.tsx:114` `expect(miengInput).toHaveValue(8)` và
`:139` `toHaveValue(null)`. Với `type="text"` chúng thành `"8"` và `""`.
Plan chỉ nhắc rủi ro chung (*"Testing Library vẫn set value chuỗi"*) mà không
chỉ ra hai assertion cụ thể.

**Fix**: liệt kê hai dòng đó trong phase (sau khi gộp theo M2, là phase-03) với
giá trị mới.

### M5 — Ô điểm chung: `null` sẽ trở thành "xoá điểm", hành vi mới chưa được nêu

`session-detail-panel.tsx:136-141` hiện bỏ qua ô không parse được và ô rỗng
(`score !== null` mới push). `MarkEntryInput.score` là `number | null |
undefined` (`teaching-schemas.ts:87`) và schema ghi rõ tri-state, nên gửi
`null` sẽ **xoá** điểm. Với contract mới (`"" → null`), nếu phase-02 chuyển
thẳng `null` vào payload thì lần đầu tiên người dùng có thể xoá điểm chung —
tính năng mới, chưa nằm trong scope.

**Fix**: phase-02 bước 2 phải nói rõ một trong hai: (a) giữ nguyên hành vi cũ,
`null` ở ô điểm chung vẫn bị bỏ qua; hoặc (b) cố ý cho phép xoá, ghi vào
Decisions + thêm test. Đừng để implementer tự đoán.

### M6 — Constraint trong plan.md mâu thuẫn với Phase 5

plan.md Constraints: *"Không đổi API, DB, schema zod wire
(`features/center/schemas/grading.ts` …)"*. phase-05 Related Code Files:
`Modify apps/web/src/features/center/schemas/grading.ts`.

**Fix**: đổi câu constraint thành *"không đổi **shape wire** của schema zod;
được phép refactor nội bộ `superRefine` miễn giữ nguyên message và path."*

### M7 — Success criteria bằng `grep` của Phase 2 vừa quá rộng vừa bắt sai chuỗi

phase-02: *"`grep -rn 'save-button-styles|Đang tải…' src/features/teaching/components src/features/center` rỗng"*.
- Quá rộng: `src/features/center/components/permission-matrix.tsx:146` và
  `src/features/center/pages/center-page.tsx:28` cũng chứa "Đang tải…" nhưng
  **ngoài scope** — tiêu chí này ép scope creep hoặc không bao giờ xanh.
- Bắt sai: hai màn teaching dùng "Đang tải điểm thành phần…" và "Đang tải danh
  sách học sinh…", không khớp chuỗi `Đang tải…`.

**Fix**: đổi thành grep theo danh sách 5 file trong scope, và dùng mẫu
`Đang tải` (không có dấu ba chấm).

### M8 — `HvSegmented variant="tabs"`: không có `tabpanel`, roving tabindex đổi hành vi Tab

`session-detail-panel.tsx:190-210` hiện là `role="tablist"` + `role="tab"`
nhưng **không** có `role="tabpanel"`, `aria-controls`, hay `id`. phase-01 thêm
điều hướng mũi tên (kéo theo roving tabindex, chỉ tab đang chọn Tab tới được).

**Tại sao quan trọng**: `role="tab"` mà không có panel liên kết là mẫu ARIA dở
dang; thêm roving tabindex làm nó "trông đúng hơn" nhưng vẫn không thông báo
được panel. Đây là hồi quy tinh vi so với nút thường.

**Fix**: phase-01 hoặc phase-02 phải bổ sung `aria-controls` + `id` trên tab và
`role="tabpanel" aria-labelledby` trên vùng nội dung của panel; hoặc dùng
`variant="segmented"` (RadioGroup) và bỏ hẳn ngữ nghĩa tab.

### M9 — Guard khi đổi buổi chưa có thiết kế; `key=` làm panel unmount trước khi hỏi

`classbook-page.tsx:286-295` render `SessionDetailPanel key={session.id}` và
`SessionsTable onSelect` do trang sở hữu (`:277-281`). phase-03 chỉ ghi
*"chọn buổi khác khi dirty → guard (panel tự expose qua `onRequestSelect`)"* —
panel không kiểm soát được `onSelect` của bảng, và khi `selectedSessionId` đổi
thì `key` đổi → panel unmount (kéo theo flush của C3) trước khi kịp hỏi.

**Fix**: mô tả rõ luồng: trang giữ `pendingSessionId`, panel báo lên
`isDirty` qua callback, trang mở guard và chỉ `setSelectedSessionId` sau khi
người dùng chọn. Hoặc chấp nhận non-goal *"đổi buổi khi còn ô chưa lưu sẽ tự
lưu"* và ghi vào Decisions.

### M10 — D2 vừa là Quyết định vừa là Câu hỏi mở

plan.md Decisions D2 chốt cho `late` chấm được; Open questions lại ghi *"xác
nhận với chủ sản phẩm"*. Đây là thay đổi hành vi nghiệp vụ (API không chặn,
`service.go:377-395` chỉ kiểm tra roster), nên không nên để implementer tự
quyết giữa chừng.

**Fix**: giải quyết trước Phase 3 và bỏ khỏi Open questions, hoặc hạ D2 thành
non-goal của đợt này.

### M11 — Hiển thị điểm cho ô không sửa được là thay đổi hành vi chưa nêu

`component-score-grid.tsx:145-156` (và `session-detail-panel.tsx:338-349`):
khi không editable, ô luôn hiện `"—"` hoặc `"Vắng"`, **không bao giờ** hiện
điểm đã lưu. phase-03 test scenario ghi *"`canWrite=false` → không có input,
chỉ text điểm"*.

**Tại sao quan trọng**: đây là cải thiện thật (người xem read-only hiện không
thấy điểm nào), nhưng nó là thay đổi hành vi, không nằm trong Requirements, và
kéo theo câu hỏi: học sinh vắng đã từng có điểm thì hiện gì.

**Fix**: đưa vào Requirements của phase-03 kèm quy tắc cho nhóm "Vắng (n)".

---

## Minor findings

- **m1 (P1)** — `hv-icon.tsx:1-27` là registry re-export **lucide-react**,
  không phải SVG viết tay. phase-01 ghi *"cùng path SVG stroke 2 như icon hiện
  có"*. Sửa thành: thêm `ArrowUp, ArrowDown, Trash2, Table, Info,
  AlertTriangle` vào `iconRegistry` và vào union `HvIconName`.
- **m2 (P3)** — Đường dẫn `score-entry-summary.ts` mâu thuẫn: khối Architecture
  ghi `components/score-entry-summary.ts`, bảng Related Code Files ghi
  `lib/score-entry-summary.ts`. Chốt `lib/`.
- **m3 (P3/P4)** — `score-entry-footer.tsx` không có trong file list của
  phase-03 nhưng phase-04 ghi *"nếu Phase 3 đã có thì chỉ dùng lại"*. Giao
  quyền sở hữu dứt khoát cho Phase 3.
- **m4 (P1)** — D6 nói xl dưới `sm` dùng `max-h-[95dvh]`; bảng size map của
  phase-01 không có dòng đó. Base content đang là `max-h-[85vh]`
  (`hv-modal.tsx:50`); `twMerge` sẽ giải quyết xung đột nếu class mới đặt sau,
  nhưng phải viết ra.
- **m5 (plan)** — Success criterion dùng 8 cột (864px). Sản phẩm cho tối đa 10
  cột (`grading.ts:24`, `dto.go` `max=10`): 180 + 10×76 + 76 = **1016px** so
  với ~1032px nội dung khả dụng (1080 trừ `p-6` hai bên,
  `hv-modal.tsx:51`). Đổi tiêu chí sang 10 cột, hoặc giảm bề rộng cột.
- **m6 (P4/P5)** — Ba yêu cầu trong report nguồn bị bỏ mà không ghi vào
  Non-goals: bấm tiêu đề cột để focus ô đầu (§4.2); Enter ở hàng cuối tự thêm
  hàng (§5.1); đủ 10 cột thì **ẩn** nút thêm + HvNotice info (§5.1, plan đổi
  thành disabled + helper). Thêm ba dòng vào Non-goals.
- **m7 (P5)** — `score-set-editor-modal.tsx:173` chỉ **render** nút Xóa khi
  `components.length > 1`; phase-05 đổi sang luôn render + disabled. Là thay
  đổi hành vi nhỏ, cần ghi và cập nhật assertion.
- **m8 (P1)** — Bảng test `parseScoreInput` thiếu ca biên quan trọng:
  `"7abc"` (hiện `Number.parseFloat` trả `7`, không phải invalid),
  `"7,5,5"`, `"1e1"`, `"  "`. Contract `"invalid"` phải nói rõ có dùng
  `parseFloat` (chấp nhận tiền tố số) hay regex nghiêm ngặt.
- **m9 (P1)** — phase-01 đặt nút Huỷ của `HvConfirmDialog` là
  `variant="secondary"`; quy ước hiện có trong repo là `variant="ghost"`
  (`score-set-editor-modal.tsx:119`, `assign-score-set-dialog.tsx:98`).
- **m10 (P4)** — Đặt `aria-label` lên `<th>` sẽ thay tên khả truy cập của
  columnheader. Ghi rõ: `aria-label` luôn bằng **tên cột đầy đủ**, không phụ
  thuộc việc có bị `line-clamp` hay không, để `getByRole("columnheader",
  {name})` ổn định.
- **m11 (P3)** — Stub `matchMedia` được cài một lần cho mỗi file test
  (`setup.ts:32`); override cục bộ sẽ rò sang các test sau trong cùng file.
  Yêu cầu khôi phục trong `afterEach`.
- **m12 (P3/P4)** — Học sinh bị đánh vắng **sau khi** đã có điểm: nhóm
  "Vắng (n)" thu gọn và hàng `colspan` ở bảng đều không hiện ô nhập, nên điểm
  cũ vừa không thấy vừa không xoá được. Cần một dòng quyết định (hiện điểm chỉ
  đọc + nút xoá, hay bỏ qua).
- **m13 (P4)** — Rủi ro hiệu năng ghi trong phase-04 chỉ nói về render lần đầu
  200 input. Vấn đề thật là **mỗi lần gõ** cập nhật Map trong `useScoreDraft`
  → re-render cả 200 input. Ghi rõ biện pháp (memo hàng, hoặc chấp nhận có đo).
- **m14 (P6)** — Bảng test ghi *"PUT/POST assign"*; endpoint là
  `POST /classes/:id/score-set` (`routes.go:17`).
- **m15 (research)** — Hai khẳng định trong
  `research-grid-ux-260902-1209` sai và không nên dựa vào:
  (a) *"`useDebouncedSave` … already the project's debounce primitive, used
  elsewhere"* — thực tế **chỉ** `__tests__/use-debounced-save.test.ts` import
  nó, không có caller production; (b) *"TanStack Query's `mutate` already
  queues/replaces"* — `mutate` không xếp hàng; hai lời gọi chồng nhau sẽ tạo
  hai request. Guard `isPending` trong plan là bắt buộc, không phải phòng xa.

---

## Verified-OK claims (đã kiểm tra và đúng)

| Claim của plan | Nguồn xác minh |
|---|---|
| `GET /score-sets` trả `{id, name, components: string[]}` | `dto.go` `ScoreSetResponse` |
| `GET /classes/:id/score-components` trả `{class_id, components:[{id,name,position}]}` | `dto.go` `ClassComponentsResponse`; `routes.go:19` |
| `PUT /sessions/:id/scores` nhận `score` nullable, null = xoá ô | `dto.go` `ScoreEntryRequest.Score *float64`; `handler.go:301` |
| API **không** trả `has_scores`, `class_count`, `source_set_id` | `dto.go` (toàn bộ); `000014_grading.up.sql:47` (`source_set_id` chỉ ở DB) |
| Service chỉ kiểm tra roster `ActiveOn`, không kiểm tra điểm danh | `service.go:377-395` |
| Snapshot: xoá bộ gốc không ảnh hưởng lớp đã gán (căn cứ D5) | `000014_grading.up.sql:41-55`; `grading.ts:62-68` |
| Batch 160–200 ô nằm trong giới hạn API | `service.go:24` `maxScoreEntries = 500` |
| `--w-page: 1080px`, `--w-content: 720px` tồn tại | `src/styles/tokens/spacing.css:32-33` |
| `HvBadge` có `variant` `neutral` và `info` | `hv-badge.tsx:6-21` |
| `HvCard` có `variant="raised"`, `padding="md"` | `hv-card.tsx:6-7` |
| `components/ui/skeleton.tsx` tồn tại (cho HvStateBlock) | `src/components/ui/` |
| `radix-ui` v1.6.7 đã là dependency, dùng import gộp | `package.json:30`; `ui/select.tsx:2` |
| `errors.components?.root` là đường dẫn đúng cho lỗi cấp mảng | `score-set-editor-modal.tsx:186-191` |
| `parseScoreInput` chỉ có 2 caller production + 1 test | `classbook-stats.ts:243`; grep toàn repo |
| `save-button-styles.ts` chỉ có 2 caller | grep toàn repo |
| `grep 'type="number"' features/teaching features/center` chỉ khớp 2 file trong scope | `component-score-grid.tsx:142`, `session-detail-panel.tsx:324` |
| `title=` dùng làm tooltip nút disabled tồn tại đúng chỗ plan nói | `class-config-page.tsx:190` |
| `detailTabs`, `CONFLICT_MESSAGE`, `score-set-editor-form`, label `"Tên cột điểm N"` tồn tại đúng tên | `session-detail-panel.tsx:39`, `assign-score-set-dialog.tsx:22`, `score-set-editor-modal.tsx:128,150` |
| Fixture test (`CLASS_ID`, `comp-mieng`, `comp-15p`, `session-05`) đúng tên | `component-score-grid.test.tsx:26-29` |
| `src/lib/hooks/` đã tồn tại (chỗ đặt `use-media-query`) | `src/lib/hooks/use-no-index.ts` |
| Tailwind v4 → `line-clamp-2` là core, không cần plugin | `package.json:37` |
| Test setup có shim `matchMedia` và `ResizeObserver` | `src/test/setup.ts:32-52` |
| Tổng effort các phase (6d) khớp `effort: 6-7d` | plan.md |

## Đánh giá scope creep

Không thấy scope creep nghiêm trọng. 6 primitive ở Phase 1 đều có caller thật
trong Phase 2–6, không có abstraction mồ côi. Non-goals cắt đúng chỗ (dnd-kit,
mẫu bộ điểm, trọng số, quét 27 trang). Hai điểm cần chỉnh: **C2** kéo theo phụ
thuộc API ngoài phạm vi web-only, và **M7** ép sửa file ngoài scope.

## Unresolved questions

1. D2 (`late` chấm được) — cần chủ sản phẩm chốt trước Phase 3 (xem M10).
2. Bảng lớp có bắt buộc hiện cột bộ điểm không? Nếu có thì phải mở plan API
   (xem C2).
3. Ô điểm chung: có mở khả năng **xoá** điểm bằng ô rỗng không (xem M5)?
4. Học sinh đã có điểm rồi bị đánh vắng: hiện điểm chỉ đọc hay ẩn (xem m12)?
