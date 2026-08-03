# Phase 5 Code Review — Frontend Foundation

Reviewer: code-reviewer · Date: 2026-08-03 · Base: `cc2213b` (uncommitted working tree)

## Scope

- Modified: `.gitignore`, `Makefile`, `docs/frontend-guidelines.md`, phase-05 plan file (status only)
- Deleted: `apps/web/.gitkeep`
- Added: `apps/web/` scaffold — 37 source files under `src/` (~1802 LOC), plus
  `package.json`, `package-lock.json`, `vite.config.ts`, 3 tsconfigs, `eslint.config.js`,
  `.prettierrc`, `.prettierignore`, `.gitignore`, `components.json`, `index.html`,
  `.env.development`, `README.md`, `public/`
- Focus: interceptors/refresh, error normalization, routing guards, env/config contract,
  cross-surface regression against committed Phases 1–4
- No `apps/api` changes; backend surfaces untouched.

### Checks performed (read-only; build/lint/E2E results taken as verified, not re-run)

| Check | Result |
|---|---|
| API envelope vs `errors.ts` normalization | matches `response.Envelope` exactly (`success`/`error{code,message,fields}`) |
| `/healthz` route location vs `apiOrigin` override | matches — `healthz` at root, `/api/v1` is a Gin group |
| `.gitignore` negation blast radius (`git check-ignore`) | narrow — `.env.production`, `.env.local`, `apps/api/.env` still ignored |
| Vite env loading per mode (`vite.loadEnv`) | `development` → `VITE_API_URL` present; **`production` → `{}`** (see H1) |
| Cross-feature imports | none; only `layouts/*` and `lib/api/interceptors.ts` reach into `features/auth` |
| `dangerouslySetInnerHTML` / `innerHTML` / `eval` / `document.cookie` | none |
| Storage of credentials | none; `localStorage` used only for the theme key |

## Overall Assessment

The foundation is coherent and largely matches the phase contract: the error envelope mapping is
provably correct against the Go source, the auth-redirect race is handled in both directions, the
token stays in memory, and the module layout is clean with no feature-to-feature coupling. The
defects that matter are at the seams the diff does not show: a production bundle that builds green
and dies blank in the browser, a `lib → feature` dependency that sets up a circular import for
Phase 6, and refresh behavior that is single-flight only inside a narrow window.

All four success criteria in the phase file are met per the recorded verification. The checkboxes
in `plans/260803-1552-fullstack-project-provisioning/phase-05-frontend-foundation.md` (lines 76–79)
are still unchecked and `status: in-progress`; leaving that to the plan owner.

## Critical Issues

None. No secrets in tracked files, no XSS sink, no authorization surface exists yet to get wrong.

## High Priority

### H1 — `make build-web` produces a bundle that throws at boot (no `VITE_API_URL` in production mode)

`apps/web/.env.development` is the only env file. Verified with Vite's own loader:

```
prod: {}
dev: {"VITE_API_URL":"http://localhost:8080/api/v1"}
```

`src/lib/config/env.ts:10-17` then fails `safeParse` and throws during module evaluation. Because
`env.ts` is pulled in transitively from `main.tsx`, the throw happens before React mounts: the user
gets a blank page and a console message, not the "readable error at boot" the phase intends
(`phase-05...md:43`). `npm run build` / `make build-web` exit 0, so nothing catches this — the exact
"green CI, broken artifact" shape.

Fix options (cheap, pick one):
- add `apps/web/.env.example` documenting the required key, and require `VITE_API_URL` to be present
  in the build environment (Vite reads `process.env.VITE_*` too), or
- fail fast in `vite.config.ts` with a `loadEnv` assertion in the `build` command, or
- render a minimal DOM fallback in `main.tsx` when config validation throws.

Deployment lands in Phase 7, but the broken-artifact target is wired now and the failure mode is a
silent white screen.

## Medium Priority

