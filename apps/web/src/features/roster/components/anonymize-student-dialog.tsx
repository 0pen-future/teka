import { HvButton, HvModal } from "@/components/hv";

import { useAnonymizeStudent } from "../hooks/use-students";
import type { Student } from "../schemas/roster-schemas";

export interface AnonymizeStudentDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  student: Student;
  onSuccess?: () => void;
}

/**
 * Student "delete" is an anonymize confirm flow, not a row delete (PRD R1):
 * personal data is erased, but receipts and payment history are kept in
 * anonymized form so past billing stays auditable. Built on `HvModal` +
 * `HvButton variant="danger"` rather than the shadcn `ConfirmDialog`, to keep
 * the roster feature's visible surface on the design system exclusively. No
 * type-to-confirm — this is a routine action and should stay one press past
 * the initial button.
 */
export function AnonymizeStudentDialog({
  open,
  onOpenChange,
  student,
  onSuccess,
}: AnonymizeStudentDialogProps) {
  const mutation = useAnonymizeStudent();

  function handleConfirm() {
    mutation.mutate(student.id, {
      onSuccess: () => {
        onOpenChange(false);
        onSuccess?.();
      },
    });
  }

  return (
    <HvModal
      open={open}
      onOpenChange={onOpenChange}
      title="Xoá học sinh"
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
            {mutation.isPending ? "Đang xoá…" : "Xoá dữ liệu"}
          </HvButton>
        </>
      }
    >
      <p>
        Xoá dữ liệu cá nhân của <strong>{student.full_name}</strong>. Phiếu thu và lịch sử thanh
        toán được giữ lại ở dạng ẩn danh.
      </p>
    </HvModal>
  );
}
