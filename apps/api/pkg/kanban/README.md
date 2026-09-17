# kanban

Pure Go Kanban domain library: configurable columns and tasks moving between
them, for one tenant at a time. Import path `teka/apps/api/pkg/kanban`.

This package knows nothing about HTTP, SQL, Gin, GORM, or any specific host
application. It depends on nothing but the Go standard library and
`github.com/google/uuid`. A `go list -deps` boundary test
(`import_boundary_test.go`) enforces this on every build; it was verified once
by adding a temporary `internal/...` import and confirming the test fails,
then reverting.

## Why a library, not a package inside a feature

`pkg/kanban` is written so it can be `git mv`'d into its own repository
later with no refactor — just add a `go.mod`. Every design choice below
follows from keeping that door open: no host-specific types leak into any
signature, and every side effect (persistence, transactions, events,
authorization) is a port the host implements.

## Ports

| Port | Responsibility |
|---|---|
| `ColumnRepository` | CRUD + reordering + existence checks for columns, always scoped by `TenantID` |
| `TaskRepository` | CRUD + soft-delete + bulk column moves + handover bookkeeping for tasks, always scoped by `TenantID` |
| `Repositories` | Groups the two repositories above into one `NewService` parameter |
| `UnitOfWork` | `Within(ctx, fn) error` — runs `fn` atomically; shape matches `database.TxManager.WithinTx` in the host app so wrapping it is a one-line adapter |
| `Policy` | The single place object-level authorization rules live: `CanManageBoard`, `CanReadTask`, `CanWriteTask`, `CanMoveTask`, `Visibility` |
| `MemberChecker` | `IsMember(ctx, tenant, actor) (bool, error)` — lets `CreateTask`/`UpdateTask` reject a non-member assignee without knowing what a "member" table is |
| `EventSink` | `Publish(ctx, event any)` — called only after a use-case's own `UnitOfWork.Within` returns `nil` |
| `Clock` | `Now() time.Time` — injected so tests assert deterministic `CompletedAt` values instead of wall-clock time |

`TaskRepository.ListBoard` also takes a `BoardFilter` (`AssigneeID`,
`Unassigned`, `DueBefore`, `DueOn`, `OpenOnly` — zero value means no filter),
ANDed with `Visibility`, never a replacement for it. `TaskRepository.CountBoard`
takes the same `Visibility` plus a `today` date and returns `BoardCounts`
(`All`, `Overdue`, `Today`, `Unassigned`, `ByAssignee`) — open (uncompleted)
tasks only, independent of any `BoardFilter` and unbounded by `ListBoard`'s
`limit`. Every date comparison in `BoardFilter`/`CountBoard` is **date-only**:
callers pass a `time.Time` truncated to a calendar date, and the adapter
compares against `due_on`'s date part, never a timestamp range — the calendar
day in question is whichever the client considers "today" (see
`internal/features/tasks/service.go`), not a server-derived timezone.

Every `ColumnRepository`/`TaskRepository` method takes `tenant TenantID` as
its first parameter after `ctx`. This is deliberate: a repository method
cannot forget to scope a query, because the compiler requires the tenant to
be passed in. There is no `ErrCrossTenant`: an id belonging to a different
tenant is indistinguishable from a missing one, and both surface as
`ErrColumnNotFound` / `ErrTaskNotFound`. This avoids an oracle that would
otherwise tell a caller "that object exists, just not for you."

## Policy split (core vs. host middleware)

`Service` enforces `Policy` itself — it does not trust a caller to have
already checked anything — because this library may be reused by a host with
no equivalent middleware. But `Policy` only covers **object-level** rules
(read/write/move a specific task, manage a specific tenant's board). It does
**not** cover **capability** rules like "may this actor create tasks at all"
or "may this actor list the board at all" — those are assumed to be enforced
by the host's own request-level authorization (e.g. a middleware checking a
permission key) before the request ever reaches `Service`. Concretely:

