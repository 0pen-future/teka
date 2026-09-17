import { HvChip, HvSelect } from "@/components/hv";

import { assigneeChips, isFiltering, type AssigneeDirectoryEntry } from "../lib/board-filters";
import { BOARD_FILTERS, type BoardCounts, type BoardFilterValue } from "../schemas/task-schemas";

const STATUS_LABELS: Record<BoardFilterValue, string> = {
  all: "Tất cả",
  mine: "Của tôi",
  overdue: "Quá hạn",
  today: "Hôm nay",
  unassigned: "Chưa giao",
};

/** Chips beyond this many collapse into the overflow `HvSelect`. */
const MAX_ASSIGNEE_CHIPS = 8;

export interface BoardFilterBarProps {
  filter: BoardFilterValue;
  assignee: string;
  counts: BoardCounts;
  members: readonly AssigneeDirectoryEntry[];
  onChange: (partial: { filter?: BoardFilterValue; assignee?: string }) => void;
}

/**
 * Status + assignee quick filters, both server-side (see D3 in the plan):
 * switching either re-fetches `GET /tasks/board` with the new params rather
 * than filtering the already-loaded board client-side. Only rendered for
 * `canViewAll` holders — a teacher's own board has nothing to narrow.
 */
export function BoardFilterBar({
  filter,
  assignee,
  counts,
  members,
  onChange,
}: BoardFilterBarProps) {
  const chips = assigneeChips(counts.by_assignee, members);
  const visibleChips = chips.slice(0, MAX_ASSIGNEE_CHIPS);
  const overflowChips = chips.slice(MAX_ASSIGNEE_CHIPS);
  const overflowSelected = overflowChips.some((chip) => chip.teacherId === assignee);

  const toggleAssignee = (teacherId: string) => {
    onChange({ assignee: assignee === teacherId ? "" : teacherId });
  };

  return (
    <div role="group" aria-label="Bộ lọc nhanh" className="flex flex-wrap items-center gap-1.5">
      <div
        role="radiogroup"
        aria-label="Trạng thái"
        className="flex flex-wrap items-center gap-1.5"
      >
        {BOARD_FILTERS.map((value) => (
          <HvChip
            key={value}
            role="radio"
            size="sm"
            pressed={filter === value}
            count={counts[value]}
            onClick={() => onChange({ filter: value })}
          >
            {STATUS_LABELS[value]}
          </HvChip>
        ))}
      </div>
      {visibleChips.length > 0 ? (
        <>
          <span aria-hidden className="mx-0.5 h-5 w-px bg-line-200" />
          <div
            role="radiogroup"
            aria-label="Giáo viên"
            className="flex flex-wrap items-center gap-1.5"
          >
            {visibleChips.map((chip) => (
              <HvChip
                key={chip.teacherId}
                role="radio"
                size="sm"
                pressed={assignee === chip.teacherId}
                count={chip.count}
                onClick={() => toggleAssignee(chip.teacherId)}
              >
                {chip.label}
              </HvChip>
            ))}
          </div>
        </>
      ) : null}
      {overflowChips.length > 0 ? (
        <HvSelect
          sheetTitle="Lọc theo giáo viên"
          aria-label="Lọc theo giáo viên khác"
          placeholder="Khác…"
          value={overflowSelected ? assignee : ""}
          onValueChange={(value) => onChange({ assignee: value })}
          options={overflowChips.map((chip) => ({
            value: chip.teacherId,
            label: chip.label,
            meta: String(chip.count),
          }))}
          className="min-h-8 min-w-0 px-2.5 text-[12px]"
        />
      ) : null}
      {isFiltering({ filter, assignee }) ? (
        <button
          type="button"
          onClick={() => onChange({ filter: "all", assignee: "" })}
          className="ml-auto rounded-full px-2.5 py-1 text-[12px] font-bold text-ink-500 hover:bg-cream-100 hover:text-ink-900"
        >
          Xoá lọc
        </button>
      ) : null}
    </div>
  );
}
