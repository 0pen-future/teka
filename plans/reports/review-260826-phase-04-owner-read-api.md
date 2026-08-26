# Code Review — Phase 4 Owner Read API (audit-logs)

Reviewer: code-reviewer subagent, 260826. Verified change set end to end, including a live Postgres experiment (throwaway container) checking the real query plan rather than the one the test asserted.

## Scope

- Files: `apps/api/internal/features/audit/{service.go,service_test.go,repository.go,dto.go,handler.go,routes.go,read_api_integration_test.go}`, `apps/api/internal/server/router.go`, regenerated `apps/api/docs/*`
- LOC: ~750 new
- Extra evidence: gorm@v1.31.2 `clause/where.go` source; `EXPLAIN (ANALYZE)` of the actual repository SQL against a seeded Postgres 16 (200k → 700k rows)

## Overall Assessment

Structurally clean and idiomatic for this repo (handler/service/repository split, `h.scope` helper matching 9 other features, envelope usage, apperror codes). Trust-boundary logic is sound: could not construct a cross-center read through cursor tampering, the `actor_id` filter, or the LIKE prefix. Two things do not hold up: the index/plan acceptance claim is proven by a test that explains a different query, and the membership-based visibility leg exposes a teacher's login history across their previous employer.

## High

**H1 — Acceptance criterion (f) not proven; the production query does a full scan + top-N sort on every page.**
`TestListQueryPlanUsesIndex` EXPLAINed a hand-written query that was not the one `Repository.List` built: it dropped the `OR` visibility leg, the `LEFT JOIN teachers`, `COALESCE`, and set `enable_seqscan = off`. Measured plan for the real query shape: `Limit -> Sort (top-N heapsort) -> Hash Left Join (teachers) -> Bitmap Heap Scan on audit_logs (BitmapOr over center_id = $1 / center_id IS NULL)`. The index found rows but never ordered them. Consequences, all measured: a Sort node always present; O(entire visible trail) per page, cursor did not reduce it; 10k visible rows → 27 ms/page, 500k → 130 ms/page; the `center_id IS NULL` bitmap leg scanned every center's auth rows globally — cross-tenant performance coupling. Suggested fix: split the two visibility legs into keyset-limited subqueries (UNION ALL + LATERAL), join `teachers` on the surviving rows only. Measured: leg 1 becomes `Index Scan using idx_audit_logs_center_time`, rows=51, 0.09 ms (vs 27 ms), no Sort; leg 2 becomes `Index Scan using idx_audit_logs_actor` per member.

**H2 — Auth rows leak a teacher's login history from their previous center (PII, cross-tenant).**
The membership leg matched center-NULL rows on current membership (`left_at IS NULL`) with no time bound. Teacher T works at center A for a year (hundreds of login rows with IP, device), leaves for center B → owner B immediately sees T's entire historical login trail from A; owner A silently loses those rows (append-only trail behaving as if evidence were deleted). `center_members.joined_at`/`left_at` already exist, so bounding the leg to the membership window is cheap. Design-adjacent: the accepted decision said "current membership" — decision needed from the user.

## Medium

**M1 — Tenant isolation depended on a GORM string heuristic.** The raw `A OR (B)` condition was parenthesized by gorm only because the SQL string contains `" OR "` (verified in gorm@v1.31.2 `clause/where.go:41-86`); a harmless reformat would silently drop the parens and break filter/keyset application. `TestListFilters` would catch the regression (hence Medium). Fix: explicit parentheses, stop depending on gorm internals for a tenancy predicate.

**M2 — `metadata` on the wire but never asserted end to end.** The only seeded row with metadata was the (deliberately invisible) failed login; the HTTP test decoded only `action`/`actor_name`. Add a visible row with metadata and assert the JSON round-trip.

## Low

- **L1** — `limit=0`/`-5` silently clamped to 50 while `limit=abc` is a 400, contradicting the handler's own stated principle; either reject `< 1` or document the clamp in swagger.
- **L2** — the 401 criterion proven against a stub, not `RequireAuth`; acceptable (middleware shared and covered elsewhere) but weaker than it reads.
- **L3** — cursor not bound to the filter set. Verified not a leak (keyset comparison only; center predicate rebuilt from scope); a foreign/tampered cursor yields a shifted or empty page of the caller's own rows. Worth one swagger line.
- **L4** — `from`/`to` both inclusive; inverted window returns 200 empty, not 400. Internally consistent; the web client should not present it as "no activity".
- **L5** — two pagination dialects now exist (`page`/`per_page` offset elsewhere, `limit`/`cursor` here). Follows the accepted design.
- **L6** — `ESCAPE '\'` assumes `standard_conforming_strings = on` (PG ≥ 9.1 default).

