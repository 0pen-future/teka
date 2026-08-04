import { HvCard } from "@/components/hv";

/**
 * Rendered for every failure mode on `/s/:token` — unknown, malformed,
 * revoked, expired, already-paid, or soft-deleted token, plus any network or
 * server error. The token maps to a specific family, so naming a student, a
 * teacher, a class, or an HTTP status here would leak information to whoever
 * holds (or guesses) the link. There is exactly one neutral message and no
 * retry button, so a wrong token is not an invitation to hammer the endpoint.
 */
export function StatementError() {
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
          Không mở được liên kết này.
        </p>
        <p className="text-[15px] text-ink-500">
          Liên kết có thể đã hết hạn. Vui lòng liên hệ thầy/cô để nhận liên kết mới.
        </p>
      </div>
    </HvCard>
  );
}
