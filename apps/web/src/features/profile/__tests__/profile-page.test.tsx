import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { afterEach, describe, expect, it } from "vitest";

import { useAuthStore } from "@/features/auth";
import { formatPhoneLocal } from "@/lib/utils";
import { API_URL, makeTeacher, ok } from "@/test/msw/handlers";
import { server } from "@/test/msw/server";
import { renderWithProviders, signInAs, testPrimaryTeacher } from "@/test/utils";

import { ProfilePage } from "../pages/profile-page";

function renderProfile() {
  signInAs(testPrimaryTeacher);
  return renderWithProviders(<ProfilePage />, { route: "/profile", path: "/profile" });
}

afterEach(() => {
  useAuthStore.getState().clearSession();
});

describe("ProfilePage", () => {
  it("prefills tên hiển thị from the session and shows the account phone read-only", async () => {
    renderProfile();

    expect(await screen.findByLabelText("Tên hiển thị")).toHaveValue(testPrimaryTeacher.full_name);
    const phoneInput = screen.getByLabelText("Số điện thoại (Zalo)");
    expect(phoneInput).toHaveValue(formatPhoneLocal(testPrimaryTeacher.phone));
    expect(phoneInput).toHaveAttribute("readonly");
    // Unbacked prototype fields start empty — no server column yet.
    expect(screen.getByLabelText("Môn dạy")).toHaveValue("");
    expect(screen.getByLabelText("Ngân hàng")).toHaveValue("");
  });

  it("saves the display name via PUT /me and updates the auth store", async () => {
    let received: { full_name?: string; timezone?: string } = {};
    server.use(
      http.put(`${API_URL}/me`, async ({ request }) => {
        received = (await request.json()) as typeof received;
        return HttpResponse.json(
          ok(
            makeTeacher({
              id: testPrimaryTeacher.id,
              phone: testPrimaryTeacher.phone,
              full_name: received.full_name,
            }),
          ),
        );
      }),
    );
    renderProfile();
    const user = userEvent.setup();

    const nameInput = await screen.findByLabelText("Tên hiển thị");
    await user.clear(nameInput);
    await user.type(nameInput, "Cô Mai");
    await user.click(screen.getByRole("button", { name: "Lưu hồ sơ" }));

    expect(await screen.findByText("Đã lưu hồ sơ")).toBeInTheDocument();
    expect(received).toMatchObject({
      full_name: "Cô Mai",
      timezone: testPrimaryTeacher.timezone,
    });
    expect(useAuthStore.getState().user?.full_name).toBe("Cô Mai");
  });

  it("rejects an empty display name without calling the API", async () => {
    let putCalls = 0;
    server.use(
      http.put(`${API_URL}/me`, () => {
        putCalls += 1;
        return HttpResponse.json(ok(makeTeacher()));
      }),
    );
    renderProfile();
    const user = userEvent.setup();

    await user.clear(await screen.findByLabelText("Tên hiển thị"));
    await user.click(screen.getByRole("button", { name: "Lưu hồ sơ" }));

    expect(await screen.findByText("Vui lòng nhập tên hiển thị")).toBeInTheDocument();
    expect(putCalls).toBe(0);
  });

  it("feeds the Zalo-footer preview from the live form values", async () => {
    renderProfile();
    const user = userEvent.setup();

    await user.type(await screen.findByLabelText("Môn dạy"), "Toán");
    await user.type(screen.getByLabelText("Chủ tài khoản"), "NGUYEN THI LAN");
    await user.type(screen.getByLabelText("Ngân hàng"), "VCB");
    await user.type(screen.getByLabelText("Số tài khoản"), "00123");

    expect(screen.getByText(/— Toán$/)).toBeInTheDocument();
    expect(screen.getByText("Chuyển khoản: NGUYEN THI LAN · VCB 00123")).toBeInTheDocument();
  });
});
