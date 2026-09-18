import type { BoardFilterValue } from "../schemas/task-schemas";

export interface AssigneeChip {
  teacherId: string;
  label: string;
  count: number;
}

/** Minimal shape `assigneeChips` needs from a member-directory entry. */
export interface AssigneeDirectoryEntry {
  teacher_id: string;
  display_name: string;
}

/**
 * Joins `counts.by_assignee` (teacher id + count) against the member
 * directory for a display name, dropping any teacher with no open tasks —
 * the chip strip is "who has work right now", not the full roster. Sorted
 * by count (busiest first) so the teachers with the most on their plate
 * lead the strip.
 */
export function assigneeChips(
  byAssignee: readonly { teacher_id: string; count: number }[],
  members: readonly AssigneeDirectoryEntry[],
): AssigneeChip[] {
  const nameFor = new Map(members.map((member) => [member.teacher_id, member.display_name]));
  return byAssignee
    .filter((entry) => entry.count > 0)
    .map((entry) => ({
      teacherId: entry.teacher_id,
      label: nameFor.get(entry.teacher_id) ?? "Không rõ",
      count: entry.count,
    }))
    .sort((a, b) => b.count - a.count || a.label.localeCompare(b.label, "vi"));
}

/** Whether the board's current filter/assignee state narrows the view at all — drives the columns' empty-state copy. */
export function isFiltering(state: { filter: BoardFilterValue; assignee: string }): boolean {
  return state.filter !== "all" || state.assignee !== "";
}
