import { useState } from "react";

import { HvBadge, HvButton, HvCard, HvModal, hvToast } from "@/components/hv";
import { useCenter, type CenterMember } from "@/features/center";
import { useCenterContext } from "@/features/teaching";
import { ApiError } from "@/lib/api/errors";
import { formatDateTime } from "@/lib/utils/format";

import { useAssignClassStaff, useClassStaff, useRemoveClassStaff } from "../hooks/use-class-staff";
import type { AssignableStaffRoleKey, ClassStaff } from "../schemas/roster-schemas";

const STAFF_ROLE_GIAO_VIEN = "giao_vien";
const ROLE_ORDER = [STAFF_ROLE_GIAO_VIEN, "hoc_vu", "tro_giang"] as const;
const ROLE_LABELS: Record<(typeof ROLE_ORDER)[number], string> = {
  giao_vien: "Giáo viên",
  hoc_vu: "Học vụ",
  tro_giang: "Trợ giảng",
};
const ASSIGNABLE_ROLES: readonly AssignableStaffRoleKey[] = ["hoc_vu", "tro_giang"];

function isActive(staff: ClassStaff): boolean {
  return staff.ended_at === null;
}

function apiErrorMessage(error: unknown, fallback: string): string | null {
  if (!error) {
    return null;
  }
  return error instanceof ApiError ? error.message : fallback;
}

/**
 * "Nhân sự lớp" — owner-only roster of who works a class and in what role.
 * `giao_vien` stays read-only here and links to the `TeacherHandoffCard`
 * further down the settings page (`#teacher-handoff`): during the dual-write
 * window the primary teacher changes only through that handoff flow, never
 * through this section's assign/remove calls (the API refuses it with 409).
 */
export function ClassStaffSection({ classId }: { classId: string }) {
  const { isOwner } = useCenterContext();
  const { data: center } = useCenter();
  const { data: staff, isPending, isError } = useClassStaff(classId, isOwner);

  if (!isOwner) {
    return null;
  }

  const members = center && "members" in center ? center.members : [];

  return (
    <HvCard className="max-w-[640px]">
      <p className="font-display text-[16px] font-bold text-ink-900">Nhân sự lớp</p>
      {isPending ? (
        <p className="mt-2 text-[13px] text-ink-400">Đang tải…</p>
      ) : isError || !staff ? (
        <p className="mt-2 text-[13px] text-coral-600">Không tải được nhân sự lớp.</p>
      ) : (
        <ClassStaffBody classId={classId} staff={staff} members={members} />
      )}
    </HvCard>
  );
}

function ClassStaffBody({
  classId,
  staff,
  members,
}: {
  classId: string;
  staff: ClassStaff[];
  members: CenterMember[];
}) {
  const activeByRole = ROLE_ORDER.reduce<Record<string, ClassStaff[]>>((acc, role) => {
    acc[role] = staff.filter((item) => item.role_key === role && isActive(item));
    return acc;
  }, {});
  const missingRoles = ROLE_ORDER.filter((role) => activeByRole[role]!.length === 0);
  // A person holds at most one active stint per class (uq_class_staff_active),
  // regardless of role — so anyone already active in any role is not a valid
  // target for another assignment.
  const activeTeacherIds = new Set(staff.filter(isActive).map((item) => item.teacher_id));
  const assignableMembers = members.filter((member) => !activeTeacherIds.has(member.id));
  const giaoVien = activeByRole[STAFF_ROLE_GIAO_VIEN]?.[0] ?? null;
  const endedStaff = staff.filter((item) => !isActive(item));

  return (
    <div className="mt-3 flex flex-col gap-4">
      {missingRoles.length > 0 ? (
        <div className="flex flex-wrap gap-2">
          {missingRoles.map((role) => (
            <HvBadge key={role} variant="warning">
              Thiếu {ROLE_LABELS[role]}
            </HvBadge>
          ))}
        </div>
      ) : null}

      <div>
        <p className="text-[13px] font-bold text-ink-700">
          {giaoVien?.role_label ?? ROLE_LABELS[STAFF_ROLE_GIAO_VIEN]}
        </p>
        <div className="mt-1 flex items-center justify-between gap-2">
          <span className="text-[14px] text-ink-900">
            {giaoVien ? giaoVien.teacher_name : "Chưa gán"}
          </span>
          <a
            href="#teacher-handoff"
            className="text-[13px] font-bold text-mint-600 hover:underline"
          >
            Bàn giao lớp
          </a>
        </div>
      </div>

      {ASSIGNABLE_ROLES.map((role) => (
        <StaffRoleGroup
          key={role}
          classId={classId}
          roleKey={role}
          roleLabel={activeByRole[role]![0]?.role_label ?? ROLE_LABELS[role]}
          activeStaff={activeByRole[role]!}
          assignableMembers={assignableMembers}
        />
      ))}

      <EndedStaffList classId={classId} endedStaff={endedStaff} />
    </div>
  );
}

