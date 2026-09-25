import { useState } from "react";
import { Link } from "react-router";

import { HvBadge, HvButton, HvChip, HvConfirmDialog, HvStateBlock, hvToast } from "@/components/hv";
import { useCenterContext } from "@/features/teaching";
import { ApiError } from "@/lib/api/errors";
import { cn } from "@/lib/utils";
import { formatDateTime } from "@/lib/utils/format";

import { ConfirmInvitationDialog } from "../components/confirm-invitation-dialog";
import {
  useAcceptClassInvitation,
  useCancelClassInvitation,
  useClassInvitations,
  useDeclineClassInvitation,
  useRemindClassInvitation,
} from "../hooks/use-class-invitations";
import {
  invitationInView,
  invitationStatusLabel,
  invitationStatusVariant,
  invitationViewLabel,
  invitationViews,
  type InvitationView,
} from "../lib/class-invitation-labels";
import type { ClassInvitation } from "../schemas/roster-schemas";

// Same grid as the class table: the prototype's 8px gap as cell padding, 18px row inset.
const headCellClassName =
  "whitespace-nowrap bg-cream-200 px-1 py-[10px] text-[11.5px] font-extrabold uppercase tracking-[0.4px] text-ink-500 first:pl-[18px] last:pr-[18px]";
const cellClassName =
  "border-t border-line-100 px-1 py-[11px] align-middle first:pl-[18px] last:pr-[18px]";
const actionClassName =
  "rounded-[10px] px-2.5 py-[5px] text-[12px] font-extrabold disabled:cursor-not-allowed disabled:opacity-50";
const primaryActionClassName = cn(actionClassName, "bg-mint-50 text-mint-600 hover:bg-mint-100");
const outlineActionClassName = cn(actionClassName, "border-[1.5px] border-line-200");
const neutralActionClassName = cn(
  outlineActionClassName,
  "text-ink-500 hover:border-sky-300 hover:text-sky-500",
);
const dangerActionClassName = cn(
  outlineActionClassName,
  "text-coral-500 hover:border-coral-300 hover:bg-coral-100",
);

function apiMessage(error: unknown, fallback: string): string {
  return error instanceof ApiError ? error.message : fallback;
}

/**
 * `/class-invitations` — one list for both roles. The API already narrows a
 * member to their own invitations and hands the owner every row, so the page
 * fetches once and filters by status client-side; the chips carry counts
 * from that single fetch. Member rows answer (accept/decline); owner rows
 * remind, cancel and confirm — confirm is what writes the stint.
 */
