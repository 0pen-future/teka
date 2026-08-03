import { PageHeader } from "@/components/shared/page-header";
import { HealthCard } from "@/features/dashboard/components/health-card";

export function DashboardPage() {
  return (
    <div className="space-y-6">
      <PageHeader title="Dashboard" description="Overview of your Teka workspace." />
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
        <HealthCard />
      </div>
    </div>
  );
}
