---
phase: 6
title: "Web UI — Onboarding Surfaces"
status: done
priority: P1
effort: "8h"
dependencies: [3, 4]
---

# Phase 6: Web UI — Onboarding Surfaces

## Overview

Delete register + join UI; add owner invite management to the center page;
add three public pages: invite accept, forgot-password, reset-password.
Update MSW fixtures, vitest suites, and Playwright e2e for the invite-only
world.

## Key Insights

- Conventions: feature folders own `api/ schemas/ hooks/ pages/ components/
  __tests__/`; cross-feature imports only via `index.ts`; routes via
  `routes.tsx` consumed by `app/router.tsx`; server state in TanStack Query;
  forms react-hook-form + zod; API errors as normalized
  `ApiError{code,message,status,fields}`.
- Web feature folders are **singular** (`features/center`, `features/auth`,
  `features/statement`, …) — the new slice is `features/invitation`, not
  `invitations`, and the existing center feature is `features/center`.
- Public pages (accept/forgot/reset) belong to the `auth` feature's route
  file except accept, which is invitation-domain → put invite management +
  accept page in a new `features/invitation` web slice mirroring the API
  slice; forgot/reset go in `features/auth`.
- Join/leave surfaces to delete are concrete and few:
  `features/center/components/join-center-form.tsx`,
  the `useJoinCenter` hook + `joinCenter` api fn + `joinCenterInput/Response`
  schemas (`hooks/use-center.ts`, `api/center-api.ts`, `schemas/center-schemas.ts`),
  and the self-leave path in `pages/center-page.tsx` (`leaveOpen` state, leave
  button, `<RemoveMemberDialog mode="leave"/>`). `remove-member-dialog.tsx`
  loses its `mode` prop and `useLeaveCenter` branch — it only ever "removes"
  (disables) now, so update its copy to "Vô hiệu hoá đăng nhập".
- E2E `auth.spec.ts` uses `/auth/register` with generated phones for
  isolation — must be rewritten around seeded users + invite flow.
- List pages must cover loading/empty/error/data quartet (frontend
  guidelines).

## Requirements

- Functional:
  - Remove `register-page.tsx`, its route, `register()` API fn, schemas,
    tests; login page drops the register link, gains "Quên mật khẩu?" link.
  - Remove join form/flow from the center page (and its API fn + tests).
  - Center page (owner view): invite section — phone input → create → dialog
    with copy-link button + DM status badge (sent/skipped/failed); pending
    invites list with expiry + revoke action; roster remove action gets
    confirm dialog copy explaining login disable ("Vô hiệu hoá đăng nhập").
  - Center page (member view): center name only — no roster, no invite, no
    self-leave affordance (deleted).
  - `/invite/:token` (public): fetch preview (center name, masked phone) →
    name + password + confirm form → success → redirect `/login` with toast;
    invalid/expired/used → single generic error state page.
  - `/forgot-password` (public): phone form → always the same success note
    ("Nếu số điện thoại hợp lệ, liên kết đặt lại đã được gửi qua Zalo").
  - `/reset-password/:token` (public): password + confirm → success →
    redirect `/login`; failure → generic error with link to forgot page.
- Non-functional: public routes live under the auth layout (no session
  required); accessibility per frontend guidelines (labels, aria on
  icon-only copy button).

## Architecture

```
apps/web/src/features/invitation/
  api/invitation-api.ts      # createInvite, listInvites, revokeInvite,
                             # previewInvite (POST body), acceptInvite
  schemas/…  hooks/…         # useInvites (query), useCreateInvite (mutation)…
  components/invite-section.tsx, invite-list.tsx, copy-link-dialog.tsx
  pages/accept-invite-page.tsx
  routes.tsx  index.ts  __tests__/
apps/web/src/features/auth/
  api/auth-api.ts            # -register, +forgotPassword, +resetPassword
  pages/forgot-password-page.tsx, reset-password-page.tsx
```

