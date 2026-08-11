import { Link } from "react-router";

import { HvBadge, ProgressBar } from "@/components/hv";
import { Spinner } from "@/components/shared/spinner";
import type { Session } from "@/features/attendance";
import { useCurrentPeriod } from "@/features/billing";
import { formatScheduleSummary, useClassesList, type Class } from "@/features/roster";
import { cn, formatMoney } from "@/lib/utils";

import { useClassPeriodSessions, useClassStudentCounts } from "../hooks/use-dashboard";

interface ClassOverviewCardProps {
  cls: Class;
  /** This class's taught sessions this period; undefined while loading. */
  sessions: Session[] | undefined;
  /** The sessions fetch failed — render "unknown", not a fake 0/0. */
  sessionsFailed: boolean;
  /** Enrolled headcount; undefined while loading or on failure. */
  studentCount: number | undefined;
}

function ClassOverviewCard({
  cls,
  sessions,
  sessionsFailed,
  studentCount,
}: ClassOverviewCardProps) {
  const countable = (sessions ?? []).filter((session) => session.status !== "cancelled");
  const confirmed = countable.filter((session) => session.attendance_confirmed_at).length;
  const isNew = sessions != null && countable.length === 0;
  const isFull = countable.length > 0 && confirmed === countable.length;

  // A class with no sessions this period has nothing to attend — its card
  // opens the roster instead, per the prototype's `Lớp mới` branch.
  const to = isNew ? `/students?class_id=${cls.id}` : `/sessions?class_id=${cls.id}`;

  return (
    <Link
      to={to}
      className={cn(
        "block rounded-[var(--radius-xl)] bg-white px-5 py-[18px] shadow-soft-md",
        "transition-shadow duration-[var(--dur-fast)] ease-[var(--ease-out)] hover:shadow-soft-lg",
        "focus-visible:outline-none focus-visible:ring-4",
      )}
    >
      <div className="flex items-center justify-between gap-2">
        <p className="font-display text-[18px] font-bold text-ink-900">{cls.name}</p>
        {sessions != null && !sessionsFailed ? (
          <HvBadge size="sm" variant={isNew ? "info" : isFull ? "success" : "danger"}>
            {isNew ? "Lớp mới" : isFull ? "Đủ điểm danh" : `Thiếu ${countable.length - confirmed}`}
          </HvBadge>
        ) : null}
      </div>
      <p className="mt-[2px] text-[13px] text-ink-500">
        {formatScheduleSummary(cls.schedules, new Date().toISOString().slice(0, 10))} ·{" "}
        {formatMoney(cls.default_unit_price)}/buổi
      </p>
      <div className="mt-3 flex gap-[14px] text-[13.5px] text-ink-700">
        <p>
          <b className="font-bold text-ink-900">{studentCount ?? "…"}</b> học sinh
        </p>
        {sessionsFailed ? (
          <p className="font-semibold text-coral-600">Không tải được buổi học</p>
        ) : (
          <p>
            <b className="font-bold text-ink-900">
              {confirmed}/{countable.length}
            </b>{" "}
            buổi đã điểm danh
          </p>
        )}
      </div>
      <ProgressBar
        className="mt-[10px]"
        size="sm"
        color={isFull || isNew ? "mint" : "missing"}
        value={countable.length > 0 ? (confirmed / countable.length) * 100 : 0}
      />
    </Link>
  );
}

/** The prototype `home` screen's "Lớp của bạn" grid — one progress card per active class. */
export function ClassOverviewCards({ className }: { className?: string }) {
  const { data: period } = useCurrentPeriod();
  const {
    data: classesPage,
    isPending,
    isError,
  } = useClassesList({ status: "active", per_page: 100 });
  const classes = classesPage?.items ?? [];
  const sessionQueries = useClassPeriodSessions(classes, period);
  const countQueries = useClassStudentCounts(classes);

  return (
    <section className={className}>
      <h2 className="font-display text-[19px] font-bold text-ink-900">Lớp của bạn</h2>
      {isPending ? (
        <div className="mt-3 flex justify-center py-6">
          <Spinner className="size-5" />
        </div>
      ) : isError ? (
        <p className="mt-3 text-[14px] font-semibold text-coral-600">
          Không tải được danh sách lớp
        </p>
      ) : classes.length === 0 ? (
        <p className="mt-3 text-[14px] text-ink-500">
          Chưa có lớp nào — tạo lớp đầu tiên ở mục "Lớp & học sinh".
        </p>
      ) : (
        <div className="mt-3 grid grid-cols-[repeat(auto-fill,minmax(280px,1fr))] gap-[14px]">
          {classes.map((cls, index) => (
            <ClassOverviewCard
              key={cls.id}
              cls={cls}
              sessions={sessionQueries[index]?.data}
              sessionsFailed={sessionQueries[index]?.isError ?? false}
              studentCount={countQueries[index]?.data}
            />
          ))}
        </div>
      )}
    </section>
  );
}
