import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { describe, expect, it, vi } from "vitest";

import { API_URL, fail, ok } from "@/test/msw/handlers";
import { server } from "@/test/msw/server";
import { renderWithProviders } from "@/test/utils";

import { ZaloLinkModal } from "../components/zalo-link-modal";
import { ZALO_CONSENT } from "../schemas/zalo-schemas";

import { mockZaloLink, testLinkId, testQrPng } from "./zalo-handlers";

/** Polls run on a ~1.5s interval; give a scripted transition room to land. */
const pollTimeout = { timeout: 6000 };
/** Must exceed pollTimeout, or a waitFor can never spend its stated budget. */
const SLOW_TEST_MS = 15_000;

function renderModal(overrides: { onLinked?: () => void; onOpenChange?: () => void } = {}) {
  const onLinked = overrides.onLinked ?? vi.fn();
  const onOpenChange = overrides.onOpenChange ?? vi.fn();
  const result = renderWithProviders(
    <ZaloLinkModal open onOpenChange={onOpenChange} onLinked={onLinked} />,
  );
  return { ...result, onLinked, onOpenChange };
}

async function acceptConsent() {
  const user = userEvent.setup();
  await user.click(await screen.findByLabelText(ZALO_CONSENT.checkboxLabel));
  await user.click(screen.getByRole("button", { name: "Tiếp tục" }));
  return user;
}

describe("ZaloLinkModal", () => {
  it("gates Tiếp tục behind the consent checkbox and sends the acknowledged version", async () => {
    const calls = mockZaloLink([{ state: "qr_ready", qr_png: testQrPng }]);
    renderModal();
    const user = userEvent.setup();

    const consent = await screen.findByLabelText(ZALO_CONSENT.checkboxLabel);
    expect(screen.getByRole("button", { name: "Tiếp tục" })).toBeDisabled();
    expect(calls.start).toBe(0);

    await user.click(consent);
    await user.click(screen.getByRole("button", { name: "Tiếp tục" }));

    await waitFor(() => expect(calls.start).toBe(1));
    expect(calls.consentVersions).toEqual([ZALO_CONSENT.version]);
  });

  it("renders the QR from the base64 payload with a countdown and a mobile save link", async () => {
    mockZaloLink([{ state: "qr_ready", qr_png: testQrPng }]);
    renderModal();
    await acceptConsent();

    const qr = await screen.findByAltText("Mã QR đăng nhập Zalo");
    expect(qr).toHaveAttribute("src", `data:image/png;base64,${testQrPng}`);
    expect(screen.getByRole("timer")).toHaveTextContent(/\d+/);

    // Scanning a QR on the phone that shows it is impossible — the download
    // path is the mobile escape hatch and must exist, hidden only from md up.
    const save = screen.getByRole("link", { name: "Lưu ảnh QR" });
    expect(save).toHaveAttribute("href", `data:image/png;base64,${testQrPng}`);
    expect(save).toHaveAttribute("download");
    expect(save.className).toContain("md:hidden");
  });

  it("swaps the QR for the waiting view once the attempt is scanned", async () => {
    mockZaloLink([{ state: "qr_ready", qr_png: testQrPng }, { state: "scanned" }]);
    renderModal();
    await acceptConsent();

    await screen.findByAltText("Mã QR đăng nhập Zalo");
    expect(await screen.findByText(/Đã quét/, undefined, pollTimeout)).toBeInTheDocument();
    expect(screen.queryByAltText("Mã QR đăng nhập Zalo")).not.toBeInTheDocument();
  });

  it(
    "reports the link and stops polling once the attempt is linked",
    async () => {
      const calls = mockZaloLink([
        { state: "qr_ready", qr_png: testQrPng },
        { state: "linked", display_name: "Cô Lan" },
      ]);
      const { onLinked } = renderModal();
      await acceptConsent();

      await waitFor(() => expect(onLinked).toHaveBeenCalledTimes(1), pollTimeout);
      expect(onLinked).toHaveBeenCalledWith("Cô Lan");

      // A terminal state must switch the interval off, not merely stop rendering.
      const pollsAtLink = calls.polls;
      await new Promise((resolve) => setTimeout(resolve, 2500));
      expect(calls.polls).toBe(pollsAtLink);
    },
    SLOW_TEST_MS,
  );

  it(
    "stops polling when the modal closes",
    async () => {
      const calls = mockZaloLink([{ state: "qr_ready", qr_png: testQrPng }]);
      const { unmount } = renderModal();
      await acceptConsent();
      await screen.findByAltText("Mã QR đăng nhập Zalo");

      // The card mounts this modal only while open, so closing is an unmount.
      unmount();
      const pollsAtClose = calls.polls;
      await new Promise((resolve) => setTimeout(resolve, 2500));

      expect(calls.polls).toBe(pollsAtClose);
    },
    SLOW_TEST_MS,
  );

  it(
    "offers a fresh challenge when the attempt expires, and recovers on retry",
    async () => {
      const calls = mockZaloLink([{ state: "qr_ready", qr_png: testQrPng }, { state: "expired" }]);
      renderModal();
      const user = await acceptConsent();

      const retry = await screen.findByRole("button", { name: "Tạo mã mới" }, pollTimeout);
      await user.click(retry);

      await waitFor(() => expect(calls.start).toBe(2));
      // The server may hand back the same id; the retry must still recover
      // rather than re-reading the previous attempt's terminal result.
      expect(
        await screen.findByAltText("Mã QR đăng nhập Zalo", undefined, pollTimeout),
      ).toBeInTheDocument();
      expect(calls.polledIds.every((id) => id === testLinkId)).toBe(true);
    },
    SLOW_TEST_MS,
  );

  it(
    "gives up and offers a retry when the attempt can no longer be polled",
    async () => {
      let polls = 0;
      server.use(
        http.post(`${API_URL}/me/zalo/link/start`, () =>
          HttpResponse.json(ok({ link_id: testLinkId }), { status: 202 }),
        ),
        // The server keeps one attempt per teacher: a second tab replacing this
        // one makes every further poll a 404, forever.
        http.get(`${API_URL}/me/zalo/link/status`, () => {
          polls += 1;
          return HttpResponse.json(fail("not_found", "link attempt not found"), { status: 404 });
        }),
      );
      renderModal();
      await acceptConsent();

      expect(
        await screen.findByRole("button", { name: "Tạo mã mới" }, pollTimeout),
      ).toBeInTheDocument();

      const pollsAtFailure = polls;
      await new Promise((resolve) => setTimeout(resolve, 2500));
      expect(polls).toBe(pollsAtFailure);
    },
    SLOW_TEST_MS,
  );

  it(
    "keeps failure copy Vietnamese instead of echoing the server's English string",
    async () => {
      mockZaloLink([{ state: "error", error_message: "could not complete the Zalo login" }]);
      renderModal();
      await acceptConsent();

      const dialog = await screen.findByRole("dialog");
      expect(
        await within(dialog).findByText(
          /Không hoàn tất được đăng nhập Zalo/,
          undefined,
          pollTimeout,
        ),
      ).toBeInTheDocument();
      expect(
        within(dialog).queryByText(/could not complete the Zalo login/),
      ).not.toBeInTheDocument();
    },
    SLOW_TEST_MS,
  );
});