Invite preview calls `POST /invitations/preview` with the token in the body
(the API moved it out of the URL path). Center page composes `<InviteSection/>`
only when `is_owner`. Only the `GET /centers/me` read model is role-shaped —
make `members` optional in that zod schema; the `PATCH /centers/me` (rename)
response is unchanged and still carries the full center payload, so keep the
rename mutation's schema as-is.

## Related Code Files

- Create: `apps/web/src/features/invitation/**` (slice above + tests)
- Create: `apps/web/src/features/auth/pages/forgot-password-page.tsx`,
  `reset-password-page.tsx` (+ schemas, api fns, tests)
- Delete: `apps/web/src/features/auth/pages/register-page.tsx` + register
  schema/api/tests; `features/center/components/join-center-form.tsx`;
  `joinCenter` fn (`api/center-api.ts`), `useJoinCenter` (`hooks/use-center.ts`),
  `joinCenterInput/Response` schemas (`schemas/center-schemas.ts`)
- Modify: `apps/web/src/features/auth/{api/auth-api.ts,routes.tsx,index.ts}`,
  login page; `apps/web/src/features/center/**` — `pages/center-page.tsx`
  (drop join section + self-leave), `components/remove-member-dialog.tsx`
  (drop `mode` prop + `useLeaveCenter`, disable-copy), `hooks/use-center.ts`
  (drop `useLeaveCenter`), `schemas/center-schemas.ts` (optional `members`);
  `apps/web/src/app/router.tsx`
- Modify: `apps/web/src/test/msw/handlers.ts` (drop register/join, add
  invitation + forgot/reset fixtures)
- Modify: `apps/web/e2e/auth.spec.ts` (+ possibly `e2e/*center*` specs)

## Implementation Steps (TDD)

### Tests Before
1. Vitest per new page/section (MSW): invite create happy + validation +
   copy-link render + DM badge variants; invite list quartet
   (loading/empty/error/data) + revoke; accept page: preview OK, submit
   success redirect, generic failure state; forgot page: always-success
   copy; reset page: success redirect + generic failure; center page member
   view hides roster/invite; login page has forgot link, no register link.
2. Update MSW handlers to the new endpoint contract first (tests reference
   them).

### Refactor
3. Delete register/join surfaces; build invitations slice + auth pages;
   rewire router + center page.

### Tests After
4. Ensure `make seed` yields a **stable owner + member pair** (fixed phones)
   the specs can log in as — extend the seeder if it doesn't (allowed: `make
   seed` must keep working per brainstorm). This is a required step, not a
   contingency.
5. E2E rewrite: login-based specs use those seeded users; new spec: owner
   invites phone → copies link (intercept response) → logged-out accept →
   login as new teacher; forgot-password spec asserts generic response page
   only (Zalo DM not e2e-testable). Because accept mutates real DB state, give
   each accept-spec run a unique invitee phone and clean it up (or reset seed)
   so reruns stay isolated.

### Regression Gate
```sh
make lint-web && make test-web && make build-web
make e2e   # against seeded dev stack
```

## Todo

- [ ] Register + join UI fully deleted (grep register|join in web src)
- [ ] `features/invitation` slice + 3 public pages with quartet coverage
- [ ] Role-shaped `GET /centers/me` consumption; rename (PATCH) untouched
- [ ] `make seed` produces a stable owner+member pair for specs
- [ ] MSW + e2e updated (isolated invitee phones), all green

## Success Criteria

- [ ] Brainstorm acceptance: web register page gone; invite/accept/forgot/
      reset UI covered by web tests; full suites green

## Risk Assessment

- E2E depends on seeded dev users — the seeder must produce a stable
  owner+member pair (fixed phones). Extending the seeder is in scope here, not
  a fallback; `make seed` must keep working per brainstorm. Accept-flow specs
  mutate real state — isolate with unique invitee phones + cleanup.
- Copy-to-clipboard in jsdom/Playwright needs shims/permissions — use
  existing test setup patterns; assert fallback text field when clipboard
  API unavailable.

## Security Considerations

Access token stays memory-only; public pages never render token values back
except inside the accept/reset URL already held by the visitor; no
enumeration hints in any error copy.

## Next Steps

Phase 7 closes docs + full verification.
