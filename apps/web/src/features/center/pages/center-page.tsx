import { ChevronRightIcon, PencilIcon } from "lucide-react";
import { useState } from "react";
import { Link } from "react-router";

import { HvBadge, HvButton, HvCard } from "@/components/hv";
import { InviteSection } from "@/features/invitation";

import { MemberList } from "../components/member-list";
import { MemberPermissionsDialog } from "../components/member-permissions-dialog";
import { RemoveMemberDialog } from "../components/remove-member-dialog";
import { RenameCenterDialog } from "../components/rename-center-dialog";
import { useCenter } from "../hooks/use-center";
import type { CenterMember } from "../schemas/center-schemas";

/**
 * "Trung tâm" settings page. `GET /centers/me` is role-shaped: an owner gets
 * the full center plus roster and can rename, invite, and disable members; a
 * non-owner member gets only the center's name — accounts join exclusively
 * through an owner-issued invite, so there is no join or self-leave path.
 */
export function CenterPage() {
  const { data, isPending, isError } = useCenter();
  const [renameOpen, setRenameOpen] = useState(false);
  const [removeTarget, setRemoveTarget] = useState<CenterMember | null>(null);
  const [permissionsTarget, setPermissionsTarget] = useState<CenterMember | null>(null);

  if (isPending) {
    return <p className="text-[14px] text-ink-400">Đang tải…</p>;
  }
  if (isError || !data) {
    return <p className="text-[14px] text-ink-500">Không tải được thông tin trung tâm.</p>;
  }

  if (!("members" in data)) {
    return (
      <div>
        <h1 className="font-display text-[26px] font-extrabold text-ink-900">Trung tâm</h1>
        <p className="mt-1 text-[14px] text-ink-500">
          Giáo viên trong cùng trung tâm chia sẻ dữ liệu lớp học và học phí.
        </p>
        <div className="mt-[18px]">
          <HvCard>
            <p className="truncate font-display text-[19px] font-bold text-ink-900">
              {data.center_name}
            </p>
            <HvBadge variant="neutral" size="sm" className="mt-1.5">
              Thành viên
            </HvBadge>
          </HvCard>
        </div>
      </div>
    );
  }

  const { center, members } = data;

  return (
    <div>
      <h1 className="font-display text-[26px] font-extrabold text-ink-900">Trung tâm</h1>
      <p className="mt-1 text-[14px] text-ink-500">
        Giáo viên trong cùng trung tâm chia sẻ dữ liệu lớp học và học phí.
      </p>

      <div className="mt-[18px] flex flex-col gap-4">
        <HvCard>
          <div className="flex items-center gap-3">
            <div className="min-w-0 flex-1">
              <p className="truncate font-display text-[19px] font-bold text-ink-900">
                {center.name}
              </p>
              <HvBadge variant="success" size="sm" className="mt-1.5">
                Chủ trung tâm
              </HvBadge>
            </div>
            <HvButton
              type="button"
              variant="ghost"
              size="sm"
              aria-label="Đổi tên trung tâm"
              onClick={() => setRenameOpen(true)}
            >
              <PencilIcon aria-hidden className="size-4" />
              Đổi tên
            </HvButton>
          </div>
        </HvCard>

        <HvCard>
          <p className="font-display text-[17px] font-bold text-ink-900">
            Giáo viên trong trung tâm
          </p>
          <MemberList
            members={members}
            canRemove
            onRemove={(member) => setRemoveTarget(member)}
            onManagePermissions={(member) => setPermissionsTarget(member)}
          />
        </HvCard>

        <HvCard interactive className="p-0">
          <Link
            to="/center/permissions"
            aria-label="Phân quyền vai trò"
            className="flex items-center gap-3 rounded-[inherit] p-[var(--pad-card)] focus-visible:outline-none focus-visible:ring-4"
          >
            <div className="min-w-0 flex-1">
              <p className="font-display text-[17px] font-bold text-ink-900">Phân quyền vai trò</p>
              <p className="mt-1 text-[13px] text-ink-500">
                Quyền của mỗi vai trò áp dụng cho mọi thành viên giữ vai trò đó. Cấp hoặc chặn riêng
                từng người bằng nút "Phân quyền" ở danh sách giáo viên.
              </p>
            </div>
            <ChevronRightIcon aria-hidden className="size-5 shrink-0 text-ink-400" />
          </Link>
        </HvCard>

        <InviteSection />
      </div>

      <RenameCenterDialog
        open={renameOpen}
        onOpenChange={setRenameOpen}
        currentName={center.name}
      />
      {removeTarget ? (
        <RemoveMemberDialog
          open
          onOpenChange={(open) => {
            if (!open) {
              setRemoveTarget(null);
            }
          }}
          member={removeTarget}
        />
      ) : null}
      {permissionsTarget ? (
        <MemberPermissionsDialog
          open
          onOpenChange={(open) => {
            if (!open) {
              setPermissionsTarget(null);
            }
          }}
          teacherId={permissionsTarget.id}
        />
      ) : null}
    </div>
  );
}
