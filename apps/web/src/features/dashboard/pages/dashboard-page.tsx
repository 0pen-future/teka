import { PageHeader } from "@/components/shared/page-header";
import { useAuthStore } from "@/features/auth";

import { PendingAttendanceAlert } from "../components/pending-attendance-alert";
import { PeriodStatusCard } from "../components/period-status-card";

export function DashboardPage() {
  const teacher = useAuthStore((state) => state.user);
  // Computed at render, not module load, so an SPA session left open across
  // midnight doesn't keep showing yesterday's date.
  const todayLabel = new Intl.DateTimeFormat("vi-VN", {
    weekday: "long",
    day: "2-digit",
    month: "2-digit",
    year: "numeric",
  }).format(new Date());

  return (
    // Order matters: the pending-attendance alert must render right after
    // the header, without scrolling, per the unattended-session warning
    // requirement (PRD R2 AC 3).
    <div className="flex flex-col gap-6">
      <PageHeader
        title={teacher ? `Chào ${teacher.full_name} 👋` : "Chào mừng"}
        description={todayLabel}
      />
      <PendingAttendanceAlert />
      <PeriodStatusCard />
    </div>
  );
}
