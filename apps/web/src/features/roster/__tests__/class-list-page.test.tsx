import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { delay, http, HttpResponse } from "msw";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { useAuthStore } from "@/features/auth";
import { centerKeys } from "@/features/center";
import { API_URL, fail, listMeta, ok } from "@/test/msw/handlers";
import { server } from "@/test/msw/server";
import { renderWithProviders, signInAs, testPrimaryTeacher } from "@/test/utils";

import { ClassListPage } from "../pages/class-list-page";
import type { Class } from "../schemas/roster-schemas";
import {
  classWithSchedule,
  getRosterStore,
  resetRosterStore,
  rosterHandlers,
} from "./roster-handlers";

/** A second class so the chip counts and filters have something to separate. */
const classEnded: Class = {
  ...classWithSchedule,
  id: "70000000-0000-4000-8000-000000000002",
  name: "Anh Văn 7B",
  code: "AV7B",
  tags: ["Anh"],
  recruiting: true,
  end_date: "2026-06-30",
  phase: "ended",
  schedules: [
    {
      id: "71000000-0000-4000-8000-000000000002",
      weekday: 6,
      start_time: "08:00",
      duration_min: 90,
      effective_from: "2026-01-05",
      effective_to: null,
    },
  ],
};

function renderPage(route = "/classes") {
  return renderWithProviders(<ClassListPage />, {
    route,
    path: "/classes",
    extraRoutes: [{ path: "/classes/:id", element: <div>class-detail-stub</div> }],
  });
}

/** Query string of the most recent `GET /classes` request. */
function captureListRequests() {
  const seen: URLSearchParams[] = [];
  server.use(
    http.get(`${API_URL}/classes`, ({ request }) => {
      seen.push(new URL(request.url).searchParams);
      const items = getRosterStore().classes;
      return HttpResponse.json(ok(items, listMeta(items.length)));
    }),
  );
  return seen;
}

