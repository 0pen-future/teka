import { HvBadge, HvButton } from "@/components/hv";
import { formatPhoneLocal } from "@/lib/utils";

import type { CenterMember } from "../schemas/center-schemas";

export interface MemberListProps {
  members: CenterMember[];
  /** Owner viewers get the remove action; members see a read-only roster. */
  canRemove: boolean;
  onRemove: (member: CenterMember) => void;
  /**
   * Owner viewers also manage per-member roles and permission overrides
   * (including the delegated send-reports grant); the action shares the
   * remove action's gating (owner-only, non-owner rows).
   */
  onManagePermissions?: (member: CenterMember) => void;
}

/**
 * The owner row never gets a remove button — the API rejects owner
 * self-removal (a center cannot be left ownerless), so the control would only
 * manufacture an error. The permissions action skips the owner row for the
 * same reason: the owner is an implicit superuser the API refuses to target.
 */
export function MemberList({ members, canRemove, onRemove, onManagePermissions }: MemberListProps) {
  return (
    <ul className="flex flex-col divide-y divide-line-200">
      {members.map((member) => (
        <li key={member.id} className="flex items-center gap-3 py-3">
          <div className="min-w-0 flex-1">
            <p className="flex items-center gap-2 text-[14px] font-extrabold text-ink-900">
              <span className="truncate">{member.full_name}</span>
              {member.is_owner ? (
                <HvBadge variant="success" size="sm">
                  Chủ
                </HvBadge>
              ) : null}
              {member.can_send_reports ? (
                <HvBadge variant="info" size="sm">
                  Thư ký gửi báo cáo
                </HvBadge>
              ) : null}
            </p>
            <p className="text-[12.5px] text-ink-500">{formatPhoneLocal(member.phone)}</p>
          </div>
          {canRemove && !member.is_owner && onManagePermissions ? (
            <HvButton
              type="button"
              variant="ghost"
              size="sm"
              aria-label={`Phân quyền cho ${member.full_name}`}
              onClick={() => onManagePermissions(member)}
            >
              Phân quyền
            </HvButton>
          ) : null}
          {canRemove && !member.is_owner ? (
            <HvButton
              type="button"
              variant="ghost"
              size="sm"
              aria-label={`Xoá ${member.full_name}`}
              onClick={() => onRemove(member)}
            >
              Xoá
            </HvButton>
          ) : null}
        </li>
      ))}
    </ul>
  );
}
