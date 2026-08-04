import { useState } from "react";
import { Link } from "react-router";

import { HvBadge, HvButton, HvCard } from "@/components/hv";
import { formatMoney } from "@/lib/utils";

import { ClassDialog } from "../components/class-dialog";
import { useClassesList } from "../hooks/use-classes";
import { useStudentsList } from "../hooks/use-students";
import { formatWeekday } from "../lib/roster-format";

function ClassStudentCount({ classId }: { classId: string }) {
  const { data } = useStudentsList({ class_id: classId, per_page: 1 });
  return <span>{data?.meta.total ?? 0} học sinh</span>;
}

/** Off-nav class list, reachable from the students screen's "Quản lý lớp" link. */
export function ClassesPage() {
  const { data, isPending } = useClassesList({ status: "all" });
  const classes = data?.items ?? [];
  const [dialogOpen, setDialogOpen] = useState(false);

  const sorted = [...classes].sort((a, b) => {
    if (a.status !== b.status) {
      return a.status === "active" ? -1 : 1;
    }
    return a.name.localeCompare(b.name, "vi");
  });

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h1 className="font-display text-[22px] font-bold text-ink-900">Quản lý lớp</h1>
        <HvButton onClick={() => setDialogOpen(true)}>Thêm lớp</HvButton>
      </div>
      {isPending ? <p className="text-[13px] text-ink-400">Đang tải…</p> : null}
      {!isPending && sorted.length === 0 ? (
        <HvCard variant="flat" className="text-center text-[13px] text-ink-400">
          Chưa có lớp nào.
        </HvCard>
      ) : null}
      <div className="flex flex-col gap-2">
        {sorted.map((klass) => (
          <Link key={klass.id} to={`/classes/${klass.id}`}>
            <HvCard variant="flat" interactive className="flex items-center justify-between gap-3">
              <div>
                <div className="flex items-center gap-2">
                  <p className="font-display text-[15px] font-bold text-ink-900">{klass.name}</p>
                  <HvBadge variant={klass.status === "active" ? "success" : "neutral"} size="sm">
                    {klass.status === "active" ? "Đang hoạt động" : "Đã lưu trữ"}
                  </HvBadge>
                </div>
                <p className="text-[13px] text-ink-400">
                  {klass.schedules
                    .map((schedule) => formatWeekday(schedule.weekday, { short: true }))
                    .join(", ") || "Chưa có lịch học"}
                  {" · "}
                  {formatMoney(klass.default_unit_price)}
                </p>
              </div>
              <ClassStudentCount classId={klass.id} />
            </HvCard>
          </Link>
        ))}
      </div>
      <ClassDialog open={dialogOpen} onOpenChange={setDialogOpen} />
    </div>
  );
}
