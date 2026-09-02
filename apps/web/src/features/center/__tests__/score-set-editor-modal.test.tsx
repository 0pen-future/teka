import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { afterEach, describe, expect, it } from "vitest";

import { useAuthStore } from "@/features/auth";
import { API_URL, listMeta, ok } from "@/test/msw/handlers";
import { server } from "@/test/msw/server";
import { renderWithProviders, signInAs, testPrimaryTeacher } from "@/test/utils";

import { ClassConfigPage } from "../pages/class-config-page";
import { makeCenterMeOwner, mockCenterMe } from "./center-handlers";

interface ScoreSetFixture {
  id: string;
  name: string;
  components: string[];
}

const SET_ID = "60000000-0000-4000-8000-000000000001";

interface Recorded {
  posts: Omit<ScoreSetFixture, "id">[];
  puts: Omit<ScoreSetFixture, "id">[];
}

function seedScoreSets(initial: ScoreSetFixture[] = []): Recorded {
  const recorded: Recorded = { posts: [], puts: [] };
  let sets = initial;
  server.use(
    http.get(`${API_URL}/classes`, () => HttpResponse.json(ok([], listMeta(0)))),
    http.get(`${API_URL}/score-sets`, () => HttpResponse.json(ok(sets))),
    http.post(`${API_URL}/score-sets`, async ({ request }) => {
      const body = (await request.json()) as Omit<ScoreSetFixture, "id">;
      recorded.posts.push(body);
      const created: ScoreSetFixture = { id: SET_ID, ...body };
      sets = [...sets, created];
      return HttpResponse.json(ok(created), { status: 201 });
    }),
    http.put(`${API_URL}/score-sets/:id`, async ({ params, request }) => {
      const body = (await request.json()) as Omit<ScoreSetFixture, "id">;
      recorded.puts.push(body);
      const updated: ScoreSetFixture = { id: String(params.id), ...body };
      sets = sets.map((set) => (set.id === updated.id ? updated : set));
      return HttpResponse.json(ok(updated));
    }),
  );
  return recorded;
}

function renderClassConfig() {
  mockCenterMe(makeCenterMeOwner());
  signInAs(testPrimaryTeacher);
  return renderWithProviders(<ClassConfigPage />, {
    route: "/center/class-config",
    path: "/center/class-config",
    extraRoutes: [{ path: "/", element: <div>Trang tổng quan</div> }],
  });
}

async function openCreateDialog(user: ReturnType<typeof userEvent.setup>) {
  await user.click(await screen.findByRole("button", { name: "+ Tạo bộ điểm" }));
  return screen.findByRole("dialog", { name: "Tạo bộ điểm mới" });
}

function previewChips(dialog: HTMLElement): string[] {
  const strip = within(dialog).getByLabelText("Xem trước tiêu đề bảng");
  return Array.from(strip.children).map((chip) => chip.textContent ?? "");
}

afterEach(() => {
  useAuthStore.getState().clearSession();
});

describe("ScoreSetEditorModal rows mode", () => {
  it("opens in rows mode with one blank row, a counter, and no delete button", async () => {
    seedScoreSets();
    renderClassConfig();
    const user = userEvent.setup();
    const dialog = await openCreateDialog(user);

    const modes = within(dialog).getByRole("radiogroup", { name: "Cách nhập cột điểm" });
    expect(within(modes).getByRole("radio", { name: "Từng cột" })).toHaveAttribute(
      "aria-checked",
      "true",
    );
    expect(within(dialog).getByLabelText("Tên cột điểm 1")).toHaveValue("");
    expect(within(dialog).getByText("1/10 cột")).toBeInTheDocument();
    expect(within(dialog).queryByRole("button", { name: /Xóa cột điểm/ })).not.toBeInTheDocument();
    expect(within(dialog).getByRole("button", { name: "Thêm cột điểm" })).toBeEnabled();
    expect(previewChips(dialog)).toEqual(["(trống)"]);
  });

  it("flags a duplicate name under its own row and does not submit", async () => {
    const recorded = seedScoreSets();
    renderClassConfig();
    const user = userEvent.setup();
    const dialog = await openCreateDialog(user);

    await user.type(within(dialog).getByLabelText("Tên bộ điểm"), "Giữa kỳ");
    await user.type(within(dialog).getByLabelText("Tên cột điểm 1"), "Miệng");
    await user.click(within(dialog).getByRole("button", { name: "Thêm cột điểm" }));
    expect(within(dialog).getByText("2/10 cột")).toBeInTheDocument();
    await user.type(within(dialog).getByLabelText("Tên cột điểm 2"), "miệng");
    await user.click(within(dialog).getByRole("button", { name: "Lưu" }));

    expect(await within(dialog).findByText("Tên cột điểm bị trùng")).toBeInTheDocument();
    expect(within(dialog).getByLabelText("Tên cột điểm 2")).toHaveAttribute("aria-invalid", "true");
    expect(within(dialog).getByLabelText("Tên cột điểm 1")).not.toHaveAttribute(
      "aria-invalid",
      "true",
    );
    expect(recorded.posts).toHaveLength(0);
  });

  it("reorders with the move buttons, previews the new order, and saves it", async () => {
    const recorded = seedScoreSets([
      { id: SET_ID, name: "Học kỳ 1", components: ["Miệng", "15 phút", "Giữa kỳ"] },
    ]);
    renderClassConfig();
    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: "Sửa" }));
    const dialog = await screen.findByRole("dialog", { name: "Sửa bộ điểm" });

    expect(previewChips(dialog)).toEqual(["Miệng", "15 phút", "Giữa kỳ"]);
    expect(within(dialog).getByText("3/10 cột")).toBeInTheDocument();
    expect(within(dialog).getAllByRole("button", { name: /Xóa cột điểm/ })).toHaveLength(3);

    await user.click(within(dialog).getByRole("button", { name: "Di chuyển cột điểm 1 xuống" }));
    expect(previewChips(dialog)).toEqual(["15 phút", "Miệng", "Giữa kỳ"]);
    expect(within(dialog).getByLabelText("Tên cột điểm 1")).toHaveValue("15 phút");
    expect(within(dialog).getByLabelText("Tên cột điểm 2")).toHaveValue("Miệng");

    await user.click(within(dialog).getByRole("button", { name: "Lưu" }));
    expect(await screen.findByText("Đã lưu bộ điểm Học kỳ 1")).toBeInTheDocument();
    expect(recorded.puts).toEqual([
      { name: "Học kỳ 1", components: ["15 phút", "Miệng", "Giữa kỳ"] },
    ]);
  });
});

