# Phase 5 — Route Spec Manifest

Plan: `plans/260906-0627-authz-write-scope-root-cause/phase-05-route-spec-manifest.md`

## Design summary

`internal/shared/routespec` (new leaf package) holds one manifest, `Specs
[]Spec`, where each `Spec` carries a route's `Method`, `Path`, authorization
`Kind` (+ `Key` for permission routes), and `Audit` classification (`Source`,
`Action`, `EntityType`, `IDParam`). Three call sites that used to maintain
their own independent tables now derive from it instead:

- `internal/server.routePolicies` — was a 128-entry literal; now
  `derivePolicies()` maps `routespec.Policies()` (the auth-facing subset of
  `Specs`) into the existing `RoutePolicy` struct. `PolicyKind` and the
  `PolicyXxx` constants are now aliases (`type PolicyKind = routespec.Kind`,
  `const PolicyPublic = routespec.KindPublic`, …) so no caller or test outside
  this file changed.
- `internal/features/audit.LookupAction` — was a 69-entry local `actions` map;
  now calls `routespec.Lookup(method, route)` and returns `ok=false` when the
  spec has no `Audit.Action` (unregistered route, or a route whose trail comes
  from a domain event instead, e.g. login/logout/enrollment-create).
- `internal/middleware/request_events.go`'s three skip sets
  (`authSessionRoutes`, `serviceAuditedRoutes`, `anonymousAuditedRoutes`) —
  were three literal `map[string]bool`; now
  `routespec.WithSource(SourceAuthSession/SourceService/SourceAnonymous)`,
  each returning a path-keyed `map[string]struct{}` (lookup sites switched
  from truthy read to `_, ok :=`).

Adding, removing, or reclassifying a route is now a one-line change in
`routespec.Specs`; none of the three consumers holds authorization or audit
data of its own anymore.

## Reconciliation table

The plan's risk section predicted the three legacy tables would show
path-string drift (missing/extra routes, mismatched `Action` strings) once
compared side by side. Building the manifest from all three sources found
**zero mismatches** — every route present in one table was present, and
agreed, in the others:

| Legacy table | Entries | Routes also in `server.routePolicies` | Routes also in `audit.actions` (where applicable) | Drift found |
|---|---|---|---|---|
| `server.routePolicies` | 128 | — | n/a | none |
| `audit.actions` | 69 | 69/69 matched (all had a policy entry) | — | none |
| `request_events.go` skip sets | 3 auth_session + 1 service + 2 anonymous = 6 paths | 6/6 matched | 6/6 consistent with policy `Kind`/mutating status | none |

## Counts

Gathered via a temporary in-package test (`fmt.Println` over `routespec.Specs`
and `WithSource`), run once, then deleted — not part of the shipped diff:

```
total specs: 128
by source: map[anonymous:2 auth_session:3 none:54 request:67 service:2]
specs with non-empty Action: 69
authSession set size: 3
service set size: 1
anonymous set size: 2
```

- `len(Specs) == len(engine.Routes())`: **128 == 128**, proven by
  `TestRoutePolicyCoversEveryRegisteredRoute` (bidirectional coverage,
  unchanged, still green).
- `Source=request` Specs (67) vs old `audit.actions` entries (69): the gap is
  exactly the two anonymous password-reset routes
  (`/api/v1/auth/forgot-password`, `/api/v1/auth/reset-password`), which carry
  `Source: SourceAnonymous` with a non-empty `Action` — they were never
  audited via the request-identity path but still had an `audit.actions`
  entry in the old table. `67 (request) + 2 (anonymous with Action) = 69`,
  matching the old map's size exactly.
- Skip-set sizes, before vs after migration (unchanged in both forms):
  - `authSessionRoutes`: 3 paths (login, logout, refresh)
  - `serviceAuditedRoutes`: 1 unique path (`/api/v1/enrollments`) — the
    manifest has 2 `Source: SourceService` Specs (POST + GET on that path),
    but the skip set is path-keyed, so they collapse to 1 entry, exactly as
    the pre-migration map did.
  - `anonymousAuditedRoutes`: 2 paths (forgot-password, reset-password)

All three sizes and contents are pinned exactly by `TestSkipMapsUnchanged` in
`request_events_test.go`, green before and after.

## Fail-closed probes (RED output quoted, then reverted)

Each probe temporarily edited `routespec.Specs`, ran the one test expected to
catch it, captured the failure, then reverted the edit and re-ran the full
suite green. Final `routespec.go` is byte-identical to its pre-probe state
(confirmed via `git status --short` showing only the untracked new-file state,
no diff noise, plus a clean `gofmt`/`go build`/`go vet`/full test pass after
reverting).

**Probe 1 — unregistered route added to the manifest:**
appended `classified("GET", "/api/v1/definitely-not-a-registered-route",
KindPublic, none())` to `Specs`.

```
--- FAIL: TestRoutePolicyCoversEveryRegisteredRoute (0.00s)
    route_policy_test.go:35: manifest declares GET /api/v1/definitely-not-a-registered-route but the engine does not register it
FAIL
```

**Probe 2 — mutating route with `Source: none` outside the documented
allowlist:** changed `POST /api/v1/students`'s audit from
`req("student.create", "student", "")` to `none()`.

```
--- FAIL: TestMutatingRoutesHaveAnAuditSource (0.00s)
    routespec_test.go:106: POST /api/v1/students: mutating route has SourceNone and is not in the documented allowlist
FAIL
```

**Probe 3 — `Source: request` with an empty `Action`:** same route, changed
its audit to `Audit{Source: SourceRequest}` (no `Action`).

