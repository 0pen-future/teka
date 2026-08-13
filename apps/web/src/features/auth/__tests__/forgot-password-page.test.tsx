import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { describe, expect, it } from "vitest";

import { API_URL, fail } from "@/test/msw/handlers";
import { server } from "@/test/msw/server";
import { renderWithProviders } from "@/test/utils";

import { ForgotPasswordPage } from "../pages/forgot-password-page";

const SUCCESS_NOTE = "Nếu số điện thoại hợp lệ, liên kết đặt lại đã được gửi qua Zalo.";

describe("ForgotPasswordPage", () => {
  it("shows the generic confirmation on success", async () => {
    const user = userEvent.setup();
    renderWithProviders(<ForgotPasswordPage />);

    await user.type(screen.getByLabelText("Số điện thoại"), "0901234567");
    await user.click(screen.getByRole("button", { name: "Gửi liên kết đặt lại" }));

    expect(await screen.findByText(SUCCESS_NOTE)).toBeInTheDocument();
  });

  it("shows the exact same generic confirmation even when the request fails", async () => {
    server.use(
      http.post(`${API_URL}/auth/forgot-password`, () =>
        HttpResponse.json(fail("INTERNAL_ERROR", "boom"), { status: 500 }),
      ),
    );
    const user = userEvent.setup();
    renderWithProviders(<ForgotPasswordPage />);

    await user.type(screen.getByLabelText("Số điện thoại"), "0901234567");
    await user.click(screen.getByRole("button", { name: "Gửi liên kết đặt lại" }));

    expect(await screen.findByText(SUCCESS_NOTE)).toBeInTheDocument();
  });

  it("blocks submission for an invalid phone without calling the API", async () => {
    const user = userEvent.setup();
    renderWithProviders(<ForgotPasswordPage />);

    await user.type(screen.getByLabelText("Số điện thoại"), "123");
    await user.click(screen.getByRole("button", { name: "Gửi liên kết đặt lại" }));

    expect(await screen.findByText("Số điện thoại không hợp lệ")).toBeInTheDocument();
    expect(screen.queryByText(SUCCESS_NOTE)).not.toBeInTheDocument();
  });
});