describe("ScoreSetEditorModal paste mode", () => {
  it("parses a pasted list into ordered columns and creates the set", async () => {
    const recorded = seedScoreSets();
    renderClassConfig();
    const user = userEvent.setup();
    const dialog = await openCreateDialog(user);

    await user.type(within(dialog).getByLabelText("Tên bộ điểm"), "Cả năm");
    await user.click(within(dialog).getByRole("radio", { name: "Dán danh sách" }));
    const box = within(dialog).getByLabelText("Danh sách cột điểm");
    const names = [
      "Miệng 1",
      "Miệng 2",
      "15 phút 1",
      "15 phút 2",
      "1 tiết",
      "Giữa kỳ",
      "Cuối kỳ",
      "Dự án",
    ];
    await user.click(box);
    await user.paste(names.join("\n"));
    expect(previewChips(dialog)).toEqual(names);

    await user.click(within(dialog).getByRole("button", { name: "Lưu" }));
    expect(await screen.findByText("Đã tạo bộ điểm Cả năm")).toBeInTheDocument();
    expect(recorded.posts).toEqual([{ name: "Cả năm", components: names }]);
  });

  it("keeps the first ten pasted names, warns, and disables adding more", async () => {
    seedScoreSets();
    renderClassConfig();
    const user = userEvent.setup();
    const dialog = await openCreateDialog(user);

    await user.click(within(dialog).getByRole("radio", { name: "Dán danh sách" }));
    await user.click(within(dialog).getByLabelText("Danh sách cột điểm"));
    await user.paste(Array.from({ length: 12 }, (_, i) => `Cột ${i + 1}`).join(", "));
    expect(within(dialog).getByRole("note")).toHaveTextContent("Chỉ giữ 10 cột đầu");
    expect(previewChips(dialog)).toHaveLength(10);

    await user.click(within(dialog).getByRole("radio", { name: "Từng cột" }));
    expect(within(dialog).getAllByLabelText(/^Tên cột điểm \d+$/)).toHaveLength(10);
    expect(within(dialog).getByLabelText("Tên cột điểm 10")).toHaveValue("Cột 10");
    expect(within(dialog).getByText("10/10 cột")).toBeInTheDocument();
    expect(within(dialog).getByText("Tối đa 10 cột")).toBeInTheDocument();
    expect(within(dialog).getByRole("button", { name: "Thêm cột điểm" })).toBeDisabled();
    expect(within(dialog).getByRole("note")).toHaveTextContent("Chỉ giữ 10 cột đầu");
  });

  it("calls out duplicates live while pasting and carries rows into the paste box", async () => {
    const recorded = seedScoreSets();
    renderClassConfig();
    const user = userEvent.setup();
    const dialog = await openCreateDialog(user);

    await user.type(within(dialog).getByLabelText("Tên cột điểm 1"), "Miệng");
    await user.click(within(dialog).getByRole("radio", { name: "Dán danh sách" }));
    const box = within(dialog).getByLabelText("Danh sách cột điểm");
    expect(box).toHaveValue("Miệng");
    expect(within(dialog).queryByText(/Tên cột điểm bị trùng/)).not.toBeInTheDocument();

    await user.type(box, "\nmiệng");
    expect(within(dialog).getByRole("status")).toHaveTextContent("Tên cột điểm bị trùng: miệng");

    await user.type(within(dialog).getByLabelText("Tên bộ điểm"), "Giữa kỳ");
    await user.click(within(dialog).getByRole("button", { name: "Lưu" }));
    await waitFor(() =>
      expect(within(dialog).getByLabelText("Tên cột điểm 2")).toHaveAttribute(
        "aria-invalid",
        "true",
      ),
    );
    expect(within(dialog).getByText("Tên cột điểm bị trùng")).toBeInTheDocument();
    expect(recorded.posts).toHaveLength(0);
  });
});
