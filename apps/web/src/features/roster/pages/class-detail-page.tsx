import { useState } from "react";
import { useParams } from "react-router";

import { HvBadge, HvButton, HvCard } from "@/components/hv";
import { formatMoney } from "@/lib/utils";

import { ClassDialog } from "../components/class-dialog";
import { EndEnrollmentDialog } from "../components/end-enrollment-dialog";
import { EnrollStudentDialog } from "../components/enroll-student-dialog";
import { ScheduleEditor } from "../components/schedule-editor";
import { useClass } from "../hooks/use-classes";
import { useEnrollmentsList } from "../hooks/use-enrollments";
import type { Enrollment } from "../schemas/roster-schemas";

/** Class header, its `ScheduleEditor`, and the enrolled-student list with enroll/end actions. */
export function ClassDetailPage() {
  const { id } = useParams<{ id: string }>();
  const { data: klass, isPending } = useClass(id);
  const { data: enrollmentsPage } = useEnrollmentsList({ class_id: id, per_page: 100 });
  const enrollments = enrollmentsPage?.items ?? [];
  const [editOpen, setEditOpen] = useState(false);
  const [enrollOpen, setEnrollOpen] = useState(false);
  const [ending, setEnding] = useState<Enrollment | undefined>(undefined);

  if (isPending) {
    return <p className="text-[13px] text-ink-400">Đang tải…</p>;
  }

  if (!klass) {
    return <p className="text-[13px] text-ink-400">Không tìm thấy lớp.</p>;
  }

  return (
    <div className="flex flex-col gap-4">
      <HvCard className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <div className="flex items-center gap-2">
            <h1 className="font-display text-[22px] font-bold text-ink-900">{klass.name}</h1>
            <HvBadge variant={klass.status === "active" ? "success" : "neutral"}>
              {klass.status === "active" ? "Đang hoạt động" : "Đã lưu trữ"}
            </HvBadge>
          </div>
          <p className="mt-1 text-[13px] text-ink-400">
            {klass.start_date} — {klass.end_date ?? "Chưa xác định"} ·{" "}
            {formatMoney(klass.default_unit_price)}
          </p>
        </div>
        <HvButton variant="ghost" size="sm" onClick={() => setEditOpen(true)}>
          Sửa
        </HvButton>
      </HvCard>

      <HvCard className="bg-cream-100">
        <h2 className="mb-3 font-display text-[16px] font-bold text-ink-900">Lịch học</h2>
        <ScheduleEditor classId={klass.id} schedules={klass.schedules} />
      </HvCard>

      <div className="flex items-center justify-between">
        <h2 className="font-display text-[16px] font-bold text-ink-900">Học sinh trong lớp</h2>
        <HvButton size="sm" onClick={() => setEnrollOpen(true)}>
          Ghi danh học sinh
        </HvButton>
      </div>
      <div className="flex flex-col gap-2">
        {enrollments.map((enrollment) => (
          <HvCard key={enrollment.id} variant="flat" className="flex items-center justify-between">
            <div>
              <p className="font-display text-[15px] font-bold text-ink-900">
                {enrollment.student_name}
              </p>
              <p className="text-[13px] text-ink-400">
                {enrollment.started_on} — {enrollment.ended_on ?? "Đang học"}
              </p>
            </div>
            {!enrollment.ended_on ? (
              <HvButton variant="ghost" size="sm" onClick={() => setEnding(enrollment)}>
                Kết thúc ghi danh
              </HvButton>
            ) : null}
          </HvCard>
        ))}
        {enrollments.length === 0 ? (
          <p className="text-[13px] text-ink-400">Lớp chưa có học sinh nào.</p>
        ) : null}
      </div>

      <ClassDialog open={editOpen} onOpenChange={setEditOpen} klass={klass} />
      <EnrollStudentDialog
        open={enrollOpen}
        onOpenChange={setEnrollOpen}
        mode="class"
        classId={klass.id}
      />
      {ending ? (
        <EndEnrollmentDialog
          open={Boolean(ending)}
          onOpenChange={(open) => {
            if (!open) {
              setEnding(undefined);
            }
          }}
          enrollment={ending}
        />
      ) : null}
    </div>
  );
}
