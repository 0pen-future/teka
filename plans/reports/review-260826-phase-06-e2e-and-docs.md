# Review — audit log phase 06 (e2e + docs)

Date: 2026-08-26
Scope: `apps/web/e2e/audit.spec.ts` (new), `docs/event-bus.md` (new),
`docs/architecture.md`, `docs/deployment.md`.
Method: every doc claim checked against the named source file; e2e selectors
checked against the web components and the seeder; `eslint e2e/audit.spec.ts`
run clean. Heavy suites not re-run (reported green this session).

## Verdict

No blocking defect. Two Medium items (one weak assertion in the new spec, one
real UI/doc mismatch inherited from the web phase) and a handful of Low
wording/robustness nits.

## Claim verification (acceptance criterion 3)

| Doc claim | Source | Result |
|---|---|---|
| Bus API `Publish` / `Subscribe(name, buf, h)` / `Close(ctx)` | `apps/api/internal/shared/events/events.go:21-32` | matches |
| `events.NewSync()` for in-line delivery in tests | `apps/api/internal/shared/events/sync_bus.go:21` | exists |
| Full queue drops with a warning; no retry | `async_bus.go:93-100` | matches |
| Panic isolation loses only its own event | `async_bus.go:64-77` | matches |
| `Close` stops intake and drains within ctx | `async_bus.go:108-130` | matches |
| Guarantee ends at the queue boundary; batcher flushes itself | `events.go:26-31`, `audit/subscriber.go:143-155` | matches |
| Catalog `http.request_completed` | `middleware/request_events.go:41` | matches |
| Catalog `auth.login_succeeded` / `login_failed` / `logged_out` | `auth/events.go:41,54,66` | matches |
| Catalog `invitations.member_joined` | `invitations/events.go:34` | matches |
| Middleware skips login/logout/refresh | `request_events.go:47-51` | matches |
| Middleware mounted on the v1 group after the global stack | `server/router.go:76-80` | matches |
| Action-map fallback `"METHOD route"` | `audit/subscriber.go:246-255`, `audit/action.go:120-125` | matches |
| Defaults 1024 / 100 / 1s / 5s | `config/config.go:158-168` | matches |
| `stop_grace_period: 30s` in both compose files | `docker-compose.yml:54`, `docker-compose.prod.yml:37` | matches |
| Worst case 10s + 5s + 5s | `server/server.go:14`, `config.go:168`, `subscriber.go:28`, `app/app.go:23,36` | matches, see L3 |
| Blind spot: 401 leaves no row except forgot/reset-password | `request_events.go:100-103`, `request_events.go:58-61` | matches |
| Blind spot: logout with a stale revoked token publishes nothing | `auth/service.go:280-291` | behavior matches, wording off (L4) |
| Blind spot: login is rate-unlimited, per-IP limiting is backlog | `auth/routes.go:12-17` vs `routes.go:24-28` (only reset flow limited) | matches |
| Pipeline queue name `"audit"` + buffer size | `app/container.go:83-84` | matches |
| Shutdown order bus → subscriber → DB | `app/container.go:133-145` | matches |

Criterion 4: no plan/phase identifiers in the spec or docs; only the dev-seeded
credentials appear (`seeds/seed.go:46-47`); page sizes 95 / 53 / 320 lines.

Criterion 5: `e2e` is inside `tsconfig.node.json` `include`, so `tsc -b`
typechecks the spec; `eslint.config.js` matches `**/*.{ts,tsx}`; `npx eslint
e2e/audit.spec.ts` reported nothing.

## Medium

**M1 — `audit.spec.ts:49-59`: the assertions do not tie the row to this run.**
The poll only looks for *some* row containing `contact.create`, then asserts
actor `Cô Lan` and status `201` on it. `roster.spec.ts:29-34` creates a contact
the same way, as the same owner, with the same 201 — so any leftover row
satisfies all three assertions. On a fresh seeded stack `audit.spec.ts` sorts
before `roster.spec.ts` and the run is sound, but a re-run against a non-reseeded
database (or a future spec that creates a contact earlier in the alphabet) turns
this into a test that passes with the capture pipeline switched off.
Fix cheaply: after creating the contact, open it (`await page.getByText(contactName).click()`)
to read its id from the URL, then assert the expanded detail row contains that id
— `audit-table.tsx:114-119` already renders `Đối tượng: contact (<id>)`.

**M2 — `apps/web/src/features/audit/components/audit-filters.tsx:16-35` does not
match the action map, contradicting `docs/event-bus.md:66-69`.** The comment
claims "action prefixes actually recorded by the API's action map", but:
- `collection.` (line 27) exists in no map entry (`audit/action.go`; the string
  only appears in `audit/read_api_integration_test.go:192` as seeded fixture
  data) — that filter option can only ever return an empty list;
- `billing.` (5 entries: `action.go:89-93`) and `teacher.` (`action.go:41`) have
  no group, so those actions are unreachable from the dropdown (free-text only).

The doc under review asserts the linkage, so either the filter list or the doc
sentence has to change. Fixing the list is the smaller change.

## Low

**L1 — `audit.spec.ts:54`: the 20s poll budget does not fit the 30s test
timeout.** `playwright.config.ts:13` sets `timeout: 30_000` for the whole test,
and login plus the contact mutation already consume a chunk of it. A genuine
capture failure will surface as an opaque `Test timeout of 30000ms exceeded`
rather than the `toPass` error. Either drop the poll to ~10s (10x the 1s flush
interval) or add `test.setTimeout(60_000)` to this spec.

