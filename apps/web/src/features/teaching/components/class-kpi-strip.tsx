import { cn } from "@/lib/utils";

export interface ClassKpi {
  label: string;
  value: string;
  /** Second line; hidden on phones where the strip folds to 2×2. */
  sub: string;
  tone?: "default" | "negative";
}

/** Four figures on a hairline — no cards, so the table below stays the page's one surface. */
export function ClassKpiStrip({ items }: { items: ClassKpi[] }) {
  return (
    <dl className="grid grid-cols-2 gap-x-8 gap-y-3 border-b-[1.5px] border-line-200 px-1 py-3 sm:grid-cols-4">
      {items.map((item) => (
        <div key={item.label} className="min-w-0">
          <dt className="text-[11.5px] font-extrabold tracking-[0.3px] text-ink-400">
            {item.label}
          </dt>
          <dd
            className={cn(
              "font-display text-[20px] leading-tight font-extrabold tabular-nums",
              item.tone === "negative" ? "text-coral-600" : "text-ink-900",
            )}
          >
            {item.value}
          </dd>
          <dd className="hidden text-[12px] text-ink-500 sm:block">{item.sub}</dd>
        </div>
      ))}
    </dl>
  );
}
