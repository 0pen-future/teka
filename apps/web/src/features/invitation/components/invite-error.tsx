import { HvCard } from "@/components/hv";

/**
 * Rendered for every failure mode on `/invite/:token` — unknown, malformed,
 * revoked, expired, or already-accepted token, plus any network/server
 * error, and the same for a rejected accept submission. There is exactly one
 * neutral message and no reason breakdown: the API's preview/accept contract
 * is anti-enumeration (`invitations.Service.Preview`/`Accept` both collapse
 * every rejection reason into the same generic response), so the UI must not
 * leak more detail than the API already refuses to.
 */
export function InviteError() {
  return (
    <HvCard variant="raised" className="flex flex-col items-center gap-4 py-10 text-center">
      <span
        aria-hidden="true"
        className="flex size-12 items-center justify-center rounded-full bg-coral-100 text-2xl font-bold text-coral-600"
      >
        !
      </span>
      <div className="flex flex-col gap-2">
        <p className="font-display text-[17px] font-bold text-ink-900">
          Không mở được lời mời này.
        </p>
        <p className="text-[15px] text-ink-500">
          Liên kết có thể đã hết hạn hoặc đã được dùng. Vui lòng liên hệ chủ trung tâm để nhận lời
          mời mới.
        </p>
      </div>
    </HvCard>
  );
}
