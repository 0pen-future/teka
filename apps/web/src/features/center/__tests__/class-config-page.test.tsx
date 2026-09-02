import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { afterEach, describe, expect, it } from "vitest";

import { useAuthStore } from "@/features/auth";
import { API_URL, fail, listMeta, makeClass, ok } from "@/test/msw/handlers";
import { server } from "@/test/msw/server";
import { renderWithProviders, signInAs, testPrimaryTeacher } from "@/test/utils";

import { ClassConfigPage } from "../pages/class-config-page";
import { makeCenterMeMember, makeCenterMeOwner, mockCenterMe } from "./center-handlers";

interface ScoreSetFixture {
  id: string;
  name: string;
  components: string[];
}

function renderClassConfig() {
  signInAs(testPrimaryTeacher);
  return renderWithProviders(<ClassConfigPage />, {
    route: "/center/class-config",
    path: "/center/class-config",
    extraRoutes: [{ path: "/", element: <div>Trang tổng quan</div> }],
  });
}

afterEach(() => {
  useAuthStore.getState().clearSession();
});

describe("ClassConfigPage owner gate", () => {
  it("redirects a non-owner deep link to the dashboard without fetching score sets", async () => {
    mockCenterMe(makeCenterMeMember());
    let scoreSetsCalls = 0;
    server.use(
      http.get(`${API_URL}/score-sets`, () => {
        scoreSetsCalls += 1;
        return HttpResponse.json(ok([]));
      }),
    );
    const { router } = renderClassConfig();

    await waitFor(() => expect(router.state.location.pathname).toBe("/"));
    expect(await screen.findByText("Trang tổng quan")).toBeInTheDocument();
    expect(scoreSetsCalls).toBe(0);
  });

  it("renders the config page for the owner", async () => {
    mockCenterMe(makeCenterMeOwner());
    server.use(
      http.get(`${API_URL}/score-sets`, () => HttpResponse.json(ok([]))),
      http.get(`${API_URL}/classes`, () => HttpResponse.json(ok([], listMeta(0)))),
    );
    renderClassConfig();

    expect(await screen.findByRole("heading", { name: "Cấu hình lớp học" })).toBeInTheDocument();
    expect(await screen.findByText("Chưa có bộ điểm nào.")).toBeInTheDocument();
    expect(screen.getByText("Chưa có lớp nào.")).toBeInTheDocument();
  });

  it("opens the editor from the empty state action", async () => {
    mockCenterMe(makeCenterMeOwner());
    server.use(
      http.get(`${API_URL}/score-sets`, () => HttpResponse.json(ok([]))),
      http.get(`${API_URL}/classes`, () => HttpResponse.json(ok([], listMeta(0)))),
    );
    renderClassConfig();
    const user = userEvent.setup();

    await user.click(await screen.findByRole("button", { name: "Tạo bộ điểm" }));
    expect(await screen.findByRole("dialog", { name: "Tạo bộ điểm mới" })).toBeInTheDocument();
  });

  it("lists each bộ điểm as a card with its column chips", async () => {
    mockCenterMe(makeCenterMeOwner());
    const sets: ScoreSetFixture[] = [
      {
        id: "60000000-0000-4000-8000-000000000011",
        name: "Giữa kỳ",
        components: ["Miệng", "15 phút"],
      },
      { id: "60000000-0000-4000-8000-000000000012", name: "Cuối kỳ", components: ["1 tiết"] },
    ];
    server.use(
      http.get(`${API_URL}/score-sets`, () => HttpResponse.json(ok(sets))),
      http.get(`${API_URL}/classes`, () => HttpResponse.json(ok([], listMeta(0)))),
    );
    renderClassConfig();

    expect(await screen.findByText("Giữa kỳ")).toBeInTheDocument();
    const strips = screen.getAllByLabelText("Xem trước tiêu đề bảng");
    expect(strips).toHaveLength(2);
    expect(Array.from(strips[0]!.children).map((chip) => chip.textContent)).toEqual([
      "Miệng",
      "15 phút",
    ]);
    expect(screen.getByText("2 cột")).toBeInTheDocument();
    expect(screen.getByText("1 cột")).toBeInTheDocument();
    expect(screen.getAllByRole("button", { name: "Sửa" })).toHaveLength(2);
  });

  it("explains why assignment is unavailable before any bộ điểm exists", async () => {
    mockCenterMe(makeCenterMeOwner());
    const klass = makeClass({ name: "Toán 9A1" });
    server.use(
      http.get(`${API_URL}/score-sets`, () => HttpResponse.json(ok([]))),
      http.get(`${API_URL}/classes`, () => HttpResponse.json(ok([klass], listMeta(1)))),
    );
    renderClassConfig();

    const table = await screen.findByTestId("class-score-set-table");
    const assign = within(table).getByRole("button", { name: "Gán bộ điểm" });
    expect(assign).toBeDisabled();
    expect(assign).not.toHaveAttribute("title");
    expect(screen.getByText("Tạo ít nhất một bộ điểm trước")).toBeInTheDocument();
    expect(within(table).getByRole("columnheader", { name: "Lớp" })).toHaveAttribute(
      "scope",
      "col",
    );
    expect(within(table).getAllByRole("columnheader")).toHaveLength(2);
    const cards = screen.getByTestId("class-score-set-cards");
    expect(within(cards).getAllByRole("listitem")).toHaveLength(1);
    expect(within(cards).getByRole("button", { name: "Gán bộ điểm" })).toBeDisabled();
  });
});

