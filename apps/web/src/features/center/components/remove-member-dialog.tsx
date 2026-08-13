import { HvButton, HvModal, hvToast } from "@/components/hv";

import { useRemoveMember } from "../hooks/use-center";
import type { CenterMember } from "../schemas/center-schemas";

export interface RemoveMemberDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  member: CenterMember;
}

/**
 * Owner-only action: disables the member's login. Every class, student, and
 * receipt they created stays with the center — only their access ends.
 */
export function RemoveMemberDialog({ open, onOpenChange, member }: RemoveMemberDialogProps) {
  const mutation = useRemoveMember();

  function handleConfirm() {
    mutation.mutate(member.id, {
      onSuccess: () => {
        hvToast("Đã vô hiệu hoá đăng nhập", { variant: "success" });
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
      title="Vô hiệu hoá đăng nhập"
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
            {mutation.isPending ? "Đang vô hiệu hoá…" : "Vô hiệu hoá đăng nhập"}
          </HvButton>
        </>
      }
    >
      <p>
        <strong>{member.full_name}</strong> sẽ không thể đăng nhập nữa. Lớp, học sinh và phiếu thu
        giáo viên này đã tạo sẽ ở lại trung tâm.
      </p>
    </HvModal>
  );
}