- `CreateTask` and `Board` do not call `Policy` for "may I do this at all" —
  only for what the result should contain (`Board`'s `Visibility`) or for data
  validity (assignee membership).
- `UpdateTask`, `DeleteTask` require `CanWriteTask`.
- `MoveTask` requires `CanMoveTask` — a task's assignee can move it without
  being able to edit or delete it.
- `CreateColumn`, `UpdateColumn`, `ReorderColumns`, `DeleteColumn` require
  `CanManageBoard`.

`DefaultPolicy` implements the four rules from the plan brief: manage board =
owner ∨ `tasks.manage_board`; read = owner ∨ `tasks.view_all` ∨ creator ∨
assignee; write = owner ∨ creator; move = owner ∨ creator ∨ assignee. A host
with different rules implements `Policy` itself; `Actor.Perms` is read-only
input, never mutated, and must be a map built fresh per request — do not hand
`Service` a map that some other part of the host mutates concurrently.

## Options

`NewService` accepts functional options: `WithMaxColumns(n)` (default column
cap per tenant), `WithMaxNameLen(n)` (column name length limit),
`WithClock(clock)` (deterministic `CompletedAt` in tests) and
`WithIDGen(fn)` (deterministic IDs in tests). See `options.go` for defaults.

## Use-case → port flow

| Use-case | Reads | Writes | Uses `UnitOfWork` | Publishes |
|---|---|---|---|---|
| `Board` | `Columns.List`, `Tasks.ListBoard` (with `BoardFilter`) | — | no | — |
| `BoardCounts` | `Tasks.CountBoard` | — | no | — |
| `CreateColumn` | `Columns.ExistsName`, `Columns.CountByTenant` | `Columns.Create` | no | — |
| `UpdateColumn` | `Columns.Get`, `Columns.ExistsName` | `Columns.Update` | no | — |
| `ReorderColumns` | `Columns.List` | `Columns.UpdatePositions` | no | — |
| `DeleteColumn` | `Columns.Get`, `Columns.List`, `Tasks.CountInColumn` | `Tasks.MoveAllToColumn`, `Columns.Delete` | **yes** | `ColumnDeleted` |
| `CreateTask` | `Columns.Get`, `Tasks.MinPositionInColumn`, `MemberChecker.IsMember` | `Tasks.Create` | no | — |
| `GetTask` | `Tasks.Get` | — | no | — |
| `UpdateTask` | `Tasks.Get`, `MemberChecker.IsMember` | `Tasks.Update` | no | — |
| `MoveTask` | `Tasks.Get`, `Columns.Get`, `Tasks.ListColumnPositions` | `Tasks.RenormalizeColumn` (when gaps run out), `Tasks.Update` | **yes** | — |
| `DeleteTask` | `Tasks.Get` | `Tasks.SoftDelete` | no | — |
| `RestoreTask` | `Tasks.GetDeleted`, `Tasks.Get` | `Tasks.Restore` | **yes** | — |
| `HandoverOnDeparture` | — | `Tasks.UnassignBy`, `Tasks.ReassignCreator` | **no** | **no** |

## Transaction and event contract

`DeleteColumn` and `MoveTask` open their own `UnitOfWork.Within`; `MoveTask`
does so purely for atomicity (position lookup, optional renormalization and
the write must see one snapshot of the column) and publishes nothing. Only
`DeleteColumn` publishes — and only after `Within` returns `nil`. This means:

- **Do not call a publishing use-case from inside your own ambient
  transaction.** If your host's `UnitOfWork` implementation joins an ambient
  transaction found in `ctx` rather than always opening a new one (as
  `database.TxManager.WithinTx` does), calling `DeleteColumn` from inside your
  own already-open transaction means its `Within` call joins yours instead of
  being the outermost boundary — the event would then publish before your own
  transaction commits.
- **`HandoverOnDeparture` is the opposite case on purpose.** It does not open
  `UnitOfWork.Within` and does not publish anything, because it is designed
  to run inside the *caller's* ambient transaction (e.g. a "remove member"
  flow in the host app). The caller owns the commit, so the caller owns
  publishing whatever event it wants afterward — using the counts
  `HandoverOnDeparture` returns. Calling it outside a transaction, or
  expecting it to publish, is a misuse of the contract.

## Column cap contract (check-then-act)

