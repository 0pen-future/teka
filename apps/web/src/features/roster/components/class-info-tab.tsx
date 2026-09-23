import { Link } from "react-router";

import { useClassStaff } from "../hooks/use-class-staff";
import { formatScheduleLabel, formatWeekday } from "../lib/roster-format";
import { activeSchedules } from "../lib/schedule-diff";
import type { Class } from "../schemas/roster-schemas";
import { ClassOpsCard } from "./class-ops-card";
import { ClassTeamSection } from "./class-team-section";
import { SectionCard, StaffList } from "./section-card";

interface ClassInfoTabProps {
  klass: Class;
  today: string;
  canWrite: boolean;
  isOwner: boolean;
}

function endTime(start: string, durationMin: number): string {
  const [hours = 0, minutes = 0] = start.split(":").map(Number);
  const total = hours * 60 + minutes + durationMin;
  const h = Math.floor(total / 60) % 24;
  const m = total % 60;
  return `${String(h).padStart(2, "0")}:${String(m).padStart(2, "0")}`;
}

/** The "Thông tin" tab: schedule, ops card, teaching team, shortcuts and the later-phase placeholders. */
export function ClassInfoTab({ klass, today, canWrite, isOwner }: ClassInfoTabProps) {
  const staff = useClassStaff(klass.id);
  const hocVu = (staff.data ?? []).filter(
    (item) => item.ended_at === null && item.role_key === "hoc_vu",
  );
  const schedules = activeSchedules(klass.schedules, today).sort(
    (a, b) =>
      mondayFirst(a.weekday) - mondayFirst(b.weekday) || a.start_time.localeCompare(b.start_time),
  );

  return (
    <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
      <SectionCard title="Lịch học hàng tuần">
        {schedules.length === 0 ? (
          <p className="text-[13px] text-ink-400">Lớp chưa có lịch học hàng tuần.</p>
        ) : (
          <>
            <p className="font-display text-[20px] font-extrabold text-ink-900">
              {formatScheduleLabel(schedules, today)}
            </p>
            <ul className="mt-2 flex flex-col gap-2">
              {schedules.map((schedule) => (
                <li
                  key={schedule.id}
                  className="flex items-center justify-between gap-2 text-[14px]"
                >
                  <span className="font-bold text-ink-900">{formatWeekday(schedule.weekday)}</span>
                  <span className="text-ink-500">
                    {schedule.start_time}–{endTime(schedule.start_time, schedule.duration_min)} ·{" "}
                    {schedule.duration_min} phút
                  </span>
                </li>
              ))}
            </ul>
          </>
        )}
      </SectionCard>

      <ClassOpsCard klass={klass} canWrite={canWrite} />

      <ClassTeamSection klass={klass} isOwner={isOwner} />

      <SectionCard title="Nhân viên phụ trách">
        <StaffList
          staff={hocVu}
          isPending={staff.isPending}
          isError={staff.isError}
          emptyText="Chưa gán học vụ cho lớp."
        />
      </SectionCard>

      <SectionCard title="Lối tắt">
        <ul className="flex flex-col gap-2 text-[14px]">
          <li>
            <ShortcutLink to={`/classbook?class_id=${klass.id}`}>Sổ đầu bài</ShortcutLink>
          </li>
          <li>
            <ShortcutLink to={`/records?class_id=${klass.id}`}>Hồ sơ học sinh</ShortcutLink>
          </li>
          <li>
            <ShortcutLink to="/sessions">Điểm danh</ShortcutLink>
          </li>
          {canWrite ? (
            <li>
              <ShortcutLink to={`/classes/${klass.id}/settings`}>Thiết lập lớp</ShortcutLink>
            </li>
          ) : null}
          {isOwner ? (
            <li>
              <ShortcutLink to="/center/class-config">Cấu hình lớp học</ShortcutLink>
            </li>
          ) : null}
        </ul>
      </SectionCard>

      <SectionCard title="Chương trình học">
        <p className="text-[13px] text-ink-400">Lớp chưa có buổi học trong chương trình.</p>
      </SectionCard>

      <SectionCard title="Lịch sử lớp">
        <p className="text-[13px] text-ink-400">Chưa có sự kiện nào được ghi lại.</p>
      </SectionCard>
    </div>
  );
}

function ShortcutLink({ to, children }: { to: string; children: React.ReactNode }) {
  return (
    <Link to={to} className="font-display font-bold text-mint-600 hover:underline">
      {children}
    </Link>
  );
}

/** Monday-first ordering for weekday 0 = Sunday. */
function mondayFirst(weekday: number): number {
  return weekday === 0 ? 7 : weekday;
}
