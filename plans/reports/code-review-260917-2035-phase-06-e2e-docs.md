# Code review — Phase 6: E2E, tài liệu, hợp đồng v2

Ngày: 2026-09-17 20:35 (Asia/Saigon) · Branch `master`, working tree chưa commit
Phạm vi: `plans/260917-1515-task-dnd-rich-text/phase-06-e2e-docs-ship.md` (trừ Step 7 / Prod)

## Phạm vi đã đọc

- Mới: `apps/web/e2e/helpers/{auth,board,drag}.ts`, `apps/web/e2e/tasks-board-{dnd,rich-text,mobile}.spec.ts`,
  `plans/260917-1515-task-dnd-rich-text/reports/api-contract-v2.md`
- Sửa: `Makefile`, `README.md`, `apps/web/playwright.config.ts`, `apps/web/e2e/tasks-board.spec.ts`,
  `docs/architecture.md`, `docs/frontend-guidelines.md`
- Đối chiếu nguồn: `use-board-dnd.ts`, `lib/kanban/positions.ts`, `task-card.tsx`, `task-column.tsx`,
  `board-mobile.tsx`, `task-description-editor.tsx`, `pkg/kanban/service.go`,
  `internal/features/tasks/{description,errors,dto,task_repository}.go`, `docs/swagger.yaml`,
  `internal/shared/{apperror,validation}`, `internal/cli/seed.go`, `docker-compose.yml`, `lefthook.yml`
- Đã chạy: `tsc -b --noEmit` (exit 0), `npx eslint e2e playwright.config.ts` (exit 0),
  `npx prettier --check e2e playwright.config.ts` (sạch), `make -n e2e-isolated` (cú pháp hợp lệ)
- Không chạy (theo yêu cầu): `make test-api`, `go test -tags=integration`, `make e2e-isolated`

## Đánh giá tổng thể

Chất lượng tốt. Ngữ nghĩa e2e khớp với `resolveDrop` thật (không phải khớp với mô tả trong plan),
hợp đồng v2 đúng từng dòng với swagger + `description.go` + `errors.go`, docs không có câu nào sai
so với source. Vấn đề đáng sửa tập trung ở đường lỗi của `make e2e-isolated` và ở độ mạnh của một
vài assertion. Không có thay đổi code app, không có thay đổi hợp đồng public.

## High

### H1 — `e2e-isolated` không dọn stack khi bước `up` thất bại

`Makefile:99-109`. `@$(E2E_COMPOSE) up -d --build --wait` là một dòng recipe riêng. Khi nó fail
(port 55432/55173 đang bận, build lỗi, healthcheck `api` quá `start_period` 120s), make dừng ngay và
khối `status=...; ... down -v` không bao giờ chạy: container và volume `teka-e2e_*` còn treo, giữ port
cho lần chạy sau. Đúng cái failure mode mà `.claude/rules/process-management.md` mô tả. Yêu cầu (d)
"luôn `down -v` kể cả khi seed/playwright fail" hiện chỉ đúng cho seed và playwright.

Sửa: đưa `up` vào trong cùng khối shell.

```make
e2e-isolated: ## Build an isolated compose stack, seed it, run Playwright, tear it down
	@status=0; \
	$(E2E_COMPOSE) up -d --build --wait || status=$$?; \
	if [ $$status -eq 0 ]; then \
		(cd $(API_DIR) && API_DATABASE_URL=$(E2E_DATABASE_URL) go run ./cmd/api seed) || status=$$?; \
	fi; \
	if [ $$status -eq 0 ]; then \
		(cd $(WEB_DIR) && E2E_BASE_URL=http://localhost:55173 npx playwright test $(E2E_ARGS)) || status=$$?; \
	fi; \
	$(E2E_COMPOSE) down -v; \
	exit $$status
```

Đã xác minh mặt an toàn: `docker-compose.yml:142-145` khai báo volume thường (không `external`), nên
`down -v` chỉ xoá volume tiền tố `teka-e2e_` — project prod `teka` không bị đụng.

## Medium

### M2 — Thông tin DB của compose và của seed có thể lệch nhau

`Makefile:91-96`. `E2E_COMPOSE` không ghi đè `POSTGRES_USER/PASSWORD/DB`, nên compose lấy chúng từ
`.env` gốc repo (`docker-compose.yml:13-15`), trong khi `E2E_DATABASE_URL` hardcode
`teka:teka_dev_password@localhost:55432/teka`. Máy nào đổi các biến này trong `.env` sẽ dựng stack lên
được rồi seed fail bằng lỗi auth khó hiểu. Fail là loud (recipe dừng, dọn stack), nhưng vô cớ.

