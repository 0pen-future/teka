import { useState } from "react";
import { Link } from "react-router";

import { HvBadge, HvButton } from "@/components/hv";

import { useClassInvitations } from "../hooks/use-class-invitations";
import { useClassStaff } from "../hooks/use-class-staff";
import type { Class, ClassInvitation } from "../schemas/roster-schemas";
import { InviteTeacherDialog } from "./invite-teacher-dialog";
import { SectionCard, StaffList } from "./section-card";

/** Stage copy for the invitations the owner still has in flight. */
const OPEN_STAGE: Partial<Record<ClassInvitation["status"], string>> = {
  pending: "Chờ nhận",
  accepted: "Đã đồng ý — chờ phân công",
};

/**
 * "Đội ngũ giảng dạy" on the class detail: who works the class now, and —
 * for the owner — who has been invited and where each invite stands. The
 * invitation list is owner-only: the API scopes a member's list to their
 * own invitations, which belong on the "Lời mời nhận lớp" page, not here.
 */
export function ClassTeamSection({ klass, isOwner }: { klass: Class; isOwner: boolean }) {
  const [inviting, setInviting] = useState(false);
  const staff = useClassStaff(klass.id);
  const invitations = useClassInvitations({ class_id: klass.id }, isOwner);
  const activeStaff = (staff.data ?? []).filter((item) => item.ended_at === null);
  const open = (invitations.data ?? []).filter((item) => item.status in OPEN_STAGE);

  return (
    <SectionCard
      title="Đội ngũ giảng dạy"
      action={
        isOwner ? (
          <HvButton size="sm" variant="secondary" onClick={() => setInviting(true)}>
            + Mời GV
          </HvButton>
        ) : undefined
      }
    >
      <StaffList
        staff={activeStaff}
        isPending={staff.isPending}
        isError={staff.isError}
        emptyText="Lớp chưa có giáo viên."
      />

      {isOwner ? (
        <div className="mt-4 border-t border-line-100 pt-3">
          <div className="flex items-center justify-between gap-2">
            <p className="text-[13px] font-bold text-ink-700">Lời mời đang mở</p>
            <Link
              to="/class-invitations"
              className="text-[13px] font-bold text-mint-600 hover:underline"
            >
              Xem lời mời
            </Link>
          </div>
          {invitations.isError ? (
            <p className="mt-1 text-[13px] text-coral-600">Không tải được lời mời.</p>
          ) : open.length === 0 ? (
            <p className="mt-1 text-[13px] text-ink-400">
              {invitations.isPending ? "Đang tải…" : "Không có lời mời nào đang mở."}
            </p>
          ) : (
            <ul className="mt-1 flex flex-col gap-2">
              {open.map((item) => (
                <li
                  key={item.id}
                  aria-label={`${item.teacher_name} — ${item.role_label}, ${OPEN_STAGE[item.status]}`}
                  className="flex flex-wrap items-center justify-between gap-2 text-[14px]"
                >
                  <span>
                    <span className="font-bold text-ink-900">{item.teacher_name}</span>
                    <span className="text-ink-500"> · {item.role_label}</span>
                  </span>
                  <HvBadge variant={item.status === "pending" ? "warning" : "info"} size="sm">
                    {OPEN_STAGE[item.status]}
                  </HvBadge>
                </li>
              ))}
            </ul>
          )}
        </div>
      ) : null}

      {isOwner && inviting ? (
        <InviteTeacherDialog
          klass={klass}
          activeTeacherIds={activeStaff.map((item) => item.teacher_id)}
          onOpenChange={setInviting}
        />
      ) : null}
    </SectionCard>
  );
}
