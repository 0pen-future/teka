# Code Review — Phase 5: route spec manifest

Reviewed: uncommitted work on `master`, 2026-09-07.
Spec: `plans/260906-0627-authz-write-scope-root-cause/phase-05-route-spec-manifest.md`
Implementer report: `plans/reports/phase5-routespec-manifest-260907-0232-route-spec-manifest.md`

## Scope

| File | Status |
|---|---|
| `apps/api/internal/shared/routespec/routespec.go` | new, 393 lines |
| `apps/api/internal/shared/routespec/routespec_test.go` | new, 233 lines |
| `apps/api/internal/server/route_policy.go` | 237 lines removed, rewritten to 48 |
| `apps/api/internal/server/route_policy_snapshot_test.go` | new, 176 lines |
| `apps/api/internal/features/audit/action.go` | 150 lines removed, rewritten to 32 |
| `apps/api/internal/features/audit/action_test.go` | +117 |
| `apps/api/internal/middleware/request_events.go` | +27/-24 |
| `apps/api/internal/middleware/request_events_test.go` | +35 |
| `apps/api/CLAUDE.md` | +5 |

Explicitly out of scope and ignored: `internal/server/policy_integration_test.go` (phase-3 reports work),
`apps/api/docs/*`, and every `internal/features/*` diff owned by concurrent slices.

## Overall assessment

The migration is correct. I did not trust the snapshot tests; I extracted the
three legacy tables from `HEAD` and diffed them against the manifest myself.
Policy data, audit-action data and skip-set membership are all identical,
including source ordering of the policy slice. Enforcement (`enforceRoutePolicy`)
reads the derived slice at runtime, so authorization behaviour is unchanged.
No Critical or High findings. What is left is data hygiene, one factually wrong
justifying comment, one duplicated invariant, and two tests that restate their
implementation.

## Independent verification (not via the tests)

**Policy table.** Parsed `git show HEAD:apps/api/internal/server/route_policy.go`
and the manifest, resolved `authctx.Perm*` constants to their literal values:
128 entries on both sides, zero set difference, and identical source order.

**Audit actions.** Parsed `git show HEAD:apps/api/internal/features/audit/action.go`:
69 entries; the manifest yields exactly 69 Specs with a non-empty `Audit.Action`;
zero set difference and zero field mismatch on `Action`/`EntityType`/`IDParam`.
Source distribution across 128 Specs is `request 67`, `none 54`, `auth_session 3`,
`anonymous 2`, `service 2`, so `67 + 2 = 69` reconciles with the old map.

**Snapshots were generated from the OLD tables, not the new manifest.**
`routePolicySnapshot` equals `HEAD`'s `routePolicies` element-for-element in the
same order; `actionSnapshot` equals `HEAD`'s `actions` map with no additions,
drops or field changes (its shuffled order is consistent with having been dumped
from a Go map). Both are honest pins.

**Skip maps.** `WithSource` keys by path only. Derived membership is
`auth_session {login, logout, refresh}`, `service {/api/v1/enrollments}`,
`anonymous {forgot-password, reset-password}` — identical to `HEAD`'s three
literal maps. The lookup rewrite in `publishRequest` from truthy read to
`_, ok :=` preserves the exact boolean expression, including the `route == ""`
short-circuit.

**Public contract.** `PolicyKind` is a true type alias, so `server.PolicyPublic`
and friends still compare against anything typed `routespec.Kind`. `RoutePolicy`
and `audit.ActionSpec` shapes are untouched. `LookupAction` is behaviourally
identical: the old table's key set equals the set of Specs with a non-empty
Action, so `ok` is false for exactly the same inputs. The silent `"METHOD route"`
fallback still exists where it always lived, in `audit/subscriber.go:307`, and is
the only consumer of `ok == false`. Nothing else calls `LookupAction`.

**Leaf property.** `go list -deps ./internal/shared/routespec` returns only
`apperror`, `authctx`, `routespec`. No feature, middleware or server package.

## Commands run

```
gofmt -l apps/api/internal                      -> no output
go vet ./internal/shared/routespec/ ./internal/server/ \
       ./internal/features/audit/ ./internal/middleware/   -> exit 0
go test -count=1 (same four packages)           -> 4x ok
```

Integration tests and `make test-api` were not run, per instruction.

## Success criteria

| Criterion | Result |
|---|---|
| No literal routes in `route_policy.go` | met, file has no `perm(`/`classified(` |
| `audit.actions` literal removed | met |
| Two-way coverage test green | met, `TestRoutePolicyCoversEveryRegisteredRoute` unchanged and passing |
| Fail-closed: missing Spec | met, coverage test, both directions |
| Fail-closed: mutating without Source | met, `routespec_test.go:97` |
| Fail-closed: `request` without Action | met, `routespec_test.go:118` |
| Fail-closed: same path, different Source | partially met, see Medium 2 |
| `routespec` is a dependency leaf | met |
| 128 Specs = 128 registered routes | met, verified independently and by test |
| 69 request/anonymous-with-action entries | met, verified independently |

