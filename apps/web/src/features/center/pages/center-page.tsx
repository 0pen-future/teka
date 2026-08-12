import { PencilIcon } from "lucide-react";
import { useState } from "react";

import { HvBadge, HvButton, HvCard } from "@/components/hv";
import { useAuthStore } from "@/features/auth";

import { JoinCenterForm } from "../components/join-center-form";
import { MemberList } from "../components/member-list";
import { RemoveMemberDialog } from "../components/remove-member-dialog";
import { RenameCenterDialog } from "../components/rename-center-dialog";
import { useCenter } from "../hooks/use-center";
import type { CenterMember } from "../schemas/center-schemas";

/**
 * "Trung tâm" settings page, fully role-gated on `center.is_owner`: owners
 * rename and manage the roster, members see a read-only roster plus the exit.
 * The join section only exists for an owner still alone in their personal
 * center — the API rejects joining once the account holds data or members, so
 * showing the form elsewhere could only produce errors.
 */
export function CenterPage() {
  const user = useAuthStore((state) => state.user);
  const { data, isPending, isError } = useCenter();
  const [renameOpen, setRenameOpen] = useState(false);
  const [removeTarget, setRemoveTarget] = useState<CenterMember | null>(null);
  const [leaveOpen, setLeaveOpen] = useState(false);

  if (isPending) {
    return <p className="text-[14px] text-ink-400">Đang tải…</p>;
  }
  if (isError || !data) {
    return <p className="text-[14px] text-ink-500">Không tải được thông tin trung tâm.</p>;
  }

  const { center, members } = data;
  const isOwner = center.is_owner;
  const isAloneInOwnCenter = isOwner && members.length === 1;
  const self = members.find((member) => member.id === user?.id);

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
              <HvBadge variant={isOwner ? "success" : "neutral"} size="sm" className="mt-1.5">
                {isOwner ? "Chủ trung tâm" : "Thành viên"}
              </HvBadge>
            </div>
            {isOwner ? (
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
            ) : null}
          </div>
        </HvCard>

        <HvCard>
          <p className="font-display text-[17px] font-bold text-ink-900">
            Giáo viên trong trung tâm
          </p>
          <MemberList
            members={members}
            canRemove={isOwner}
            onRemove={(member) => setRemoveTarget(member)}
          />
        </HvCard>

        {isAloneInOwnCenter ? (
          <HvCard>
            <p className="font-display text-[17px] font-bold text-ink-900">
              Gia nhập trung tâm khác
            </p>
            <p className="mt-0.5 mb-3 text-[12.5px] text-ink-500">
              Nhập số điện thoại của chủ trung tâm để chuyển tài khoản của bạn vào trung tâm đó. Chỉ
              tài khoản chưa có dữ liệu mới gia nhập được.
            </p>
            <JoinCenterForm />
          </HvCard>
        ) : null}

        {!isOwner && self ? (
          <div className="flex justify-end">
            <HvButton type="button" variant="ghost" onClick={() => setLeaveOpen(true)}>
              Rời trung tâm
            </HvButton>
          </div>
        ) : null}
      </div>

      {isOwner ? (
        <RenameCenterDialog
          open={renameOpen}
          onOpenChange={setRenameOpen}
          currentName={center.name}
        />
      ) : null}
      {removeTarget ? (
        <RemoveMemberDialog
          open
          onOpenChange={(open) => {
            if (!open) {
              setRemoveTarget(null);
            }
          }}
          member={removeTarget}
          mode="remove"
        />
      ) : null}
      {!isOwner && self ? (
        <RemoveMemberDialog
          open={leaveOpen}
          onOpenChange={setLeaveOpen}
          member={self}
          mode="leave"
        />
      ) : null}
    </div>
  );
}