### M1 — `lib/api/interceptors.ts:4` imports the auth feature store (layer inversion, future import cycle)

`docs/frontend-guidelines.md` places `lib/` beneath features ("api client, config/env, utils"), yet
`interceptors.ts` imports `@/features/auth/stores/auth-store`. Today `auth-store.ts` imports only
`zustand`, so there is no cycle. In Phase 6 the auth feature will import `apiClient` for
login/register/me; the moment anything in that store's module graph reaches
`lib/api/client.ts` → `interceptors.ts` → `auth-store.ts`, ESM resolves one side to a partially
initialized module and `useAuthStore` can be `undefined` at interceptor setup. Invert now while it
is a 15-line change: keep the token accessor in `lib/api/` (e.g. a `setTokenAccessor`/`getToken`
pair) and have the store register itself, so `lib/` never names a feature.

### M2 — Refresh is single-flight only inside the request window; late 401s restart it, including after the session is dead (`interceptors.ts:23-42, 56-67`)

`refreshInFlight` is cleared in `.finally()`. A request that was already in flight with the stale
token and 401s *after* the refresh settled sees `refreshInFlight === null` and issues another
`POST /auth/refresh`, even though a fresh token is already in the store. Worse, after a failed
refresh (`clearSession()` at line 35), every subsequent 401 repeats the whole cycle — N dead refresh
attempts against a revoked cookie, which is precisely the traffic reuse-detection backends alarm on.
Two guards close both holes:

- before refreshing, compare the token that was sent (`config.headers.Authorization`) with
  `useAuthStore.getState().accessToken`; if it already changed, just retry with the current token;
- keep a module-level "session ended" flag (or check `accessToken === null`) and skip refreshing
  entirely once a refresh has failed, resetting it on `setSession`.

### M3 — `toApiError` throws on a `null` JSON body (`lib/api/errors.ts:47`)

`err.response.data.error` dereferences `data` unguarded. Axios leaves `data` as `""` for an empty
body (safe), but a literal `null` JSON payload — plausible from a proxy, gateway, or a future
handler — makes this a `TypeError` raised *inside* the response interceptor's rejection handler. The
result is a non-`ApiError` rejection escaping the layer whose whole contract is "components never
touch raw errors" (`errors.ts:8-9`). Use `err.response.data?.error` and type `data` as
`ErrorEnvelope | null | undefined`.

### M4 — Dev server host (`127.0.0.1`) does not match the API CORS allowlist (`localhost:5173`)

`vite.config.ts:19` pins `host: "127.0.0.1"`, so Vite prints `http://127.0.0.1:5173/`. The committed
Phase 1–4 contract allows only `http://localhost:5173` (`.env.example:22`, `.env:22`) and
`middleware/cors.go:17-23` runs with `AllowCredentials: true`, so there is no wildcard fallback: a
developer who opens the URL Vite printed gets every API call blocked, including the refresh cookie.
The E2E run passed because it used `localhost`. Either add `http://127.0.0.1:5173` to
`API_CORS_ORIGINS` in `.env.example` (and `.env`), or set `host: "localhost"` / `strictPort` with
the loopback rationale reworked. Not a Phase 5 regression to committed code, but a cross-phase
inconsistency that will cost someone an afternoon.

### M5 — `make setup` no longer bootstraps a working `make lint-web` / `make web-dev`

`scripts/setup.sh:8` installs root dependencies only. The Makefile targets that previously printed
the friendly `not_yet` notice now run `npm run …` inside `apps/web`, which fails with a raw
`vite: not found` / `eslint: not found` on a fresh clone. Add an `apps/web` install step to
`setup.sh` (mirroring the existing `npm ci` / `npm install` branch) so the Phase 1 "one command
bootstraps the repo" contract still holds.

### M6 — `apps/web/README.md` is the unmodified Vite template and describes a toolchain this repo does not use

