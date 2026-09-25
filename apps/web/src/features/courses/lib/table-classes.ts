/** Shared class names for the prototype's catalog tables (paths, courses, course detail). */
export const headCellClassName =
  "whitespace-nowrap bg-cream-200 px-1 py-[10px] text-left text-[11.5px] font-extrabold uppercase tracking-[0.4px] text-ink-500 first:pl-[18px] last:pr-[18px]";

export const cellClassName =
  "border-t border-line-100 px-1 py-[11px] align-middle first:pl-[18px] last:pr-[18px]";

export const tableCardClassName = "overflow-x-auto rounded-[20px] bg-white shadow-soft-md";

const rowActionBase =
  "rounded-[10px] border-[1.5px] border-line-200 bg-transparent px-2.5 py-[5px] text-[12px] font-extrabold text-ink-500 disabled:cursor-not-allowed disabled:opacity-50";

/** Outline row action that turns sky on hover ("Sửa", "Mã"). */
export const skyActionClassName = `${rowActionBase} hover:border-sky-300 hover:text-sky-500`;

/** Outline row action that turns mint on hover ("Quản lý chặng", "Sửa" on courses). */
export const mintActionClassName = `${rowActionBase} hover:border-mint-400 hover:text-mint-600`;

/** Dashed pill/box used for inline "+ add" affordances. */
export const dashedAddClassName =
  "border-dashed border-line-300 bg-transparent font-extrabold text-mint-600 hover:border-mint-400 hover:bg-mint-50 disabled:cursor-not-allowed disabled:opacity-50";

export function formatVnd(amount: number): string {
  return `${new Intl.NumberFormat("vi-VN").format(amount)}đ`;
}
