import { HvButton, HvModal, hvToast } from "@/components/hv";

import { useSetSendReports } from "../hooks/use-center";
import type { CenterMember } from "../schemas/center-schemas";

export interface SendReportsPermissionDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  member: CenterMember;
}

/**
 * Owner-only confirm before granting or revoking the delegated send-reports
 * permission. The copy spells out the exclusivity model: plain members cannot
 * send reports themselves, so the center needs at least one holder (or the
 * owner) for reports to go out at all.
 */
export function SendReportsPermissionDialog({
  open,
  onOpenChange,
  member,
}: SendReportsPermissionDialogProps) {
  const mutation = useSetSendReports();
  const granting = !member.can_send_reports;

  function handleConfirm() {
    mutation.mutate(
      { teacherId: member.id, granted: granting },
      {
        onSuccess: () => {
          hvToast(granting ? "Đã giao quyền gửi báo cáo" : "Đã thu hồi quyền gửi báo cáo", {
            variant: "success",
          });
          onOpenChange(false);
        },
        onError: () => {
          hvToast("Có lỗi xảy ra, thử lại sau", { variant: "danger" });
        },
      },
    );
  }

  return (
    <HvModal
      open={open}
      onOpenChange={onOpenChange}
      title={granting ? "Giao quyền gửi báo cáo" : "Thu hồi quyền gửi báo cáo"}
      footer={
        <>
          <HvButton type="button" variant="ghost" onClick={() => onOpenChange(false)}>
            Huỷ
          </HvButton>
          <HvButton
            type="button"
            variant={granting ? "primary" : "danger"}
            disabled={mutation.isPending}
            onClick={handleConfirm}
          >
            {mutation.isPending ? "Đang cập nhật…" : granting ? "Giao quyền" : "Thu hồi quyền"}
          </HvButton>
        </>
      }
    >
      {granting ? (
        <>
          <p>
            <strong>{member.full_name}</strong> sẽ đọc được bảng kê và công nợ của mọi giáo viên
            trong trung tâm, và gửi báo cáo học phí cho phụ huynh bằng Zalo cá nhân của chính mình.
          </p>
          <p className="mt-2">
            Giáo viên thường không tự gửi báo cáo — chỉ người giữ quyền này và chủ trung tâm gửi
            được.
          </p>
        </>
      ) : (
        <>
          <p>
            <strong>{member.full_name}</strong> sẽ không còn đọc được bảng kê, công nợ của giáo viên
            khác và không thể gửi báo cáo nữa. Đợt gửi đang chạy (nếu có) sẽ dừng lại.
          </p>
          <p className="mt-2">
            Giáo viên thường không tự gửi báo cáo — hãy đảm bảo còn người giữ quyền (hoặc chủ trung
            tâm) phụ trách việc gửi.
          </p>
        </>
      )}
    </HvModal>
  );
}