The file talks about Oxlint, `.oxlintrc.json`, `oxlint-tsgolint`, and `@vitejs/plugin-react-swc`
(lines 3, 14-30) while the project standardizes on ESLint flat config + Prettier. Committing it
ships documentation that actively contradicts `eslint.config.js` and
`docs/frontend-guidelines.md`. Replace with a few lines (scripts, env, where the guidelines live) or
delete it.

### M7 — `VITE_API_URL` now has two sources of truth

Root `.env.example:26-29` still presents `VITE_API_URL` as "for running the dev server directly on
the host", but Vite reads `apps/web/.env.development` — it never loads the repo-root file. Editing
the documented value has no effect on the dev server. Keys are unchanged, so no contract break, but
the comment is now wrong. Either point the root comment at `apps/web/.env.development` or drop the
key from the root example once Phase 7 settles how Compose injects it.

## Low Priority

- **L1 — Router does not compose feature route arrays.** `src/app/router.tsx:3-11` imports page
  components directly; the phase contract says "features export routes, `app/` owns the tree"
  (`phase-05...md:24`). Harmless at 5 routes, but the accepted architecture drifted silently and
  `docs/frontend-guidelines.md` was written to match the code rather than the plan. Decide which one
  wins before Phase 6 adds routes.
- **L2 — Theme does not follow live system changes.** `theme-provider.tsx:20` reads `matchMedia`
  during render with no `change` listener, so `theme: "system"` is resolved once per render pass and
  never updates while the app is open. Acceptable for this phase (and noted as such in the review
  brief); the fix is a five-line `useSyncExternalStore` or effect-registered listener. Also
  `resolvedTheme` is exported on the context but nothing consumes it (`sonner.tsx:13` uses `theme`).
- **L3 — Unguarded `localStorage` in `theme-provider.tsx:17,33`.** Throws `SecurityError` in
  restricted contexts (blocked storage, some embedded webviews). `ThemeProvider` sits *above*
  `ErrorBoundary` (which lives in `RootLayout`, inside `RouterProvider`), so the failure is a white
  page. Wrap in try/catch.
- **L4 — `ErrorBoundary` cannot actually recover.** `error-boundary.tsx:38` clears state and
  re-renders the identical subtree, which usually re-throws; there is no reset on location change
  and no `errorElement` on the root route, so router-level errors fall through to React Router's
  default screen instead of the branded card. Consider resetting on `useLocation().key` (via a
  wrapper) and adding a route-level `errorElement`.
- **L5 — `ProtectedRoute` drops `location.hash`** (`protected-route.tsx:15` builds `from` from
  `pathname + search` only), so a deep link with a fragment loses it across the login round-trip.
- **L6 — `/auth/` exclusion is string-fragile** (`interceptors.ts:58`). `config.url?.startsWith("/auth/")`
  misses a call written as `auth/refresh` or as an absolute URL. The `_retried: true` marker passed to
  the refresh POST (`interceptors.ts:27`) is dead weight given the URL check — keep one mechanism,
  preferably the flag, which cannot be defeated by URL formatting.
- **L7 — Cancellation and timeouts both normalize to `NETWORK_ERROR: Cannot reach the server`**
  (`errors.ts:55`). No query passes an `AbortSignal` today, so this is latent; once Phase 6 wires
  `signal` into `queryFn`, every unmount will look like an outage. Branch on `axios.isCancel(err)`
  and on `err.code === "ECONNABORTED"` for a truthful timeout message.
- **L8 — `setAccessToken` can leave `user: null` while `useIsAuthenticated()` is `true`**
  (`auth-store.ts:27,32`). The dashboard already tolerates it (`dashboard-layout.tsx:47`), but the
  Phase 6 refresh-on-boot path needs a `/me` fetch or authentication should key off `user` too.
- **L9 — Generated-but-unused surface.** `components/ui/{dialog,field,table}.tsx` have zero
  importers, and `react-hook-form` / `@hookform/resolvers` are installed but unused. The plan asked
  for these primitives, so this is sanctioned pre-provisioning, not scope creep — worth confirming
  the plan's `form` component is genuinely superseded by radix-nova's `field`.
