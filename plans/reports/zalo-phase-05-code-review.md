# Phase 5 code review — frontend profile Zalo connect

Reviewed: `apps/web/src/features/profile/{schemas/zalo-schemas.ts, api/zalo-api.ts,
hooks/use-zalo.ts, components/zalo-connect-card.tsx, components/zalo-link-modal.tsx,
pages/profile-page.tsx, __tests__/*}` and `src/test/msw/handlers.ts`, against
`plans/260806-2112-zalo-personal-auth/phase-05-frontend-profile-zalo-connect.md`.

## Verdict

The happy path is correct and the client-side security posture is genuinely good.
The gap is the unhappy path: a poll that never succeeds is invisible to the
teacher, unbounded in time, and unrecoverable without closing the modal.

Verification re-run and confirmed: 143 tests / 30 files green, eslint 0 errors,
`tsc -b --noEmit` exit 0, build succeeds. One correction — eslint reports **4**
pre-existing react-hook-form warnings, not 1: `profile-page.tsx:50`,
`roster/components/class-dialog.tsx:112`, `roster/components/student-dialog.tsx:158`,
`roster/pages/class-settings-page.tsx:118`. None originate in this slice.

## Direct answers

### 1. Is any credential material, link_id, or qr_png persisted durably? **NO**

`qr_png` flows response → in-memory query cache → a `data:` URI in JSX only
(`zalo-link-modal.tsx:78`, `:162`, `:189`). `use-zalo.ts:42` sets `gcTime: 0`, so
the attempt's cache entry is dropped the instant the modal unmounts — it does not
survive across mounts. `link_id` lives only in component state
(`zalo-link-modal.tsx:35`) and the query key (`use-zalo.ts:12`); neither component
touches the router, so it never reaches the URL. A grep of `features/profile`
returns no `console.*`, no `localStorage`, no `sessionStorage`; the only storage
users in the app are the theme provider (`components/shared/theme-provider.tsx:19,42`)
and the auth store, which is explicitly memory-only
(`features/auth/stores/auth-store.ts:8`). No query persister is configured.
Server-side the client is never offered credentials at all — `LinkSnapshot` is
documented as carrying none (`apps/api/internal/features/zalo/link_manager.go:74-76`).

One intended exception worth naming: `zalo-link-modal.tsx:188-194` writes the QR
to the device gallery via `<a download>`. That QR is a login challenge, and
saving it is criterion 5's explicit requirement; it expires in 105s.

### 2. Can polling run away? **NO for the three cases you named; YES for a fourth**

- **After a terminal state — no.** `use-zalo.ts:38-39` returns `false` for
  `linked|expired|error`. Proven, not just claimed, at
  `zalo-link-modal.test.tsx:83-88`, which freezes the poll count across 2.5s.
- **After modal close — no.** `zalo-connect-card.tsx:102` mounts the modal only
  while open, so closing destroys the query observer, which clears the interval;
  `gcTime: 0` then removes the entry.
- **After unmount — no.** Same mechanism. The countdown interval is likewise
  cleared by the effect's cleanup (`zalo-link-modal.tsx:66`).
- **Yes, when no poll ever succeeds.** `use-zalo.ts:38-39` keys off
  `query.state.data?.state`, which stays `undefined` while every request errors,
  so the interval stays at 1500ms indefinitely for as long as the modal is open.
  Bounded by the modal, so not a leak past unmount, but it is an unbounded 1.5s
  request loop with zero UI indication. See H1.

### 3. Are all eight Success Criteria met by the code? **SIX YES, TWO PARTIAL**

| # | Criterion | Verdict |
|---|---|---|
| 1 | Card shows linked / not-linked / expired | Partial — states render (`zalo-connect-card.tsx:53-99`), but a *failed* status query renders as "not linked" (H2) |
| 2 | Consent gates `Tiếp tục`, sends version, QR + countdown | Yes — `:87`, `:46`, `:78`, `:162`, `:56-67` |
| 3 | `scanned`/`confirmed` swap the QR, still polling | Yes — `:145-157`; both are non-terminal per `zalo-schemas.ts:51-53` |
| 4 | `linked` closes, toasts, flips card | Yes — `:69-74` → `zalo-connect-card.tsx:27-31` |
| 5 | `Lưu ảnh QR` under `md` | Yes — `:188-194`, with the iOS caveat in M6 |
| 6 | `Ngắt kết nối` confirms then returns to not-linked | Yes — `zalo-connect-card.tsx:104-128` |
| 7 | Keyboard + SR labelled; polling stops on close/terminal | Partial — labelling is good, but transitions are unannounced (M5), and the one test asserting stop-on-close was deleted in the test rewrite |
| 8 | Tests green, lint/typecheck clean | Yes |

