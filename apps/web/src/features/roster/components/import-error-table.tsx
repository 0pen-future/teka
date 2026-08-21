import type { ImportErrorsPayload } from "../schemas/import-schemas";

const headCellClassName =
  "sticky top-0 z-10 bg-cream-200 px-[14px] py-[10px] text-[12px] font-extrabold uppercase tracking-[0.4px] text-ink-500";

const cellClassName = "border-t border-line-100 px-[14px] py-[10px] align-top";

/**
 * The whole defect list from a rejected workbook. Nothing was written, so
 * this is a worklist rather than a partial result: the operator fixes every
 * row named here, then re-checks. Sheet and line are shown as their own
 * columns because they are what the operator navigates by in Excel.
 *
 * The list scrolls inside the card — a 500-row workbook can produce hundreds
 * of defects, and pushing the "Kiểm tra lại" affordance below the fold makes
 * the page look stuck.
 */
export function ImportErrorTable({ payload }: { payload: ImportErrorsPayload }) {
  return (
    <div className="flex flex-col gap-2">
      <p className="text-[13.5px] text-ink-500">
        File có {payload.errors.length} dòng cần sửa. Chưa có dữ liệu nào được ghi vào hệ thống —
        sửa file rồi kiểm tra lại.
      </p>
      <div className="overflow-hidden rounded-[20px] bg-white shadow-soft-md">
        <div className="max-h-[52vh] overflow-auto">
          <table className="w-full min-w-[560px] border-collapse text-left text-[13.5px]">
            <thead>
              <tr>
                <th className={headCellClassName}>Sheet</th>
                <th className={headCellClassName}>Dòng</th>
                <th className={headCellClassName}>Cột</th>
                <th className={headCellClassName}>Lỗi</th>
              </tr>
            </thead>
            <tbody>
              {payload.errors.map((rowError, index) => (
                // Two defects can share sheet+line+column (a row can fail on
                // more than one rule), so the index is the only stable key.
                <tr key={`${rowError.sheet}-${rowError.line}-${rowError.column ?? ""}-${index}`}>
                  <td className={cellClassName}>{rowError.sheet}</td>
                  <td className={`${cellClassName} font-extrabold text-ink-900`}>
                    {rowError.line}
                  </td>
                  <td className={cellClassName}>{rowError.column ?? "—"}</td>
                  <td className={cellClassName}>{rowError.message}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
      {payload.truncated ? (
        <p className="text-[13px] text-ink-400">
          Còn {payload.truncated} lỗi nữa chưa hiển thị. Sửa các lỗi trên rồi kiểm tra lại.
        </p>
      ) : null}
    </div>
  );
}
