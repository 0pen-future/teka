import { HvButton, hvToast } from "@/components/hv";
import { formatPhoneLocal } from "@/lib/utils";

import { useInvites, useRevokeInvite } from "../hooks/use-invitation";
import { formatExpiresAt } from "../lib/format-expiry";

/**
 * Only pending invites are actionable (revoke), so this list shows pending
 * ones only — an accepted, revoked, or expired row would just be dead
 * weight with no action to take on it.
 */
export function InviteList() {
  const { data, isPending, isError } = useInvites();
  const revokeMutation = useRevokeInvite();

  if (isPending) {
    return <p className="text-[13px] text-ink-400">Đang tải lời mời…</p>;
  }
  if (isError) {
    return <p className="text-[13px] text-ink-500">Không tải được danh sách lời mời.</p>;
  }

  const pending = data.filter((invite) => invite.status === "pending");

  function handleRevoke(id: string, phone: string) {
    revokeMutation.mutate(id, {
      onSuccess: () =>
        hvToast(`Đã thu hồi lời mời ${formatPhoneLocal(phone)}`, { variant: "success" }),
      onError: () => hvToast("Có lỗi xảy ra, thử lại sau", { variant: "danger" }),
    });
  }

  return (
    <div>
      <p className="font-display text-[15px] font-bold text-ink-900">Lời mời đang chờ</p>
      {pending.length === 0 ? (
        <p className="mt-2 text-[13px] text-ink-400">Chưa có lời mời nào đang chờ.</p>
      ) : (
        <ul className="mt-2 flex flex-col divide-y divide-line-200">
          {pending.map((invite) => (
            <li key={invite.id} className="flex items-center gap-3 py-3">
              <div className="min-w-0 flex-1">
                <p className="text-[14px] font-extrabold text-ink-900">
                  {formatPhoneLocal(invite.phone)}
                </p>
                <p className="text-[12.5px] text-ink-500">
                  Hết hạn {formatExpiresAt(invite.expires_at)}
                </p>
              </div>
              <HvButton
                type="button"
                variant="ghost"
                size="sm"
                aria-label={`Thu hồi lời mời ${formatPhoneLocal(invite.phone)}`}
                disabled={revokeMutation.isPending}
                onClick={() => handleRevoke(invite.id, invite.phone)}
              >
                Thu hồi
              </HvButton>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
