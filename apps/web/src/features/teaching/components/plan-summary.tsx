import type { LessonPlan } from "../lib/teaching-store";

/**
 * Read-only body of a giáo án: goal, bullet activities, BTVN box and the
 * attached file name. Shared by the classbook session detail's "Giáo án" tab
 * and the owner's review panel so the two screens can never drift apart.
 */
export function PlanSummary({ plan }: { plan: LessonPlan }) {
  return (
    <>
      <div className="mt-1 text-[13px] text-ink-500">{plan.goal}</div>
      <div className="mt-2.5 flex flex-col gap-[5px]">
        {plan.activities.map((activity) => (
          <div key={activity} className="flex gap-2 text-[13px] text-ink-700">
            <span className="font-black text-mint-500">•</span>
            <span>{activity}</span>
          </div>
        ))}
      </div>
      <div className="mt-2 rounded-xl bg-cream-100 px-3 py-2 text-[12.5px] text-ink-500">
        <b className="text-ink-700">BTVN:</b> {plan.homework}
      </div>
      {plan.fileName ? (
        <div className="mt-2 text-[12.5px] font-bold text-ink-500">📎 {plan.fileName}</div>
      ) : null}
    </>
  );
}
