import { CheckIcon } from "lucide-react";
import { RadioGroup } from "radix-ui";

import { cn } from "@/lib/utils";

import { COLUMN_COLOR_LABELS, COLUMN_COLOR_ORDER, COLUMN_DOT } from "../lib/column-colors";
import type { ColumnColor } from "../schemas/task-schemas";

export interface ColumnColorPickerProps {
  value: ColumnColor;
  onValueChange: (color: ColumnColor) => void;
  "aria-label": string;
  disabled?: boolean;
}

/**
 * Four 40px swatches (the platform's minimum touch target) for a column's
 * tint. `RadioGroup.Item` gives each swatch `role="radio"`/`aria-checked`
 * for free; the selected one gets a ring plus a check mark so the state
 * isn't color-only.
 */
export function ColumnColorPicker({
  value,
  onValueChange,
  "aria-label": ariaLabel,
  disabled,
}: ColumnColorPickerProps) {
  return (
    <RadioGroup.Root
      aria-label={ariaLabel}
      orientation="horizontal"
      value={value}
      onValueChange={(next) => onValueChange(next as ColumnColor)}
      className="flex items-center gap-2"
    >
      {COLUMN_COLOR_ORDER.map((color) => (
        <RadioGroup.Item
          key={color}
          value={color}
          disabled={disabled}
          aria-label={COLUMN_COLOR_LABELS[color]}
          className={cn(
            "flex size-10 shrink-0 cursor-pointer items-center justify-center rounded-full border-2 border-white",
            "transition-shadow focus-visible:outline-none focus-visible:ring-4 focus-visible:ring-mint-100",
            "disabled:cursor-not-allowed disabled:opacity-50",
            "data-[state=checked]:ring-2 data-[state=checked]:ring-ink-900",
            COLUMN_DOT[color],
          )}
        >
          {value === color ? <CheckIcon aria-hidden className="size-4 text-white" /> : null}
        </RadioGroup.Item>
      ))}
    </RadioGroup.Root>
  );
}