describe("Score set CRUD", () => {
  it("creates, edits, and deletes a bộ điểm", async () => {
    mockCenterMe(makeCenterMeOwner());
    let sets: ScoreSetFixture[] = [];
    server.use(
      http.get(`${API_URL}/classes`, () => HttpResponse.json(ok([], listMeta(0)))),
      http.get(`${API_URL}/score-sets`, () => HttpResponse.json(ok(sets))),
      http.post(`${API_URL}/score-sets`, async ({ request }) => {
        const body = (await request.json()) as Omit<ScoreSetFixture, "id">;
        const created: ScoreSetFixture = { id: "60000000-0000-4000-8000-000000000001", ...body };
        sets = [...sets, created];
        return HttpResponse.json(ok(created), { status: 201 });
      }),
      http.put(`${API_URL}/score-sets/:id`, async ({ params, request }) => {
        const body = (await request.json()) as Omit<ScoreSetFixture, "id">;
        const updated: ScoreSetFixture = { id: String(params.id), ...body };
        sets = sets.map((set) => (set.id === updated.id ? updated : set));
        return HttpResponse.json(ok(updated));
      }),
      http.delete(`${API_URL}/score-sets/:id`, ({ params }) => {
        sets = sets.filter((set) => set.id !== params.id);
        return new HttpResponse(null, { status: 204 });
      }),
    );
    renderClassConfig();
    const user = userEvent.setup();

    // Create.
    await user.click(await screen.findByRole("button", { name: "+ Tạo bộ điểm" }));
    const createDialog = await screen.findByRole("dialog");
    await user.type(within(createDialog).getByLabelText("Tên bộ điểm"), "Giữa kỳ");
    await user.type(within(createDialog).getByLabelText("Tên cột điểm 1"), "Kiểm tra miệng");
    await user.click(within(createDialog).getByRole("button", { name: "Lưu" }));

    expect(await screen.findByText("Đã tạo bộ điểm Giữa kỳ")).toBeInTheDocument();
    expect(await screen.findByText("Giữa kỳ")).toBeInTheDocument();

    // Edit.
    await user.click(screen.getByRole("button", { name: "Sửa" }));
    const editDialog = await screen.findByRole("dialog");
    const nameInput = within(editDialog).getByLabelText("Tên bộ điểm");
    await user.clear(nameInput);
    await user.type(nameInput, "Cuối kỳ");
    await user.click(within(editDialog).getByRole("button", { name: "Lưu" }));

    expect(await screen.findByText("Đã lưu bộ điểm Cuối kỳ")).toBeInTheDocument();
    expect(await screen.findByText("Cuối kỳ")).toBeInTheDocument();

    // Delete (confirm dialog).
    await user.click(screen.getByRole("button", { name: "Xóa" }));
    const confirmDialog = await screen.findByRole("dialog", { name: "Xóa bộ điểm Cuối kỳ?" });
    await user.click(within(confirmDialog).getByRole("button", { name: "Xác nhận xóa" }));

    expect(await screen.findByText("Đã xóa bộ điểm Cuối kỳ")).toBeInTheDocument();
    expect(await screen.findByText("Chưa có bộ điểm nào.")).toBeInTheDocument();
  });
});

