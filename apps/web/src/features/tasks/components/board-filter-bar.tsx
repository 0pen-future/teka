import type { ReactNode } from "react";

import { cn } from "@/lib/utils";

import { assigneeChips, type AssigneeDirectoryEntry } from "../lib/board-filters";
import { BOARD_FILTERS, type BoardCounts, type BoardFilterValue } from "../schemas/task-schemas";
import { AssigneeAvatar } from "./assignee-avatar";

const STATUS_LABELS: Record<BoardFilterValue, string> = {
  all: "Tất cả",
  mine: "Của tôi",
  overdue: "Quá hạn",
  today: "Hôm nay",
  unassigned: "Chưa giao",
};

interface FilterChipProps {
  pressed: boolean;
  /** Draws the chip in coral while idle — "Quá hạn" with something behind it. */
  warn?: boolean;
  count: number;
  leading?: ReactNode;
  onClick: () => void;
  children: string;
}

/** One `aria-pressed` toggle; the label and count are sibling text nodes so the accessible name reads "Quá hạn 3". */
function FilterChip({ pressed, warn = false, count, leading, onClick, children }: FilterChipProps) {
  return (
    <button
      type="button"
      aria-pressed={pressed}
      onClick={onClick}
      className={cn(
        "inline-flex min-h-9 items-center gap-1.5 rounded-full border-[1.5px] border-line-200 bg-white px-3",
        "font-body text-[13px] font-bold text-ink-500 transition-colors hover:border-line-300 hover:text-ink-900",
        "focus-visible:outline-none focus-visible:ring-4 focus-visible:ring-mint-100",
        pressed && "border-ink-900 bg-ink-900 text-white hover:border-ink-900 hover:text-white",
        !pressed &&
          warn &&
          "border-coral-300 text-coral-600 hover:border-coral-300 hover:text-coral-600",
      )}
    >
      {leading}
      {children}
      {/* Explicit space so the accessible name reads "Quá hạn 3", not "Quá hạn3"; the flex gap swallows it visually. */}{" "}
      <span className="tabular-nums opacity-75">{count}</span>
    </button>
  );
}

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
 *
 * There is no separate "clear" control: pressing a status chip again (or
 * "Tất cả") drops back to the full board, and a teacher chip toggles off on
 * its second click.
 */
export function BoardFilterBar({
  filter,
  assignee,
  counts,
  members,
  onChange,
}: BoardFilterBarProps) {
  const chips = assigneeChips(counts.by_assignee, members);

  const toggleStatus = (value: BoardFilterValue) => {
    onChange({ filter: filter === value && value !== "all" ? "all" : value });
  };

  const toggleAssignee = (teacherId: string) => {
    onChange({ assignee: assignee === teacherId ? "" : teacherId });
  };

  return (
    <div role="group" aria-label="Bộ lọc nhanh" className="flex flex-wrap items-center gap-1.5">
      {BOARD_FILTERS.map((value) => (
        <FilterChip
          key={value}
          pressed={filter === value}
          warn={value === "overdue" && counts.overdue > 0}
          count={counts[value]}
          onClick={() => toggleStatus(value)}
        >
          {STATUS_LABELS[value]}
        </FilterChip>
      ))}
      {chips.length > 0 ? (
        <>
          <span aria-hidden className="mx-1 h-[22px] w-[1.5px] bg-line-300" />
          {chips.map((chip) => (
            <FilterChip
              key={chip.teacherId}
              pressed={assignee === chip.teacherId}
              count={chip.count}
              leading={
                <AssigneeAvatar name={chip.label} size="xs" decorative className="-ml-1.5" />
              }
              onClick={() => toggleAssignee(chip.teacherId)}
            >
              {chip.label}
            </FilterChip>
          ))}
        </>
      ) : null}
    </div>
  );
}
