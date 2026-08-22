export interface ClassStat {
  label: string;
  value: string;
  sub: string;
}

/** The classbook's five-stat strip — label / Baloo value / sub line per card. */
export function ClassStatCards({ stats }: { stats: ClassStat[] }) {
  return (
    <div className="grid grid-cols-2 gap-3 md:grid-cols-3 xl:grid-cols-5">
      {stats.map((stat) => (
        <div key={stat.label} className="rounded-[20px] bg-white px-4 py-[14px] shadow-soft-md">
          <div className="text-[11.5px] font-extrabold tracking-[0.3px] text-ink-400">
            {stat.label}
          </div>
          <div className="mt-0.5 font-display text-[22px] font-extrabold text-ink-900">
            {stat.value}
          </div>
          <div className="text-[12px] text-ink-500">{stat.sub}</div>
        </div>
      ))}
    </div>
  );
}