export function ClassInvitationsPage() {
  const { isOwner } = useCenterContext();
  const [view, setView] = useState<InvitationView>("all");
  const [cancelling, setCancelling] = useState<ClassInvitation | null>(null);
  const [confirming, setConfirming] = useState<ClassInvitation | null>(null);
  const list = useClassInvitations();
  const accept = useAcceptClassInvitation();
  const decline = useDeclineClassInvitation();
  const remind = useRemindClassInvitation();
  const cancel = useCancelClassInvitation();

  const all = list.data ?? [];
  const rows = all.filter((item) => invitationInView(item.status, view));
  const busyId = [accept, decline, remind].find((m) => m.isPending)?.variables ?? null;

  function run(
    mutation: typeof accept,
    invitation: ClassInvitation,
    success: string,
    failure: string,
  ) {
    mutation.mutate(invitation.id, {
      onSuccess: () => hvToast(success),
      onError: (error) => hvToast(apiMessage(error, failure), { variant: "danger" }),
    });
  }

  return (
    <div className="flex flex-col gap-4">
      <div>
        <h1 className="font-display text-[26px] font-extrabold text-ink-900">Lời mời nhận lớp</h1>
        <p className="mt-1 text-[14px] text-ink-500">
          Giáo viên được mời vào lớp phải xác nhận trước khi xuất hiện trong Đội ngũ giảng dạy.
        </p>
      </div>

      <div role="radiogroup" aria-label="Lọc theo trạng thái" className="flex flex-wrap gap-2">
        {invitationViews.map((item) => (
          <HvChip
            key={item}
            role="radio"
            size="sm"
            pressed={item === view}
            count={
              list.data ? all.filter((row) => invitationInView(row.status, item)).length : undefined
            }
            onClick={() => setView(item)}
          >
            {invitationViewLabel[item]}
          </HvChip>
        ))}
      </div>

      {list.isPending ? (
        <HvStateBlock state="loading" title="Đang tải lời mời" />
      ) : list.isError ? (
        <HvStateBlock
          state="error"
          title="Không tải được lời mời"
          action={
            <HvButton size="sm" variant="ghost" onClick={() => void list.refetch()}>
              Thử lại
            </HvButton>
          }
        />
      ) : (
        <div className="overflow-x-auto rounded-[20px] bg-white shadow-soft-md">
          <table className="w-full min-w-[820px] table-fixed border-collapse text-left text-[13.5px]">
            <colgroup>
              <col className="w-[62px]" />
              <col className="w-[30%]" />
              <col className="w-[21%]" />
              <col className="w-[15%]" />
              <col className="w-[15%]" />
              <col className="w-[238px]" />
            </colgroup>
            <thead>
              <tr>
                <th className={headCellClassName}>STT</th>
                <th className={headCellClassName}>Lớp học</th>
                <th className={headCellClassName}>Giáo viên</th>
                <th className={headCellClassName}>Gửi lúc</th>
                <th className={headCellClassName}>Trạng thái</th>
                <th className={headCellClassName}>
                  <span className="sr-only">Thao tác</span>
                </th>
              </tr>
            </thead>
            <tbody>
              {rows.length === 0 ? (
                <tr>
                  <td
                    colSpan={6}
                    className="border-t border-line-100 p-[34px] text-center font-bold text-ink-400"
                  >
                    Không có lời mời nào.
                  </td>
                </tr>
              ) : null}
              {rows.map((item, index) => {
                const busy = busyId === item.id;
                const open = item.status === "pending" || item.status === "accepted";
                return (
                  <tr key={item.id} className="transition-colors hover:bg-cream-100">
                    <td className={cn(cellClassName, "font-extrabold text-ink-400")}>
                      {index + 1}
                    </td>
                    <td className={cellClassName}>
                      {isOwner || item.status === "assigned" ? (
                        <Link
                          to={`/classes/${item.class_id}`}
                          className="font-extrabold text-ink-900 hover:text-sky-500"
                        >
                          {item.class_name}
                        </Link>
                      ) : (
                        // The class detail is own-rows for members: until the
                        // stint exists the invitee has nothing to open there.
                        <span className="font-extrabold text-ink-900">{item.class_name}</span>
                      )}
                      <div className="truncate text-[12px] text-ink-400">
                        {item.course_name ?? "Chưa gắn khóa"}
                      </div>
                      {item.message ? (
                        <div className="mt-0.5 text-[12px] italic text-ink-500">
                          “{item.message}”
                        </div>
                      ) : null}
                    </td>
                    <td className={cellClassName}>
                      <div className="font-bold text-ink-900">{item.teacher_name}</div>
                      <div className="text-[12px] text-ink-400">{item.role_label}</div>
                    </td>
                    <td className={cn(cellClassName, "text-ink-500")}>
                      {formatDateTime(item.sent_at)}
                      {item.reminded_at ? (
                        <span className="block text-[12px] text-ink-400">
                          Nhắc lại {formatDateTime(item.reminded_at)}
                        </span>
                      ) : null}
                    </td>
                    <td className={cellClassName}>
                      <HvBadge variant={invitationStatusVariant[item.status]} size="sm">
                        {invitationStatusLabel[item.status]}
                      </HvBadge>
                    </td>
                    <td className={cellClassName}>
                      <div className="flex flex-wrap justify-end gap-1">
                        {isOwner && open ? (
                          <>
                            <button
                              type="button"
                              className={primaryActionClassName}
                              disabled={busy}
                              onClick={() => setConfirming(item)}
                            >
                              {item.role_key === "giao_vien" ? "GV nhận lớp" : "Phân công"}
                            </button>
                            {item.status === "pending" ? (
                              <button
                                type="button"
                                className={neutralActionClassName}
                                disabled={busy}
                                onClick={() =>
                                  run(
                                    remind,
                                    item,
                                    `Đã nhắc lại ${item.teacher_name}`,
                                    "Không nhắc lại được.",
                                  )
                                }
                              >
                                Nhắc lại
                              </button>
                            ) : null}
                            <button
                              type="button"
                              className={dangerActionClassName}
                              disabled={busy}
                              onClick={() => setCancelling(item)}
                            >
                              Hủy
                            </button>
                          </>
                        ) : null}
                        {!isOwner && item.status === "pending" ? (
                          <>
                            <button
                              type="button"
                              className={primaryActionClassName}
                              disabled={busy}
                              onClick={() =>
                                run(
                                  accept,
                                  item,
                                  `Đã nhận lời mời lớp ${item.class_name}`,
                                  "Không nhận được lời mời.",
                                )
                              }
                            >
                              Chấp nhận
                            </button>
                            <button
                              type="button"
                              className={dangerActionClassName}
                              disabled={busy}
                              onClick={() =>
                                run(
                                  decline,
                                  item,
                                  `Đã từ chối lời mời lớp ${item.class_name}`,
                                  "Không từ chối được.",
                                )
                              }
                            >
                              Từ chối
                            </button>
                          </>
                        ) : null}
                      </div>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}

      <HvConfirmDialog
        open={cancelling !== null}
        onOpenChange={(open) => {
          if (!open) setCancelling(null);
        }}
        title={cancelling ? `Hủy lời mời của ${cancelling.teacher_name}?` : "Hủy lời mời?"}
        description="Người được mời sẽ không còn thấy lời mời này. Bạn có thể gửi lại sau."
        confirmLabel="Hủy lời mời"
        cancelLabel="Giữ lại"
        tone="danger"
        pending={cancel.isPending}
        onConfirm={() => {
          if (!cancelling) return;
          cancel.mutate(cancelling.id, {
            onSuccess: () => {
              hvToast(`Đã hủy lời mời của ${cancelling.teacher_name}`);
              setCancelling(null);
            },
            onError: (error) =>
              hvToast(apiMessage(error, "Không hủy được lời mời."), { variant: "danger" }),
          });
        }}
      />

      {confirming ? (
        <ConfirmInvitationDialog
          invitation={confirming}
          onOpenChange={(open) => {
            if (!open) setConfirming(null);
          }}
        />
      ) : null}
    </div>
  );
}
