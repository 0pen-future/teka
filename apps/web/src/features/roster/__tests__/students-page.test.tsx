import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { useAuthStore } from "@/features/auth";
import { API_URL, listMeta, ok } from "@/test/msw/handlers";
import { server } from "@/test/msw/server";
import { renderWithProviders, signInAs, testPrimaryTeacher } from "@/test/utils";

import { StudentsPage } from "../pages/students-page";
import { classWithSchedule, resetRosterStore, rosterHandlers } from "./roster-handlers";

/** Six active classes — one past the threshold that reveals "Tìm lớp…". */
function sixClasses() {
  return [
    classWithSchedule,
    ...["Toán 7B", "Văn 6A", "Văn 8C", "Anh 9A", "Lý 8B"].map((name, index) => ({
      ...classWithSchedule,
      id: `70000000-0000-4000-8000-00000000001${index}`,
      name,
    })),
  ];
}

function useSixClasses() {
  server.use(
    http.get(`${API_URL}/classes`, () => {
      const items = sixClasses();
      return HttpResponse.json(ok(items, listMeta(items.length)));
    }),
  );
}

function renderStudentsPage() {
  signInAs(testPrimaryTeacher);
  return renderWithProviders(<StudentsPage />, { route: "/students", path: "/students" });
}

beforeEach(() => {
  resetRosterStore();
  server.use(...rosterHandlers);
});

afterEach(() => {
  useAuthStore.getState().clearSession();
});

describe("StudentsPage class search", () => {
  it("hides the class search while five or fewer classes exist", async () => {
    renderStudentsPage();

    await screen.findByRole("tab", { name: "Toán 6A" });
    expect(screen.queryByRole("searchbox", { name: "Tìm lớp" })).not.toBeInTheDocument();
  });

  it("filters only real class tabs, keeping the unenrolled tab", async () => {
    useSixClasses();
    renderStudentsPage();

    const search = await screen.findByRole("searchbox", { name: "Tìm lớp" });
    expect(await screen.findAllByRole("tab")).toHaveLength(7);

    await userEvent.type(search, "văn");
    expect(screen.getAllByRole("tab").map((tab) => tab.textContent)).toEqual([
      "Văn 6A",
      "Văn 8C",
      "Chưa ghi danh",
    ]);

    await userEvent.clear(search);
    expect(screen.getAllByRole("tab")).toHaveLength(7);
  });

  it("notes when no class matches while the unenrolled tab stays", async () => {
    useSixClasses();
    renderStudentsPage();

    const search = await screen.findByRole("searchbox", { name: "Tìm lớp" });
    await userEvent.type(search, "hoá 12");

    expect(screen.getAllByRole("tab").map((tab) => tab.textContent)).toEqual(["Chưa ghi danh"]);
    expect(screen.getByText('Không có lớp nào khớp "hoá 12"')).toBeInTheDocument();
  });
});