Sửa: ghim luôn ba biến trong `E2E_COMPOSE` để hai phía không thể lệch:

```make
E2E_COMPOSE := POSTGRES_USER=teka POSTGRES_PASSWORD=teka_dev_password POSTGRES_DB=teka \
	POSTGRES_PORT=55432 API_HTTP_PORT=58080 WEB_PORT=55173 ADMINER_PORT=58081 \
	...
```

### M3 — `dragTo` không cuộn phần tử vào viewport trước khi đo toạ độ

`apps/web/e2e/helpers/drag.ts:16-21`. `boundingBox()` chờ phần tử visible nhưng **không** cuộn nó vào
tầm nhìn; toạ độ trả về có thể nằm ngoài viewport và `page.mouse.move` khi đó rơi vào phần tử khác.
Hôm nay rủi ro thấp vì việc mới luôn nằm đầu cột, nhưng `docs/frontend-guidelines.md` vừa quảng cáo
chạy `make e2e` lặp lại trên stack `make dev`, nơi cột `Cần làm` dài dần và board không có khung cuộn
riêng (`task-column.tsx:88-91` dùng `flex-1`, `task-board-page.tsx` không giới hạn chiều cao → cuộn
theo window).

Sửa, trong `pointIn`:

```ts
await locator.scrollIntoViewIfNeeded();
const box = await locator.boundingBox();
```

### M4 — Assertion cột đích quá lỏng so với Success Criteria

`apps/web/e2e/tasks-board-dnd.spec.ts:46`: `await expect.poll(() => topCardTitles(done, 50)).toContain(first)`.
Phase yêu cầu `B = [1]`. `internal/cli/seed.go` **không** tạo task nào, nên trên stack `teka-e2e` cột
`Hoàn thành` rỗng trước khi spec chạy và có thể assert chính xác. Ở dạng hiện tại, assertion vẫn xanh
kể cả khi card rơi sai vị trí trong cột đích.

Sửa: `await expect.poll(() => topCardTitles(done, 1)).toEqual([first]);` (giữ `toContain` như một lời
nhắc "stack dùng lại" là không cần thiết — comment trong `helpers/board.ts:11-16` đã nói điều đó).

### M5 — `tasks-board.spec.ts` vẫn giữ bản sao `login`/`column` riêng

`apps/web/e2e/tasks-board.spec.ts:3-17` trùng gần như từng dòng với `helpers/auth.ts:8-14` và
`helpers/board.ts:3-5`. `docs/frontend-guidelines.md` (diff, dòng "Shared setup lives in `e2e/helpers/`")
nay mô tả một quy ước mà spec anh em ngay cạnh không theo. Các spec khác (`audit`, `attendance`, `auth`…)
trùng lặp từ trước và nằm ngoài phạm vi phase, nhưng bốn spec `tasks-board*` thì nên thống nhất.

Sửa: trong `tasks-board.spec.ts`, import `loginAsOwner` và `column` từ `./helpers/`, bỏ hai bản sao.

## Low

### L6 — `card()` dựng `RegExp` từ tiêu đề chưa escape

`helpers/board.ts:7-9`. Tiêu đề hiện tại không có ký tự đặc biệt nên an toàn, nhưng `name` của
`getByRole` vốn đã là so khớp **substring**, không phân biệt hoa thường — dùng chuỗi trực tiếp
(`scope.getByRole("option", { name: title })`) vừa ngắn hơn vừa bỏ hẳn cái bẫy escape.

### L7 — `topCardTitles` gắn chặt với hình dạng DOM của card

`helpers/board.ts:17-22` lấy `querySelector("p")`. Đúng vì `TaskCardBody` (`task-card.tsx:52`) đặt tiêu
đề ở `<p>` đầu tiên, nhưng ngay dưới nó còn một `<p>` preview mô tả (`task-card.tsx:54-56`): chỉ cần đảo
thứ tự hai phần tử là toàn bộ 3 spec đọc nhầm và im lặng so sánh preview. Ít nhất nên ghi rõ ràng buộc
"tiêu đề là `<p>` đầu tiên" trong comment của helper.

### L8 — Assertion phủ định của quick swipe đọc ngay lập tức