Criterion 7's structural half is solid: `HvModal` supplies a Radix focus trap,
Esc/overlay close and a linked description (`components/hv/hv-modal.tsx:121-140`);
the checkbox is labelled (`:117-129`), the QR has alt text (`:164`), the countdown
is a `role="timer"` with a polite region that changes only every 10s (`:177-183`),
and reduced motion is respected (`:173`).

### 4. Any regression? **NO**

`profile-page.tsx` changes are one import (`:13`) and one JSX swap (`:135`); the
bank card, data card, message-footer preview and save flow are untouched, and
their four tests still pass. `src/test/msw/handlers.ts:420` only *adds* a default
`GET /me/zalo`, so no existing suite changes behavior. No exported signature
changed. No new lint, type, or build errors.

## Findings by severity

### HIGH

**H1 — A failing poll gives a permanent spinner and a permanent request loop.**
`zalo-link-modal.tsx:41` destructures only `data`; `failed` at `:76` considers only
`startFailed`, `expired`, `error`. A query error is therefore invisible by
construction. With no poll ever succeeding, `renderBody` falls through to the
"Đang tạo mã QR…" spinner (`:202-207`) and `renderFooter` returns `null` (`:100`) —
no message, no retry, only the X or Esc — while `use-zalo.ts:38-39` keeps firing
every 1.5s forever.

This is reachable in production. `LinkManager` holds exactly one active attempt
per teacher and returns `ErrLinkNotFound` → 404 whenever the presented id is not
the current one (`link_manager.go:189-198`). Opening the modal in a second tab
therefore guarantees the first tab 404s on every poll, permanently. An API restart
mid-attempt does the same, since the attempt map is in-memory.

*Fix:* read `isError`/`failureCount` from the query, fold into `failed`, and
return `false` from `refetchInterval` after N consecutive failures.

**H2 — A failed status query renders as "not connected."**
`zalo-connect-card.tsx:18` — on error `isPending` is false and `status` is
undefined, so `linked` is false and `:80` shows "Đăng nhập với Zalo" / "Chưa kết
nối" to a teacher who *is* linked. During an API blip they will start a redundant
link. *Fix:* branch on `isError` with a Vietnamese "không tải được trạng thái"
plus retry.

**H3 — The teacher's error screen is in English.**
`zalo-link-modal.tsx:137` prefers the server's `error_message`, which is the Go
constant `linkFailureMessage = "could not complete the Zalo login"`
(`link_manager.go:54`). That violates the Vietnamese-copy rule on the one screen a
teacher sees when things break — and `zalo-link-modal.test.tsx:104-111` pins that
English sentence as expected UI text, cementing it. *Fix:* always render the
Vietnamese fallback; keep `error_message` for diagnostics only.

### MEDIUM

**M1 — Closing at the moment of linking leaves the card stale.**
`zalo-connect-card.tsx:102` invalidates status only inside `handleLinked`. If the
account links server-side while the teacher is closing the modal, the card reads
"Chưa kết nối" until `staleTime` (30s, `app/providers.tsx:12`) plus a refetch
trigger. *Fix:* invalidate on close too, whatever the outcome.

**M2 — A failed unlink says nothing.** `zalo-connect-card.tsx:33-40` passes only
`onSuccess`; a 500 or 409 leaves the confirm dialog sitting there silently.
*Fix:* add `onError` with a danger toast.

**M3 — Retry recovery depends on the server issuing a fresh `link_id`.**
`zalo-link-modal.tsx:45-54` calls `setLinkId(started.link_id)`; if that equals the
current id the update is a no-op, the cached terminal `expired` result persists,
`refetchInterval` stays `false`, and "Tạo mã mới" visibly does nothing. The test at
`zalo-link-modal.test.tsx:91-101` uses a mock that returns the same id every time
and passes anyway, because it asserts only `calls.start === 2` — it proves the POST
fires, not that the flow recovers. *Fix:* `removeQueries` the old key (or clear
`linkId` first) and assert the QR returns.

**M4 — The countdown has no local expiry fallback.** `:56-67` clamps at 0 and waits
for the server to say `expired`. Combined with H1, a teacher whose polls 404 stares
at a dead QR at 0s indefinitely. *Fix:* treat `secondsLeft === 0` as expired
locally and show the retry footer.

**M5 — State transitions are not announced.** The waiting view (`:145-157`) and the
error view (`:134-142`) swap in with no live region; the only one present is
`Spinner`'s `<output aria-label="Loading">` (`components/shared/spinner.tsx:7`),
whose label is also English. *Fix:* wrap the body status text in `aria-live="polite"`.

**M6 — `download` on a `data:` URI is unreliable on iOS Safari.** `:188-194` is
criterion 5's mobile escape hatch; Safari tends to open a preview instead of
saving. *Fix:* build a Blob and use `URL.createObjectURL`, revoking on unmount.

### LOW

