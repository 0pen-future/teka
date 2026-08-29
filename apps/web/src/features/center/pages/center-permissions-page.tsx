import { Navigate } from "react-router";

import { HvCard } from "@/components/hv";
import { useCenterContext } from "@/features/teaching";

import { PermissionMatrix } from "../components/permission-matrix";

/**
 * Owner-only "Phân quyền vai trò" page. The matrix read model is owner-only
 * server-side, so a non-owner deep-linking here redirects to the dashboard
 * before any request could 403 — same gate shape as the audit page.
 */
export function CenterPermissionsPage() {
  const { isOwner, isResolved, isError } = useCenterContext();

  if (!isResolved && !isError) {
    return null;
  }
  if (!isOwner) {
    return <Navigate to="/" replace />;
  }

  return (
    <div>
      <h1 className="font-display text-[26px] font-extrabold text-ink-900">Phân quyền vai trò</h1>
      <p className="mt-1 text-[14px] text-ink-500">
        Quyền của mỗi vai trò áp dụng cho mọi thành viên giữ vai trò đó. Cấp hoặc chặn riêng từng
        người bằng nút "Phân quyền" ở danh sách giáo viên trong Cài đặt trung tâm.
      </p>
      <div className="mt-[18px]">
        <HvCard>
          <PermissionMatrix />
        </HvCard>
      </div>
    </div>
  );
}