`tasks-board-mobile.spec.ts:40`: `expect(await topCardTitles(todo, CARD_COUNT)).toEqual(titles)` chạy
ngay sau `expect.poll(scrollOffset)`. Nếu drag lỡ kích hoạt, optimistic update của reducer có thể chưa
kịp render tại thời điểm đọc. Poll ngược (`expect.poll(...).toEqual(titles)` với timeout ngắn) không
mạnh hơn về logic, nhưng chờ một nhịp bằng `page.waitForTimeout(100)` trước khi đọc sẽ đóng khe cửa này.

### L9 — Ngưỡng `CARD_COUNT = 10` để tràn viewport là sát

`tasks-board-mobile.spec.ts:8`. Pixel 7 cao 915px; 10 card cộng tab strip vừa đủ tràn. Nếu chiều cao card
giảm, `expect.poll(scrollOffset).toBeGreaterThan(before)` sẽ đỏ với thông báo khó đọc. Thêm một assert
tiền đề (`document.documentElement.scrollHeight > window.innerHeight`) sẽ chỉ thẳng nguyên nhân.

### L10 — `E2E_ARGS` là điểm mở rộng không được ghi ở đâu

`Makefile:106`. Không xuất hiện trong `make help`, README hay docs. Hoặc ghi một dòng trong README, hoặc
bỏ.

### L11 — Vị trí đoạn "mô tả HTML subset" trong `docs/architecture.md`

`docs/architecture.md:29-45`: đoạn về trust boundary của mô tả nằm trong bullet `apps/api/pkg/kanban`
thuộc mục "Hard-boundary libraries", trong khi chủ sở hữu thật là `internal/features/tasks`. Câu chữ có
nói rõ "The `tasks` feature … owns the description's trust boundary" nên không sai, chỉ là bullet đang
gánh hai chủ đề. Phase gợi ý "mục Tasks" nhưng file không có mục đó — nếu muốn đúng ý phase thì thêm một
câu ở mục Applications thay vì nối dài bullet.

### L12 — `api-contract-v2.md` thiếu 413

`plans/260917-1515-task-dnd-rich-text/reports/api-contract-v2.md:86-90`. Body vượt giới hạn middleware
trả `PayloadTooLarge` (`internal/shared/validation/validation.go:78-80`), khác với 422 của `max=20000`.
Một dòng cho 413 sẽ giúp client phân biệt.

### L13 — `npm run e2e` đổi tên test (project prefix)

`playwright.config.ts:20-30` thêm `projects`, nên mọi báo cáo/lệnh lọc theo tên test giờ mang tiền tố
`[desktop]`/`[mobile]`. Không có CI nào trong repo phụ thuộc vào đó (đã kiểm), chỉ ghi nhận.

## Đối chiếu Success Criteria (trừ Prod)

| Tiêu chí | Kết quả |
|---|---|
| AC12 — 3 spec mới xanh, spec cũ xanh, mobile chỉ chạy ở project mobile | Đạt về cấu hình; `testIgnore`/`testMatch` đúng, `workers: 1` giữ nguyên. Assertion cột đích còn lỏng (M4) |
| Docs — 3 file docs + 2 README, link kiểm tra được | Đạt. `Position strategy` có thật (`apps/api/pkg/kanban/README.md:126`), README lib web đã mô tả dnd là opt-in, link `eslint.config.js` sống |
| `reports/api-contract-v2.md` khớp swagger | Đạt (chi tiết bên dưới) |
| Bằng chứng gate đầy đủ trong `plan.md` | **Chưa đạt** — `plan.md` không có mục "Test evidence"; `phase-06` vẫn `status: pending` |

### Chi tiết đã fact-check cho hợp đồng v2

Mọi khẳng định trong `api-contract-v2.md` đều khớp source:

- 422 `after_task_id` → `apperror.Invalid` = `http.StatusUnprocessableEntity`
  (`apperror.go:57-58`), message `"phải là việc đang nằm trong cột đích"` (`errors.go:74-75`).
- 400 cho UUID hỏng: lỗi unmarshal không phải `validator.ValidationErrors` → `BAD_REQUEST`
  (`validation.go:66-74`).
- `max=20000` → `validator.ValidationErrors` → 422 kèm `fields.description`; swagger có
  `maxLength: 20000` (`swagger.yaml:1921`, `2004`).