describe("Assigning a bộ điểm to a class", () => {
  const klass = makeClass({ name: "Toán 9A1" });
  const midterm: ScoreSetFixture = {
    id: "60000000-0000-4000-8000-000000000002",
    name: "Giữa kỳ",
    components: ["Kiểm tra miệng"],
  };
  const final: ScoreSetFixture = {
    id: "60000000-0000-4000-8000-000000000003",
    name: "Cuối kỳ",
    components: ["15 phút", "1 tiết"],
  };

  function seedAssignment(currentNames: string[]) {
    server.use(
      http.get(`${API_URL}/classes`, () => HttpResponse.json(ok([klass], listMeta(1)))),
      http.get(`${API_URL}/score-sets`, () => HttpResponse.json(ok([midterm, final]))),
      http.get(`${API_URL}/classes/:id/score-components`, () =>
        HttpResponse.json(
          ok({
            class_id: klass.id,
            components: currentNames.map((name, position) => ({
              id: `70000000-0000-4000-8000-00000000000${position + 1}`,
              class_id: klass.id,
              name,
              position,
            })),
          }),
        ),
      ),
    );
  }

  async function openAssignDialog(user: ReturnType<typeof userEvent.setup>) {
    const table = await screen.findByTestId("class-score-set-table");
    await user.click(within(table).getByRole("button", { name: "Gán bộ điểm" }));
    return screen.findByRole("dialog", { name: "Bộ điểm — Toán 9A1" });
  }

  it("previews each set as a radio card and assigns the chosen one", async () => {
    mockCenterMe(makeCenterMeOwner());
    seedAssignment([]);
    const assigned: string[] = [];
    server.use(
      http.post(`${API_URL}/classes/:id/score-set`, async ({ request }) => {
        const body = (await request.json()) as { set_id: string };
        assigned.push(body.set_id);
        return HttpResponse.json(ok({ class_id: klass.id, components: [] }));
      }),
    );
    renderClassConfig();
    const user = userEvent.setup();
    const dialog = await openAssignDialog(user);

    expect(within(dialog).getByRole("note")).toHaveTextContent(
      "Lớp đã ghi nhận điểm sẽ không đổi hoặc xóa được bộ điểm.",
    );
    expect(await within(dialog).findByText("Chưa gán bộ điểm")).toBeInTheDocument();
    expect(within(dialog).queryByRole("combobox")).not.toBeInTheDocument();
    expect(within(dialog).queryByRole("button", { name: "Xóa gán" })).not.toBeInTheDocument();
    const assignButton = within(dialog).getByRole("button", { name: "Gán" });
    expect(assignButton).toBeDisabled();

    const radios = within(dialog).getAllByRole("radio");
    expect(radios.map((radio) => radio.getAttribute("aria-label"))).toEqual([
      "Giữa kỳ, 1 cột: Kiểm tra miệng",
      "Cuối kỳ, 2 cột: 15 phút, 1 tiết",
    ]);
    const finalCard = within(dialog).getByRole("radio", { name: /^Cuối kỳ, 2 cột/ });
    expect(
      Array.from(within(finalCard).getByLabelText("Xem trước tiêu đề bảng").children).map(
        (chip) => chip.textContent,
      ),
    ).toEqual(["15 phút", "1 tiết"]);

    await user.click(finalCard);
    expect(finalCard).toHaveAttribute("aria-checked", "true");
    expect(assignButton).toBeEnabled();
    await user.click(assignButton);

    expect(await screen.findByText("Đã gán bộ điểm cho Toán 9A1")).toBeInTheDocument();
    expect(assigned).toEqual([final.id]);
  });

  it("hints which set the class currently uses by matching its columns", async () => {
    mockCenterMe(makeCenterMeOwner());
    seedAssignment(["15 phút", "1 tiết"]);
    renderClassConfig();
    const user = userEvent.setup();
    const dialog = await openAssignDialog(user);

    const finalCard = await within(dialog).findByRole("radio", { name: /^Cuối kỳ, 2 cột/ });
    expect(within(finalCard).getByText("Đang dùng")).toBeInTheDocument();
    expect(
      within(within(dialog).getByRole("radio", { name: /^Giữa kỳ, 1 cột/ })).queryByText(
        "Đang dùng",
      ),
    ).not.toBeInTheDocument();
    expect(within(dialog).getByRole("button", { name: "Xóa gán" })).toBeEnabled();
  });

  it("locks the dialog with the Vietnamese conflict message on a 409 and stays locked on reopen", async () => {
    mockCenterMe(makeCenterMeOwner());
    seedAssignment([]);
    let postCount = 0;
    server.use(
      http.post(`${API_URL}/classes/:id/score-set`, () => {
        postCount += 1;
        return HttpResponse.json(fail("CONFLICT", "class already has recorded scores"), {
          status: 409,
        });
      }),
    );
    renderClassConfig();
    const user = userEvent.setup();

    const dialog = await openAssignDialog(user);
    await user.click(within(dialog).getByRole("radio", { name: /^Giữa kỳ, 1 cột/ }));
    await user.click(within(dialog).getByRole("button", { name: "Gán" }));

    expect(await within(dialog).findByRole("alert")).toHaveTextContent(
      "Lớp đã có điểm được ghi nhận nên không thể đổi hoặc xóa bộ điểm. Xóa điểm đã nhập của lớp trước khi thực hiện lại.",
    );
    expect(within(dialog).getByRole("radio", { name: /^Giữa kỳ, 1 cột/ })).toBeDisabled();
    expect(within(dialog).getByRole("button", { name: "Gán" })).toBeDisabled();
    expect(within(dialog).queryByRole("note")).not.toBeInTheDocument();
    expect(postCount).toBe(1);

    await user.click(within(dialog).getByRole("button", { name: "Đóng" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());

    const reopened = await openAssignDialog(user);
    expect(within(reopened).getByRole("alert")).toHaveTextContent("Lớp đã có điểm được ghi nhận");
    expect(within(reopened).getByRole("radio", { name: /^Giữa kỳ, 1 cột/ })).toBeDisabled();
    expect(within(reopened).getByRole("button", { name: "Gán" })).toBeDisabled();
    expect(postCount).toBe(1);
  });
});
