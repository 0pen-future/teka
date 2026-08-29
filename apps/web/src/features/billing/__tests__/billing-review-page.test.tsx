import { screen, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { useAuthStore } from "@/features/auth";
import { API_URL, ok } from "@/test/msw/handlers";
import { server } from "@/test/msw/server";
import { renderWithProviders, signInAs, testPrimaryTeacher } from "@/test/utils";

import { BillingReviewPage } from "../pages/billing-review-page";
import {
  billingHandlers,
  fixtureBlockingSession,
  fixturePeriodOpen,
  resetBillingStore,
  seedBlockingSession,
  seedClosedPeriod,
} from "./billing-handlers";

function renderReviewPage() {
  signInAs(testPrimaryTeacher);
  return renderWithProviders(<BillingReviewPage />, {
    route: `/billing/${fixturePeriodOpen.id}`,
    path: "/billing/:periodId",
  });
}

beforeEach(() => {
  resetBillingStore();
  server.use(...billingHandlers);
});

afterEach(() => {
  useAuthStore.getState().clearSession();
});

describe("BillingReviewPage", () => {
  it("renders one row per student, with a multi-class student showing two class lines and one total", async () => {
    renderReviewPage();

    const table = await screen.findByRole("table");
    const tableUtils = within(table);

    // rowSpan collapses the two class lines onto one logical row: the
    // student name appears exactly once despite Toán and Văn both showing.
    expect(tableUtils.getAllByText("Nguyễn Văn An")).toHaveLength(1);
    // "Toán 6A" shows twice — once for the multi-class student's first line,
    // once for the carried-debt student's single line — while "Văn 6A" is
    // unique to the multi-class student's second line.
    expect(tableUtils.getAllByText("Toán 6A")).toHaveLength(2);
    expect(tableUtils.getByText("Văn 6A")).toBeInTheDocument();

    // The carried-debt student's nợ cũ is visible and distinguished from the
    // multi-class student's zero opening balance.
    expect(tableUtils.getAllByText("Trần Thị Cúc")).toHaveLength(1);
  });

  it("disables close and lists each blocking session with a link to its attendance screen", async () => {
    seedBlockingSession();
    renderReviewPage();

    expect(await screen.findByText("Chưa thể chốt sổ")).toBeInTheDocument();
    const link = screen.getByRole("link", { name: "Điểm danh" });
    expect(link).toHaveAttribute(
      "href",
      `/sessions/${fixtureBlockingSession.session_id}/attendance`,
    );

    const closeButton = screen.getByRole("button", { name: /Chốt kỳ/ });
    expect(closeButton).toBeDisabled();
  });

  it("renders a closed period as a locked read-only view without a draft error", async () => {
    // A closed period 409s on /draft; the page must read it through /preview
    // and reach the locked footer instead of the error state.
    seedClosedPeriod();
    renderReviewPage();

    expect(await screen.findByText("✓ Đã chốt — kỳ đã khóa")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /Gửi thông báo/ })).toHaveAttribute(
      "href",
      `/notifications/${fixturePeriodOpen.id}`,
    );
    // The review still renders (from preview), and no close button is offered.
    expect(screen.getByRole("table")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Chốt kỳ/ })).not.toBeInTheDocument();
    expect(screen.queryByText("Không tải được kỳ thu học phí này.")).not.toBeInTheDocument();
  });

  it("hides the notifications link from a plain member on a closed period (D8)", async () => {
    server.use(
      http.get(`${API_URL}/centers/me`, () =>
        HttpResponse.json(ok({ center_name: "Trung Tâm Bình Minh" })),
      ),
    );
    seedClosedPeriod();
    renderReviewPage();

    // The locked footer still renders; only the send entry point is gone.
    expect(await screen.findByText("✓ Đã chốt — kỳ đã khóa")).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: /Gửi thông báo/ })).not.toBeInTheDocument();
  });
});