- **L10 — `eslint.config.js` lints nothing but `.ts`/`.tsx`** (`eslint.config.js:12`), so the config
  file itself and any future `.js` are silently unconfigured; `eslint .` reports them as ignored
  rather than checking them.
- **L11 — `make fmt` still prints `not_yet`** (`Makefile:100-102`) although `npm run format` exists.
  Outside the plan's Makefile scope; noted so it is a decision, not an oversight.
- **L12 — `docs/frontend-guidelines.md` claims `eslint-plugin-jsx-a11y` runs "in CI"**, but
  `.github/workflows/` contains only `.gitkeep`. Lefthook covers pre-commit today; reword or wait
  for the CI phase.

## Regression / Contract Checks

- **Makefile (Phases 1–4 surfaces):** only `web-dev`, `lint-web`, `build-web` changed; every API,
  migration, and test target is byte-identical. `lint`/`build` aggregate targets now depend on web
  deps being installed — see M5.
- **`.gitignore` exception:** `!apps/web/.env.development` is exact-path and re-includes nothing
  else; `apps/web/.env.production`, `apps/web/.env.local`, and `apps/api/.env` remain ignored
  (verified with `git check-ignore -v`). The parent directory is not excluded, so the negation is
  valid.
- **Lefthook:** globs `apps/web/**/*.{ts,tsx,js,jsx,css,json,html}` resolve against the new tree;
  both web commands guard on `node_modules/.bin/*` and no-op on a fresh clone, so hooks degrade
  gracefully rather than blocking commits.
- **`.env.example`:** unchanged (no diff). `VITE_API_URL` semantics (base includes `/api/v1`,
  `healthz` at origin) are consistent across `env.ts:22`, `health-card.tsx:22-24`,
  `docs/frontend-guidelines.md`, and the Go router (`server/router.go:50`, `server/health.go:19`).
- **Error envelope:** `errors.ts:32-56` maps `response.ErrorBody` field-for-field including
  `fields`; nothing to drift on.
- **Security:** access token in Zustand memory only, `withCredentials` for the refresh cookie, no
  token in `localStorage`/`sessionStorage`, no `dangerouslySetInnerHTML`, no `document.cookie`
  access, no secrets in `.env.development` (localhost URL only, and the file documents why `VITE_*`
  is public).

## Recommended Actions

1. H1 — make the production env contract explicit (`.env.example` for web + build-time assertion, or
   a visible boot-failure fallback) so `make build-web` cannot ship a blank page.
2. M1 — invert the `lib/api → features/auth` dependency before Phase 6 introduces the cycle.
3. M2 — add the token-changed check and a session-ended guard to the refresh path.
4. M3 — one-character optional chain in `toApiError`.
5. M4/M5 — align the dev origin with `API_CORS_ORIGINS` and install web deps in `scripts/setup.sh`.
6. M6/M7 — replace the template README; fix the root `.env.example` comment.
7. Low items are safe to batch into Phase 6 alongside the real auth flows.

## Metrics

- Type coverage: `strict` + `noUncheckedIndexedAccess` + `noUnusedLocals/Parameters` on; no `any`,
  no `@ts-expect-error`, no `eslint-disable` anywhere in `src/`. Two narrowing casts
  (`interceptors.ts:27,57`) and one `as React.CSSProperties` in generated `sonner.tsx:32`.
- Test coverage: 0% — no frontend tests exist; `make test-web` is still `not_yet` by design
  (testing strategy lands with the frontend features phase).
- Linting issues: 0 reported (`npm run lint`, `format:check`, `tsc -b --noEmit`, `npm run build` all
  clean per the phase verification).

## Unresolved Questions