- `ATTEMPT_TTL_SECONDS = 105` (`zalo-link-modal.tsx:13`) duplicates the server's
  `defaultAttemptTTL` (`link_manager.go:43`), which is configurable via
  `opts.AttemptTTL` — if ops tunes it, the countdown lies. Returning `expires_at`
  from the API would remove the duplication.
- `onLinked(displayName)` is declared (`zalo-link-modal.tsx:19`) but the card
  discards it (`zalo-connect-card.tsx:27`). Use it in the toast or drop it.
- `useZaloLinkStatus`'s `enabled` argument is always `true` at its only call site,
  since the modal is mounted only while open — dead plumbing.
- `refetchIntervalInBackground` is left `false`, so polling pauses while the
  teacher is in the Zalo app on mobile and resumes via `refetchOnWindowFocus`.
  Works, but "still polling" holds only for a focused tab.
- The rewrite of `zalo-polling-errors.test.tsx` deleted the only test that asserted
  polling stops on modal close. The behavior is correct in code, but criterion 7's
  second half is now untested.
- `pollTimeout = { timeout: 6000 }` exceeds vitest's default 5000ms `testTimeout`
  (no override in `vitest.config.ts`), so those waits can never use their stated
  budget. "reports the link and stops polling" measured 4134ms on an idle machine —
  83% of the 5s budget, and it will exceed it on a loaded runner. Set
  `testTimeout: 15000` for these files or drive the polls with fake timers.
- Phantom assertion: `zalo-link-modal.test.tsx:112`, `queryByText(/zalo_personal:/)`,
  can never fail — the mock never returns that prefix. The real guarantee is
  already enforced at `apps/api/internal/features/zalo/handler_test.go:312`.

## Recommended order

H1 → H2 → H3 (and update the test pinning the English string) → M1-M4 → M5/M6 →
low items.

## Unresolved questions

1. Should `error_message` ever reach the teacher, or should the client own all
   failure copy? That decides H3's shape.
2. Is `AttemptTTL` tuned per environment, or effectively fixed at 105s? That
   decides whether the countdown needs an API-supplied expiry.
3. `zalo-polling-errors.test.tsx` is not in the phase file's Related Code Files
   list and was rewritten twice during review — is it in scope for this phase?

---

## Disposition

**Fixed.** H1 (runaway polling with an invisible error) — `useZaloLinkStatus`
now stops on a consecutive-failure budget, `ZALO_MAX_POLL_ERRORS = 3`, read off
`query.state.errorUpdateCount`; the modal declares the attempt lost at the same
threshold, so the UI never says "failed" while the hook is still retrying. A
single glitchy poll is still ridden out. H2 — the card no longer draws an
unreadable status as "not connected"; it says so and offers a retry, because
inviting an already-linked teacher to link again is the worse failure. H3 — all
failure copy is client-owned Vietnamese. The server sends one fixed English
sentence for every failure and deliberately withholds the upstream detail, so
`error_message` carries nothing a teacher could act on and nothing is lost by
not rendering it. M1 — closing the modal invalidates the status query. M2 — a
failed unlink raises a danger toast instead of failing silently. M3 — an
explicit `phase` state replaces the derived one, so a retry cannot flash the
consent screen. M4 — the countdown reaching zero is treated as expiry locally,
so a dead QR does not stay on screen when polls cannot land. M5 — the failure
paragraph and the scanned heading are `aria-live="polite"`.

**Partially mitigated.** M6 — the QR save link keeps `download` but gains
`target="_blank" rel="noopener"`. iOS Safari ignores `download` on a `data:`
URI, and without the new tab the image would replace the page and take the
running attempt with it. A Blob URL would be the complete fix; it needs
lifecycle management for the object URL across retries, which is more machinery
than the remaining gap justifies.

**Not changed.** `ATTEMPT_TTL_SECONDS = 105` duplicates the server's
configurable `defaultAttemptTTL`. Removing the duplication means returning the
deadline in the `link/start` response — an API contract change outside this
phase. The countdown is informational and the local expiry fallback above means
a drifted constant degrades gracefully rather than stranding the teacher.

**Test-suite corrections found while applying these.** The mock's scripted step
was derived from the cumulative poll count, so a retry replayed the terminal
state instead of rewinding; it now keeps its own cursor, reset on each
`link/start`. Polling tests declared a 6000ms `waitFor` budget under vitest's
5000ms default, which no test could ever spend; they now pass an explicit
per-test timeout. A phantom `/zalo_personal:/` assertion was removed, and the
stop-on-close test was restored using `unmount()`.

Verification after the fixes: 30 files / 146 tests pass, eslint reports 0 errors
(4 pre-existing react-hook-form warnings, none in this feature),
`tsc -b --noEmit` exits 0, prettier is clean, and `npm run build` succeeds.
