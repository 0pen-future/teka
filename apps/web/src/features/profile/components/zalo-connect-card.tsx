import * as React from "react";
import { useQueryClient } from "@tanstack/react-query";

import { HvButton, HvCard, HvModal, hvToast } from "@/components/hv";

import { useUnlinkZalo, useZaloStatus, zaloKeys } from "../hooks/use-zalo";

import { ZaloLinkModal } from "./zalo-link-modal";

/**
 * "Kết nối Zalo" on the profile page: not connected, connected as someone, or
 * connected but expired. Only the last two offer `Ngắt kết nối`; an expired
 * session says "quét lại" rather than "kết nối", because the account is still
 * linked — only its stored session died.
 */
export function ZaloConnectCard() {
  const queryClient = useQueryClient();
  const { data: status, isPending, isError, refetch } = useZaloStatus();
  const unlinkMutation = useUnlinkZalo();
  const [linkOpen, setLinkOpen] = React.useState(false);
  const [confirmOpen, setConfirmOpen] = React.useState(false);

  const linked = status?.linked === true;
  const expired = linked && status?.status === "expired";
  const accountName = status?.display_name ?? "tài khoản Zalo";

  // Closing covers the case where the account linked server-side just as the
  // teacher dismissed the modal: without this the card claims "chưa kết nối"
  // until the status query goes stale on its own.
  const handleLinkOpenChange = React.useCallback(
    (next: boolean) => {
      setLinkOpen(next);
      if (!next) {
        void queryClient.invalidateQueries({ queryKey: zaloKeys.status() });
      }
    },
    [queryClient],
  );

  const handleLinked = React.useCallback(
    (displayName?: string) => {
      handleLinkOpenChange(false);
      hvToast(displayName ? `Đã kết nối Zalo · ${displayName}` : "Đã kết nối Zalo", {
        variant: "success",
      });
    },
    [handleLinkOpenChange],
  );

  function handleUnlink() {
    unlinkMutation.mutate(undefined, {
      onSuccess: () => {
        setConfirmOpen(false);
        hvToast("Đã ngắt kết nối Zalo");
      },
      onError: () => {
        hvToast("Không ngắt kết nối được. Vui lòng thử lại.", { variant: "danger" });
      },
    });
  }

  return (
    <HvCard>
      <p className="font-display text-[17px] font-bold text-ink-900">Kết nối Zalo</p>
      <p className="mt-0.5 text-[12.5px] text-ink-500">
        Đăng nhập bằng Zalo để gửi thông báo học phí hàng loạt từ chính tài khoản của bạn.
      </p>

      {isPending ? (
        <p className="mt-3.5 text-[13px] text-ink-400">Đang tải trạng thái kết nối…</p>
      ) : null}

      {/* An unreadable status must not be drawn as "not connected": a teacher
          who is already linked would be invited to link a second time. */}
      {isError ? (
        <div className="mt-3.5 flex flex-col items-start gap-2.5">
          <p className="text-[12.5px] text-coral-500">Không tải được trạng thái kết nối Zalo.</p>
          <HvButton type="button" size="sm" variant="ghost" onClick={() => void refetch()}>
            Thử lại
          </HvButton>
        </div>
      ) : null}

      {!isPending && !isError && linked ? (
        <div className="mt-3.5 flex flex-col gap-2.5">
          <p className="font-display text-[15px] font-bold text-ink-900">
            Đã kết nối · {accountName}
          </p>
          {expired ? (
            <p className="text-[12.5px] text-coral-500">
              Phiên Zalo đã hết hạn — quét lại mã để tiếp tục gửi thông báo.
            </p>
          ) : (
            <p className="text-[12.5px] text-ink-400">
              Thông báo học phí sẽ gửi từ tài khoản Zalo này.
            </p>
          )}
          <div className="flex flex-wrap gap-2">
            {expired ? (
              <HvButton type="button" size="sm" onClick={() => setLinkOpen(true)}>
                Quét lại mã
              </HvButton>
            ) : null}
            <HvButton type="button" size="sm" variant="ghost" onClick={() => setConfirmOpen(true)}>
              Ngắt kết nối
            </HvButton>
          </div>
        </div>
      ) : null}

      {!isPending && !isError && !linked ? (
        <>
          <button
            type="button"
            onClick={() => setLinkOpen(true)}
            className="mt-3.5 flex w-full cursor-pointer items-center gap-3 rounded-[16px] bg-[#0068ff] px-4 py-3 text-[14.5px] font-extrabold text-white shadow-[0_5px_0_#0052cc] transition-transform active:translate-y-1 active:shadow-none"
          >
            <span
              aria-hidden
              className="flex size-8 shrink-0 items-center justify-center rounded-[10px] bg-white text-[11px] font-black text-[#0068ff]"
            >
              Zalo
            </span>
            Đăng nhập với Zalo
          </button>
          <p className="mt-2.5 text-[12px] text-ink-400">
            Chưa kết nối — thông báo học phí sẽ phải sao chép và gửi thủ công.
          </p>
        </>
      ) : null}

      {/* Mounted only while open so a closed attempt leaves nothing behind. */}
      {linkOpen ? (
        <ZaloLinkModal open onOpenChange={handleLinkOpenChange} onLinked={handleLinked} />
      ) : null}

      <HvModal
        open={confirmOpen}
        onOpenChange={setConfirmOpen}
        title="Ngắt kết nối Zalo"
        footer={
          <>
            <HvButton type="button" variant="ghost" onClick={() => setConfirmOpen(false)}>
              Huỷ
            </HvButton>
            <HvButton
              type="button"
              variant="danger"
              disabled={unlinkMutation.isPending}
              onClick={handleUnlink}
            >
              {unlinkMutation.isPending ? "Đang ngắt…" : "Ngắt kết nối"}
            </HvButton>
          </>
        }
      >
        <p>
          Teka sẽ xoá phiên đăng nhập Zalo đã lưu. Thông báo học phí sẽ phải sao chép và gửi thủ
          công cho đến khi bạn kết nối lại.
        </p>
      </HvModal>
    </HvCard>
  );
}
