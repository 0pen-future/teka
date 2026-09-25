import { HvButton, HvModal, hvToast } from "@/components/hv";
import { ApiError } from "@/lib/api/errors";

import { useConfirmClassInvitation } from "../hooks/use-class-invitations";
import { useClassStaff } from "../hooks/use-class-staff";
import type { ClassInvitation } from "../schemas/roster-schemas";

const STAFF_ROLE_GIAO_VIEN = "giao_vien";

/**
 * Owner's confirm step. A giao_vien invite is a handoff: the copy names
 * the teacher being replaced (read from the class's active stint) and the
 * toast reports the planned sessions the API moved; any other role is a
 * plain staff assignment.
 */
export function ConfirmInvitationDialog({
  invitation,
  onOpenChange,
}: {
  invitation: ClassInvitation;
  onOpenChange: (open: boolean) => void;
}) {
  const isHandoff = invitation.role_key === STAFF_ROLE_GIAO_VIEN;
  const staff = useClassStaff(invitation.class_id, isHandoff);
  const confirm = useConfirmClassInvitation();
  const current = (staff.data ?? []).find(
    (item) => item.role_key === STAFF_ROLE_GIAO_VIEN && item.ended_at === null,
  );
  // A handoff must name who is being replaced before it runs, so the button
  // waits for the staff list and stays off when it cannot be loaded.
  const handoffUnknown = isHandoff && (staff.isPending || staff.isError);
  const errorMessage = confirm.error
    ? confirm.error instanceof ApiError
      ? confirm.error.message
      : "Không phân công được. Thử lại sau."
    : null;

  function handleConfirm() {
    confirm.mutate(invitation.id, {
      onSuccess: (result) => {
        hvToast(
          isHandoff
            ? `${result.teacher_name} đã nhận lớp ${result.class_name} · chuyển ${result.moved_planned_sessions} buổi sắp tới`
            : `Đã phân công ${result.teacher_name} làm ${result.role_label.toLowerCase()} lớp ${result.class_name}`,
        );
        onOpenChange(false);
      },
    });
  }

  return (
    <HvModal
      open
      onOpenChange={(open) => {
        if (confirm.isPending && !open) return;
        onOpenChange(open);
      }}
      title={
        isHandoff
          ? `${invitation.teacher_name} nhận lớp ${invitation.class_name}`
          : `Phân công ${invitation.teacher_name} làm ${invitation.role_label.toLowerCase()}`
      }
      footer={
        <>
          <HvButton
            type="button"
            variant="ghost"
            size="sm"
            onClick={() => onOpenChange(false)}
            disabled={confirm.isPending}
          >
            Huỷ
          </HvButton>
          <HvButton
            type="button"
            size="sm"
            onClick={handleConfirm}
            disabled={confirm.isPending || handoffUnknown}
          >
            {confirm.isPending ? "Đang phân công…" : "Xác nhận"}
          </HvButton>
        </>
      }
    >
      {isHandoff ? (
        staff.isPending ? (
          <p className="text-ink-400">Đang tải giáo viên hiện tại…</p>
        ) : staff.isError ? (
          <p className="text-coral-600">
            Không tải được giáo viên hiện tại của lớp.{" "}
            <button
              type="button"
              className="font-bold underline"
              onClick={() => void staff.refetch()}
            >
              Thử lại
            </button>
          </p>
        ) : current && current.teacher_id !== invitation.teacher_id ? (
          <p>
            <strong>{invitation.teacher_name}</strong> sẽ làm giáo viên chính thay{" "}
            {current.teacher_name}. Các buổi sắp tới của lớp chuyển sang giáo viên mới.
          </p>
        ) : (
          <p>
            <strong>{invitation.teacher_name}</strong> sẽ làm giáo viên chính của lớp. Các buổi sắp
            tới của lớp chuyển sang giáo viên mới.
          </p>
        )
      ) : (
        <p>
          <strong>{invitation.teacher_name}</strong> sẽ được gán vai trò{" "}
          {invitation.role_label.toLowerCase()} cho lớp {invitation.class_name} ngay bây giờ.
        </p>
      )}
      {invitation.status === "pending" ? (
        <p className="mt-2 text-[13px] text-ink-500">
          Người được mời chưa trả lời — phân công ngay sẽ bỏ qua bước chấp nhận.
        </p>
      ) : null}
      {errorMessage ? <p className="mt-2 text-[13px] text-coral-600">{errorMessage}</p> : null}
    </HvModal>
  );
}
