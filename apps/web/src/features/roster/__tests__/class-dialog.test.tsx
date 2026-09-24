import { screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { useAuthStore } from "@/features/auth";
import { API_URL, fail } from "@/test/msw/handlers";
import { server } from "@/test/msw/server";
import { renderWithProviders, signInAs, testPrimaryTeacher } from "@/test/utils";

import { ClassDialog } from "../components/class-dialog";
import {
  classSchedule,
  classWithSchedule,
  courseOptionToan,
  getRosterStore,
  resetRosterStore,
  rosterHandlers,
} from "./roster-handlers";

function renderDialog() {
  return renderWithProviders(<ClassDialog open onOpenChange={() => undefined} />, {
    route: "/classes",
    path: "/classes",
  });
}

async function pickCourse(user: ReturnType<typeof userEvent.setup>, label: RegExp) {
  await user.click(screen.getByRole("combobox", { name: "Khóa học" }));
  await user.click(within(await screen.findByRole("listbox")).getByRole("option", { name: label }));
}

async function fillRequiredFields(user: ReturnType<typeof userEvent.setup>, name: string) {
  const dialog = screen.getByRole("dialog", { name: "Tạo lớp mới" });
  await user.type(within(dialog).getByLabelText("Tên lớp"), name);
  await user.click(within(dialog).getByRole("button", { name: "T2" }));
  await user.type(within(dialog).getByLabelText("Giờ học khung 1"), "18:00");
}

function renderEditDialog(onOpenChange = () => undefined) {
  return renderWithProviders(
    <ClassDialog mode="edit" classId={classWithSchedule.id} open onOpenChange={onOpenChange} />,
    { route: "/classes", path: "/classes" },
  );
}

describe("ClassDialog", () => {
  beforeEach(() => {
    resetRosterStore();
    server.use(...rosterHandlers);
    signInAs(testPrimaryTeacher);
  });

  afterEach(() => {
    useAuthStore.getState().clearSession();
  });

  describe("edit mode", () => {
    it("offers a close action when the class no longer exists", async () => {
      const user = userEvent.setup();
      const onOpenChange = vi.fn();
      server.use(
        http.get(`${API_URL}/classes/:id`, () =>
          HttpResponse.json(fail("NOT_FOUND", "class not found"), { status: 404 }),
        ),
      );
      renderEditDialog(onOpenChange);
      expect(await screen.findByText("Không tìm thấy lớp")).toBeInTheDocument();
      await user.click(screen.getByRole("button", { name: "Đóng" }));
      expect(onOpenChange).toHaveBeenCalledWith(false);
    });

    it("prefills the existing name and timetable", async () => {
      renderEditDialog();
      const dialog = await screen.findByRole("dialog", { name: "Sửa lớp học" });
      expect(await within(dialog).findByLabelText("Tên lớp")).toHaveValue(classWithSchedule.name);
      expect(within(dialog).getByLabelText("Giờ học khung 1")).toHaveValue("18:00");
      await waitFor(() =>
        expect(within(dialog).getByRole("button", { name: "T3" })).toHaveAttribute(
          "aria-pressed",
          "true",
        ),
      );
    });

    it("saves the changed timetable without deleting the replaced historical row", async () => {
      const user = userEvent.setup();
      const onOpenChange = vi.fn();
      renderEditDialog(onOpenChange);
      const time = await screen.findByLabelText("Giờ học khung 1");
      await waitFor(() =>
        expect(screen.getByRole("button", { name: "T3" })).toHaveAttribute("aria-pressed", "true"),
      );
      await user.clear(time);
      await user.type(time, "19:30");
      await user.click(screen.getByRole("button", { name: "Lưu thay đổi" }));
      await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false));
      const schedules = getRosterStore().classes[0]!.schedules;
      expect(schedules).toHaveLength(2);
      expect(schedules.find((s) => s.id === classSchedule.id)?.effective_to).not.toBeNull();
      expect(schedules.find((s) => s.id !== classSchedule.id)?.start_time).toBe("19:30");
    });

    it("skips PUT for schedule-only edits and adds before closing the old row", async () => {
      const user = userEvent.setup();
      const requests: string[] = [];
      const onRequest = ({ request }: { request: Request }) => {
        if (request.url.includes(`/classes/${classWithSchedule.id}`) && request.method !== "GET") {
          requests.push(`${request.method} ${new URL(request.url).pathname}`);
        }
      };
      server.events.on("request:start", onRequest);
      try {
        const onOpenChange = vi.fn();
        renderEditDialog(onOpenChange);
        const time = await screen.findByLabelText("Giờ học khung 1");
        await waitFor(() =>
          expect(screen.getByRole("button", { name: "T3" })).toHaveAttribute(
            "aria-pressed",
            "true",
          ),
        );
        await user.clear(time);
        await user.type(time, "19:30");
        await user.click(screen.getByRole("button", { name: "Lưu thay đổi" }));
        await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false));
        expect(requests).toEqual([
          `POST /api/v1/classes/${classWithSchedule.id}/schedules`,
          `PUT /api/v1/classes/${classWithSchedule.id}/schedules/${classSchedule.id}`,
        ]);
      } finally {
        server.events.removeListener("request:start", onRequest);
      }
    });

    it("keeps the dialog open when a later schedule write fails", async () => {
      const user = userEvent.setup();
      const onOpenChange = vi.fn();
      server.use(
        http.post(`${API_URL}/classes/:id/schedules`, () =>
          HttpResponse.json(fail("INTERNAL", "schedule failed"), { status: 500 }),
        ),
      );
      renderEditDialog(onOpenChange);
      const name = await screen.findByLabelText("Tên lớp");
      await user.clear(name);
      await user.type(name, "Toán 6A mới");
      await user.click(screen.getByRole("button", { name: "T7" }));
      await user.click(screen.getByRole("button", { name: "Lưu thay đổi" }));
      expect(await screen.findByText(/Chỉ lưu được một phần thay đổi/)).toBeInTheDocument();
      expect(onOpenChange).not.toHaveBeenCalled();
    });
  });

  it("offers the active courses and prefills the unit price from the picked course", async () => {
    const user = userEvent.setup();
    renderDialog();
    const combobox = await screen.findByRole("combobox", { name: "Khóa học" });
    await waitFor(() => expect(combobox).toBeEnabled());

    await user.click(combobox);
    const listbox = await screen.findByRole("listbox");
    expect(within(listbox).getByRole("option", { name: "Không gắn khóa học" })).toBeInTheDocument();
    await user.click(within(listbox).getByRole("option", { name: /Toán 6 nền tảng/ }));

    expect(screen.getByLabelText("Đơn giá / buổi (đ)")).toHaveValue("180.000");
  });

  it("keeps a price the teacher already typed when a course is picked afterwards", async () => {
    const user = userEvent.setup();
    renderDialog();
    await waitFor(() => expect(screen.getByRole("combobox", { name: "Khóa học" })).toBeEnabled());
    const price = screen.getByLabelText("Đơn giá / buổi (đ)");
    await user.clear(price);
    await user.type(price, "250000");

    await pickCourse(user, /Văn 9 luyện thi/);

    expect(price).toHaveValue("250.000");
  });

  it("re-prefills the price when the course changes and the price was only ever prefilled", async () => {
    const user = userEvent.setup();
    renderDialog();
    await waitFor(() => expect(screen.getByRole("combobox", { name: "Khóa học" })).toBeEnabled());
    const price = screen.getByLabelText("Đơn giá / buổi (đ)");

    await pickCourse(user, /Toán 6 nền tảng/);
    expect(price).toHaveValue("180.000");
    await pickCourse(user, /Văn 9 luyện thi/);
    expect(price).toHaveValue("200.000");

    await user.clear(price);
    await user.type(price, "250000");
    await pickCourse(user, /Toán 6 nền tảng/);
    expect(price).toHaveValue("250.000");
  });

  it("sends course_id with the create payload and omits it when no course is picked", async () => {
    const user = userEvent.setup();
    renderDialog();
    await waitFor(() => expect(screen.getByRole("combobox", { name: "Khóa học" })).toBeEnabled());
    await fillRequiredFields(user, "Toán 6B");
    await pickCourse(user, /Toán 6 nền tảng/);
    await user.click(screen.getByRole("button", { name: "Tạo lớp" }));

    await waitFor(() => expect(getRosterStore().classes).toHaveLength(2));
    const created = getRosterStore().classes[1]!;
    expect(created.course).toEqual({
      id: courseOptionToan.id,
      code: courseOptionToan.code,
      name: courseOptionToan.name,
    });
    expect(created.default_unit_price).toBe(courseOptionToan.default_unit_price);
  });

  it("creates an unattached class when the course stays blank", async () => {
    const user = userEvent.setup();
    renderDialog();
    await waitFor(() => expect(screen.getByRole("combobox", { name: "Khóa học" })).toBeEnabled());
    await fillRequiredFields(user, "Toán 6C");
    await user.click(screen.getByRole("button", { name: "Tạo lớp" }));

    await waitFor(() => expect(getRosterStore().classes).toHaveLength(2));
    expect(getRosterStore().classes[1]!.course).toBeNull();
  });
});
