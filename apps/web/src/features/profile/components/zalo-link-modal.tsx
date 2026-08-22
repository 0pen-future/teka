import * as React from "react";

import { HvButton, HvModal } from "@/components/hv";
import { Spinner } from "@/components/shared/spinner";

import { useStartZaloLink, useZaloLinkStatus, ZALO_MAX_POLL_ERRORS } from "../hooks/use-zalo";
import { isTerminalLinkState, ZALO_CONSENT } from "../schemas/zalo-schemas";

/**
 * How long the server lets one attempt run. The countdown is informational —
 * the server expires the attempt, and the next poll reports it.
 */
const ATTEMPT_TTL_SECONDS = 105;

export interface ZaloLinkModalProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Called once, when a poll reports the account is linked. */
  onLinked: (displayName?: string) => void;
}

/**
 * Consent → QR → linked, in one modal. Step one is the acknowledgement (no
 * attempt starts until the box is ticked); step two shows the challenge and
 * polls until it resolves. `scanned`/`confirmed` deliberately replace the QR
 * with a "waiting on your phone" view — the teacher's next action is there,
 * not on this screen.
 *
 * Mount this only while it is open: unmounting is what drops a finished
 * attempt, so reopening always starts from consent with a fresh challenge
 * instead of one the server has since expired.
 */
