# Adding an API permission

Use this guide when a new authenticated endpoint needs configurable center
authorization. Teka's catalog is code-owned: the API declares valid keys,
while PostgreSQL stores only role and member assignments. A new key therefore
takes effect only after the API version containing it is deployed.

If an endpoint fits an existing permission, reuse that key and start at
[step 5](#5-classify-the-route). Do not create one permission per endpoint
when multiple endpoints represent the same user capability.

Executable owners:

- [catalog.go](../apps/api/internal/shared/authctx/catalog.go) owns definitions
  and [permissions.go](../apps/api/internal/shared/authctx/permissions.go) owns
  effective-set behavior.
- [routespec.go](../apps/api/internal/shared/routespec/routespec.go) owns
  every route's HTTP classification together with its audit classification;
  [route_policy_enforce.go](../apps/api/internal/server/route_policy_enforce.go)
  enforces the policy half.
- [the centers feature](../apps/api/internal/features/centers) owns assignment.
- [use-center-context.ts](../apps/web/src/features/teaching/hooks/use-center-context.ts)
  owns frontend permission lookup.

## 1. Define the capability

Write down the capability before choosing a key so authorization is not
coupled to one HTTP route.

| Decision | Example |
|---|---|
| Capability | Export students |
| Key | `students.export` |
| Kind and risk | `special`, `medium` |
| Endpoint | `GET /api/v1/students/export` |
| Data scope | Own students unless the caller also has `students.view_all` |
| Existing-role default | No; owners grant it explicitly |

Use `<resource>.<action>`. Choose `crud` for a canonical resource action,
`scope` only for `<resource>.view_all`, and `special` for commands such
as export, approve, close, or send. Operations that must never be delegated
use the `owner_only` route policy and must not become catalog permissions.

## 2. Declare the key

In [catalog.go](../apps/api/internal/shared/authctx/catalog.go):

1. Add a named constant beside the resource's other keys.
2. Add a `PermDef` to `permCatalog` beside that resource's entries.
3. Supply its kind, risk, Vietnamese label, and description.
4. Increment `CatalogVersion` and update its generation comment.

```go
const PermStudentsExport = "students.export"

// In permCatalog:
def(
	PermStudentsExport,
	PermKindSpecial,
	RiskMedium,
	"Xuất danh sách học viên",
	"Xuất danh sách học viên trong phạm vi được phép xem.",
),
```

The version change makes stale permission-management clients reload instead of
replacing assignments from an old catalog view. Align the mirrored version and
fixture in [web test handlers](../apps/web/src/test/msw/handlers.ts).

`def()` and `viewAll()` set `PermDef.DefaultGrant` to `true`: the key is
eligible for `DefaultRoleKeys()`, the baseline `DefaultRoleKeys()` filters by
`Grantable && Kind != PermKindScope && !legacyIdentitySet[key] && DefaultGrant`,
so a scope key (`viewAll`) never actually reaches an existing role even though
it reports `DefaultGrant: true`. Use `optIn()` instead of `def()` for a key
that must never be silently granted to an existing role — it sets
`DefaultGrant: false` and keeps every other attribute identical to `def()`.
Reach for `optIn()` for a new sensitive or high-risk capability by default;
see [step 4](#4-choose-the-database-rollout-policy) for when a `def()` key
still needs a manual backfill migration instead of relying on
`DefaultRoleKeys()` alone.

Do not define the key in a database table or duplicate its label in TypeScript.
The API catalog is authoritative.

## 3. Consider an implied key instead of a new grant

Some capabilities already carry a read entitlement by construction. A member
trusted to send a family a statement or notification necessarily needs to see
the billing, statement, contact, and notification data that message is built
from — granting the send permission without also widening those reads would
just force an owner to hand out four more keys alongside it.

[`impliedKeys`](../apps/api/internal/shared/authctx/catalog.go) maps a special
key to the `<resource>.view_all` keys it carries for reads only:

```go
var impliedKeys = map[string][]string{
	PermReportsSend: {
		PermBillingViewAll,
		PermStatementsViewAll,
		PermNotificationsViewAll,
		PermContactsViewAll,
	},
}
```

`BuildPermSet` in [permissions.go](../apps/api/internal/shared/authctx/permissions.go)
applies implied keys after role and grant/deny resolution, so they are never
stored as their own row and never removable by denying the source key alone —
deny the source key itself to remove both. They appear in `EffectiveKeys()`
and the member's `permissions` list exactly like a direct grant, so a UI or
test asserting the effective set must expect them.

Only add an implied key for a genuine subset relationship — the source
permission is meaningless without the read it implies. Implied keys must only
ever widen `Scope.CenterWideFor` reads, never `Scope.WriteWide` writes; a
test-time scoping guard (`go test ./tools/...`, also `make scopelint`) and the
parity tests described in [step 8](#8-prove-the-policy) hold that line.

## 4. Choose the database rollout policy

An opt-in permission needs no migration. After the API deploys, an owner
assigns it through the permission UI or API. This preserves validation,
compare-and-set protection, and audit evidence; avoid direct production SQL.

If existing roles must receive it automatically:

1. Declare it with `def()` (or `viewAll()` for a `<resource>.view_all` key),
   never `optIn()`, so `DefaultGrant` is `true` and the key is eligible for
   `DefaultRoleKeys()`.
2. Add the next immutable `NNNNNN_slug.up.sql` and `.down.sql` pair under
   [migrations](../apps/api/migrations).
3. Backfill `center_role_permissions` with conflict-safe insertion, and
   `center_member_permissions` too when a role-less live member should also
   receive the default (see migration `000022_task_board.up.sql` for the
   two-table pattern).
4. Remove only that key from role (and member) assignments in the down
   migration.
5. Update the migration/default parity tests in that package.

A key declared with `optIn()` must never be granted by a backfill migration:
an owner assigns it by hand through the permission UI or API. Once the key
has shipped, a down migration only needs to undo schema it introduced, not
sweep grants. The exception is a pre-ship rollback: a down file that also
removes the opt-in keys it introduced (as `000022_task_board.down.sql` does)
is only safe before the feature reaches production, and must say so in its
header — after that, revert with a new forward migration instead.

Treat backfilling as a security decision. Sensitive new capabilities should
normally use `optIn()` and fail closed, remaining unassigned until an owner
opts a role or member in.

## 5. Classify the route

After registering the endpoint in its feature module, add exactly one entry
for its method and Gin route template to `Specs` in
`internal/shared/routespec/routespec.go`. The entry carries both decisions
for the route: its authorization `Kind` (and `Key` for permission-gated
routes) and its `Audit` classification (`Source`, plus `Action`,
`EntityType`, `IDParam` for `SourceRequest` or `SourceAnonymous` mutations
worth a trail row):

```go
perm("GET", "/api/v1/students/export", authctx.PermStudentsExport, none()),
perm("POST", "/api/v1/students", authctx.PermStudentsCreate,
	req("student.create", "student", "")),
```

That entry is the only place the decision lives: `internal/server`'s route
policy, `internal/features/audit`'s action lookup, and `internal/middleware`'s
request-audit skip sets all derive from it, so there is nothing else to
update. The shared middleware checks `Scope` before the handler. Owners pass
implicitly; members need the effective key. The manifest tests fail closed: a
registered route missing from `Specs` fails
`TestRoutePolicyCoversEveryRegisteredRoute`, a mutating route with no audit
source fails `TestMutatingRoutesHaveAnAuditSource` unless it is on the
documented `SourceNone` allowlist, a `SourceRequest` entry with no `Action`
fails `TestRequestSourceRoutesHaveAnAction`, and a skip source sharing a path
with an audited route fails `TestSkipSourceNeverSharesPathWithAuditedRoute`.

Use `authctx.Require(scope, key)` at a service boundary only when the
operation can bypass HTTP middleware. Do not scatter duplicate checks.

## 6. Preserve data scope

A route permission answers **may the caller perform the capability?** It does
not answer **which tenant or rows may they access?**

For this example, `students.export` permits export,
`students.view_all` may widen the read set, and repository `center_id`
predicates always isolate tenants. Read expansion must use
`Scope.CenterWideFor(<resource>.view_all)`; it must never widen writes. A
write reaches another teacher's rows only through `Scope.WriteWide()` (the
owner alone), so a repository keeps a read port and a write port apart
(`readScoped` / `writeScoped`, or a `readNarrow` helper for inline
predicates). `apps/api/tools/scopelint` enforces this at test time:
`CenterWideFor` may appear only inside a repository function whose name
contains `read`. Never accept request `center_id` or `teacher_id` as
authorization context. See
[Tenancy](./api-guidelines.md#tenancy).

## 7. Gate the frontend when applicable

The API remains the security boundary. Use the effective set:

```tsx
const { has } = useCenterContext();
const canExportStudents = has("students.export");
```

Use the same key for navigation/actions, deep-link guards, and
permission-dependent queries. Do not duplicate labels in TypeScript. See
[Permission gating](./frontend-guidelines.md#permission-gating).

## 8. Prove the policy

Add focused tests beside the catalog, route policy, affected feature, and UI.

| Caller | Assignment | Expected |
|---|---|---|
| Owner | None | Allowed |
| Member | Role or direct grant | Allowed within existing data scope |
| Member | None | `403 Forbidden` |
| Member | Role grant plus member deny | `403 Forbidden` |
| Unauthenticated caller | None | `401 Unauthorized` |
| Caller from another center | Any grant | No cross-center data |

```bash
make test-api-unit
make test-web
make lint
```

Run `make test-api` for migrations or database behavior. Run
`make api-docs` when endpoint annotations change; never edit generated
OpenAPI files manually.

## 9. Deploy and assign

1. Deploy every API instance containing the catalog entry and route policy.
2. Apply the migration if an existing-role backfill is intentional.
3. Assign an opt-in permission through the owner UI or API.
4. Deploy the gated frontend when applicable.
5. Verify one allowed and one denied request.

Do not pre-populate a new key while old API instances remain authoritative.
During a rolling deployment, wait until every instance serves the new catalog
before editing assignments. Assignment changes affect the member's next
request because permissions are resolved from PostgreSQL, not the JWT.

## Review checklist

- [ ] Reused an existing capability when its meaning matches.
- [ ] Selected `permission` versus `owner_only` intentionally.
- [ ] Declared the key and incremented `CatalogVersion`.
- [ ] Considered whether the capability implies a read it should carry.
- [ ] Chose opt-in versus existing-role backfill explicitly.
- [ ] Classified every new authenticated route.
- [ ] Preserved tenant, object, and class-staff scoping.
- [ ] Added frontend gating without treating it as enforcement.
- [ ] Tested allow, deny, owner, override, and cross-center behavior.
- [ ] Planned API deployment before assignment.
