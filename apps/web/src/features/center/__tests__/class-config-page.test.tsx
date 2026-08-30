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

    // Delete (two-step confirm).
    await user.click(screen.getByRole("button", { name: "Xóa" }));
    await user.click(screen.getByRole("button", { name: "Xác nhận xóa" }));

    expect(await screen.findByText("Đã xóa bộ điểm Cuối kỳ")).toBeInTheDocument();
    expect(await screen.findByText("Chưa có bộ điểm nào.")).toBeInTheDocument();
  });
});

describe("Assigning a bộ điểm to a class", () => {
  it("locks the dialog with the Vietnamese conflict message on a 409", async () => {
    mockCenterMe(makeCenterMeOwner());
    const klass = makeClass({ name: "Toán 9A1" });
    const scoreSet: ScoreSetFixture = {
      id: "60000000-0000-4000-8000-000000000002",
      name: "Giữa kỳ",
      components: ["Kiểm tra miệng"],
    };
    server.use(
      http.get(`${API_URL}/classes`, () => HttpResponse.json(ok([klass], listMeta(1)))),
      http.get(`${API_URL}/score-sets`, () => HttpResponse.json(ok([scoreSet]))),
      http.get(`${API_URL}/classes/:id/score-components`, () =>
        HttpResponse.json(ok({ class_id: klass.id, components: [] })),
      ),
      http.post(`${API_URL}/classes/:id/score-set`, () =>
        HttpResponse.json(fail("CONFLICT", "class already has recorded scores"), { status: 409 }),
      ),
    );
    renderClassConfig();
    const user = userEvent.setup();

    await user.click(await screen.findByRole("button", { name: "Gán bộ điểm" }));
    const dialog = await screen.findByRole("dialog");
    await user.selectOptions(
      within(dialog).getByRole("combobox", { name: "Chọn bộ điểm để gán" }),
      scoreSet.id,
    );
    await user.click(within(dialog).getByRole("button", { name: "Gán" }));

    expect(
      await within(dialog).findByText(
        "Lớp đã có điểm được ghi nhận nên không thể đổi hoặc xóa bộ điểm. Xóa điểm đã nhập của lớp trước khi thực hiện lại.",
      ),
    ).toBeInTheDocument();
    expect(within(dialog).getByRole("combobox", { name: "Chọn bộ điểm để gán" })).toBeDisabled();
    expect(within(dialog).getByRole("button", { name: "Gán" })).toBeDisabled();
  });
});
