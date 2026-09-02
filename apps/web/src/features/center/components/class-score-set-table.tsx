import { HvButton } from "@/components/hv";

export interface ClassScoreSetRow {
  classId: string;
  className: string;
}

export interface ClassScoreSetTableProps {
  rows: ClassScoreSetRow[];
  /** False until the center has at least one bộ điểm to assign. */
  canAssign: boolean;
  onAssign: (row: ClassScoreSetRow) => void;
}

/**
 * Class list for assignment: a real `<table>` from `md` up and a card list
 * below it. Both markups render every row; CSS decides which one shows, so
 * tests scope by `data-testid`. There is no "assigned set" column on
 * purpose — the API only exposes per-class score components, and fetching
 * them here would be one request per class.
 */
export function ClassScoreSetTable({ rows, canAssign, onAssign }: ClassScoreSetTableProps) {
  const assignButton = (row: ClassScoreSetRow, block: boolean) => (
    <HvButton
      size="sm"
      variant="ghost"
      block={block}
      disabled={!canAssign}
      onClick={() => onAssign(row)}
    >
      Gán bộ điểm
    </HvButton>
  );

  return (
    <>
      <div data-testid="class-score-set-table" className="mt-3 hidden md:block">
        <table className="w-full border-collapse text-[13.5px]">
          <caption className="sr-only">Danh sách lớp và bộ điểm</caption>
          <thead>
            <tr>
              <th scope="col" className="py-2 pr-3 text-left font-bold text-ink-500">
                Lớp
              </th>
              <th scope="col" className="py-2 pr-3 text-right font-bold text-ink-500">
                Bộ điểm
              </th>
            </tr>
          </thead>
          <tbody>
            {rows.map((row) => (
              <tr key={row.classId} className="border-t border-line-200">
                <td className="py-2 pr-3 text-ink-900">{row.className}</td>
                <td className="py-2 pr-3 text-right">{assignButton(row, false)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <ul data-testid="class-score-set-cards" className="mt-3 flex flex-col gap-2 md:hidden">
        {rows.map((row) => (
          <li
            key={row.classId}
            className="flex flex-col gap-2 rounded-[var(--radius-md)] border border-line-200 p-3"
          >
            <p className="text-[14px] font-bold text-ink-900">{row.className}</p>
            {assignButton(row, true)}
          </li>
        ))}
      </ul>
    </>
  );
}