/**
 * A collapsed "Đã kết thúc" disclosure — soft-closed stints (`ended_at` set)
 * still grant history reads, so they need to stay visible somewhere with a
 * way to fully revoke (`mode=void`) an assignment that was made in error.
 */
function EndedStaffList({ classId, endedStaff }: { classId: string; endedStaff: ClassStaff[] }) {
  const [expanded, setExpanded] = useState(false);
  const [revoking, setRevoking] = useState<ClassStaff | null>(null);

  if (endedStaff.length === 0) {
    return null;
  }

  return (
    <div className="border-t border-line-200 pt-3">
      <HvButton
        size="sm"
        variant="ghost"
        onClick={() => setExpanded((value) => !value)}
        aria-expanded={expanded}
      >
        {expanded ? "Ẩn" : "Hiện"} đã kết thúc ({endedStaff.length})
      </HvButton>

      {expanded ? (
        <ul className="mt-2 flex flex-col gap-1">
          {endedStaff.map((item) => (
            <li key={item.id} className="flex items-center justify-between gap-2">
              <span className="text-[13px] text-ink-400">
                {item.teacher_name} — {item.role_label} · kết thúc {formatDateTime(item.ended_at!)}
              </span>
              <HvButton size="sm" variant="ghost" onClick={() => setRevoking(item)}>
                Thu hồi
              </HvButton>
            </li>
          ))}
        </ul>
      ) : null}

      {revoking ? (
        <RemoveStaffDialog
          classId={classId}
          staff={revoking}
          voidOnly
          onOpenChange={(open) => {
            if (!open) {
              setRevoking(null);
            }
          }}
        />
      ) : null}
    </div>
  );
}

function StaffRoleGroup({
  classId,
  roleKey,
  roleLabel,
  activeStaff,
  assignableMembers,
}: {
  classId: string;
  roleKey: AssignableStaffRoleKey;
  roleLabel: string;
  activeStaff: ClassStaff[];
  assignableMembers: CenterMember[];
}) {
  const [adding, setAdding] = useState(false);
  const [pickedId, setPickedId] = useState("");
  const [removing, setRemoving] = useState<ClassStaff | null>(null);
  const assign = useAssignClassStaff(classId);
  const errorMessage = apiErrorMessage(assign.error, "Không gán được nhân sự. Thử lại sau.");

  function handleAssign() {
    if (!pickedId) {
      return;
    }
    assign.mutate(
      { teacher_id: pickedId, role_key: roleKey },
      {
        onSuccess: (result) => {
          hvToast(`Đã thêm ${result.teacher_name} — ${roleLabel}`);
          setAdding(false);
          setPickedId("");
        },
      },
    );
  }

  return (
    <div>
      <p className="text-[13px] font-bold text-ink-700">{roleLabel}</p>
      {activeStaff.length === 0 ? (
        <p className="mt-1 text-[13px] text-ink-400">Chưa gán {roleLabel.toLowerCase()}</p>
      ) : (
        <ul className="mt-1 flex flex-col gap-1">
          {activeStaff.map((item) => (
            <li key={item.id} className="flex items-center justify-between gap-2">
              <span className="text-[14px] text-ink-900">{item.teacher_name}</span>
              <HvButton size="sm" variant="ghost" onClick={() => setRemoving(item)}>
                Gỡ
              </HvButton>
            </li>
          ))}
        </ul>
      )}

      {adding ? (
        <div className="mt-2 flex flex-wrap items-center gap-2">
          <select
            aria-label={`Chọn ${roleLabel.toLowerCase()}`}
            value={pickedId}
            onChange={(event) => setPickedId(event.target.value)}
            disabled={assign.isPending}
            className="min-h-11 rounded-[var(--radius-md)] border border-line-200 bg-white px-3 text-[14px] text-ink-900"
          >
            <option value="">— Chọn thành viên —</option>
            {assignableMembers.map((member) => (
              <option key={member.id} value={member.id}>
                {member.full_name}
              </option>
            ))}
          </select>
          <HvButton size="sm" onClick={handleAssign} disabled={!pickedId || assign.isPending}>
            {assign.isPending ? "Đang thêm…" : "Xác nhận"}
          </HvButton>
          <HvButton
            size="sm"
            variant="ghost"
            onClick={() => {
              setAdding(false);
              setPickedId("");
            }}
            disabled={assign.isPending}
          >
            Huỷ
          </HvButton>
        </div>
      ) : (
        <HvButton
          className="mt-2"
          size="sm"
          variant="secondary"
          onClick={() => setAdding(true)}
          disabled={assignableMembers.length === 0}
        >
          + Thêm {roleLabel.toLowerCase()}
        </HvButton>
      )}

      {errorMessage ? <p className="mt-1 text-[13px] text-coral-600">{errorMessage}</p> : null}

      {removing ? (
        <RemoveStaffDialog
          classId={classId}
          staff={removing}
          onOpenChange={(open) => {
            if (!open) {
              setRemoving(null);
            }
          }}
        />
      ) : null}
    </div>
  );
}

