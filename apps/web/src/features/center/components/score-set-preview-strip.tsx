import { HvBadge } from "@/components/hv";

export interface ScoreSetPreviewStripProps {
  names: string[];
  size?: "sm" | "md";
}

/**
 * Header preview for a bộ điểm: the column names as chips in position
 * order, exactly as they will appear across the top of a class's score table.
 * Blank rows show as "(trống)" so a half-filled editor still previews.
 */
export function ScoreSetPreviewStrip({ names, size = "sm" }: ScoreSetPreviewStripProps) {
  return (
    <span
      role="group"
      aria-label="Xem trước tiêu đề bảng"
      className="flex flex-wrap items-center gap-1.5"
    >
      {names.map((name, index) => {
        const label = name.trim();
        return (
          <HvBadge key={index} variant="neutral" size={size}>
            {label.length > 0 ? label : "(trống)"}
          </HvBadge>
        );
      })}
    </span>
  );
}
