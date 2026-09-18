import type { ColumnColor } from "../schemas/task-schemas";
import { COLUMN_COLORS } from "../schemas/task-schemas";

/** Display order for the color picker's swatches; mirrors `COLUMN_COLORS`. */
export const COLUMN_COLOR_ORDER: readonly ColumnColor[] = COLUMN_COLORS;

/**
 * Column body background per tint. `ink-500` on each of these clears WCAG
 * 2.1 AA (>=4.5:1) for normal text — verified in the plan's validation log.
 * "none" is one step darker than the page (`cream-100`) so the column reads
 * as a tray on it.
 */
export const COLUMN_TINT: Record<ColumnColor, string> = {
  none: "bg-cream-200",
  sky: "bg-sky-50",
  sun: "bg-sun-100",
  mint: "bg-mint-50",
};

/**
 * A column's body tint. An uncolored "done" column falls back to mint so the
 * finished lane reads as such without the owner having to pick a color.
 */
export function columnTintClassName(column: { color: ColumnColor; isDone: boolean }): string {
  return column.isDone && column.color === "none" ? COLUMN_TINT.mint : COLUMN_TINT[column.color];
}

/** Swatch dot per tint for the color picker — a saturated step up from `COLUMN_TINT` so it reads on white. */
export const COLUMN_DOT: Record<ColumnColor, string> = {
  none: "bg-ink-500",
  sky: "bg-sky-500",
  sun: "bg-sun-600",
  mint: "bg-mint-600",
};

export const COLUMN_COLOR_LABELS: Record<ColumnColor, string> = {
  none: "Không màu",
  sky: "Xanh dương",
  sun: "Vàng nắng",
  mint: "Xanh bạc hà",
};
