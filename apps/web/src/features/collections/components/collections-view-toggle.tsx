import { cn } from "@/lib/utils";

import type { CollectionsView } from "../types/collections-types";

export interface CollectionsViewToggleProps {
  value: CollectionsView;
  onChange: (view: CollectionsView) => void;
}

const options: { value: CollectionsView; label: string }[] = [
  { value: "contact", label: "Theo phụ huynh" },
  { value: "class", label: "Theo lớp" },
];

/** Prototype `segStyle` — pill container, active segment mint-400 filled. */
export function CollectionsViewToggle({ value, onChange }: CollectionsViewToggleProps) {
  return (
    <div
      role="tablist"
      aria-label="Chế độ xem"
      className="inline-flex rounded-[var(--radius-pill)] border border-line-200 bg-white p-1"
    >
      {options.map((option) => (
        <button
          key={option.value}
          type="button"
          role="tab"
          aria-selected={value === option.value}
          onClick={() => onChange(option.value)}
          className={cn(
            "min-h-9 rounded-[var(--radius-pill)] px-4 font-display text-[14px] font-bold transition-colors",
            value === option.value ? "bg-mint-400 text-white" : "text-ink-500 hover:bg-cream-100",
          )}
        >
          {option.label}
        </button>
      ))}
    </div>
  );
}
