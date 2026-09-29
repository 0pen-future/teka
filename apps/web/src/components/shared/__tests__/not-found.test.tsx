import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";

import { renderWithProviders } from "@/test/utils";

import { NotFound } from "../not-found";

describe("NotFound", () => {
  it("explains the missing page in Vietnamese with a single heading", () => {
    renderWithProviders(<NotFound />, { route: "/khong-ton-tai", path: "*" });

    expect(screen.getAllByRole("heading", { level: 1 })).toHaveLength(1);
    expect(screen.getByRole("heading", { name: "Không tìm thấy trang" })).toBeInTheDocument();
    expect(screen.getByText("Đường dẫn có thể đã đổi hoặc không còn tồn tại.")).toBeInTheDocument();
  });

  it("links back to the home page", async () => {
    const { router } = renderWithProviders(<NotFound />, {
      route: "/khong-ton-tai",
      path: "/khong-ton-tai",
      extraRoutes: [{ path: "/", element: <p>Trang chủ</p> }],
    });

    await userEvent.click(screen.getByRole("link", { name: "Về trang chủ" }));

    expect(router.state.location.pathname).toBe("/");
  });
});
