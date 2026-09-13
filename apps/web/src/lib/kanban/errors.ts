/**
 * Domain error union for {@link KanbanDataSource} implementations. The lib
 * never maps HTTP status codes itself — that translation is the adapter's
 * job (see README.md "Ports"). All 9 `KanbanDataSource` methods reject with
 * one of these shapes.
 */

export type KanbanErrorEntity = "column" | "task";

export type KanbanError =
  | { kind: "not-found"; entity: KanbanErrorEntity }
  | { kind: "conflict" }
  | { kind: "forbidden" }
  | { kind: "validation"; fields: Record<string, string> }
  | { kind: "unknown"; cause: unknown };

const KANBAN_ERROR_KINDS: ReadonlySet<string> = new Set<KanbanError["kind"]>([
  "not-found",
  "conflict",
  "forbidden",
  "validation",
  "unknown",
]);

export function isKanbanError(value: unknown): value is KanbanError {
  if (typeof value !== "object" || value === null || !("kind" in value)) {
    return false;
  }
  const kind = value.kind;
  return typeof kind === "string" && KANBAN_ERROR_KINDS.has(kind);
}