`CreateColumn` enforces `Config.MaxColumns` by calling
`ColumnRepository.CountByTenant` and then `ColumnRepository.Create` as two
separate calls — this is a classic check-then-act race under concurrent
requests for the same tenant. The core does not lock anything (it does not
know what locking primitive, if any, the host's datastore offers). **The
adapter must serialize concurrent `CreateColumn` calls per tenant** (e.g. a
Postgres advisory lock keyed by tenant id, held for the duration of the
check-then-act) if an exact cap matters more than an occasional off-by-one
under heavy concurrent create traffic.

## Position strategy

`Task.Position` is a `float64` and a column reads in ascending `Position`
(ties broken by creation time). Two placements exist:

- **Top of column** — `CreateTask`, and `MoveTask` with `after == nil`: the
  task gets `min(existing positions) - 1` (0 for an empty column).
  `CreateTask` reads the minimum via `TaskRepository.MinPositionInColumn`;
  `MoveTask` takes it from the first row of `ListColumnPositions` so the top
  move holds the same column lock as every other move (see below). Repeated
  top insertion only ever decreases the value, so it never bisects and never
  loses precision.
- **After a given task** — `MoveTask` with `after` set (drag-and-drop): the
  core calls `TaskRepository.ListColumnPositions` for the destination column,
  finds `after`, and takes the midpoint between it and its successor (the
  successor search skips the moving task itself, so a same-column reorder
  behaves like a cross-column one). With no successor the task gets
  `after.Position + 1`. `after` must be a live task of the destination column
  and not the moving task, otherwise `ErrInvalidAfterTask` (wrapping
  `ErrInvalidInput`) is returned.

Bisecting halves the gap each time, so the core enforces a floor:
`minPositionGap` (`1e-6`). When a midpoint would leave a gap below it, the
core calls `TaskRepository.RenormalizeColumn` with the column's current order
(positions become `0..n-1`), re-lists, and computes the midpoint again. No two
neighbours in a column are therefore ever closer than `minPositionGap`, and
no periodic renormalization job is needed.

All of this runs inside one `UnitOfWork.Within`. Because placement is a
read-then-write, `ListColumnPositions` must also serialize concurrent moves
into the same column for the rest of the transaction — the Postgres adapter
takes `pg_advisory_xact_lock` keyed on tenant + column before listing — so two
simultaneous moves after the same anchor cannot compute the same midpoint,
and a top move cannot commit between another move's renormalization read and
its per-row writes (which would overwrite the top move's position). Deletes
do not take the lock; `RenormalizeColumn` therefore skips rows that vanished
since the listing instead of failing the move.

## Minimal adapter example

```go
repos := kanban.Repositories{
    Columns: myColumnRepo{db: db},
    Tasks:   myTaskRepo{db: db},
}
uow := myUnitOfWorkAdapter{tx: database.NewTxManager(db)} // wraps WithinTx
svc := kanban.NewService(
    repos,
    uow,
    kanban.DefaultPolicy{},
    myMemberChecker{db: db},
    myEventSinkAdapter{bus: bus}, // wraps events.Bus.Publish
)

cols, tasks, err := svc.Board(ctx, tenant, actor, kanban.BoardFilter{}, 0)
```

`myEventSinkAdapter.Publish(ctx, event any)` type-switches on `event` (today
only `kanban.ColumnDeleted`) and forwards to the host's own event bus, adding
any host-specific fields (e.g. `center_id`) the bus's event type requires.

`myUnitOfWorkAdapter.Within` typically just calls
`txManager.WithinTx(ctx, fn)` — see `internal/database/tx.go` in this module
for the shape it mirrors. Every repository method must read its persistence
handle from `ctx` (e.g. via `database.FromContext`), not from a struct field,
so the same repository works both inside and outside a `Within` call.

## Localization boundary

This library contains no user-facing strings, and no code comment or doc in
this package names a specific language's column labels. `DefaultColumns(idGen,
specs)` seeds `Column.Position` and IDs from `[]DefaultColumnSpec{Name, IsDone, Color}`, but
the names themselves belong entirely to the adapter (see
`internal/features/centers/default_columns.go` and migration `000022` in this
repo for Teka's own localized names).

## Extraction procedure

To pull this package into its own repository later:

1. `git mv apps/api/pkg/kanban <new-repo>/`.
2. Add a `go.mod` in the new repo: `module <new-module-path>`, requiring
   `github.com/google/uuid` at the version already used here.
3. Update the import path in every file from `teka/apps/api/pkg/kanban` to
   the new module path (only relevant to consumers, since the package itself
   has no self-imports).
4. Point the host app's `go.mod` at the new module (a `require` + optionally
   a `replace` during transition) and update its adapter's import.

No internal refactor is needed — the boundary test already guarantees there
is nothing to untangle.
