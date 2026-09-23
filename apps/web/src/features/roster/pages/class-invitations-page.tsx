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

const headCellClassName =
  "sticky top-0 z-10 bg-cream-200 px-[18px] py-[10px] text-[12px] font-extrabold uppercase tracking-[0.4px] text-ink-500";
const cellClassName = "border-t border-line-100 px-[18px] py-[11px] align-middle";

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
          {isOwner
            ? "Theo dõi lời mời đã gửi; phân công khi giáo viên đã đồng ý."
            : "Lời mời tham gia lớp gửi đến bạn. Chấp nhận để chủ trung tâm phân công."}
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
      ) : rows.length === 0 ? (
        <HvStateBlock
          state="empty"
          title={
            all.length === 0 ? "Chưa có lời mời nào." : "Không có lời mời nào ở trạng thái này."
          }
          description={
            all.length === 0 && isOwner
              ? "Mời giáo viên từ mục Đội ngũ giảng dạy của một lớp."
              : undefined
          }
        />
      ) : (
        <div className="overflow-x-auto rounded-[var(--radius-lg)] border border-line-200 bg-white">
          <table className="w-full min-w-[880px] border-collapse text-left text-[14px]">
            <thead>
              <tr>
                <th className={headCellClassName}>Lớp</th>
                <th className={headCellClassName}>GV</th>
                <th className={headCellClassName}>Vai trò</th>
                <th className={headCellClassName}>Gửi lúc</th>
                <th className={headCellClassName}>Trạng thái</th>
                <th className={cn(headCellClassName, "text-right")}>
                  <span className="sr-only">Thao tác</span>
                </th>
              </tr>
            </thead>
            <tbody>
              {rows.map((item) => {
                const busy = busyId === item.id;
                return (
                  <tr key={item.id}>
                    <td className={cn(cellClassName, "font-extrabold text-ink-900")}>
                      {isOwner || item.status === "assigned" ? (
                        <Link to={`/classes/${item.class_id}`} className="hover:text-mint-600">
                          {item.class_name}
                        </Link>
                      ) : (
                        // The class detail is own-rows for members: until the
                        // stint exists the invitee has nothing to open there.
                        item.class_name
                      )}
                      {item.message ? (
                        <p className="mt-0.5 text-[12px] font-normal text-ink-500">
                          {item.message}
                        </p>
                      ) : null}
                    </td>
                    <td className={cn(cellClassName, "text-ink-900")}>{item.teacher_name}</td>
                    <td className={cn(cellClassName, "text-ink-700")}>{item.role_label}</td>
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
                    <td className={cn(cellClassName, "text-right")}>
                      <div className="flex flex-wrap justify-end gap-1">
                        {isOwner ? (
                          <>
                            {item.status === "pending" ? (
                              <HvButton
                                size="sm"
                                variant="ghost"
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
                              </HvButton>
                            ) : null}
                            {item.status === "pending" || item.status === "accepted" ? (
                              <>
                                <HvButton
                                  size="sm"
                                  variant="ghost"
                                  disabled={busy}
                                  onClick={() => setCancelling(item)}
                                >
                                  Hủy
                                </HvButton>
                                <HvButton
                                  size="sm"
                                  disabled={busy}
                                  onClick={() => setConfirming(item)}
                                >
                                  {item.role_key === "giao_vien" ? "GV nhận lớp" : "Phân công"}
                                </HvButton>
                              </>
                            ) : null}
                          </>
                        ) : item.status === "pending" ? (
                          <>
                            <HvButton
                              size="sm"
                              variant="ghost"
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
                            </HvButton>
                            <HvButton
                              size="sm"
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
                            </HvButton>
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
