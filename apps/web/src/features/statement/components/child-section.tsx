import { HvCard } from "@/components/hv";
import { formatMoney } from "@/lib/utils";

import { formatChipDate } from "../lib/format-chip-date";
import type { StatementChild } from "../types/statement-types";
import { ClassBlock } from "./class-block";

export interface ChildSectionProps {
  child: StatementChild;
}

/**
 * One child's card: a class block per enrollment, the carried-adjustment
 * banner and manual adjustments when present, nợ cũ as its own line, and the
 * child's subtotal — every amount taken verbatim from the server.
 */
export function ChildSection({ child }: ChildSectionProps) {
  return (
    <HvCard variant="raised" className="flex flex-col gap-3">
      <div>
        <h2 className="font-display text-[16px] font-bold text-ink-900">{child.student_name}</h2>
        {child.display_note ? (
          <p className="text-[13px] text-ink-400">{child.display_note}</p>
        ) : null}
      </div>

      {child.classes.map((classData, index) => (
        <ClassBlock key={`${classData.class_name}-${index}`} classData={classData} />
      ))}

      {child.carried_adjustment ? (
        <div className="rounded-[var(--radius-lg)] bg-sun-100 p-3 text-[13px] text-sun-600">
          {`Điều chỉnh chuyển từ kỳ trước: ${formatMoney(child.carried_adjustment.amount)} cho buổi ${child.carried_adjustment.session_dates.map(formatChipDate).join(", ")}.`}
        </div>
      ) : null}

      {child.adjustments.map((adjustment, index) => (
        <p key={index} className="text-[14px] font-semibold text-sun-600">
          {`Điều chỉnh: ${adjustment.amount >= 0 ? "+" : ""}${formatMoney(adjustment.amount)}`}
        </p>
      ))}

      {child.opening_balance !== 0 ? (
        <p className="text-[14px] font-semibold text-coral-600">
          {`Nợ cũ: ${formatMoney(child.opening_balance)}`}
        </p>
      ) : null}

      <div className="flex items-center justify-between border-t border-dashed border-line-200 pt-3">
        <span className="text-[14px] font-semibold text-ink-700">Tạm tính cho con</span>
        <span className="font-display text-[16px] font-bold text-ink-900">
          {formatMoney(child.subtotal)}
        </span>
      </div>
    </HvCard>
  );
}