```
--- FAIL: TestRequestSourceRoutesHaveAnAction (0.00s)
    routespec_test.go:122: POST /api/v1/students: SourceRequest with empty Action
FAIL
```

**Probe 4 — two Specs at the same path disagree on audit source:** changed
`DELETE /api/v1/classes/:id/score-set`'s audit from
`req("class.score_set.clear", "class", "id")` to `Audit{Source:
SourceService}`, conflicting with `POST` at the same path.

```
--- FAIL: TestSamePathSameAuditSource (0.00s)
    routespec_test.go:142: DELETE /api/v1/classes/:id/score-set: audit source "service" conflicts with POST's "request" at the same path
FAIL
```

All four probes were reverted immediately after capturing the failure; the
full four-package suite (`server`, `audit`, `middleware`, `routespec`) was
green again after each revert.

## Verification command outputs

```
$ gofmt -l . | grep -v vendor
(no output)

$ go build ./...
(exit 0)

$ go vet ./...
(exit 0)

$ go list -deps ./internal/shared/routespec/... | grep -E 'teka/apps/api/internal/(features|middleware|server)'
(no output — routespec stays a leaf package)

$ go test -count=1 ./internal/server/... ./internal/features/audit/... ./internal/middleware/... ./internal/shared/routespec/...
ok  	teka/apps/api/internal/server	0.087s
ok  	teka/apps/api/internal/features/audit	0.051s
ok  	teka/apps/api/internal/middleware	0.012s
ok  	teka/apps/api/internal/shared/routespec	0.009s

$ go vet -tags integration ./internal/features/audit/...
(exit 0)
$ go test -tags integration -run '^$' ./internal/features/audit/...
ok  	teka/apps/api/internal/features/audit	(no tests to run) [compiles cleanly under integration tag, no DB/Docker touched]

$ make lint-api
0 issues.
```

**`make api-docs`**: ran per the phase's verification checklist. It produced a
diff in `apps/api/docs/{docs.go,swagger.json,swagger.yaml}` (3 files, 51
insertions, 4 deletions). Traced via read-only `git diff -- apps/api/docs` —
the diff is entirely the `/api/v1/billing-periods/:id/notifications/bulk`
(mark-sent) endpoint's Swagger description text and a new 404 response
definition. I touched zero swag-annotated handler files in this phase; this
is `swag init` picking up another agent's concurrent uncommitted handler
changes elsewhere in the tree, not anything from this phase's work. I did not
revert it — reverting `apps/api/docs` requires `git checkout`, which the task
explicitly forbids me from running. Flagging for team-lead: either accept the
docs diff as-is (harmless, unrelated, will get regenerated identically once
that other work lands) or have it reverted by someone permitted to run git
mutations.

## Proposed "Adding a route" docs paragraph

Not applied — `docs/api-guidelines.md` and `docs/adding-permissions.md` are
owned by another agent. For whoever updates them, here is the paragraph to
insert wherever the current text explains how a new endpoint's authorization
or audit trail is registered:

> **Adding a route's authorization and audit classification.** Every
> registered route needs exactly one entry in `internal/shared/routespec`
> (`Specs` in `internal/shared/routespec/routespec.go`): its authorization
> `Kind` (and `Key`, for permission-gated routes) plus its `Audit`
> classification (`Source`, and — for `SourceRequest` or `SourceAnonymous`
> mutations worth a trail row — `Action`, `EntityType`, `IDParam`). This one
> entry is the only place that decision lives; `internal/server`'s route
> policy, `internal/features/audit`'s action lookup, and
> `internal/middleware`'s request-audit skip sets all read it, so there is
> nothing else to update. The manifest's own tests fail closed: a route
> missing from `Specs` fails `TestRoutePolicyCoversEveryRegisteredRoute`, a
> mutating route with no audit source fails
> `TestMutatingRoutesHaveAnAuditSource` unless it's on the documented
> `SourceNone` allowlist, and a `SourceRequest` entry with no `Action` fails
> `TestRequestSourceRoutesHaveAnAction`.

## Files modified

- `internal/shared/routespec/` (new package: `routespec.go` + `routespec_test.go`, pre-existing from earlier in this session, unchanged this pass)
- `internal/server/route_policy.go` — rewritten to alias/derive from `routespec`
- `internal/server/route_policy_snapshot_test.go` (new) — 128-entry pinned snapshot
- `internal/features/audit/action.go` — rewritten to delegate to `routespec.Lookup`
- `internal/features/audit/action_test.go` — added 69-entry pinned snapshot + `TestActionSnapshotUnchanged`
- `internal/middleware/request_events.go` — three skip sets now `routespec.WithSource(...)`
- `internal/middleware/request_events_test.go` — added `TestSkipMapsUnchanged`
- `apps/api/CLAUDE.md` — one pointer line under Structure for `internal/shared/routespec/`

Side effect, not hand-edited: `apps/api/docs/{docs.go,swagger.json,swagger.yaml}` changed by `make api-docs`, traced to unrelated concurrent work (see above), left unreverted per the no-git-mutation constraint.

---

Status: DONE_WITH_CONCERNS
Summary: routespec manifest built and all three consumers (server policy, audit action lookup, middleware skip sets) migrated with byte-identical runtime behavior, proven by pinned snapshots, unchanged pre-existing tests, and four reverted fail-closed probes; reconciliation across the three legacy tables found zero drift, contradicting the phase's risk prediction.
Concerns: `make api-docs` (run per the verification checklist) modified `apps/api/docs/*` with a diff attributable to another agent's concurrent, unrelated handler-annotation work, not this phase. I left it unreverted because reverting requires `git checkout`, which I was told not to run. Needs a decision from team-lead: accept as harmless, or have someone with git-mutation permission revert it.