## Checks that passed (verified, not assumed)

- SQL injection: every user value a bind parameter; `likeEscaper` single-pass so `\` → `\\` is not re-escaped; `class_` cannot match `classX` (test-covered).
- `actor_id` + visibility: filter ANDed after the visibility clause; a foreign actor id returns rows only if that actor genuinely acted in the caller's center; `actor_name` projected only for already-visible rows.
- Cursor timestamp precision: no skipped rows — cursor originates from a DB-scanned timestamptz (already microsecond-rounded), `RFC3339Nano` lossless for microseconds, both sides pass through the same rounding.
- Keyset ordering assertion: UUID `String()` comparison valid (lowercase canonical hex ordering matches PG uuid byte ordering).
- `+1` probe: `next_cursor` non-empty only when a next page provably exists.
- Error propagation: repository errors map to a generic 500 message; no SQL text reaches the client.
- Docs: `/audit-logs` present in swagger with parameters and 400/401/403 matching behavior (gap: no `@Failure 500` annotation — repo-wide convention omits it).
- Router mount: two lines, correct middleware pair, no public-contract change to other features.
- No scope creep, no new abstractions, no lint suppressions, no swallowed errors.

## Recommended Actions

1. H1 — rewrite `Repository.List` as the UNION ALL / LATERAL keyset form, or at minimum fix the EXPLAIN test to explain the real SQL.
2. H2 — decide: accept cross-employer auth visibility, or bound the membership leg with `joined_at`/`left_at`.
3. M1 — explicit parentheses around the visibility predicate.
4. M2 — assert `metadata` round-trip for a visible row.
5. L1 — make `limit < 1` either a 400 or a documented clamp.

Status: DONE_WITH_CONCERNS
Summary: Trust-boundary logic holds up under adversarial reading (no injection, no cursor-based leak, no actor-filter leak), but acceptance criterion (f) was proven by a test that explained a different query than production — the real plan was a bitmap scan plus top-N sort costing O(entire trail) per page — and the membership visibility leg exposed a teacher's login history from their previous center.
Concerns/Blockers: H1 and H2 to be resolved before the phase is marked complete.

---

## Fixes applied after review (260826, session)

User decision (AskUserQuestion): H2 = bound auth-row visibility to the membership window `[joined_at, left_at)`.

- **H1 + H2 fixed together** — `repository.go` rewritten: `listSQL(spec)` builds a UNION ALL of two keyset-limited legs (center leg on `idx_audit_logs_center_time`; membership leg as `center_members CROSS JOIN LATERAL` per member on `idx_audit_logs_actor`, bounded by `occurred_at >= joined_at AND (left_at IS NULL OR occurred_at < left_at)`), then `LEFT JOIN teachers` over the surviving ≤ limit rows. Filters apply inside both legs before each LIMIT so a filtered page can never miss rows. Only constant fragments concatenated; all user values remain bind parameters.
- **Plan proof replaced** — old `TestListQueryPlanUsesIndex` deleted; new white-box `TestListQueryPlanUsesIndexes` (`list_plan_integration_test.go`, package `audit`) seeds 2400 rows, runs `ANALYZE`, and EXPLAINs the exact `listSQL` output with a cursor (page-2 shape), no planner overrides. Measured plan: `Merge Append` over two pre-sorted streams (no global Sort), center leg `Index Scan using idx_audit_logs_center_time` with the row-value keyset in Index Cond and LIMIT pushed down, membership leg `Index Scan using idx_audit_logs_actor` with the window bounds in Index Cond, zero `Seq Scan on audit_logs`. Asserted in-test.
- **H2 semantics tested** — `TestListVisibilityAndMembership` extended with pinned membership windows (`setMembership` helper): a pre-join auth row is invisible everywhere; after the member leaves center A and joins center B, the in-window login row stays in A's trail and never appears in B's.
- **M1** — moot: the OR clause no longer exists; each leg is its own parenthesized subquery.
- **M2 fixed** — visible seeded row now carries metadata; `TestReadEndpointHTTP` asserts the exact map round-trips to the wire.
- **L1 fixed** — swagger `limit` description now documents the clamp ("out-of-range values are clamped").
- **L3 fixed** — swagger description notes a cursor is only valid alongside the filters that produced it.
- **L2, L4, L5, L6** — accepted as documented; L4 noted for the Phase 5 web client.

Verification after fixes: `go build ./...`, `go vet` (incl. integration tags), `gofmt` clean, `golangci-lint run` and `--build-tags integration` → 0 issues, `go test ./... -race` all ok, `go test -tags integration ./internal/features/audit/ -race` all ok, `make api-docs` regenerated and `go build ./docs/` clean.
