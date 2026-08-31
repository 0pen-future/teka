import { useEffect, useState } from "react";

import { HvBadge, HvButton, HvModal, hvToast } from "@/components/hv";
import { cn } from "@/lib/utils";

import { isStaleConflict } from "../api/permission-api";
import { useCenterPermissions, useReplaceRolePermissions } from "../hooks/use-center-permissions";
import {
  buildCatalogTabs,
  type CatalogGroup,
  type CatalogTab,
  type PermissionInfo,
  type Role,
} from "../schemas/permission-schemas";

/**
 * Roles × catalog checkbox matrix, one underline tab per catalog resource
 * group (admin stubs share a "Quản trị" tab). Owns
 * its read model (the endpoint is owner-only, so the component must only
 * mount on the owner branch). Edits stay local per role until its "Lưu"
 * button sends the full checked set (the API only has replace semantics)
 * together with the CAS pair — catalog_version and the role's
 * assignment_version from the model the edit was composed on. A lost race
 * answers 409: the hook refetches the read model, the draft stays put, and
 * the owner re-saves after reviewing — never an automatic retry.
 *
 * Widening a role with a high-risk key pauses on a confirmation that names
 * the keys and how many members currently hold the role — computed from the
 * same read model the save will send versions from, so the count can never
 * describe a different state than the commit. Narrowing needs no
 * confirmation: it only removes access.
 */
