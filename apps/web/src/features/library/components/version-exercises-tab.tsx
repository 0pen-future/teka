import { SearchIcon } from "lucide-react";

import { HvStateBlock } from "@/components/hv";
import { Input } from "@/components/ui/input";
import { cn } from "@/lib/utils";

import { useVersionDetail } from "../hooks/use-library";
import { useSearch } from "../hooks/use-search";
import { CopyCodeButton } from "./copy-code-button";
import type { TemplateVersion } from "../schemas/library-schemas";

const headCellClassName =
  "sticky top-0 z-10 bg-cream-200 px-[18px] py-[10px] text-[12px] font-extrabold uppercase tracking-[0.4px] text-ink-500";
const cellClassName = "border-t border-line-100 px-[18px] py-[11px] align-middle";

interface AggregatedExercise {
  id: string;
  code: string;
  title: string;
  skill: string | null;
  level: string | null;
  lessonTitles: string[];
}

interface VersionExercisesTabProps {
  version: TemplateVersion;
}

/** Read-only roll-up of every exercise used by the version's lessons, deduped, with usage counts. */
export function VersionExercisesTab({ version }: VersionExercisesTabProps) {
  const detail = useVersionDetail(version.id);
  const { query, setQuery, q } = useSearch();

  if (detail.isPending) return <HvStateBlock state="loading" title="Đang tải bài tập" />;
  if (detail.isError) return <HvStateBlock state="error" title="Không tải được bài tập" />;

  const byId = new Map<string, AggregatedExercise>();
  for (const lesson of detail.data.lessons) {
    for (const exercise of lesson.exercises) {
      const row =
        byId.get(exercise.id) ??
        ({
          id: exercise.id,
          code: exercise.code,
          title: exercise.title,
          skill: exercise.skill,
          level: exercise.level,
          lessonTitles: [],
        } satisfies AggregatedExercise);
      row.lessonTitles.push(lesson.title);
      byId.set(exercise.id, row);
    }
  }
  const rows = [...byId.values()].sort((a, b) => a.title.localeCompare(b.title));
  const filtered =
    q === ""
      ? rows
      : rows.filter(
          (row) =>
            row.title.toLowerCase().includes(q.toLowerCase()) ||
            row.code.toLowerCase().includes(q.toLowerCase()),
        );

  return (
    <div className="flex flex-col gap-3">
      <div className="relative w-[260px]">
        <SearchIcon
          aria-hidden="true"
          className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-ink-400"
        />
        <Input
          type="search"
          aria-label="Tìm bài tập theo tên hoặc mã…"
          placeholder="Tìm bài tập theo tên hoặc mã…"
          value={query}
          onChange={(event) => setQuery(event.target.value)}
          className="pl-9"
        />
      </div>
      {filtered.length === 0 ? (
        <HvStateBlock state="empty" title="Không có bài tập nào khớp." />
      ) : (
        <div className="overflow-x-auto rounded-[var(--radius-lg)] border border-line-200 bg-white">
          <table className="w-full min-w-[720px] border-collapse text-left text-[14px]">
            <thead>
              <tr>
                <th className={cn(headCellClassName, "w-[64px]")}>STT</th>
                <th className={headCellClassName}>Mã</th>
                <th className={headCellClassName}>Tiêu đề bài tập</th>
                <th className={headCellClassName}>Kỹ năng</th>
                <th className={headCellClassName}>Cấp độ</th>
                <th className={headCellClassName}>Dùng trong</th>
              </tr>
            </thead>
            <tbody>
              {filtered.map((row, index) => (
                <tr key={row.id} className="border-t border-line-100">
                  <td className={cn(cellClassName, "font-mono text-[13px] text-ink-500")}>
                    {index + 1}
                  </td>
                  <td className={cellClassName}>
                    <CopyCodeButton code={row.code} />
                  </td>
                  <td className={cn(cellClassName, "font-bold text-ink-900")}>{row.title}</td>
                  <td className={cn(cellClassName, "text-ink-700")}>{row.skill ?? "—"}</td>
                  <td className={cn(cellClassName, "text-ink-700")}>{row.level ?? "—"}</td>
                  <td
                    className={cn(cellClassName, "text-ink-700")}
                    title={row.lessonTitles.join(", ")}
                  >
                    {row.lessonTitles.length} buổi
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