**L2 — `audit.spec.ts:59`: `row.getByText("201")` matches anywhere in the row.**
It works today, but the row also renders a formatted timestamp, IP, and entity
type; any of them growing a `201` substring turns this into a strict-mode
violation, not a clean failure. Scope it to the status cell instead.

**L3 — `docs/event-bus.md:92-95` and `docs/deployment.md:114-118`: the ~20s
budget omits two steps.** `app/container.go:134-135` runs
`Notifications.Close()` (which waits for an in-flight sending run to observe its
cancel, `notifications/run_manager.go:230-245`) and `Zalo.Close()` *before* the
bus drain. The 30s grace still has headroom, but "~20s worst case" is really a
lower bound; wording it as "at least ~20s (HTTP drain, background senders, bus
drain, final flush)" would be accurate.

**L4 — `docs/event-bus.md:79-80` misstates the logout mechanism.** The doc says
the per-token check "fails before the service reaches the publish". In
`auth/service.go:280-291` the family *is* revoked; only the publish is skipped
because `alreadyRevoked` is true. A second case is undocumented: an unknown
token returns early at `service.go:271-273`, also with no event.

**L5 — `docs/event-bus.md:81-83`: `auth.login_failed` names the event, not the
row.** The stored action is `auth.login_fail` (`audit/subscriber.go:192`), which
is also what an owner types into the action filter. Also "(clipped)" applies to
user agent and path (`subscriber.go:126-127`); the phone is *masked*
(`auth/events.go:25-30`), not clipped.

**L6 — `docs/event-bus.md:41` "one event per mutating request, success or
failure alike" has a second exception.** Besides the 401 case already listed,
`request_events.go:96-99` also drops requests whose `FullPath()` is empty, i.e.
mutating calls to unregistered paths (404). Worth one clause in the blind spots
list, since "the trail shows every mutation attempt" is exactly the assumption
an owner would make.

**L7 — `audit.spec.ts:71-72` comment claims more than the test proves**
("without the page firing a doomed request"). The two assertions only check the
URL and the absent heading. Either assert it (`page.on("request", …)` or
`page.route` on `**/audit-logs*`) or soften the comment — the guard itself is
real (`audit-page.tsx:22-29`), it is just not what this test verifies.

## Notes for risk calibration

- The spec adds no production code, so the phase's blast radius is docs plus
  test signal — which is why M1 (signal quality) is the highest-value fix here.
- `docs/event-bus.md` correctly links field lists instead of copying them, so
  the usual drift vector (duplicated struct fields) is absent.
- `architecture.md:44-46` and the request-lifecycle diagram match the actual
  wiring (`app/app.go:23,36`, `server/router.go:76-80`).

## Unresolved questions

1. Is the e2e suite guaranteed to run against a freshly seeded database in CI?
   If not, M1 is not hypothetical.
2. Was `collection.` in the filter list intentional (a planned action name) or a
   copy from the read-API test fixture?

## Fixes applied after review (260826, session)

- **M1 fixed** — spec giờ tạo contact rồi **đổi tên** nó (`PUT /contacts/:id`,
  action `contact.update`); assertion mở chi tiết row đầu tiên và đối chiếu
  `PUT /api/v1/contacts/<id>` với id lấy từ URL của run hiện tại. Đã chứng
  minh: chạy 2 lần liên tiếp trên cùng DB (lần 2 không reseed) đều pass —
  row cũ không thể thỏa assertion vì id khác. Trả lời Unresolved Q1: CI
  (`web-ci.yml:150`) seed fresh mỗi run (runner + compose mới), M1 là rủi ro
  local-rerun và đã được chống.
- **M2 fixed** — `collection.` là copy nhầm từ fixture của read-API test
  (Unresolved Q2); đã thay bằng `billing.` ("Hóa đơn") và thêm `teacher.`
  ("Hồ sơ giáo viên") — ACTION_GROUPS giờ khớp 1-1 với 18 prefix thực trong
  `features/audit/action.go`.
- **L1 fixed** — poll budget 20s → 10s, nằm gọn trong test timeout 30s.
- **L2 fixed** — assertion badge dùng `{ exact: true }` cho "200"/"Cô Lan".
- **L3 fixed** — docs đổi "~20s worst case" thành "at least ~20s, other
  subsystems closing in between" (cả event-bus.md lẫn deployment.md).
- **L4 fixed** — blind spot logout viết lại đúng cơ chế: unknown token return
  sớm; token của family đã revoke vẫn revoke (idempotent) nhưng skip publish
  (đối chiếu `auth/service.go` Logout).
- **L5 fixed** — row action ghi đúng `auth.login_fail` + "phone is stored
  masked" (đối chiếu `subscriber.go:192,195`).
- **L6 fixed** — thêm ghi chú unmatched-route (404) bị skip vào blind spots.
- **L7 fixed** — bỏ claim "không bắn request" khỏi comment e2e (đã có unit
  test 0-request cover).

Verification sau fix: vitest audit+layouts 29/29 pass; eslint spec+filters
sạch; typecheck sạch; audit.spec.ts 2/2 pass trên stack isolated fresh-seed
**và** 2/2 pass lần hai trên DB tái sử dụng; stack `down -v`, port trả sạch.