/**
 * Two removal modes: "Kết thúc vai trò" soft-closes immediately (the person
 * keeps read access to the class's history — not destructive enough to need
 * a second click), while "Gán nhầm — thu hồi" hard-deletes the row and needs
 * an explicit second confirm, matching the two-click arm/confirm pattern the
 * teacher-handoff card on this page already uses.
 */
function RemoveStaffDialog({
  classId,
  staff,
  onOpenChange,
  voidOnly = false,
}: {
  classId: string;
  staff: ClassStaff;
  onOpenChange: (open: boolean) => void;
  /**
   * Entry point was an already-ended stint (the "Đã kết thúc" list) — there is
   * no soft-close left to offer, so the dialog opens straight into the armed
   * void step and "Quay lại" cancels instead of stepping back to it.
   */
  voidOnly?: boolean;
}) {
  const [voidArmed, setVoidArmed] = useState(voidOnly);
  const remove = useRemoveClassStaff(classId);
  const errorMessage = apiErrorMessage(remove.error, "Không gỡ được nhân sự. Thử lại sau.");

  function handleSoftClose() {
    remove.mutate(
      { staffId: staff.id },
      {
        onSuccess: () => {
          hvToast(`Đã kết thúc vai trò của ${staff.teacher_name}`);
          onOpenChange(false);
        },
      },
    );
  }

  function handleVoid() {
    if (!voidArmed) {
      setVoidArmed(true);
      return;
    }
    remove.mutate(
      { staffId: staff.id, options: { void: true } },
      {
        onSuccess: () => {
          hvToast(`Đã thu hồi vai trò của ${staff.teacher_name}`);
          onOpenChange(false);
        },
      },
    );
  }

  return (
    <HvModal
      open
      onOpenChange={onOpenChange}
      title={`Gỡ ${staff.teacher_name}`}
      footer={
        voidArmed ? (
          <>
            <HvButton
              type="button"
              variant="ghost"
              onClick={() => (voidOnly ? onOpenChange(false) : setVoidArmed(false))}
              disabled={remove.isPending}
            >
              {voidOnly ? "Huỷ" : "Quay lại"}
            </HvButton>
            <HvButton
              type="button"
              variant="danger"
              onClick={handleVoid}
              disabled={remove.isPending}
            >
              {remove.isPending ? "Đang thu hồi…" : "Xác nhận thu hồi"}
            </HvButton>
          </>
        ) : (
          <>
            <HvButton
              type="button"
              variant="ghost"
              onClick={() => onOpenChange(false)}
              disabled={remove.isPending}
            >
              Huỷ
            </HvButton>
            <HvButton
              type="button"
              variant="secondary"
              onClick={handleSoftClose}
              disabled={remove.isPending}
            >
              {remove.isPending ? "Đang lưu…" : "Kết thúc vai trò"}
            </HvButton>
            <HvButton
              type="button"
              variant="danger"
              onClick={handleVoid}
              disabled={remove.isPending}
            >
              Gán nhầm — thu hồi
            </HvButton>
          </>
        )
      }
    >
      {voidArmed ? (
        <p>
          Thu hồi sẽ xoá hẳn bản ghi này — <strong>{staff.teacher_name}</strong> mất luôn quyền xem
          lịch sử lớp này. Dùng khi gán nhầm người.
        </p>
      ) : (
        <p>
          Kết thúc vai trò giữ lại lịch sử — <strong>{staff.teacher_name}</strong> vẫn xem được dữ
          liệu cũ của lớp. Nếu gán nhầm ngay từ đầu, chọn thu hồi.
        </p>
      )}
      {errorMessage ? <p className="mt-2 text-[13px] text-coral-600">{errorMessage}</p> : null}
    </HvModal>
  );
}
