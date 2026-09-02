import { HvBadge, HvButton, HvCard } from "@/components/hv";

import type { ScoreSet } from "../schemas/grading";
import { ScoreSetPreviewStrip } from "./score-set-preview-strip";

export interface ScoreSetCardProps {
  scoreSet: ScoreSet;
  onEdit: () => void;
  onDelete: () => void;
}

/** One bộ điểm in the config list: name, column chips in order, edit/delete. */
export function ScoreSetCard({ scoreSet, onEdit, onDelete }: ScoreSetCardProps) {
  return (
    <li>
      <HvCard variant="raised" padding="md" className="flex flex-col gap-3">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <div className="flex items-center gap-2">
            <p className="text-[14.5px] font-bold text-ink-900">{scoreSet.name}</p>
            <HvBadge variant="neutral">{scoreSet.components.length} cột</HvBadge>
          </div>
          <div className="flex items-center gap-1">
            <HvButton size="sm" variant="ghost" onClick={onEdit}>
              Sửa
            </HvButton>
            <HvButton size="sm" variant="ghost" className="text-coral-500" onClick={onDelete}>
              Xóa
            </HvButton>
          </div>
        </div>
        <ScoreSetPreviewStrip names={scoreSet.components} />
      </HvCard>
    </li>
  );
}
