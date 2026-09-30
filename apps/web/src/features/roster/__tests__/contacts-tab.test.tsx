import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { useAuthStore } from "@/features/auth";
import { API_URL, ok } from "@/test/msw/handlers";
import { server } from "@/test/msw/server";
import { renderWithProviders, signInAs, testPrimaryTeacher } from "@/test/utils";

import { StudentsPage } from "../pages/students-page";
import {
  contactSingleChild,
  contactTwoChildren,
  resetRosterStore,
  rosterHandlers,
} from "./roster-handlers";

/** A member-shaped `/centers/me` holding exactly `permissions`. */
function asMember(permissions: string[]) {
  server.use(
    http.get(`${API_URL}/centers/me`, () =>
      HttpResponse.json(ok({ center_name: "Trung Tâm Bình Minh", permissions })),
    ),
  );
}

/** Serves the month's balances and records each requested `year-month`. */
function mockBalances(outstandingByContact: Record<string, number>) {
  const months: string[] = [];
  server.use(
    http.get(`${API_URL}/collections/contact-balances`, ({ request }) => {
      const url = new URL(request.url);
      months.push(`${url.searchParams.get("year")}-${url.searchParams.get("month")}`);
      return HttpResponse.json(
        ok(
          Object.entries(outstandingByContact).map(([contact_id, outstanding]) => ({
            contact_id,
            outstanding,
          })),
        ),
      );
    }),
  );
  return months;
}

function renderContactsTab(route = "/students?tab=contacts") {
  signInAs(testPrimaryTeacher);
  return renderWithProviders(<StudentsPage />, { route, path: "/students" });
}

async function contactRow(name: string) {
  const list = await screen.findByRole("list", { name: "Danh sách người liên hệ" });
  return (await within(list).findByText(name)).closest("li")!;
}

beforeEach(() => {
  resetRosterStore();
  server.use(...rosterHandlers);
});

afterEach(() => {
  useAuthStore.getState().clearSession();
});

describe("StudentsPage contacts tab", () => {
  it("lists contacts without debt or write actions for contacts.view_all alone", async () => {
    asMember(["students.list", "contacts.view_all"]);
    const months = mockBalances({ [contactTwoChildren.id]: 450_000 });
    renderContactsTab();

    const row = await contactRow(contactTwoChildren.full_name);
    expect(within(row).getByText("2 học sinh")).toBeInTheDocument();
    expect(within(row).getByRole("link")).toHaveAttribute(
      "href",
      `/contacts/${contactTwoChildren.id}`,
    );
    expect(within(row).queryByText(/Còn nợ/)).not.toBeInTheDocument();
    expect(screen.queryByLabelText("Công nợ tháng")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Thêm người liên hệ" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Tự động ghép Zalo" })).not.toBeInTheDocument();
    // The balances endpoint needs billing.view_all, so it is never asked.
    expect(months).toEqual([]);
  });

  it("shows each family's outstanding for the month and refetches when the month changes", async () => {
    asMember(["students.list", "contacts.view_all", "billing.view_all"]);
    const months = mockBalances({ [contactTwoChildren.id]: 450_000 });
    renderContactsTab("/students?tab=contacts&month=2026-01");

    const owing = await contactRow(contactTwoChildren.full_name);
    expect(await within(owing).findByText("450.000 ₫")).toBeInTheDocument();
    // A family missing from the month's rows owes nothing.
    const settled = await contactRow(contactSingleChild.full_name);
    expect(within(settled).getByText("—")).toBeInTheDocument();
    expect(months).toEqual(["2026-1"]);

    const picker = screen.getByLabelText("Công nợ tháng");
    expect(picker).toHaveValue("2026-01");
    // jsdom's month input takes no typing; set the value as the picker does.
    fireEvent.change(picker, { target: { value: "2026-02" } });
    await waitFor(() => expect(months).toEqual(["2026-1", "2026-2"]));
  });

  it("keeps adding contacts for the owner only", async () => {
    renderContactsTab();

    expect(await screen.findByRole("button", { name: "Thêm người liên hệ" })).toBeInTheDocument();
    expect(await screen.findByRole("button", { name: "Tự động ghép Zalo" })).toBeInTheDocument();
  });

  it("offers Zalo auto-matching to a member holding reports.send", async () => {
    asMember(["students.list", "contacts.view_all", "reports.send"]);
    renderContactsTab();

    await contactRow(contactSingleChild.full_name);
    expect(await screen.findByRole("button", { name: "Tự động ghép Zalo" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Thêm người liên hệ" })).not.toBeInTheDocument();
  });

  it("searches contacts by phone through the page search", async () => {
    renderContactsTab();

    await contactRow(contactSingleChild.full_name);
    await userEvent.type(
      screen.getByRole("searchbox", { name: "Tìm theo tên hoặc số điện thoại" }),
      "987654",
    );
    const list = screen.getByRole("list", { name: "Danh sách người liên hệ" });
    await waitFor(() =>
      expect(within(list).queryByText(contactSingleChild.full_name)).not.toBeInTheDocument(),
    );
    expect(within(list).getByText(contactTwoChildren.full_name)).toBeInTheDocument();
  });
});