export function PermissionMatrix() {
  const { data, isPending, isError } = useCenterPermissions();
  const mutation = useReplaceRolePermissions();
  // roleId → draft checked set; absent key = no local edits for that role.
  const [drafts, setDrafts] = useState<Record<string, string[]>>({});
  // null until the owner picks a tab; the first catalog tab renders then.
  const [activeTabId, setActiveTabId] = useState<string | null>(null);
  const [savingRoleId, setSavingRoleId] = useState<string | null>(null);
  // A high-risk save waiting for the owner's explicit go-ahead.
  const [confirming, setConfirming] = useState<{
    role: Role;
    highRisk: PermissionInfo[];
  } | null>(null);

  const hasDirty = data?.roles.some((role) => isDirty(role)) ?? false;

  // Unsent drafts survive tab-switches (React state) but not navigation away;
  // the browser prompt is the only guard the platform offers for that.
  useEffect(() => {
    if (!hasDirty) {
      return;
    }
    const warn = (event: BeforeUnloadEvent) => {
      event.preventDefault();
    };
    window.addEventListener("beforeunload", warn);
    return () => {
      window.removeEventListener("beforeunload", warn);
    };
  }, [hasDirty]);

  function checkedOf(role: Role): string[] {
    return drafts[role.id] ?? role.permissions;
  }

  // Keys where the role's draft disagrees with the server, in both
  // directions; empty when the role has no draft or an equal one.
  function draftDiff(role: Role): string[] {
    const draft = drafts[role.id];
    if (!draft) {
      return [];
    }
    const server = new Set(role.permissions);
    const checked = new Set(draft);
    return [
      ...draft.filter((key) => !server.has(key)),
      ...role.permissions.filter((key) => !checked.has(key)),
    ];
  }

  function isDirty(role: Role): boolean {
    return draftDiff(role).length > 0;
  }

  function toggle(role: Role, key: string) {
    const current = checkedOf(role);
    const next = current.includes(key) ? current.filter((k) => k !== key) : [...current, key];
    setDrafts((prev) => ({ ...prev, [role.id]: next }));
  }

  function requestSave(role: Role) {
    if (!data) {
      return;
    }
    const server = new Set(role.permissions);
    const added = new Set(checkedOf(role).filter((key) => !server.has(key)));
    const highRisk = data.catalog.filter((p) => added.has(p.key) && p.risk === "high");
    if (highRisk.length > 0) {
      setConfirming({ role, highRisk });
      return;
    }
    commit(role);
  }

  function commit(role: Role) {
    if (!data) {
      return;
    }
    setConfirming(null);
    setSavingRoleId(role.id);
    mutation.mutate(
      {
        roleId: role.id,
        permissions: checkedOf(role),
        catalogVersion: data.catalog_version,
        assignmentVersion: role.assignment_version,
      },
      {
        onSuccess: () => {
          setDrafts((prev) => {
            const next = { ...prev };
            delete next[role.id];
            return next;
          });
          hvToast(`Đã lưu quyền cho vai trò ${role.name}`, { variant: "success" });
        },
        onError: (err) => {
          // The draft stays either way; on a stale-version conflict the
          // server message tells the owner to review the refetched model.
          if (isStaleConflict(err) && err instanceof Error) {
            hvToast(err.message, { variant: "danger" });
          } else {
            hvToast("Có lỗi xảy ra, thử lại sau", { variant: "danger" });
          }
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
  const { roles, members } = data;
  const tabs = buildCatalogTabs(data.catalog);
  const activeTab = tabs.find((tab) => tab.id === activeTabId) ?? tabs[0];
  const affectedCount = confirming
    ? members.filter((m) => m.role_id === confirming.role.id).length
    : 0;

  // Union of edited keys across every role's draft, so a tab can flag that it
  // holds changes the save buttons below would commit — drafts survive tab
  // switches, but an unmarked hidden tab would make them easy to forget.
  const dirtyKeys = new Set(roles.flatMap(draftDiff));
  function tabHasDirty(tab: CatalogTab): boolean {
    return tab.groups.some((group) => group.entries.some((entry) => dirtyKeys.has(entry.key)));
  }

  function renderGroupTable(group: CatalogGroup) {
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
            {group.entries.map((permission) => (
              <tr key={permission.key} className="border-t border-line-200">
                <td className="py-2 pr-3 text-ink-700" title={permission.description || undefined}>
                  {permission.label}
                  {permission.risk === "high" ? (
                    <HvBadge variant="danger" size="sm" className="ml-2">
                      Rủi ro cao
                    </HvBadge>
                  ) : null}
                </td>
                {roles.map((role) => (
                  <td key={role.id} className="px-2 py-2 text-center">
                    <input
                      type="checkbox"
                      aria-label={`${permission.label} — ${role.name}`}
                      checked={checkedOf(role).includes(permission.key)}
                      disabled={mutation.isPending}
                      onChange={() => toggle(role, permission.key)}
                      className="size-4 accent-mint-600"
                    />
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-4">
      <div
        className="flex flex-wrap gap-x-[22px] border-b-[1.5px] border-line-200"
        role="tablist"
        aria-label="Nhóm quyền"
      >
        {tabs.map((tab) => (
          <button
            key={tab.id}
            type="button"
            role="tab"
            aria-selected={activeTab?.id === tab.id}
            onClick={() => setActiveTabId(tab.id)}
            className={cn(
              "border-b-[3px] px-0.5 py-2.5 text-[14.5px] font-extrabold focus-visible:ring-4 focus-visible:outline-none",
              activeTab?.id === tab.id
                ? "border-mint-400 text-ink-900"
                : "border-transparent text-ink-400",
            )}
          >
            {tab.label}
            {tabHasDirty(tab) ? (
              <>
                <span
                  aria-hidden
                  title="Có thay đổi chưa lưu"
                  className="ml-1.5 inline-block size-1.5 rounded-full bg-mint-400 align-middle"
                />
                <span className="sr-only"> — có thay đổi chưa lưu</span>
              </>
            ) : null}
          </button>
        ))}
      </div>

      {activeTab ? (
        activeTab.groups.length === 1 ? (
          renderGroupTable(activeTab.groups[0]!)
        ) : (
          <div className="flex flex-col gap-4">
            {activeTab.groups.map((group) => (
              <section key={group.resource}>
                <h3 className="text-[14px] font-bold text-ink-900">{group.label}</h3>
                {renderGroupTable(group)}
              </section>
            ))}
          </div>
        )
      ) : null}

      <div className="flex flex-wrap items-center justify-end gap-4">
        {roles.map((role) => (
          <div key={role.id} className="flex items-center gap-2">
            <span className="text-[13px] text-ink-500">{role.name}</span>
            <HvButton
              type="button"
              variant="primary"
              size="sm"
              disabled={!isDirty(role) || mutation.isPending}
              onClick={() => requestSave(role)}
            >
              {savingRoleId === role.id ? "Đang lưu…" : "Lưu"}
            </HvButton>
          </div>
        ))}
      </div>

      <HvModal
        open={confirming !== null}
        onOpenChange={(open) => {
          if (!open) {
            setConfirming(null);
          }
        }}
        title="Xác nhận quyền rủi ro cao"
        footer={
          <>
            <HvButton type="button" variant="ghost" onClick={() => setConfirming(null)}>
              Hủy
            </HvButton>
            <HvButton
              type="button"
              variant="primary"
              onClick={() => {
                if (confirming) {
                  commit(confirming.role);
                }
              }}
            >
              Xác nhận
            </HvButton>
          </>
        }
      >
        {confirming ? (
          <div className="flex flex-col gap-2 text-[13.5px] text-ink-700">
            <p>
              Vai trò <strong>{confirming.role.name}</strong> sẽ nhận thêm quyền rủi ro cao:
            </p>
            <ul className="list-disc pl-5">
              {confirming.highRisk.map((p) => (
                <li key={p.key}>{p.label}</li>
              ))}
            </ul>
            <p>
              Thay đổi áp dụng ngay cho <strong>{affectedCount} thành viên</strong> đang giữ vai trò
              này.
            </p>
          </div>
        ) : null}
      </HvModal>
    </div>
  );
}