export function ZaloLinkModal({ open, onOpenChange, onLinked }: ZaloLinkModalProps) {
  const [accepted, setAccepted] = React.useState(false);
  // Which half of the flow is on screen. Derived state cannot express this: a
  // retry clears the attempt id, and "no id yet" must not send an already
  // consented teacher back to the checkbox.
  const [phase, setPhase] = React.useState<"consent" | "attempt">("consent");
  const [linkId, setLinkId] = React.useState<string>();
  const [startedAt, setStartedAt] = React.useState(0);
  const [secondsLeft, setSecondsLeft] = React.useState(ATTEMPT_TTL_SECONDS);
  const reportedRef = React.useRef(false);

  const { mutate: startLink, isPending: isStarting, isError: startFailed } = useStartZaloLink();
  const { data: attempt, errorUpdateCount } = useZaloLinkStatus(linkId);
  // A single failed poll is ridden out; the attempt is only declared lost once
  // the hook has stopped polling it.
  const pollFailed = errorUpdateCount >= ZALO_MAX_POLL_ERRORS;
  const state = attempt?.state;
  const consentCheckboxId = React.useId();

  const begin = React.useCallback(() => {
    // Drop the previous attempt's query first. Its cached terminal state would
    // otherwise survive a retry that reuses the same id, leaving the poll
    // switched off and the button apparently doing nothing.
    reportedRef.current = false;
    setPhase("attempt");
    setLinkId(undefined);
    setStartedAt(0);
    startLink(ZALO_CONSENT.version, {
      onSuccess: (started) => {
        setLinkId(started.link_id);
        setStartedAt(Date.now());
        setSecondsLeft(ATTEMPT_TTL_SECONDS);
      },
    });
  }, [startLink]);

  React.useEffect(() => {
    if (startedAt === 0 || isTerminalLinkState(state)) {
      return;
    }
    let timer = 0;
    const tick = () => {
      const elapsed = Math.floor((Date.now() - startedAt) / 1000);
      const left = Math.max(0, ATTEMPT_TTL_SECONDS - elapsed);
      setSecondsLeft(left);
      if (left === 0) {
        window.clearInterval(timer);
      }
    };
    tick();
    timer = window.setInterval(tick, 1000);
    return () => window.clearInterval(timer);
  }, [startedAt, state]);

  React.useEffect(() => {
    if (state === "linked" && !reportedRef.current) {
      reportedRef.current = true;
      onLinked(attempt?.display_name);
    }
  }, [state, attempt?.display_name, onLinked]);

  // The countdown running out is treated as expiry locally: waiting for the
  // server to say so leaves a dead QR on screen when the polls cannot land.
  const timedOut = startedAt !== 0 && secondsLeft === 0 && !isTerminalLinkState(state);
  const failed = startFailed || pollFailed || timedOut || state === "expired" || state === "error";
  const showConsent = phase === "consent";
  const qrSource = attempt?.qr_png ? `data:image/png;base64,${attempt.qr_png}` : undefined;

  /**
   * Always client-owned Vietnamese. The server sends one fixed English
   * sentence for every failure and deliberately withholds the upstream detail,
   * so `error_message` carries nothing a teacher could act on.
   */
  function failureMessage() {
    if (startFailed) {
      return "Không tạo được mã QR. Vui lòng thử lại.";
    }
    if (pollFailed) {
      return "Mất liên lạc với phiên đăng nhập này — có thể bạn đã mở một mã khác. Tạo mã mới để thử lại.";
    }
    if (state === "error") {
      return "Không hoàn tất được đăng nhập Zalo. Tạo mã mới để thử lại.";
    }
    return "Mã QR đã hết hạn. Tạo mã mới để thử lại.";
  }

  function renderFooter() {
    if (showConsent) {
      return (
        <>
          <HvButton type="button" variant="ghost" onClick={() => onOpenChange(false)}>
            Huỷ
          </HvButton>
          <HvButton type="button" disabled={!accepted || isStarting} onClick={begin}>
            {isStarting ? "Đang tạo mã…" : "Tiếp tục"}
          </HvButton>
        </>
      );
    }
    if (failed) {
      return (
        <HvButton type="button" onClick={begin} disabled={isStarting}>
          Tạo mã mới
        </HvButton>
      );
    }
    return null;
  }

  function renderBody() {
    if (showConsent) {
      return (
        <div className="flex flex-col gap-3">
          <ul className="flex flex-col gap-2 text-[13.5px] text-ink-700">
            {ZALO_CONSENT.points.map((point) => (
              <li key={point} className="flex gap-2.5">
                <span aria-hidden className="font-black text-mint-600">
                  ✓
                </span>
                <span>{point}</span>
              </li>
            ))}
          </ul>
          <label
            htmlFor={consentCheckboxId}
            className="flex cursor-pointer items-start gap-2.5 rounded-xl bg-cream-200 p-3 text-[13.5px] font-bold text-ink-900"
          >
            <input
              id={consentCheckboxId}
              type="checkbox"
              checked={accepted}
              onChange={(event) => setAccepted(event.target.checked)}
              className="mt-0.5 size-5 shrink-0 accent-mint-400"
            />
            {ZALO_CONSENT.checkboxLabel}
          </label>
        </div>
      );
    }

    if (failed) {
      return (
        <p aria-live="polite" className="text-[13.5px] text-ink-700">
          {failureMessage()}
        </p>
      );
    }

    if (state === "scanned" || state === "confirmed") {
      return (
        <div className="flex flex-col items-center gap-3 py-4 text-center">
          <Spinner className="size-8 text-mint-600" />
          <p aria-live="polite" className="font-display text-[15px] font-bold text-ink-900">
            Đã quét · chờ xác nhận trên điện thoại
          </p>
          <p className="text-[13px] text-ink-500">
            Mở Zalo trên điện thoại và bấm đồng ý để hoàn tất kết nối.
          </p>
        </div>
      );
    }

    if (qrSource) {
      return (
        <div className="flex flex-col items-center gap-3">
          <img
            src={qrSource}
            alt="Mã QR đăng nhập Zalo"
            className="size-[220px] rounded-[16px] border border-line-200 bg-white p-2"
          />
          <p className="text-center text-[13px] text-ink-500">
            Mở Zalo trên điện thoại → Quét mã → hướng camera vào mã này.
          </p>
          <div className="w-full">
            <div className="h-[6px] overflow-hidden rounded-full bg-cream-200">
              <div
                className="h-full rounded-full bg-mint-400 transition-[width] duration-1000 ease-linear motion-reduce:transition-none"
                style={{ width: `${(secondsLeft / ATTEMPT_TTL_SECONDS) * 100}%` }}
              />
            </div>
            <p role="timer" className="mt-1.5 text-center text-[12.5px] text-ink-400">
              <span aria-hidden>Mã hết hạn sau {secondsLeft}s</span>
              {/* Announced in 10-second steps: a per-second live region would
                  talk over everything else on the screen. */}
              <span className="sr-only" aria-live="polite">
                Mã QR còn hiệu lực khoảng {Math.ceil(secondsLeft / 10) * 10} giây
              </span>
            </p>
          </div>
          {/* A teacher on their phone cannot scan a code shown on that phone —
              saving the image lets them pick it from the gallery in Zalo.
              Opening in a new tab matters on iOS Safari, which ignores
              `download` for a data: URI: without it the image would replace
              this page and take the running attempt with it. */}
          <a
            href={qrSource}
            download="zalo-qr.png"
            target="_blank"
            rel="noopener"
            className="rounded-xl border-[1.5px] border-line-300 px-3.5 py-2 text-[13px] font-extrabold text-ink-500 transition-colors hover:border-mint-400 hover:text-mint-600 md:hidden"
          >
            Lưu ảnh QR
          </a>
          <p className="text-center text-[12px] text-ink-400 md:hidden">
            Đang dùng điện thoại? Lưu ảnh, mở Zalo → Quét mã → chọn ảnh từ thư viện.
          </p>
        </div>
      );
    }

    return (
      <div className="flex flex-col items-center gap-3 py-6">
        <Spinner className="size-8 text-mint-600" />
        <p className="text-[13.5px] text-ink-500">Đang tạo mã QR…</p>
      </div>
    );
  }

  return (
    <HvModal
      open={open}
      onOpenChange={onOpenChange}
      title="Kết nối Zalo"
      description="Đăng nhập bằng tài khoản Zalo cá nhân để Teka gửi thông báo học phí thay bạn."
      footer={renderFooter()}
    >
      {renderBody()}
    </HvModal>
  );
}
