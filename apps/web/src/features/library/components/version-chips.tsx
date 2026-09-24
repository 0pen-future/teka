import { HvButton, HvChip } from "@/components/hv";

import { versionLabel, versionStatusVariant } from "../lib/library-labels";
import type { TemplateVersion } from "../schemas/library-schemas";

interface VersionChipsProps {
  versions: TemplateVersion[];
  selectedId: string | undefined;
  canEdit: boolean;
  hasDraft: boolean;
  onSelect: (versionNo: number) => void;
  onNewDraft: () => void;
}

/** Chip row that picks the version the tabs below read; the newest version leads. */
export function VersionChips({
  versions,
  selectedId,
  canEdit,
  hasDraft,
  onSelect,
  onNewDraft,
}: VersionChipsProps) {
  const ordered = [...versions].sort((a, b) => b.version_no - a.version_no);
  return (
    <div className="flex flex-wrap items-center gap-3 rounded-[var(--radius-lg)] border border-line-200 bg-white p-3">
      <span className="text-[12px] font-extrabold uppercase tracking-[0.4px] text-ink-500">
        Phiên bản
      </span>
      <div role="radiogroup" aria-label="Phiên bản" className="flex flex-wrap gap-2">
        {ordered.map((version) => (
          <HvChip
            key={version.id}
            role="radio"
            pressed={version.id === selectedId}
            dot={versionStatusVariant[version.status]}
            onClick={() => onSelect(version.version_no)}
          >
            {versionLabel(version)}
          </HvChip>
        ))}
      </div>
      {canEdit && !hasDraft ? (
        <HvButton size="sm" variant="secondary" className="ml-auto" onClick={onNewDraft}>
          + Bản nháp mới
        </HvButton>
      ) : null}
    </div>
  );
}
