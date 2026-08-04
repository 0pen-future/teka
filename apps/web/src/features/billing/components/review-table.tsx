import { HvButton, HvCard } from "@/components/hv";
import { formatMoney } from "@/lib/utils";

import type { ReviewRow } from "../schemas/billing-schemas";

export interface ReviewTableProps {
  rows: ReviewRow[];
  closed: boolean;
  onAdjust: (row: ReviewRow) => void;
}

const headerClassName =
  "px-3 py-2 text-left text-[11px] font-semibold uppercase tracking-wide text-ink-400";
const cellClassName = "px-3 py-3 text-[15px] text-ink-700";
const numericCellClassName = `${cellClassName} text-right font-display font-bold`;

/**
 * `sm+` review table: sticky first column, horizontal scroll confined to the
 * wrapper (`overflow-x-auto`), never the page. R1 AC 2 requires one row per
 * student even when they attend two classes — implemented as one `<tr>` per
 * class line with `rowSpan` on the student-level columns (name, nợ cũ,
 * điều chỉnh, tổng, action), so the invoice stays visually and semantically
 * one row while each class line is still legible on its own line. The
 * class-name cell keeps the `--mint-50` tint the Design Spec's group-header
 * rows call for.
 */
export function ReviewTable({ rows, closed, onAdjust }: ReviewTableProps) {
  return (
    <HvCard variant="flat" padding="sm" className="overflow-x-auto p-0">
      <table className="w-full min-w-[720px] border-collapse">
        <thead>
          <tr className="border-b border-line-100">
            <th className={`${headerClassName} sticky left-0 bg-white`}>Học sinh</th>
            <th className={headerClassName}>Lớp &amp; số buổi</th>
            <th className={headerClassName}>Vắng</th>
            <th className={headerClassName}>Đơn giá</th>
            <th className={headerClassName}>Thành tiền</th>
            <th className={headerClassName}>Nợ cũ</th>
            <th className={headerClassName}>Điều chỉnh</th>
            <th className={headerClassName}>Tổng</th>
            <th className={headerClassName} aria-hidden />
          </tr>
        </thead>
        <tbody>
          {rows.map((row) => {
            const lines = row.lines.length > 0 ? row.lines : [null];
            return lines.map((line, index) => {
              const first = index === 0;
              const rowSpan = lines.length;
              return (
                <tr
                  key={`${row.student_id}-${line?.enrollment_id ?? "no-line"}`}
                  className="border-b border-line-100 last:border-0"
                >
                  {first ? (
                    <td
                      rowSpan={rowSpan}
                      className={`${cellClassName} sticky left-0 bg-white font-semibold`}
                    >
                      {row.student_name}
                      <p className="text-[12px] font-normal text-ink-400">{row.contact_name}</p>
                    </td>
                  ) : null}
                  {line ? (
                    <>
                      <td className={`${cellClassName} bg-mint-50`}>
                        <span className="font-display font-bold text-mint-600">
                          {line.class_name}
                        </span>
                        <span className="ml-2 text-[13px] text-ink-500">
                          {line.billable_count} buổi
                        </span>
                      </td>
                      <td className={numericCellClassName}>{line.absent_count}</td>
                      <td className={numericCellClassName}>{formatMoney(line.unit_price)}</td>
                      <td className={numericCellClassName}>{formatMoney(line.amount)}</td>
                    </>
                  ) : (
                    <td className={cellClassName} colSpan={4}>
                      Kỳ này chưa có buổi học nào
                    </td>
                  )}
                  {first ? (
                    <>
                      <td
                        rowSpan={rowSpan}
                        className={`${numericCellClassName} ${row.opening_balance !== 0 ? "text-coral-600" : ""}`}
                      >
                        {formatMoney(row.opening_balance)}
                      </td>
                      <td
                        rowSpan={rowSpan}
                        className={`${numericCellClassName} ${row.adjustment_total !== 0 ? "text-sun-600" : ""}`}
                      >
                        {row.adjustment_total > 0 ? "+" : ""}
                        {formatMoney(row.adjustment_total)}
                      </td>
                      <td rowSpan={rowSpan} className={numericCellClassName}>
                        {formatMoney(row.total_due)}
                      </td>
                      <td rowSpan={rowSpan} className={cellClassName}>
                        <HvButton
                          type="button"
                          variant="ghost"
                          size="sm"
                          disabled={closed || !row.invoice_id}
                          onClick={() => onAdjust(row)}
                        >
                          Sửa
                        </HvButton>
                      </td>
                    </>
                  ) : null}
                </tr>
              );
            });
          })}
        </tbody>
      </table>
    </HvCard>
  );
}
