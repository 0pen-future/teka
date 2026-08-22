import { useAuthStore } from "@/features/auth";

import { ClassOverviewCards } from "../components/class-overview-cards";
import { DashboardStats } from "../components/dashboard-stats";
import { PendingAttendanceAlert } from "../components/pending-attendance-alert";

/** Time-of-day salutation, prototype-style ("Chào buổi tối, …!"). */
function greetingLabel(hour: number): string {
  if (hour < 11) {
    return "Chào buổi sáng";
  }
  if (hour < 13) {
    return "Chào buổi trưa";
  }
  if (hour < 18) {
    return "Chào buổi chiều";
  }
  return "Chào buổi tối";
}

/** The prototype's "Tổng quan" screen: greeting, pending-attendance banner, stats, class grid. */
export function DashboardPage() {
  const teacher = useAuthStore((state) => state.user);
  // Computed at render, not module load, so an SPA session left open across
  // midnight doesn't keep showing yesterday's date.
  const now = new Date();
  const todayLabel = new Intl.DateTimeFormat("vi-VN", {
    weekday: "long",
    day: "2-digit",
    month: "2-digit",
    year: "numeric",
  }).format(now);

  return (
    <div>
      <div className="flex flex-wrap items-baseline gap-3">
        <h1 className="font-display text-[28px] font-extrabold text-ink-900">
          {teacher ? `${greetingLabel(now.getHours())}, ${teacher.full_name}!` : "Chào mừng!"}
        </h1>
        <p className="text-[14px] text-ink-400">{todayLabel}</p>
      </div>
      {/* Order matters: the pending-attendance alert must render right after
          the header, without scrolling, per the unattended-session warning
          requirement (PRD R2 AC 3). */}
      <PendingAttendanceAlert className="mt-[18px]" />
      <DashboardStats className="mt-[18px]" />
      <ClassOverviewCards className="mt-[26px]" />
    </div>
  );
}
