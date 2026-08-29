import { useState } from "react";

import { HvButton, hvToast } from "@/components/hv";

import { useCenterPermissions, useReplaceRolePermissions } from "../hooks/use-center-permissions";
import { REPORTS_SEND_KEY, type Role } from "../schemas/permission-schemas";

/**
 * Roles × catalog checkbox matrix. Owns its read model (the endpoint is
 * owner-only, so the component must only mount on the owner branch). Edits
 * stay local per role until its "Lưu" button sends the full checked set (the
 * API only has replace semantics); a saved draft is dropped so the re-read
 * model becomes the source again. The `reports.send` row is disabled on every
 * role: while the legacy `can_send_reports` column is authoritative, that
 * permission is assignable only per member and the API 422s it on role sets.
 */
export function PermissionMatrix() {
  const { data, isPending, isError } = useCenterPermissions();
  const mutation = useReplaceRolePermissions();
  // roleId → draft checked set; absent key = no local edits for that role.
  const [drafts, setDrafts] = useState<Record<string, string[]>>({});
  const [savingRoleId, setSavingRoleId] = useState<string | null>(null);

  function checkedOf(role: Role): string[] {
    return drafts[role.id] ?? role.permissions;
  }

  function isDirty(role: Role): boolean {
    const draft = drafts[role.id];
    if (!draft) {
      return false;
    }
    const server = new Set(role.permissions);
    return draft.length !== role.permissions.length || draft.some((key) => !server.has(key));
  }

  function toggle(role: Role, key: string) {
    const current = checkedOf(role);
    const next = current.includes(key) ? current.filter((k) => k !== key) : [...current, key];
    setDrafts((prev) => ({ ...prev, [role.id]: next }));
  }

  function save(role: Role) {
    setSavingRoleId(role.id);
    // The restricted key never travels on role sets — the API 422s it during
    // dual-life, and its cell is disabled, so a stray server-side occurrence
    // must not poison an otherwise valid save.
    const permissions = checkedOf(role).filter((key) => key !== REPORTS_SEND_KEY);
    mutation.mutate(
      { roleId: role.id, permissions },
      {
        onSuccess: () => {
          setDrafts((prev) => {
            const next = { ...prev };
            delete next[role.id];
            return next;
          });
          hvToast(`Đã lưu quyền cho vai trò ${role.name}`, { variant: "success" });
        },
        onError: () => {
          hvToast("Có lỗi xảy ra, thử lại sau", { variant: "danger" });
        },
        onSettled: () => {
          setSavingRoleId(null);
        },
      },
    );
  }

  if (isPending) {
    return <p className="mt-3 text-[13px] text-ink-400">Đang tải…</p>;
  }
  if (isError || !data) {
    return <p className="mt-3 text-[13px] text-ink-500">Không tải được phân quyền.</p>;
  }
  const { catalog, roles } = data;

  return (
    <div className="overflow-x-auto">
      <table className="w-full min-w-[480px] border-collapse text-[13.5px]">
        <thead>
          <tr>
            <th className="py-2 pr-3 text-left font-bold text-ink-500">Quyền</th>
            {roles.map((role) => (
              <th key={role.id} className="px-2 py-2 text-center font-bold text-ink-900">
                {role.name}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {catalog.map((permission) => {
            const restricted = permission.key === REPORTS_SEND_KEY;
            return (
              <tr key={permission.key} className="border-t border-line-200">
                <td className="py-2 pr-3 text-ink-700">{permission.label}</td>
                {roles.map((role) => (
                  <td
                    key={role.id}
                    className="px-2 py-2 text-center"
                    title={restricted ? "Cấp theo từng thành viên" : undefined}
                  >
                    <input
                      type="checkbox"
                      aria-label={`${permission.label} — ${role.name}`}
                      checked={checkedOf(role).includes(permission.key)}
                      disabled={restricted || mutation.isPending}
                      onChange={() => toggle(role, permission.key)}
                      className="size-4 accent-mint-600"
                    />
                  </td>
                ))}
              </tr>
            );
          })}
          <tr className="border-t border-line-200">
            <td className="py-2 pr-3" />
            {roles.map((role) => (
              <td key={role.id} className="px-2 py-2 text-center">
                <HvButton
                  type="button"
                  variant="primary"
                  size="sm"
                  disabled={!isDirty(role) || mutation.isPending}
                  onClick={() => save(role)}
                >
                  {savingRoleId === role.id ? "Đang lưu…" : "Lưu"}
                </HvButton>
              </td>
            ))}
          </tr>
        </tbody>
      </table>
    </div>
  );
}
