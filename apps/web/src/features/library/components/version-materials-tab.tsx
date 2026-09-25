import { SearchIcon } from "lucide-react";

import { HvStateBlock } from "@/components/hv";
import { Input } from "@/components/ui/input";
import { cn } from "@/lib/utils";

import { useVersionDetail } from "../hooks/use-library";
import { useSearch } from "../hooks/use-search";
import { materialFormatLabel, materialKindIcon, materialKindLabel } from "../lib/library-labels";
import type { MaterialKind, TemplateVersion } from "../schemas/library-schemas";

const headCellClassName =
  "sticky top-0 z-10 bg-cream-200 px-[18px] py-[10px] text-[12px] font-extrabold uppercase tracking-[0.4px] text-ink-500";
const cellClassName = "border-t border-line-100 px-[18px] py-[11px] align-middle";

interface AggregatedMaterial {
  id: string;
  kind: MaterialKind;
  title: string;
  url: string | null;
  lessonTitles: string[];
}

interface VersionMaterialsTabProps {
  version: TemplateVersion;
}

/** Read-only roll-up of every learning material used by the version's lessons, deduped, with usage counts. */
export function VersionMaterialsTab({ version }: VersionMaterialsTabProps) {
  const detail = useVersionDetail(version.id);
  const { query, setQuery, q } = useSearch();

  if (detail.isPending) return <HvStateBlock state="loading" title="Đang tải tài liệu" />;
  if (detail.isError) return <HvStateBlock state="error" title="Không tải được tài liệu" />;

  const byId = new Map<string, AggregatedMaterial>();
  for (const lesson of detail.data.lessons) {
    for (const material of lesson.materials) {
      const row =
        byId.get(material.id) ??
        ({
          id: material.id,
          kind: material.kind,
          title: material.title,
          url: material.url,
          lessonTitles: [],
        } satisfies AggregatedMaterial);
      row.lessonTitles.push(lesson.title);
      byId.set(material.id, row);
    }
  }
  const rows = [...byId.values()].sort((a, b) => a.title.localeCompare(b.title));
  const filtered =
    q === "" ? rows : rows.filter((row) => row.title.toLowerCase().includes(q.toLowerCase()));

  return (
    <div className="flex flex-col gap-3">
      <div className="relative w-[260px]">
        <SearchIcon
          aria-hidden="true"
          className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-ink-400"
        />
        <Input
          type="search"
          aria-label="Tìm tài liệu theo tên…"
          placeholder="Tìm tài liệu theo tên…"
          value={query}
          onChange={(event) => setQuery(event.target.value)}
          className="pl-9"
        />
      </div>
      {filtered.length === 0 ? (
        <HvStateBlock state="empty" title="Không có tài liệu nào khớp." />
      ) : (
        <div className="overflow-x-auto rounded-[var(--radius-lg)] border border-line-200 bg-white">
          <table className="w-full min-w-[720px] border-collapse text-left text-[14px]">
            <thead>
              <tr>
                <th className={cn(headCellClassName, "w-[64px]")}>STT</th>
                <th className={headCellClassName}>Loại</th>
                <th className={headCellClassName}>Tài liệu</th>
                <th className={headCellClassName}>Định dạng</th>
                <th className={headCellClassName}>Dùng trong</th>
              </tr>
            </thead>
            <tbody>
              {filtered.map((row, index) => {
                const Icon = materialKindIcon[row.kind];
                return (
                  <tr key={row.id} className="border-t border-line-100">
                    <td className={cn(cellClassName, "font-mono text-[13px] text-ink-500")}>
                      {index + 1}
                    </td>
                    <td className={cellClassName}>
                      <span className="inline-flex items-center gap-1.5 text-ink-700">
                        <Icon aria-hidden="true" className="size-4" />
                        {materialKindLabel[row.kind]}
                      </span>
                    </td>
                    <td className={cn(cellClassName, "font-bold text-ink-900")}>
                      {row.url ? (
                        <a
                          href={row.url}
                          target="_blank"
                          rel="noreferrer"
                          className="hover:text-mint-600"
                        >
                          {row.title}
                        </a>
                      ) : (
                        row.title
                      )}
                    </td>
                    <td className={cn(cellClassName, "text-ink-700")}>
                      {materialFormatLabel(row.url)}
                    </td>
                    <td
                      className={cn(cellClassName, "text-ink-700")}
                      title={row.lessonTitles.join(", ")}
                    >
                      {row.lessonTitles.length} buổi
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
