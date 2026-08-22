import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { describe, expect, it, vi } from "vitest";

import { API_URL, fail, ok } from "@/test/msw/handlers";
import { server } from "@/test/msw/server";
import { renderWithProviders } from "@/test/utils";

import { ZaloConnectCard } from "../components/zalo-connect-card";
import { ZaloLinkModal } from "../components/zalo-link-modal";
import { ZALO_CONSENT } from "../schemas/zalo-schemas";

import { mockZaloLink, testLinkId, testQrPng } from "./zalo-handlers";

const pollTimeout = { timeout: 6000 };

describe("ZaloLinkModal failure paths", () => {
  it("offers a retry when link/start itself fails", async () => {
    let startAttempts = 0;
    server.use(
      http.post(`${API_URL}/me/zalo/link/start`, () => {
        startAttempts += 1;
        return startAttempts === 1
          ? HttpResponse.json(fail("internal_error", "Server error"), { status: 500 })
          : HttpResponse.json(ok({ link_id: testLinkId }), { status: 202 });
      }),
      http.get(`${API_URL}/me/zalo/link/status`, () =>
        HttpResponse.json(ok({ state: "qr_ready", qr_png: testQrPng })),
      ),
    );
    renderWithProviders(<ZaloLinkModal open onOpenChange={vi.fn()} onLinked={vi.fn()} />);
    const user = userEvent.setup();

    await user.click(await screen.findByLabelText(ZALO_CONSENT.checkboxLabel));
    await user.click(screen.getByRole("button", { name: "Tiếp tục" }));

    const retry = await screen.findByRole("button", { name: "Tạo mã mới" });
    expect(screen.getByText(/Không tạo được mã/)).toBeInTheDocument();

    await user.click(retry);
    await screen.findByAltText("Mã QR đăng nhập Zalo");
    expect(startAttempts).toBe(2);
  });

  it("keeps the QR on screen when a poll returns an unparseable body", async () => {
    const calls = mockZaloLink([{ state: "qr_ready", qr_png: testQrPng }]);
    server.use(
      http.get(`${API_URL}/me/zalo/link/status`, ({ request }) => {
        calls.polls += 1;
        calls.polledIds.push(new URL(request.url).searchParams.get("id") ?? "");
        // One bad body mid-flight must not tear down the attempt.
        return calls.polls === 2
          ? HttpResponse.text("invalid", { status: 200 })
          : HttpResponse.json(ok({ state: "qr_ready", qr_png: testQrPng }));
      }),
    );
    renderWithProviders(<ZaloLinkModal open onOpenChange={vi.fn()} onLinked={vi.fn()} />);
    const user = userEvent.setup();

    await user.click(await screen.findByLabelText(ZALO_CONSENT.checkboxLabel));
    await user.click(screen.getByRole("button", { name: "Tiếp tục" }));
    await screen.findByAltText("Mã QR đăng nhập Zalo");

    await new Promise((resolve) => setTimeout(resolve, 2500));

    expect(screen.getByAltText("Mã QR đăng nhập Zalo")).toBeInTheDocument();
  });

  it("treats confirmed like scanned and still reaches linked", async () => {
    mockZaloLink([
      { state: "qr_ready", qr_png: testQrPng },
      { state: "confirmed" },
      { state: "linked", display_name: "Cô Lan" },
    ]);
    const onLinked = vi.fn();
    renderWithProviders(<ZaloLinkModal open onOpenChange={vi.fn()} onLinked={onLinked} />);
    const user = userEvent.setup();

    await user.click(await screen.findByLabelText(ZALO_CONSENT.checkboxLabel));
    await user.click(screen.getByRole("button", { name: "Tiếp tục" }));
    await screen.findByAltText("Mã QR đăng nhập Zalo");

    expect(await screen.findByText(/Đã quét/, undefined, pollTimeout)).toBeInTheDocument();
    expect(screen.queryByAltText("Mã QR đăng nhập Zalo")).not.toBeInTheDocument();

    await waitFor(() => expect(onLinked).toHaveBeenCalledTimes(1), pollTimeout);
  });
});

describe("ZaloConnectCard rescan entry point", () => {
  it("routes an expired session back through consent, not straight to a QR", async () => {
    server.use(
      http.get(`${API_URL}/me/zalo`, () =>
        HttpResponse.json(ok({ linked: true, display_name: "Cô Lan", status: "expired" })),
      ),
    );
    mockZaloLink([{ state: "qr_ready", qr_png: testQrPng }]);
    renderWithProviders(<ZaloConnectCard />);
    const user = userEvent.setup();

    await user.click(await screen.findByRole("button", { name: "Quét lại mã" }));

    expect(await screen.findByLabelText(ZALO_CONSENT.checkboxLabel)).toBeInTheDocument();
    expect(screen.queryByAltText("Mã QR đăng nhập Zalo")).not.toBeInTheDocument();

    await user.click(screen.getByLabelText(ZALO_CONSENT.checkboxLabel));
    await user.click(screen.getByRole("button", { name: "Tiếp tục" }));

    await screen.findByAltText("Mã QR đăng nhập Zalo");
  });
});
