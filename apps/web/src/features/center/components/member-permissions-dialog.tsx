import { useState } from "react";

import { HvBadge, HvButton, HvModal, hvToast } from "@/components/hv";

import { isStaleConflict } from "../api/permission-api";
import {
  useAssignMemberRole,
  useCenterPermissions,
  useReplaceMemberOverrides,
} from "../hooks/use-center-permissions";
import {
  groupCatalog,
  REPORTS_SEND_KEY,
  type MemberPermissions,
} from "../schemas/permission-schemas";

export interface MemberPermissionsDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  teacherId: string;
}

type OverrideMode = "inherit" | "grant" | "deny";

function initialModes(member: MemberPermissions): Record<string, OverrideMode> {
  const modes: Record<string, OverrideMode> = {};
  for (const key of member.grants) {
    modes[key] = "grant";
  }
  for (const key of member.denies) {
    modes[key] = "deny";
  }
  return modes;
}

/**
 * Per-member RBAC editor: a role select that applies immediately, and an
 * override editor (per catalog key: theo vai trò / cấp riêng / chặn riêng)
 * saved as one full replace. The old send-reports toggle folds in here as a
 * `reports.send` grant — while the legacy column is authoritative the API
 * dual-writes it from that override, so no separate toggle remains. Only
 * mounted for non-owner rows; the owner is an implicit superuser the API
 * refuses to target (404).
 */
