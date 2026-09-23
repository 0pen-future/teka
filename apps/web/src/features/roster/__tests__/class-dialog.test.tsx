import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { useAuthStore } from "@/features/auth";
import { server } from "@/test/msw/server";
import { renderWithProviders, signInAs, testPrimaryTeacher } from "@/test/utils";

import { ClassDialog } from "../components/class-dialog";
import {
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

describe("ClassDialog", () => {
  beforeEach(() => {
    resetRosterStore();
    server.use(...rosterHandlers);
    signInAs(testPrimaryTeacher);
  });

  afterEach(() => {
    useAuthStore.getState().clearSession();
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