1. Who owns `VITE_API_URL` for non-dev builds — Compose/Dockerfile args in Phase 7, or a committed
   `apps/web/.env.production`? H1's fix depends on the answer.
2. Should the router consume feature-exported route arrays (plan) or import pages directly (current
   code)? One of the two documents needs to change.
3. Is `127.0.0.1` binding a deliberate constraint, or can the dev server go back to `localhost` to
   match the existing CORS allowlist?

## Resolution (2026-08-03, same session)

All High/Medium findings resolved; verification re-run afterwards: `npm run lint`,
`format:check`, `tsc -b --noEmit`, `npm run build` all clean, full headless E2E suite
9/9 PASS (redirect, stub login round-trip, health card ok, theme toggle + persistence,
404, zero console errors, NETWORK_ERROR after API kill), plus a new headless check that
a `VITE_API_URL`-less production build renders the readable startup error.

- **H1 — fixed (fallback) + deferred (value ownership).** `main.tsx` now imports the app
  modules dynamically inside try/catch: a config failure renders "Teka failed to start"
  with the zod message instead of a blank page (verified against `vite preview` of a
  production build with no env file). Who supplies the real production `VITE_API_URL`
  stays a Phase 7 decision, now documented in `docs/frontend-guidelines.md`,
  `apps/web/README.md`, and the root `.env.example`. Side benefit: the dynamic entry
  split the bundle; the >500 kB chunk warning is gone (largest chunk now 186 kB).
- **M1 — fixed.** New `lib/api/auth-bridge.ts` (`connectAuthBridge` + accessor); the
  interceptors call the bridge, and `features/auth/stores/auth-store.ts` registers
  itself at module load. `lib/` no longer imports feature code (import direction is now
  feature → lib only).
- **M2 — fixed.** Before refreshing, the interceptor compares the Authorization header
  the failed request carried with the current store token and retries directly when a
  refresh already settled; a `refreshDead` gate in the bridge stops further
  `/auth/refresh` attempts after a failed refresh, re-opened by `setSession` on the next
  login.
- **M3 — fixed.** `toApiError` extracts the envelope through a type guard
  (`extractErrorBody`) that tolerates `null`, non-object, and HTML bodies.
- **M4 — fixed.** `vite.config.ts` host pin reverted to the default (`localhost`, the
  allowlisted origin Vite prints); `.env.example` `API_CORS_ORIGINS` additionally
  allowlists `http://127.0.0.1:5173` for hand-typed URLs. IPv4 reachability re-verified
  (`curl http://localhost:5173` and the E2E suite both pass on the default binding).
- **M5 — fixed.** `scripts/setup.sh` installs `apps/web` dependencies with the same
  `npm ci`/`npm install` branch as the root.
- **M6 — fixed.** `apps/web/README.md` rewritten for the actual stack (scripts table,
  env contract, pointer to `docs/frontend-guidelines.md`).
- **M7 — fixed.** Root `.env.example` web comment now states Vite never reads that file
  and points at `apps/web/.env.development` / build-time injection.
- **L1 — fixed** (it was a plan-contract deviation, not a style choice): features now
  export route arrays (`features/{auth,dashboard,users}/routes.tsx`) and
  `app/router.tsx` composes them.
- **L3 — fixed.** `theme-provider.tsx` wraps both `localStorage` accesses in try/catch,
  falling back to `system`.
- **L5 — fixed.** `ProtectedRoute` includes `location.hash` in `from`.
- **L2, L4, L6, L7, L8, L10, L11, L12 — deferred to Phase 6/8** per the reviewer's own
  batching recommendation; none affects a Phase 5 success criterion. L9 confirmed as
  sanctioned pre-provisioning (plan lists dialog/field/table and react-hook-form for
  Phase 6).

Unresolved question 1 is answered "Phase 7 owns the value; the app now fails readably
until then"; question 2 resolved in the plan's favor (route arrays); question 3 resolved
by reverting to `localhost` + widening the example allowlist.
