import { HvButton, HvModal, hvToast } from "@/components/hv";

import { useLeaveCenter, useRemoveMember } from "../hooks/use-center";
import type { CenterMember } from "../schemas/center-schemas";

export interface RemoveMemberDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  member: CenterMember;
  /**
   * "remove": the owner detaches someone else — only the roster changes.
   * "leave": the caller detaches themself — their whole scope changes, so the
   * hook flushes the entire cache instead of just the roster.
   */
  mode: "remove" | "leave";
}

const COPY = {
  remove: {
    title: "Xoá thành viên",
    confirm: "Xoá khỏi trung tâm",
    pending: "Đang xoá…",
    toast: "Đã xoá thành viên",
  },
  leave: {
    title: "Rời trung tâm",
    confirm: "Rời khỏi trung tâm",
    pending: "Đang rời…",
    toast: "Bạn đã rời trung tâm",
  },
} as const;

/**
 * Both directions of the same DELETE, framed around the tenancy rule that
 * matters to the user: membership ends, but every class, student, and receipt
 * created while inside stays with the center.
 */
export function RemoveMemberDialog({ open, onOpenChange, member, mode }: RemoveMemberDialogProps) {
  const removeMutation = useRemoveMember();
  const leaveMutation = useLeaveCenter();
  const mutation = mode === "remove" ? removeMutation : leaveMutation;
  const copy = COPY[mode];

  function handleConfirm() {
    mutation.mutate(member.id, {
      onSuccess: () => {
        hvToast(copy.toast, { variant: "success" });
        onOpenChange(false);
      },
      onError: () => {
        hvToast("Có lỗi xảy ra, thử lại sau", { variant: "danger" });
      },
    });
  }

  return (
    <HvModal
      open={open}
      onOpenChange={onOpenChange}
      title={copy.title}
      footer={
        <>
          <HvButton type="button" variant="ghost" onClick={() => onOpenChange(false)}>
            Huỷ
          </HvButton>
          <HvButton
            type="button"
            variant="danger"
            disabled={mutation.isPending}
            onClick={handleConfirm}
          >
            {mutation.isPending ? copy.pending : copy.confirm}
          </HvButton>
        </>
      }
    >
      {mode === "remove" ? (
        <p>
          <strong>{member.full_name}</strong> sẽ không còn truy cập trung tâm. Lớp, học sinh và
          phiếu thu giáo viên này đã tạo sẽ ở lại trung tâm.
        </p>
      ) : (
        <p>
          Bạn sẽ không còn truy cập dữ liệu của trung tâm. Lớp, học sinh và phiếu thu bạn đã tạo sẽ
          ở lại trung tâm.
        </p>
      )}
    </HvModal>
  );
}
