import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { describe, expect, it } from "vitest";

import { API_URL, ok } from "@/test/msw/handlers";
import { server } from "@/test/msw/server";
import { renderWithProviders } from "@/test/utils";

import { ZaloConnectCard } from "../components/zalo-connect-card";
import { ZALO_CONSENT } from "../schemas/zalo-schemas";

import { mockZaloLink, mockZaloStatus, testQrPng } from "./zalo-handlers";

const pollTimeout = { timeout: 6000 };

describe("ZaloConnectCard", () => {
  it("invites the teacher to link when no account is connected", async () => {
    renderWithProviders(<ZaloConnectCard />);

    expect(await screen.findByRole("button", { name: "Đăng nhập với Zalo" })).toBeInTheDocument();
    expect(screen.getByText(/Chưa kết nối/)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Ngắt kết nối" })).not.toBeInTheDocument();
  });

  it("shows the linked account and no login button", async () => {
    mockZaloStatus({ linked: true, display_name: "Cô Lan", status: "linked" });
    renderWithProviders(<ZaloConnectCard />);

    expect(await screen.findByText("Đã kết nối · Cô Lan")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Ngắt kết nối" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Đăng nhập với Zalo" })).not.toBeInTheDocument();
  });

  it("says so when the status cannot be read, instead of claiming not-connected", async () => {
    server.use(http.get(`${API_URL}/me/zalo`, () => new HttpResponse(null, { status: 500 })));
    renderWithProviders(<ZaloConnectCard />);

    expect(await screen.findByText(/Không tải được trạng thái/)).toBeInTheDocument();
    // Offering "connect" here would invite an already-linked teacher to link twice.
    expect(screen.queryByRole("button", { name: "Đăng nhập với Zalo" })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Thử lại" })).toBeInTheDocument();
  });

  it("asks for a rescan when the stored session expired", async () => {
    mockZaloStatus({ linked: true, display_name: "Cô Lan", status: "expired" });
    renderWithProviders(<ZaloConnectCard />);

    expect(await screen.findByRole("button", { name: "Quét lại mã" })).toBeInTheDocument();
    expect(screen.getByText(/hết hạn/)).toBeInTheDocument();
  });

  it("unlinks after a confirm and returns to the not-connected state", async () => {
    let linked = true;
    let deletes = 0;
    server.use(
      http.get(`${API_URL}/me/zalo`, () =>
        HttpResponse.json(
          ok(
            linked ? { linked: true, display_name: "Cô Lan", status: "linked" } : { linked: false },
          ),
        ),
      ),
      http.delete(`${API_URL}/me/zalo`, () => {
        deletes += 1;
        linked = false;
        return new HttpResponse(null, { status: 204 });
      }),
    );
    renderWithProviders(<ZaloConnectCard />);
    const user = userEvent.setup();

    await user.click(await screen.findByRole("button", { name: "Ngắt kết nối" }));
    const dialog = await screen.findByRole("dialog");
    await user.click(within(dialog).getByRole("button", { name: "Ngắt kết nối" }));

    await waitFor(() => expect(deletes).toBe(1));
    expect(await screen.findByRole("button", { name: "Đăng nhập với Zalo" })).toBeInTheDocument();
  });

  it("closes the modal and flips to linked when the poll reports success", async () => {
    let linked = false;
    server.use(
      http.get(`${API_URL}/me/zalo`, () =>
        HttpResponse.json(
          ok(
            linked ? { linked: true, display_name: "Cô Lan", status: "linked" } : { linked: false },
          ),
        ),
      ),
    );
    mockZaloLink([
      { state: "qr_ready", qr_png: testQrPng },
      { state: "linked", display_name: "Cô Lan" },
    ]);
    renderWithProviders(<ZaloConnectCard />);
    const user = userEvent.setup();

    await user.click(await screen.findByRole("button", { name: "Đăng nhập với Zalo" }));
    await user.click(await screen.findByLabelText(ZALO_CONSENT.checkboxLabel));
    await user.click(screen.getByRole("button", { name: "Tiếp tục" }));
    await screen.findByAltText("Mã QR đăng nhập Zalo");
    linked = true;

    expect(await screen.findByText(/Đã kết nối Zalo/, undefined, pollTimeout)).toBeInTheDocument();
    expect(await screen.findByText("Đã kết nối · Cô Lan")).toBeInTheDocument();
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  }, 15_000);
});