- 4000 rune sau khi bỏ thẻ (`description.go:15,88-90`), thông điệp `"tối đa 4000 ký tự"` (`errors.go:76-77`).
- Web 2000 ký tự chữ + 20000 HTML (`task-schemas.ts:118-119`, `task-description-editor.tsx:18`).
- `rel="nofollow noreferrer"`, link đầy đủ thêm `noopener` + `target="_blank"`
  (`description.go:38-41`; đối xứng ở `rich-text.ts:25-27`).
- Vị trí: đầu cột `min − 1`, 0 nếu rỗng (`service.go:455-461`); sau anchor = trung điểm, `+1` nếu không
  có kế tiếp (`service.go:475-493`); renormalize `0..n−1` khi gap < `1e-6` (`service.go:410`,
  `task_repository.go:129-147`); serialize bằng `pg_advisory_xact_lock` theo tenant+column
  (`task_repository.go:101-105`) — mô tả "khoá cột" trong tài liệu là chính xác.
- Body v1 `{column_id}` vẫn hợp lệ: `AfterTaskID` là `Optional[uuid.UUID]`, vắng = nil = đầu cột
  (`dto.go:176-178`, `service.go:298-308`).

### Ngữ nghĩa e2e so với source

- `[1,3,2]`: `resolveDrop` với `overType: "task"` cùng cột lấy index của `over` **trước** khi loại task
  đang kéo (`positions.ts:128-131`) → kéo card 3 lên card 2 cho index 1 → `afterTaskId = first`. Khớp
  `tasks-board-dnd.spec.ts:31-32`.
- `role="option"` sau khi thả: card lấy role từ prop getter của lib, dnd-kit `attributes` cố tình không
  spread (`task-card.tsx:99-104`) → assert ở dòng 50 có giá trị thật.
- `data-dragging` chỉ tồn tại khi đang kéo (`task-card.tsx:159`) → assert phủ định ở dòng 51 không rỗng nghĩa.
- Long-press vs quick swipe: `TouchSensor` `delay 250 / tolerance 5` (`use-board-dnd.ts:102`); helper di
  chuyển ngay khi `holdMs: 0` nên vượt tolerance và huỷ timer → đúng là "cuộn, không kéo". Không mở dialog
  sau khi thả được bảo vệ bởi `CLICK_SUPPRESSION_MS = 300` (`task-card.tsx:36`), và spec assert đúng điều đó.
- Tên accessible dùng trong spec rich text đều tồn tại: `Đậm`, `Danh sách chấm`, `Liên kết`, `Địa chỉ liên kết`,
  `Áp dụng` (`task-description-editor.tsx:184,208,220,234,252`), dialog `Tạo công việc` / `Chi tiết công việc`
  (`task-form-modal.tsx:198`), textbox `Mô tả` qua `aria-labelledby` (`task-description-editor.tsx:51`).
  Nút `Bỏ liên kết` chỉ render khi con trỏ đang ở trong link nên không gây strict-mode với `Liên kết`.

## Kiểm tra bắt buộc còn lại

- (c) `playwright.config.ts`: `desktop` có `testIgnore: /-mobile\.spec\.ts$/`, `mobile` có
  `testMatch` + `devices["Pixel 7"]`, `workers: 1` và `fullyParallel: false` giữ nguyên. Đạt.
- (f) `tsconfig.node.json` include `"e2e"`, `module: nodenext` → import `.js` là bắt buộc và các spec
  đều dùng đúng; `tsc -b`, `eslint`, `prettier --check` đều sạch (đã chạy).
- (g) `git status`/`git diff` không đụng file app nào; không đổi hợp đồng public. Đạt.

## Recommended Actions

1. Sửa H1 (bọc `up` vào khối `status`) — chặn orphan container/port.
2. Sửa M4 (assert `topCardTitles(done, 1)).toEqual([first])`) và M3 (`scrollIntoViewIfNeeded`).
3. Sửa M2 (ghim POSTGRES_* trong `E2E_COMPOSE`) và M5 (cho `tasks-board.spec.ts` dùng helpers).
4. Thêm mục "Test evidence" vào `plan.md` và đóng `phase-06` sau khi `make test-api` xong (việc của lead).
5. Các mục Low: xử lý tuỳ khẩu vị, L7 và L12 đáng làm nhất.

## Unresolved Questions

- `make test-api` đang chạy nền lúc review; chưa có kết quả để đối chiếu với AC "bằng chứng gate đầy đủ".
- Không rõ lead có muốn đồng bộ helper cho cả các spec cũ (`audit`, `attendance`, `auth`…) hay giữ phạm vi
  ở bốn spec `tasks-board*` (khuyến nghị: giữ phạm vi).
