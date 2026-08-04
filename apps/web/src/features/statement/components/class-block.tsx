import { formatMoney } from "@/lib/utils";

import type { StatementClass } from "../types/statement-types";
import { SessionDateList, type SessionChipTone } from "./session-date-list";

/**
 * The server sends exactly these three session statuses on a public
 * statement; an unrecognized status simply appears in none of the groups
 * rather than crashing the page.
 */
const SESSION_STATUS_GROUPS: {
  status: string;
  label: string;
  suffix: string;
  tone: SessionChipTone;
}[] = [
  { status: "present", label: "Có mặt", suffix: "✓", tone: "mint" },
  { status: "absent", label: "Vắng", suffix: "✕", tone: "coral" },
  { status: "cancelled", label: "Buổi huỷ", suffix: "huỷ", tone: "cream" },
];

export interface ClassBlockProps {
  classData: StatementClass;
}

/**
 * One enrollment's session history and fee formula. A child attending
 * several classes gets one of these per class. The formula line renders only
 * server-supplied values — `billable_count`, `unit_price`, and `amount` —
 * never a client-side `billable_count * unit_price` recomputation.
 */
export function ClassBlock({ classData }: ClassBlockProps) {
  return (
    <div className="flex flex-col gap-2 border-t border-line-100 pt-3 first:border-t-0 first:pt-0">
      <span className="inline-flex w-fit items-center rounded-[var(--radius-pill)] bg-mint-50 px-2.5 py-1 text-[13px] font-bold text-mint-600">
        {classData.class_name}
      </span>
      {SESSION_STATUS_GROUPS.map(({ status, label, suffix, tone }) => (
        <SessionDateList
          key={status}
          label={label}
          suffix={suffix}
          tone={tone}
          dates={classData.sessions
            .filter((session) => session.status === status)
            .map((session) => session.date)}
        />
      ))}
      <p className="text-[14px] text-ink-700">
        {`${classData.billable_count} buổi × ${formatMoney(classData.unit_price)} = ${formatMoney(classData.amount)}`}
      </p>
    </div>
  );
}
