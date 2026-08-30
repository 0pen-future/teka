import { screen } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { useAuthStore } from "@/features/auth";
import { API_URL, listMeta, ok } from "@/test/msw/handlers";
import { server } from "@/test/msw/server";
import { renderWithProviders, signInAs, testPrimaryTeacher } from "@/test/utils";

import { ContactsPage } from "../pages/contacts-page";
import { contactSingleChild, resetRosterStore, rosterHandlers } from "./roster-handlers";

beforeEach(() => {
  resetRosterStore();
  server.use(...rosterHandlers);
  signInAs(testPrimaryTeacher);
});

afterEach(() => {
  useAuthStore.getState().clearSession();
});

describe("ContactsPage", () => {
  it("lets the owner add a contact", async () => {
    renderWithProviders(<ContactsPage />, { route: "/contacts", path: "/contacts" });

    await screen.findByText(contactSingleChild.full_name);
    expect(await screen.findByRole("button", { name: "Thêm người liên hệ" })).toBeInTheDocument();
  });

  it("shows a member who manages the roster instead of an empty search result", async () => {
    server.use(
      // Member-shaped `/centers/me`; the server answers a plain member's
      // contact list with an empty page rather than a 403.
      http.get(`${API_URL}/centers/me`, () =>
        HttpResponse.json(ok({ center_name: "Trung Tâm Bình Minh" })),
      ),
      http.get(`${API_URL}/contacts`, () => HttpResponse.json(ok([], listMeta(0)))),
    );

    renderWithProviders(<ContactsPage />, { route: "/contacts", path: "/contacts" });

    expect(
      await screen.findByText("Chủ trung tâm quản lý danh bạ & hồ sơ học sinh."),
    ).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Thêm người liên hệ" })).not.toBeInTheDocument();
  });
});