export function MemberPermissionsDialog({
  open,
  onOpenChange,
  teacherId,
}: MemberPermissionsDialogProps) {
  const { data } = useCenterPermissions();
  const assignRole = useAssignMemberRole();
  const replaceOverrides = useReplaceMemberOverrides();

  const member = data?.members.find((m) => m.teacher_id === teacherId);
  // Draft override modes; null = not initialized yet (first render with data).
  const [modes, setModes] = useState<Record<string, OverrideMode> | null>(null);

  if (!data || !member) {
    // The read model failed or is still loading: keep the modal visible so
    // the "Phân quyền" click never becomes a silent no-op.
    return (
      <HvModal open={open} onOpenChange={onOpenChange} title="Phân quyền">
        <p className="text-[13px] text-ink-500">
          {data ? "Không tìm thấy thành viên trong dữ liệu phân quyền." : "Đang tải phân quyền…"}
        </p>
      </HvModal>
    );
  }

  const draft = modes ?? initialModes(member);
  const role = data.roles.find((r) => r.id === member.role_id);
  const rolePermissions = new Set(role?.permissions ?? []);

  const dirty =
    modes !== null &&
    data.catalog.some(({ key }) => {
      const server: OverrideMode = member.grants.includes(key)
        ? "grant"
        : member.denies.includes(key)
          ? "deny"
          : "inherit";
      return (draft[key] ?? "inherit") !== server;
    });

  function handleRoleChange(roleId: string) {
    assignRole.mutate(
      { teacherId, roleId },
      {
        onSuccess: () => {
          hvToast("Đã đổi vai trò", { variant: "success" });
        },
        onError: () => {
          hvToast("Có lỗi xảy ra, thử lại sau", { variant: "danger" });
        },
      },
    );
  }

  function handleSave() {
    if (!data || !member) {
      return;
    }
    const grants = data.catalog.map((p) => p.key).filter((key) => draft[key] === "grant");
    const denies = data.catalog.map((p) => p.key).filter((key) => draft[key] === "deny");
    replaceOverrides.mutate(
      {
        teacherId,
        grants,
        denies,
        // The CAS pair from the model this draft was composed on; a lost
        // race 409s, the hook refetches, and the dialog stays open with the
        // draft for a reviewed re-save.
        catalogVersion: data.catalog_version,
        assignmentVersion: member.assignment_version,
      },
      {
        onSuccess: () => {
          hvToast("Đã lưu phân quyền", { variant: "success" });
          onOpenChange(false);
        },
        onError: (err) => {
          if (isStaleConflict(err) && err instanceof Error) {
            hvToast(err.message, { variant: "danger" });
          } else {
            hvToast("Có lỗi xảy ra, thử lại sau", { variant: "danger" });
          }
        },
      },
    );
  }

  return (
    <HvModal
      open={open}
      onOpenChange={onOpenChange}
      title={`Phân quyền — ${member.full_name}`}
      footer={
        <>
          <HvButton type="button" variant="ghost" onClick={() => onOpenChange(false)}>
            Đóng
          </HvButton>
          <HvButton
            type="button"
            variant="primary"
            disabled={!dirty || replaceOverrides.isPending}
            onClick={handleSave}
          >
            {replaceOverrides.isPending ? "Đang lưu…" : "Lưu"}
          </HvButton>
        </>
      }
    >
      <div className="flex flex-col gap-2">
        <label htmlFor="member-role" className="text-[13px] font-bold text-ink-700">
          Vai trò
        </label>
        <select
          id="member-role"
          value={member.role_id ?? ""}
          onChange={(event) => handleRoleChange(event.target.value)}
          disabled={assignRole.isPending}
          className="min-h-11 rounded-[var(--radius-md)] border border-line-200 bg-white px-3 text-[14px] text-ink-900"
        >
          {member.role_id === null ? (
            // A pre-RBAC membership holds no role row; assigning one is
            // one-way, so the placeholder is shown but not re-selectable.
            <option value="" disabled>
              Giáo viên (mặc định)
            </option>
          ) : null}
          {data.roles.map((r) => (
            <option key={r.id} value={r.id}>
              {r.name}
            </option>
          ))}
        </select>
      </div>

      <p className="mt-3 text-[12.5px] text-ink-500">
        Chặn riêng luôn thắng: quyền bị chặn riêng sẽ không có hiệu lực kể cả khi vai trò hoặc cấp
        riêng có cấp quyền đó.
      </p>

      {groupCatalog(data.catalog).map((group) => (
        <div key={group.resource} className="mt-4">
          <p className="text-[12.5px] font-bold text-ink-500">{group.label}</p>
          <div className="flex flex-col divide-y divide-line-200">
            {group.entries.map((permission) => {
              const mode = draft[permission.key] ?? "inherit";
              const effective =
                mode === "grant" || (mode === "inherit" && rolePermissions.has(permission.key));
              return (
                <div key={permission.key} className="flex items-center gap-2 py-2">
                  <div className="min-w-0 flex-1">
                    <p className="truncate text-[13.5px] text-ink-700">{permission.label}</p>
                    {effective ? (
                      <HvBadge
                        variant={mode === "grant" ? "info" : "success"}
                        size="sm"
                        className="mt-1"
                      >
                        {mode === "grant" ? "Cấp riêng" : "Từ vai trò"}
                      </HvBadge>
                    ) : (
                      <HvBadge
                        variant={mode === "deny" ? "danger" : "neutral"}
                        size="sm"
                        className="mt-1"
                      >
                        {mode === "deny" ? "Chặn riêng" : "Không có"}
                      </HvBadge>
                    )}
                  </div>
                  <select
                    aria-label={`Quyền ${permission.label}`}
                    value={mode}
                    onChange={(event) => {
                      const next = event.target.value as OverrideMode;
                      setModes({ ...draft, [permission.key]: next });
                    }}
                    className="min-h-11 rounded-[var(--radius-md)] border border-line-200 bg-white px-2 text-[13px] text-ink-900"
                    title={
                      permission.key === REPORTS_SEND_KEY
                        ? "Quyền gửi báo cáo chỉ cấp theo từng thành viên"
                        : undefined
                    }
                  >
                    <option value="inherit">Theo vai trò</option>
                    <option value="grant">Cấp riêng</option>
                    <option value="deny">Chặn riêng</option>
                  </select>
                </div>
              );
            })}
          </div>
        </div>
      ))}
    </HvModal>
  );
}
