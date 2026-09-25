import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { delay, http, HttpResponse } from "msw";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { useAuthStore } from "@/features/auth";
import { centerKeys } from "@/features/center";
import { API_URL, fail, listMeta, ok } from "@/test/msw/handlers";
import { server } from "@/test/msw/server";
import { renderWithProviders, signInAs, testPrimaryTeacher } from "@/test/utils";

import { ClassDetailHeader } from "../components/class-detail-header";
import { ClassListPage, RecruitingClassListPage } from "../pages/class-list-page";
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

/** Open for recruitment: the flag is on and the class has not started yet. */
const classRecruiting: Class = {
  ...classWithSchedule,
  id: "70000000-0000-4000-8000-000000000003",
  name: "Toán 8C",
  code: "TOAN8C",
  tags: [],
  recruiting: true,
  start_date: "2099-01-05",
  phase: "upcoming",
  schedules: [],
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
    expect(screen.getByRole("button", { name: "Tải lại" })).toBeInTheDocument();
    expect(screen.getByRole("searchbox", { name: "Tìm lớp học" })).toHaveAttribute(
      "placeholder",
      "Tìm kiếm theo tên, mã lớp học",
    );
    // The create action waits for /centers/me: it needs classes.create.
    expect(await screen.findByRole("button", { name: "+ Lớp học" })).toBeInTheDocument();

    const table = await screen.findByRole("table");
    const rows = within(table).getAllByRole("row").slice(1);
    expect(rows).toHaveLength(2);

    const first = rows[0]!;
    expect(within(first).getByText("1")).toBeInTheDocument();
    expect(within(first).getByText("Toán 6A")).toBeInTheDocument();
    expect(within(first).getByText("TOAN6A")).toBeInTheDocument();
    // One line per weekly session, with its end time from the duration.
    expect(within(first).getByText("Thứ Ba, 18:00 - 19:30")).toBeInTheDocument();
    expect(within(first).getByText("Toán")).toBeInTheDocument();
    expect(within(first).getByText("Khối 6")).toBeInTheDocument();
    expect(within(first).getByText("Đang học")).toBeInTheDocument();
    expect(within(first).getByText("05/01/2026")).toBeInTheDocument();
    // No end date falls back to a dash.
    expect(within(first).getByText("—")).toBeInTheDocument();
    expect(within(first).getByRole("button", { name: "Mở lớp Toán 6A" })).toBeInTheDocument();

    const second = rows[1]!;
    expect(within(second).getByText("2")).toBeInTheDocument();
    expect(within(second).getByText("Thứ Bảy, 08:00 - 09:30")).toBeInTheDocument();
    expect(within(second).getByText("Đã kết thúc")).toBeInTheDocument();
    expect(within(second).getByText("30/06/2026")).toBeInTheDocument();
    expect(
      within(second).getByRole("button", { name: `Sửa lớp ${classEnded.name}` }),
    ).toBeInTheDocument();
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

  it("shows the empty note inside the table when there is no class", async () => {
    getRosterStore().classes.length = 0;
    renderPage();
    const table = await screen.findByRole("table");
    expect(within(table).getByText(/^Chưa có lớp học nào\./)).toBeInTheDocument();
  });

  it("shows an error block, never the empty copy, when the list fails", async () => {
    server.use(
      http.get(`${API_URL}/classes`, () =>
        HttpResponse.json(fail("INTERNAL", "boom"), { status: 500 }),
      ),
    );
    renderPage();
    expect(await screen.findByRole("alert")).toHaveTextContent("Không tải được danh sách lớp");
    expect(screen.queryByText(/Chưa có lớp học nào/)).not.toBeInTheDocument();
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
    // Anh Văn 7B keeps the flag but has ended, so it is not open for recruitment.
    expect(within(group).getByRole("radio", { name: /Cần tuyển sinh/ })).toHaveTextContent("0");
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

  it("the recruiting chip asks the API for the classes open for recruitment", async () => {
    const user = userEvent.setup();
    getRosterStore().classes.push({ ...classRecruiting });
    renderPage();
    await screen.findByRole("table");

    await user.click(screen.getByRole("radio", { name: /Cần tuyển sinh/ }));

    await waitFor(() => {
      expect(screen.queryByText("Toán 6A")).not.toBeInTheDocument();
    });
    // Flagged but ended: filtered server-side like any class past recruitment.
    expect(screen.queryByText("Anh Văn 7B")).not.toBeInTheDocument();
    expect(screen.getByText("Toán 8C")).toBeInTheDocument();
  });

  it("opens the edit dialog without navigating away or losing list filters", async () => {
    const user = userEvent.setup();
    const { router } = renderPage("/classes?view=all&q=toan");
    const button = await screen.findByRole("button", { name: `Sửa lớp ${classWithSchedule.name}` });
    await user.click(button);
    expect(await screen.findByRole("dialog", { name: "Sửa lớp học" })).toBeInTheDocument();
    expect(router.state.location.pathname).toBe("/classes");
    expect(router.state.location.search).toContain("q=toan");
  });

  it("hides edit for a member without a class staff role", async () => {
    server.use(
      http.get(`${API_URL}/centers/me`, () =>
        HttpResponse.json(
          ok({ center_name: "Trung Tâm Bình Minh", permissions: ["classes.list"] }),
        ),
      ),
    );
    renderPage();
    await screen.findByRole("table");
    await waitFor(() =>
      expect(
        screen.queryByRole("button", { name: `Sửa lớp ${classWithSchedule.name}` }),
      ).not.toBeInTheDocument(),
    );
  });

  it("clicking a row navigates to the class detail", async () => {
    const user = userEvent.setup();
    renderPage();
    const table = await screen.findByRole("table");
    await user.click(within(table).getByText("Toán 6A"));
    expect(await screen.findByText("class-detail-stub")).toBeInTheDocument();
  });

  it("the Mở action opens the class detail", async () => {
    const user = userEvent.setup();
    renderPage();
    await user.click(await screen.findByRole("button", { name: "Mở lớp Anh Văn 7B" }));
    expect(await screen.findByText("class-detail-stub")).toBeInTheDocument();
  });

  it("Tải lại refetches the list and the chip counts", async () => {
    const seen = captureListRequests();
    let statsCalls = 0;
    server.use(
      http.get(`${API_URL}/classes/stats`, () => {
        statsCalls += 1;
        return HttpResponse.json(
          ok({ all: 2, upcoming: 0, running: 1, ended: 1, archived: 0, recruiting: 1 }),
        );
      }),
    );
    const user = userEvent.setup();
    renderPage();
    await screen.findByRole("table");
    await waitFor(() => expect(statsCalls).toBe(1));
    const listCalls = seen.length;
    await user.click(screen.getByRole("button", { name: "Tải lại" }));
    await waitFor(() => expect(seen.length).toBe(listCalls + 1));
    await waitFor(() => expect(statsCalls).toBe(2));
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

describe("RecruitingClassListPage", () => {
  beforeEach(() => {
    resetRosterStore();
    getRosterStore().classes.push({ ...classEnded }, { ...classRecruiting });
    server.use(...rosterHandlers);
    signInAs(testPrimaryTeacher);
  });

  afterEach(() => {
    useAuthStore.getState().clearSession();
  });

  function renderRecruiting(route = "/classes/recruiting") {
    return renderWithProviders(<RecruitingClassListPage />, {
      route,
      path: "/classes/recruiting",
      extraRoutes: [{ path: "/classes/:id", element: <DetailStub /> }],
    });
  }

  it("lists only the classes open for recruitment under its own header", async () => {
    const seen = captureListRequests();
    const stats: URLSearchParams[] = [];
    server.use(
      http.get(`${API_URL}/classes/stats`, ({ request }) => {
        stats.push(new URL(request.url).searchParams);
        return HttpResponse.json(
          ok({ all: 1, upcoming: 1, running: 0, ended: 0, archived: 0, recruiting: 1 }),
        );
      }),
    );
    renderRecruiting();

    expect(screen.getByRole("heading", { name: "Lớp cần tuyển sinh" })).toBeInTheDocument();
    expect(
      screen.getByText("Các lớp đang bật “Cần tuyển sinh” — chưa đủ sĩ số hoặc sắp khai giảng."),
    ).toBeInTheDocument();
    await screen.findByRole("table");
    await waitFor(() => expect(seen.at(-1)?.get("recruiting")).toBe("true"));
    await waitFor(() => expect(stats.at(-1)?.get("recruiting")).toBe("true"));

    const group = screen.getByRole("radiogroup", { name: "Lọc theo trạng thái" });
    await waitFor(() => {
      expect(within(group).getByRole("radio", { name: /Tất cả/ })).toHaveTextContent("1");
    });
    // The whole page is the recruiting set; a chip for it would repeat "Tất cả".
    expect(within(group).queryByRole("radio", { name: /Cần tuyển sinh/ })).not.toBeInTheDocument();
  });

  it("shows only the open class from the store and its own empty copy", async () => {
    renderRecruiting();
    const table = await screen.findByRole("table");
    await waitFor(() => expect(within(table).getAllByRole("row").slice(1)).toHaveLength(1));
    expect(within(table).getByText("Toán 8C")).toBeInTheDocument();
    expect(within(table).queryByText("Anh Văn 7B")).not.toBeInTheDocument();
  });

  it("says there is nothing to recruit for when no class is open", async () => {
    getRosterStore().classes.length = 0;
    renderRecruiting();
    const table = await screen.findByRole("table");
    expect(within(table).getByText("Không có lớp nào cần tuyển sinh.")).toBeInTheDocument();
  });

  it("a stale recruiting view in the URL falls back to Tất cả", async () => {
    renderRecruiting("/classes/recruiting?view=recruiting");
    const group = await screen.findByRole("radiogroup", { name: "Lọc theo trạng thái" });
    expect(within(group).getByRole("radio", { name: /Tất cả/ })).toHaveAttribute(
      "aria-checked",
      "true",
    );
  });

  it("Sửa opens the edit dialog in place", async () => {
    const user = userEvent.setup();
    const { router } = renderRecruiting();
    await user.click(
      await screen.findByRole("button", { name: `Sửa lớp ${classRecruiting.name}` }),
    );
    expect(await screen.findByRole("dialog", { name: "Sửa lớp học" })).toBeInTheDocument();
    expect(router.state.location.pathname).toBe("/classes/recruiting");
  });

  it("Mở opens the class detail, whose back link returns to Lớp cần tuyển sinh", async () => {
    const user = userEvent.setup();
    const { router } = renderRecruiting();
    await user.click(await screen.findByRole("button", { name: `Mở lớp ${classRecruiting.name}` }));
    const back = await screen.findByRole("link", { name: "Lớp cần tuyển sinh" });
    expect(router.state.location.pathname).toBe(`/classes/${classRecruiting.id}`);
    expect(back).toHaveAttribute("href", "/classes/recruiting");
  });
});

/** The real detail header over a fixed class, so the back link reads the router state. */
function DetailStub() {
  return <ClassDetailHeader klass={classRecruiting} canWrite={false} onEdit={() => undefined} />;
}