The one incomplete item from the phase's Related Code Files is the docs update
(`docs/api-guidelines.md` or `docs/adding-permissions.md`). The implementer
deferred it because another slice owns those files and left the paragraph in its
report. That is a reasonable call but the phase is not fully closed until the
lead merges it.

## Findings

### Critical
None.

### High
None.

### Medium

**M1. A read route is classified `SourceService`, justified by an incorrect claim.**
`internal/shared/routespec/routespec.go:259`

```go
perm("GET", "/api/v1/enrollments", authctx.PermEnrollmentsList, Audit{Source: SourceService}),
```

The comment at `routespec.go:255` justifies this with "the two skip sets are
derived per path, not per method". That reasoning does not hold. `WithSource`
unions paths across Specs, so `POST /api/v1/enrollments` alone already puts
`/api/v1/enrollments` in the service set; marking the `GET` changes nothing about
the derived set. `TestSamePathSameAuditSource` does not force it either, since it
skips non-mutating methods. So the entry is unnecessary, and it asserts something
false about a read: a `GET` publishes no domain event, which is what
`SourceService` means everywhere else in the manifest. Runtime impact today is
zero, but it is the one place in the manifest where `Source` does not mean what
its own doc comment says, and it will be copied by the next person adding a read
next to a service-audited write.

Fix: `perm("GET", "/api/v1/enrollments", authctx.PermEnrollmentsList, none())`,
and replace the trailing half of the comment with the real reason the path is
skipped, which is the `POST`.

**M2. The same-path invariant is method-scoped, so path-only derivation is not
actually guarded by it.**
`internal/shared/routespec/routespec_test.go:134`

`TestSamePathSameAuditSource` only compares mutating Specs. The failure mode that
path-only derivation actually creates is the opposite one: a *non-mutating* Spec
given a skip source silently drops its path into a skip set and disables auditing
for the mutating Spec sharing that path. Marking `GET /api/v1/students` as
`SourceService` would stop `POST /api/v1/students` from ever producing an audit
row, and this test would stay green.

The exact-contents pins (`TestKnownSkipSetsUnchanged`,
`TestSkipMapsUnchanged`) do catch it today, so this is not a live hole. But those
pins are edited by hand whenever a skip route is legitimately added, whereas an
invariant is not.

Note that the invariant cannot simply be widened to all methods: `POST
/api/v1/students` is `request` and `GET /api/v1/students` is `none`, so a total
"same path, same source" rule is unsatisfiable by construction.

Fix: state the invariant that path-only derivation really needs, and drop the
method filter:

```go
// No path may carry both a skip source and a Spec the request middleware is
// supposed to audit — the skip sets are keyed by path, so the skip would
// silently swallow the audited route's row.
func TestSkipSourcesDoNotShareAPathWithAnAuditedRoute(t *testing.T) {
    skip := map[string]string{}   // path -> method that declared the skip
    audited := map[string]string{} // path -> method audited via the request row
    for _, s := range Specs {
        switch s.Audit.Source {
        case SourceService, SourceAuthSession:
            skip[s.Path] = s.Method
        case SourceRequest, SourceAnonymous:
            audited[s.Path] = s.Method
        }
    }
    for path, m := range skip {
        if other, clash := audited[path]; clash {
            t.Errorf("%s %s is skipped by path, which also swallows %s %s's audit row",
                m, path, other, path)
        }
    }
}
```

This subsumes the current test for the cases that matter and does not depend on
the method filter. Applying M1 first keeps it green.

**M3. "Mutating method" is defined twice, in two packages, with no link.**
`internal/shared/routespec/routespec_test.go:10` and
`internal/middleware/request_events.go:93`

Both list `POST, PUT, PATCH, DELETE`. The manifest test's guarantee ("every
mutating route declared an audit source") is only meaningful if its definition of
mutating matches the middleware's. Today they agree by coincidence of copy-paste.
The whole point of this phase was to stop two packages holding the same fact.

Fix: export it from the leaf both packages already import.

```go
// IsMutating reports whether a method can change state, and therefore whether
// the request middleware considers the route for an audit row.
func IsMutating(method string) bool {
    switch method {
    case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
        return true
    default:
        return false
    }
}
```

Then `publishRequest` becomes `if !routespec.IsMutating(c.Request.Method) { return }`
and the test drops its local helper.

### Low