describe("ClassListPage", () => {
  beforeEach(() => {
    resetRosterStore();
    getRosterStore().classes.push({ ...classEnded });
    server.use(...rosterHandlers);
    signInAs(testPrimaryTeacher);
  });

  afterEach(() => {
    useAuthStore.getState().clearSession();
  });

  it("renders the header, then the table with schedule, tags, phase and dates", async () => {
    renderPage();
    expect(screen.getByRole("heading", { name: "Danh sách lớp học" })).toBeInTheDocument();
    // The create action waits for /centers/me: it needs classes.create.
    expect(await screen.findByRole("button", { name: "+ Lớp học" })).toBeInTheDocument();

    const table = await screen.findByRole("table");
    const rows = within(table).getAllByRole("row").slice(1);
    expect(rows).toHaveLength(2);

    const first = rows[0]!;
    expect(within(first).getByText("Toán 6A")).toBeInTheDocument();
    expect(within(first).getByText("TOAN6A")).toBeInTheDocument();
    expect(within(first).getByText("Tối Thứ Ba")).toBeInTheDocument();
    expect(within(first).getByText("Toán")).toBeInTheDocument();
    expect(within(first).getByText("Khối 6")).toBeInTheDocument();
    expect(within(first).getByText("Đang học")).toBeInTheDocument();
    expect(within(first).getByText("05/01/2026")).toBeInTheDocument();
    // Neither a course nor an end date: both cells fall back to a dash.
    expect(within(first).getAllByText("—")).toHaveLength(2);

    const second = rows[1]!;
    expect(within(second).getByText("Đã kết thúc")).toBeInTheDocument();
    expect(within(second).getByText("30/06/2026")).toBeInTheDocument();
    expect(within(second).getByRole("link", { name: "Sửa" })).toHaveAttribute(
      "href",
      `/classes/${classEnded.id}/settings`,
    );
  });

  it("shows a loading block while the list is pending", async () => {
    server.use(
      http.get(`${API_URL}/classes`, async () => {
        await delay("infinite");
        return HttpResponse.json(ok([], listMeta(0)));
      }),
    );
    renderPage();
    expect(await screen.findByRole("status")).toHaveTextContent("Đang tải danh sách lớp");
  });

  it("shows an empty block with a create action when no class matches", async () => {
    getRosterStore().classes.length = 0;
    renderPage();
    expect(await screen.findByText("Chưa có lớp học nào.")).toBeInTheDocument();
    expect(screen.queryByRole("table")).not.toBeInTheDocument();
  });

  it("shows an error block, never the empty copy, when the list fails", async () => {
    server.use(
      http.get(`${API_URL}/classes`, () =>
        HttpResponse.json(fail("INTERNAL", "boom"), { status: 500 }),
      ),
    );
    renderPage();
    expect(await screen.findByRole("alert")).toHaveTextContent("Không tải được danh sách lớp");
    expect(screen.queryByText("Chưa có lớp học nào.")).not.toBeInTheDocument();
  });

  it("renders the status chips with the counts from /classes/stats", async () => {
    renderPage();
    const group = await screen.findByRole("radiogroup", { name: "Lọc theo trạng thái" });
    // Counts arrive with the stats query; the chips render before it resolves.
    await waitFor(() => {
      expect(within(group).getByRole("radio", { name: /Tất cả/ })).toHaveTextContent("2");
    });
    expect(within(group).getByRole("radio", { name: /Đang học/ })).toHaveTextContent("1");
    expect(within(group).getByRole("radio", { name: /Đã kết thúc/ })).toHaveTextContent("1");
    expect(within(group).getByRole("radio", { name: /Cần tuyển sinh/ })).toHaveTextContent("1");
    expect(within(group).getByRole("radio", { name: /Tất cả/ })).toHaveAttribute(
      "aria-checked",
      "true",
    );
  });

  it("picking a phase chip re-queries with that phase and writes it to the URL", async () => {
    const user = userEvent.setup();
    const seen = captureListRequests();
    renderPage();
    await screen.findByRole("table");

    await user.click(screen.getByRole("radio", { name: /Đã kết thúc/ }));

    await waitFor(() => {
      const last = seen.at(-1)!;
      expect(last.get("phase")).toBe("ended");
      expect(last.get("status")).toBe("all");
    });
    expect(screen.getByRole("radio", { name: /Đã kết thúc/ })).toHaveAttribute(
      "aria-checked",
      "true",
    );
  });

  it("the recruiting chip keeps only recruiting classes", async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByRole("table");

    await user.click(screen.getByRole("radio", { name: /Cần tuyển sinh/ }));

    await waitFor(() => {
      expect(screen.queryByText("Toán 6A")).not.toBeInTheDocument();
    });
    expect(screen.getByText("Anh Văn 7B")).toBeInTheDocument();
  });

  it("clicking a row navigates to the class detail", async () => {
    const user = userEvent.setup();
    renderPage();
    const table = await screen.findByRole("table");
    await user.click(within(table).getByText("Toán 6A"));
    expect(await screen.findByText("class-detail-stub")).toBeInTheDocument();
  });

  it("restores weekday, shift and search from the URL and sends them to the API", async () => {
    const seen = captureListRequests();
    renderPage("/classes?weekday=6&shift=morning&q=anh");
    await screen.findByRole("table");

    await waitFor(() => {
      const last = seen.at(-1)!;
      expect(last.get("weekday")).toBe("6");
      expect(last.get("shift")).toBe("morning");
      expect(last.get("q")).toBe("anh");
    });
    expect(screen.getByRole("searchbox", { name: "Tìm lớp học" })).toHaveValue("anh");
    expect(screen.getByRole("combobox", { name: /Thứ 7/ })).toBeInTheDocument();
    expect(screen.getByRole("combobox", { name: /Sáng/ })).toBeInTheDocument();
  });

  it("debounces the search box before writing it to the URL and re-querying", async () => {
    const user = userEvent.setup();
    const seen = captureListRequests();
    renderPage();
    await screen.findByRole("table");
    const before = seen.length;

    await user.type(screen.getByRole("searchbox", { name: "Tìm lớp học" }), "to");
    // Typing alone triggers no request; only the settled value does.
    expect(seen.length).toBe(before);

    await waitFor(() => {
      expect(seen.at(-1)!.get("q")).toBe("to");
    });
    expect(seen.length).toBe(before + 1);
  });

  it("opens the create dialog from + Lớp học", async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByRole("table");
    await user.click(await screen.findByRole("button", { name: "+ Lớp học" }));
    expect(await screen.findByRole("dialog", { name: "Tạo lớp mới" })).toBeInTheDocument();
  });

  it("hides + Lớp học from a member without classes.create", async () => {
    server.use(
      http.get(`${API_URL}/centers/me`, () =>
        HttpResponse.json(
          ok({ center_name: "Trung Tâm Bình Minh", permissions: ["classes.list"] }),
        ),
      ),
    );
    const { queryClient } = renderPage();
    await screen.findByRole("table");
    await waitFor(() => expect(queryClient.getQueryState(centerKeys.me)?.status).toBe("success"));
    expect(screen.queryByRole("button", { name: "+ Lớp học" })).not.toBeInTheDocument();
  });

  it("follows the URL when the search is cleared from outside instead of re-arming the old value", async () => {
    const user = userEvent.setup();
    const seen = captureListRequests();
    const { router } = renderPage();
    await screen.findByRole("table");

    await user.type(screen.getByRole("searchbox", { name: "Tìm lớp học" }), "to");
    await waitFor(() => {
      expect(seen.at(-1)!.get("q")).toBe("to");
    });

    // Back button: the URL drops q while the input still holds the old text.
    await router.navigate("/classes");
    await waitFor(() => {
      expect(screen.getByRole("searchbox", { name: "Tìm lớp học" })).toHaveValue("");
    });
    await waitFor(() => {
      expect(seen.at(-1)!.get("q")).toBeNull();
    });
    const settled = seen.length;
    await new Promise((resolve) => setTimeout(resolve, 400));
    expect(seen.length).toBe(settled);
    expect(router.state.location.search).toBe("");
  });

  it("tells the reader when the catalog was cut to the first page", async () => {
    server.use(
      http.get(`${API_URL}/classes`, () =>
        HttpResponse.json(ok(getRosterStore().classes, listMeta(150, 1, 100))),
      ),
    );
    renderPage();
    await screen.findByRole("table");
    expect(screen.getByRole("note")).toHaveTextContent(
      "Đang hiển thị 2/150 lớp. Thu hẹp bằng bộ lọc hoặc từ khoá để thấy phần còn lại.",
    );
  });
});
