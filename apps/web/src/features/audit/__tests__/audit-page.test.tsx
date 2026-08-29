import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { useAuthStore } from "@/features/auth";
import { formatDateTime } from "@/lib/utils";
import { API_URL, fail, ok } from "@/test/msw/handlers";
import { server } from "@/test/msw/server";
import { renderWithProviders, signInAs, testPrimaryTeacher } from "@/test/utils";

import { AuditPage } from "../pages/audit-page";
import { auditHandlers, auditMemberId, auditRequests, resetAuditStore } from "./audit-handlers";

function renderAuditPage() {
  signInAs(testPrimaryTeacher);
  return renderWithProviders(<AuditPage />, {
    route: "/audit",
    path: "/audit",
    extraRoutes: [{ path: "/", element: <div>Trang tổng quan</div> }],
  });
}

beforeEach(() => {
  resetAuditStore();
  server.use(...auditHandlers);
});

afterEach(() => {
  useAuthStore.getState().clearSession();
});

describe("AuditPage", () => {
  it("renders the first page of logs for the owner", async () => {
    renderAuditPage();

    expect(await screen.findByText("class.create")).toBeInTheDocument();
    // Actor names also live inside the selects' hidden native options —
    // only the table copy proves the rows rendered.
    const table = screen.getByRole("table");
    expect(within(table).getByText("auth.login")).toBeInTheDocument();
    expect(within(table).getByText("Cô Lan")).toBeInTheDocument();
    expect(within(table).getByText("Thầy Minh")).toBeInTheDocument();
    // Only the first page is fetched up front.
    expect(screen.queryByText("student.delete")).not.toBeInTheDocument();
    // Timestamps render in the browser's local timezone via the shared helper.
    expect(screen.getByText(formatDateTime("2026-08-26T10:30:00Z"))).toBeInTheDocument();
    // 2xx status renders as a success badge.
    expect(screen.getByText("201")).toHaveClass("text-mint-600");
  });

  it("appends the next cursor page on Tải thêm and hides the button on the last page", async () => {
    const user = userEvent.setup();
    renderAuditPage();
    await screen.findByText("class.create");

    await user.click(screen.getByRole("button", { name: "Tải thêm" }));

    expect(await screen.findByText("student.delete")).toBeInTheDocument();
    // Appended, not replaced.
    expect(screen.getByText("class.create")).toBeInTheDocument();
    // Deleted teacher renders the placeholder, not an empty cell.
    expect(screen.getByText("(đã xóa)")).toBeInTheDocument();
    // 4xx warning and 5xx danger badges.
    expect(screen.getByText("403")).toHaveClass("text-sun-600");
    expect(screen.getByText("500")).toHaveClass("text-coral-600");
    // The second request carried the first page's cursor.
    expect(auditRequests.at(-1)?.searchParams.get("cursor")).toBe("offset-2");
    // next_cursor rỗng → no further page to load.
    expect(screen.queryByRole("button", { name: "Tải thêm" })).not.toBeInTheDocument();
  });

  it("applies the free-text action filter on submit and resets pagination", async () => {
    const user = userEvent.setup();
    renderAuditPage();
    await screen.findByText("class.create");

    await user.type(screen.getByLabelText("Hành động"), "auth.{Enter}");

    await waitFor(() => expect(auditRequests.at(-1)?.searchParams.get("action")).toBe("auth."));
    // A filter change starts over from page 1 — cursors belong to their filters.
    expect(auditRequests.at(-1)?.searchParams.get("cursor")).toBeNull();
    await waitFor(() => expect(screen.queryByText("class.create")).not.toBeInTheDocument());
    expect(screen.getByText("auth.login")).toBeInTheDocument();
  });

  it("filters by actor through the member select", async () => {
    const user = userEvent.setup();
    renderAuditPage();
    await screen.findByText("class.create");

    await user.click(screen.getByRole("combobox", { name: "Giáo viên" }));
    await user.click(await screen.findByRole("option", { name: "Thầy Minh" }));

    await waitFor(() =>
      expect(auditRequests.at(-1)?.searchParams.get("actor_id")).toBe(auditMemberId),
    );
    await screen.findByText("auth.login");
    expect(screen.queryByText("class.create")).not.toBeInTheDocument();
  });

  it("sends from/to as RFC3339 instants covering the picked days inclusively", async () => {
    renderAuditPage();
    await screen.findByText("class.create");

    fireEvent.change(screen.getByLabelText("Từ ngày"), { target: { value: "2026-08-01" } });
    await waitFor(() =>
      expect(auditRequests.at(-1)?.searchParams.get("from")).toBe(
        new Date("2026-08-01T00:00:00").toISOString(),
      ),
    );

    fireEvent.change(screen.getByLabelText("Đến ngày"), { target: { value: "2026-08-26" } });
    await waitFor(() =>
      expect(auditRequests.at(-1)?.searchParams.get("to")).toBe(
        new Date("2026-08-26T23:59:59.999").toISOString(),
      ),
    );
  });

  it("expands a row to show metadata and request details", async () => {
    const user = userEvent.setup();
    renderAuditPage();
    await screen.findByText("class.create");

    await user.click(screen.getByRole("button", { name: "Chi tiết class.create" }));

    expect(await screen.findByText(/"class_name": "Toán 6A"/)).toBeInTheDocument();
    expect(screen.getByText(/Mozilla\/5\.0 \(X11; Linux x86_64\)/)).toBeInTheDocument();
    // The entity's id only surfaces in the expanded details.
    expect(screen.getByText(/c0000000-0000-4000-8000-000000000001/)).toBeInTheDocument();
  });

  it("keeps loaded rows and shows an inline error when loading more fails", async () => {
    const user = userEvent.setup();
    renderAuditPage();
    await screen.findByText("class.create");

    server.use(
      http.get(`${API_URL}/audit-logs`, () =>
        HttpResponse.json(fail("INTERNAL_ERROR", "boom"), { status: 500 }),
      ),
    );
    await user.click(screen.getByRole("button", { name: "Tải thêm" }));

    expect(
      await screen.findByText("Không tải được nhật ký hoạt động. Thử lại sau."),
    ).toBeInTheDocument();
    // A transient failure must not blank the trail already on screen.
    expect(screen.getByText("class.create")).toBeInTheDocument();
    expect(screen.getByText("auth.login")).toBeInTheDocument();
  });

  it("renders the log for a member granted audit.read", async () => {
    server.use(
      http.get(`${API_URL}/centers/me`, () =>
        HttpResponse.json(ok({ center_name: "Trung Tâm Bình Minh", permissions: ["audit.read"] })),
      ),
    );
    renderAuditPage();

    expect(await screen.findByText("class.create")).toBeInTheDocument();
    // The teacher filter offers only the roster the caller can see — a
    // grantee gets no member list from /centers/me, so just the trigger.
    expect(screen.getByRole("combobox", { name: "Giáo viên" })).toBeInTheDocument();
  });

  it("redirects members to the dashboard without fetching logs", async () => {
    server.use(
      http.get(`${API_URL}/centers/me`, () =>
        HttpResponse.json(ok({ center_name: "Trung Tâm Bình Minh" })),
      ),
    );
    const { router } = renderAuditPage();

    await waitFor(() => expect(router.state.location.pathname).toBe("/"));
    expect(await screen.findByText("Trang tổng quan")).toBeInTheDocument();
    expect(auditRequests).toHaveLength(0);
  });
});