**L1. `Policies()` copies defensively while `Specs` is exported and mutable.**
`internal/shared/routespec/routespec.go:358`, with the pinning test at
`routespec_test.go:153`

`Policies()` returns a copy and `TestPoliciesReturnsACopy` asserts it, but any
caller can write `routespec.Specs[0].Key = "x"` directly. Worse, `specIndex`
(`routespec.go:365`) is a cache built once at init, so a mutation of `Specs`
would leave `Lookup` and `WithSource` disagreeing about the same route. The
defence guards the door that was never open.

`Specs` has to stay reachable because `audit/action_test.go:139` and
`routespec_test.go` iterate it. Cheapest coherent fix: keep the export, and say
so on the var — "frozen at init; `specIndex` caches it, so mutating `Specs` after
init desynchronises `Lookup` from `WithSource`". If you would rather close it,
unexport to `specs` and add `func All() []Spec` returning a copy; only tests
change.

**L2. The `Policies()` doc comment contradicts itself.**
`internal/shared/routespec/routespec.go:353-357`

It says callers "must not mutate the returned slice's backing array in place",
then admits in the same sentence that "mutation is actually safe per-element".
Both cannot be the operative rule, and the second is the true one, since `copy`
allocates a fresh array and `Spec` holds only strings. Cut the parenthetical and
keep one sentence: `Policies` returns a copy, so callers may not affect `Specs`.

**L3. Carried-over plan reference in a code comment.**
`internal/shared/routespec/routespec.go:251` still says "by the phase-1
irregular-mapping decision". The repo's review rules forbid plan IDs and phase
numbers in code comments. The sibling reference ("the phase-1 inventory frozen as
data", `HEAD`'s `route_policy.go:54`) was correctly dropped during the rewrite;
this one was not. State the invariant instead: the picker exists only to create,
so it rides the create key.

**L4. Two tests restate their own implementation.**
`routespec_test.go:185` (`TestWithSourceMatchesManifest`) rebuilds `WithSource`
line for line inside the test and compares. Any bug reproduced in both stays
invisible. `TestKnownSkipSetsUnchanged` at `routespec_test.go:208` is the test
that has real content, and `TestSkipMapsUnchanged` in the middleware package
covers the same six paths *plus* the wiring of the three vars, so it strictly
dominates. Consider deleting `TestWithSourceMatchesManifest` and, if you want the
manifest package to keep its own pin, keeping only `TestKnownSkipSetsUnchanged`.

**L5. Package doc overstates history.**
`routespec.go:3` says the three tables "used to drift independently". The
reconciliation found zero drift; they *could* drift. Accuracy matters in a doc
comment that is the package's rationale.

## Positive observations, for risk calibration

- The snapshot pins are honest. I verified byte-level that both were dumped from
  `HEAD`'s tables rather than regenerated from the new manifest, which is the
  failure mode that would have made this whole migration unverifiable.
- `TestActionSnapshotUnchanged` runs coverage in both directions: every snapshot
  entry must resolve, and every manifest Spec with an Action must be in the
  snapshot. A one-directional version would have missed additions.
- The manifest keeps the old file's grouping and per-group comments, so the
  diff-to-read for a reviewer stays small and the domain rationale survived
  the move.

## Recommended actions

1. Apply M1, then M2 in that order. They are one change: fix the data, then
   widen the invariant that the data was distorted to satisfy.
2. Apply M3, exporting `IsMutating` from `routespec`.
3. Apply L2 and L3 while touching the file. Both are one-line edits.
4. Decide L1 explicitly (comment or unexport) rather than leaving the asymmetry.
5. Merge the "Adding a route" docs paragraph from the implementer's report into
   the docs surface once the phase-3 slice releases those files.

## Metrics

| Measure | Value |
|---|---|
| Type coverage | full, no `any`, no assertions, no lint suppressions in the diff |
| Lint | `gofmt` clean, `go vet` clean on all four packages |
| Tests | 4 packages, all passing, `-count=1` |
| Manifest entries verified against `HEAD` | 128 policy, 69 audit, 6 skip paths |

**Score: 8.5 / 10.** Data migration is provably lossless and the fail-closed
tests are real. Held back by one misclassified route defended by an incorrect
comment, an invariant scoped too narrowly to guard the thing it exists to guard,
and a duplicated "mutating" definition in the very refactor whose purpose was
removing duplicated route facts.

## Unresolved questions

1. Is the `GET /api/v1/enrollments` `SourceService` classification deliberate for
   a reason not captured in the comment? I found none in the code or the phase
   file, and read it as an artifact of building the manifest per path.
2. Who merges the deferred docs paragraph once the phase-3 slice releases
   `docs/api-guidelines.md`?
