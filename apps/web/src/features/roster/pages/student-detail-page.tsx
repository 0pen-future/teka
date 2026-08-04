import { useState } from "react";
import { Link, useNavigate, useParams } from "react-router";

import { HvBadge, HvButton, HvCard } from "@/components/hv";
import { formatMoney, formatPhoneLocal } from "@/lib/utils";

import { AnonymizeStudentDialog } from "../components/anonymize-student-dialog";
import { EndEnrollmentDialog } from "../components/end-enrollment-dialog";
import { EnrollStudentDialog } from "../components/enroll-student-dialog";
import { StudentDialog } from "../components/student-dialog";
import { useEnrollmentsList } from "../hooks/use-enrollments";
import { useStudent } from "../hooks/use-students";
import type { Enrollment } from "../schemas/roster-schemas";

/** Student header, its owning contact, and the enrollment list with enroll/end actions. */
export function StudentDetailPage() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const { data: student, isPending } = useStudent(id);
  const { data: enrollmentsPage } = useEnrollmentsList({ student_id: id, per_page: 50 });
  const enrollments = enrollmentsPage?.items ?? [];
  const [editOpen, setEditOpen] = useState(false);
  const [anonymizeOpen, setAnonymizeOpen] = useState(false);
  const [enrollOpen, setEnrollOpen] = useState(false);
  const [ending, setEnding] = useState<Enrollment | undefined>(undefined);

  if (isPending) {
    return <p className="text-[13px] text-ink-400">Đang tải…</p>;
  }

  if (!student) {
    return <p className="text-[13px] text-ink-400">Không tìm thấy học sinh.</p>;
  }

  return (
    <div className="flex flex-col gap-4">
      <HvCard className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <div className="flex items-center gap-2">
            <h1 className="font-display text-[22px] font-bold text-ink-900">{student.full_name}</h1>
            {student.display_note ? <HvBadge variant="info">{student.display_note}</HvBadge> : null}
          </div>
          <Link
            to={`/contacts/${student.contact_id}`}
            className="mt-1 inline-block text-[14px] text-ink-500 hover:text-mint-600"
          >
            {student.contact_name}
          </Link>
          {" · "}
          <a
            href={`tel:${student.contact_phone}`}
            className="text-[14px] font-bold text-mint-600 underline-offset-4 hover:underline"
          >
            {formatPhoneLocal(student.contact_phone)}
          </a>
        </div>
        <div className="flex gap-2">
          <HvButton variant="ghost" size="sm" onClick={() => setEditOpen(true)}>
            Sửa
          </HvButton>
          <HvButton variant="danger" size="sm" onClick={() => setAnonymizeOpen(true)}>
            Xoá
          </HvButton>
        </div>
      </HvCard>

      <div className="flex items-center justify-between">
        <h2 className="font-display text-[16px] font-bold text-ink-900">Ghi danh</h2>
        <HvButton size="sm" onClick={() => setEnrollOpen(true)}>
          Ghi danh vào lớp
        </HvButton>
      </div>
      <div className="flex flex-col gap-2">
        {enrollments.map((enrollment) => (
          <HvCard key={enrollment.id} variant="flat" className="flex items-center justify-between">
            <div>
              <p className="font-display text-[15px] font-bold text-ink-900">
                {enrollment.class_name}
              </p>
              <p className="text-[13px] text-ink-400">
                {enrollment.started_on} — {enrollment.ended_on ?? "Đang học"} ·{" "}
                {formatMoney(enrollment.unit_price)}
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
          <p className="text-[13px] text-ink-400">Học sinh chưa được ghi danh vào lớp nào.</p>
        ) : null}
      </div>

      <StudentDialog open={editOpen} onOpenChange={setEditOpen} student={student} />
      <AnonymizeStudentDialog
        open={anonymizeOpen}
        onOpenChange={setAnonymizeOpen}
        student={student}
        onSuccess={() => void navigate("/students", { replace: true })}
      />
      <EnrollStudentDialog
        open={enrollOpen}
        onOpenChange={setEnrollOpen}
        mode="student"
        studentId={student.id}
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
