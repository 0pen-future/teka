import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { useAuthStore } from "@/features/auth";
import { API_URL, fail, ok, secondaryTeacher } from "@/test/msw/handlers";
import { server } from "@/test/msw/server";
import { renderWithProviders, signInAs, testPrimaryTeacher } from "@/test/utils";

import { ClassDialog } from "../components/class-dialog";
import {
  classParentToan5,
  classSchedule,
  classWithSchedule,
  courseOptionToan,
  getRosterStore,
  resetRosterStore,
  rosterHandlers,
} from "./roster-handlers";

/** Non-owner center member — `/centers/me`'s member body carries no `members` key. */
const memberCenterHandler = http.get(`${API_URL}/centers/me`, () =>
  HttpResponse.json(ok({ center_name: "Trung Tâm Bình Minh", permissions: [] })),
);

/** Overrides `GET /classes/:id` so the fixture class carries the given staff roles. */
function classWithStaffRoles(roles: string[]) {
  return http.get(`${API_URL}/classes/:id`, () =>
    HttpResponse.json(ok({ ...classWithSchedule, my_staff_roles: roles })),
  );
}

/** Matches the page's `today()` (UTC ISO date). */
const todayIso = () => new Date().toISOString().slice(0, 10);

function dayBefore(date: string): string {
  const parsed = new Date(`${date}T00:00:00`);
  parsed.setDate(parsed.getDate() - 1);
  return `${parsed.getFullYear()}-${String(parsed.getMonth() + 1).padStart(2, "0")}-${String(
    parsed.getDate(),
  ).padStart(2, "0")}`;
}

function renderWizard(onOpenChange = () => undefined) {
  signInAs(testPrimaryTeacher);
  return renderWithProviders(
    <ClassDialog mode="edit" classId={classWithSchedule.id} open onOpenChange={onOpenChange} />,
    { route: "/classes", path: "/classes" },
  );
}

function savedClass(id = classWithSchedule.id) {
  return getRosterStore().classes.find((klass) => klass.id === id);
}

async function pickOption(
  user: ReturnType<typeof userEvent.setup>,
  combobox: string,
  option: RegExp,
) {
  const trigger = screen.getByRole("combobox", { name: combobox });
  await waitFor(() => expect(trigger).toBeEnabled());
  await user.click(trigger);
  await user.click(
    within(await screen.findByRole("listbox")).getByRole("option", { name: option }),
  );
}

/** Waits for the prefill, then attaches the course every saved class needs. */
async function readyToSave(user: ReturnType<typeof userEvent.setup>) {
  await waitFor(() =>
    expect(screen.getByLabelText("Tên lớp *")).toHaveValue(classWithSchedule.name),
  );
  await pickOption(user, "Khóa học", /Toán 6 nền tảng/);
}

beforeEach(() => {
  resetRosterStore();
  server.use(...rosterHandlers);
});

afterEach(() => {
  useAuthStore.getState().clearSession();
});

describe("ClassDialog edit mode (class wizard)", () => {
  it("renders the six sections with the section nav, prefilled from the class", async () => {
    renderWizard();

    const dialog = await screen.findByRole("dialog", { name: "Sửa lớp học" });
    expect(await within(dialog).findByLabelText("Tên lớp *")).toHaveValue(classWithSchedule.name);
    expect(
      within(dialog).getByText("Thay đổi áp dụng ngay; lịch đã điểm danh không đổi."),
    ).toBeInTheDocument();
    const nav = within(dialog).getByRole("navigation", { name: "Các mục của lớp", hidden: true });
    for (const label of [
      "Thông tin lớp học",
      "Liên kết lớp",
      "Hình thức học",
      "Lịch học hàng tuần",
      "Nguồn lực dự kiến",
      "Thông tin bổ sung",
    ]) {
      expect(within(nav).getByRole("button", { name: label, hidden: true })).toBeInTheDocument();
      expect(within(dialog).getByRole("heading", { name: label })).toBeInTheDocument();
    }
    expect(
      within(nav).getByRole("button", { name: "Thông tin lớp học", hidden: true }),
    ).toHaveAttribute("aria-current", "true");

    expect(screen.getByLabelText("Mã lớp")).toHaveValue(classWithSchedule.code);
    expect(screen.getByLabelText("Giờ bắt đầu lịch 1")).toHaveValue("18:00");
    expect(screen.getByLabelText("Thời lượng lịch 1 (phút)")).toHaveValue(90);
    expect(screen.getByText("Thứ Ba 18:00–19:30")).toBeInTheDocument();
    expect(screen.getByRole("radio", { name: "Học theo lịch" })).toBeChecked();
    expect(screen.getByRole("button", { name: "Toán" })).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByRole("button", { name: "Ưu tiên" })).toHaveAttribute(
      "aria-pressed",
      "false",
    );
    expect(screen.getByText("Mặc định lấy theo giá khóa học")).toBeInTheDocument();
    expect(screen.getByText("Chưa có phòng học; có thể bỏ qua.")).toBeInTheDocument();
  });

  it("saves a class that never had a course without asking for one", async () => {
    const user = userEvent.setup();
    const onOpenChange = vi.fn();
    renderWizard(onOpenChange);

    const name = await screen.findByLabelText("Tên lớp *");
    await waitFor(() => expect(name).toHaveValue(classWithSchedule.name));
    await user.type(name, " B");
    await user.click(screen.getByRole("button", { name: "Lưu thay đổi" }));

    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false));
    expect(savedClass()?.name).toBe(`${classWithSchedule.name} B`);
    expect(savedClass()?.course).toBeNull();
  });

  it("takes the course price when a course is picked", async () => {
    const user = userEvent.setup();
    renderWizard();

    await readyToSave(user);

    expect(
      screen.getByText("Giá khóa Toán 6 nền tảng: 180.000đ/buổi — lớp có thể đặt khác"),
    ).toBeInTheDocument();
    expect(screen.getByLabelText("Học phí / buổi (đ)")).toHaveValue("180.000");
    // The price moved away from the class's own, so the wizard says what it affects.
    expect(
      screen.getByText(/Học phí mới chỉ áp cho lượt ghi danh từ nay về sau/),
    ).toBeInTheDocument();
  });

  it("keeps a class-specific price when the current course is picked again", async () => {
    const user = userEvent.setup();
    renderWizard();

    await readyToSave(user);
    const rate = screen.getByLabelText("Học phí / buổi (đ)");
    await user.clear(rate);
    await user.type(rate, "150000");
    await pickOption(user, "Khóa học", /Toán 6 nền tảng/);

    expect(rate).toHaveValue("150.000");
  });

  it("saves info, room and a changed time, closing the old schedule row", async () => {
    const user = userEvent.setup();
    const onOpenChange = vi.fn();
    renderWizard(onOpenChange);

    await readyToSave(user);
    const name = screen.getByLabelText("Tên lớp *");
    await user.clear(name);
    await user.type(name, "Toán 6A nâng cao");
    await user.click(screen.getByRole("button", { name: "Ưu tiên" }));
    await user.type(screen.getByLabelText("Phòng học"), "P.201");
    const time = screen.getByLabelText("Giờ bắt đầu lịch 1");
    await user.clear(time);
    await user.type(time, "19:30");
    await user.type(screen.getByLabelText("Ghi chú"), "Mang máy tính");
    await user.click(screen.getByRole("button", { name: "Lưu thay đổi" }));

    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false));
    const saved = savedClass();
    expect(saved?.name).toBe("Toán 6A nâng cao");
    expect(saved?.course?.name).toBe("Toán 6 nền tảng");
    expect(saved?.default_unit_price).toBe(180000);
    expect(saved?.tags).toEqual(["Toán", "Khối 6", "Ưu tiên"]);
    expect(saved?.room).toBe("P.201");
    expect(saved?.note).toBe("Mang máy tính");
    // Close-and-replace: the old row stays, closed yesterday; the new one starts today.
    expect(saved?.schedules).toHaveLength(2);
    const closed = saved?.schedules.find((s) => s.id === classSchedule.id);
    expect(closed?.effective_to).toBe(dayBefore(todayIso()));
    const replacement = saved?.schedules.find((s) => s.id !== classSchedule.id);
    expect(replacement).toMatchObject({
      weekday: 2,
      start_time: "19:30",
      duration_min: 90,
      effective_from: todayIso(),
    });
  });

  it("applies a quick preset as the weekly timetable", async () => {
    const user = userEvent.setup();
    const onOpenChange = vi.fn();
    renderWizard(onOpenChange);

    await readyToSave(user);
    await user.click(screen.getByRole("button", { name: "Chọn nhanh" }));
    await user.click(screen.getByRole("button", { name: "T7 · CN — 09:00 (120’)" }));
    expect(screen.getByText("Thứ Bảy 09:00–11:00")).toBeInTheDocument();
    expect(screen.getByText("Chủ Nhật 09:00–11:00")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Lưu thay đổi" }));

    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false));
    const active = savedClass()?.schedules.filter((s) => s.effective_to === null);
    expect(active?.map((s) => [s.weekday, s.start_time, s.duration_min]).sort()).toEqual([
      [0, "09:00", 120],
      [6, "09:00", 120],
    ]);
  });

  it("rejects two schedule rows on the same weekday", async () => {
    const user = userEvent.setup();
    const onOpenChange = vi.fn();
    renderWizard(onOpenChange);

    await readyToSave(user);
    // The new row defaults to Thứ Ba, the day row 1 already meets on.
    await user.click(screen.getByRole("button", { name: "+ Thêm lịch học" }));
    await user.type(screen.getByLabelText("Giờ bắt đầu lịch 2"), "20:00");
    await user.click(screen.getByRole("button", { name: "Lưu thay đổi" }));

    expect(
      (await screen.findAllByText("Ngày này đã có lịch học — mỗi ngày chỉ một lịch")).length,
    ).toBeGreaterThan(0);
    expect(onOpenChange).not.toHaveBeenCalled();
  });

  it("switches to self-paced study and retires the timetable", async () => {
    const user = userEvent.setup();
    const onOpenChange = vi.fn();
    renderWizard(onOpenChange);

    await readyToSave(user);
    await user.click(screen.getByRole("radio", { name: "Tự học" }));
    expect(screen.getByText("Lớp tự học — không cần lịch cố định.")).toBeInTheDocument();
    expect(
      screen.getByText(
        "Lớp tự học không chiếm slot thời khóa biểu; học viên học theo chương trình mẫu.",
      ),
    ).toBeInTheDocument();
    expect(screen.queryByLabelText("Giờ bắt đầu lịch 1")).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Lưu thay đổi" }));

    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false));
    const saved = savedClass();
    expect(saved?.study_mode).toBe("self_paced");
    expect(saved?.schedules.find((s) => s.id === classSchedule.id)?.effective_to).toBe(
      dayBefore(todayIso()),
    );
  });

  it("links a next class and rejects the same class on both sides", async () => {
    getRosterStore().classes.push({ ...classParentToan5 });
    const user = userEvent.setup();
    const onOpenChange = vi.fn();
    renderWizard(onOpenChange);

    await readyToSave(user);
    await pickOption(user, "Lớp trước", /Toán 5B/);
    await pickOption(user, "Lớp sau", /Toán 5B/);
    await user.click(screen.getByRole("button", { name: "Lưu thay đổi" }));
    expect(
      (await screen.findAllByText("Lớp trước và lớp sau phải khác nhau")).length,
    ).toBeGreaterThan(0);

    await pickOption(user, "Lớp trước", /— Không —/);
    await user.click(screen.getByRole("button", { name: "Lưu thay đổi" }));

    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false));
    expect(savedClass()?.next_class_id).toBe(classParentToan5.id);
    expect(savedClass(classParentToan5.id)?.parent_class_id).toBe(classWithSchedule.id);
  });

  it("finds a free room for the chosen timetable", async () => {
    // A clashing Tuesday-evening class holds P.101; a morning class keeps P.102.
    getRosterStore().classes.push(
      {
        ...classParentToan5,
        id: "70000000-0000-4000-8000-000000000011",
        phase: "running",
        room: "P.101",
        schedules: [{ ...classSchedule, id: "s-11" }],
      },
      {
        ...classParentToan5,
        id: "70000000-0000-4000-8000-000000000012",
        phase: "running",
        room: "P.102",
        schedules: [{ ...classSchedule, id: "s-12", start_time: "08:00" }],
      },
    );
    const user = userEvent.setup();
    renderWizard();

    await screen.findByLabelText("Tên lớp *");
    await user.click(screen.getByRole("button", { name: "Tìm phòng trống" }));

    await waitFor(() => expect(screen.getByLabelText("Phòng học")).toHaveValue("P.102"));
    expect(
      await screen.findByText("Phòng P.102 trống vào các khung giờ đã chọn"),
    ).toBeInTheDocument();
    expect(screen.getByText("Phòng P.102 — kiểm tra trùng lịch khi xác nhận.")).toBeInTheDocument();
  });

  it("asks for a timetable before looking for a free teacher", async () => {
    const user = userEvent.setup();
    renderWizard();

    await screen.findByLabelText("Giờ bắt đầu lịch 1");
    await user.click(screen.getByRole("button", { name: "Xóa lịch 1" }));
    expect(
      screen.getByText("Chưa có lịch. Dùng “Chọn nhanh” hoặc “+ Thêm lịch học”."),
    ).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Tìm giáo viên rảnh" }));

    expect(
      await screen.findByText("Thêm lịch học trước để tìm giáo viên rảnh"),
    ).toBeInTheDocument();
  });

  it("invites the planned teacher after saving", async () => {
    const user = userEvent.setup();
    const onOpenChange = vi.fn();
    renderWizard(onOpenChange);

    await readyToSave(user);
    await user.click(screen.getByRole("button", { name: "Tìm giáo viên rảnh" }));
    await waitFor(() =>
      expect(screen.getByRole("combobox", { name: "Giáo viên dự kiến" })).toHaveTextContent(
        secondaryTeacher.full_name,
      ),
    );
    await user.click(screen.getByRole("button", { name: "Lưu thay đổi" }));

    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false));
    const invitation = getRosterStore().classInvitations.find(
      (item) => item.teacher_id === secondaryTeacher.id && item.status === "pending",
    );
    expect(invitation?.role_key).toBe("giao_vien");
  });

  it("flags a duplicate class code on the code field", async () => {
    server.use(
      http.put(`${API_URL}/classes/:id`, () =>
        HttpResponse.json(
          { success: false, error: { code: "CLASS_CODE_TAKEN", message: "class code taken" } },
          { status: 409 },
        ),
      ),
    );
    const user = userEvent.setup();
    const onOpenChange = vi.fn();
    renderWizard(onOpenChange);

    await readyToSave(user);
    const code = screen.getByLabelText("Mã lớp");
    await user.clear(code);
    await user.type(code, "TOAN5B");
    await user.click(screen.getByRole("button", { name: "Lưu thay đổi" }));

    expect((await screen.findAllByText("Mã lớp TOAN5B đã tồn tại")).length).toBeGreaterThan(0);
    expect(onOpenChange).not.toHaveBeenCalled();
  });

  it("skips the PUT for a timetable-only edit and adds before closing the old row", async () => {
    getRosterStore().classes[0]!.course = {
      id: courseOptionToan.id,
      code: courseOptionToan.code,
      name: courseOptionToan.name,
    };
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
      renderWizard(onOpenChange);
      const time = await screen.findByLabelText("Giờ bắt đầu lịch 1");
      await waitFor(() => expect(time).toHaveValue("18:00"));
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

  it("keeps the wizard open when a later schedule write fails", async () => {
    server.use(
      http.post(`${API_URL}/classes/:id/schedules`, () =>
        HttpResponse.json(fail("INTERNAL", "schedule failed"), { status: 500 }),
      ),
    );
    const user = userEvent.setup();
    const onOpenChange = vi.fn();
    renderWizard(onOpenChange);

    await readyToSave(user);
    await user.click(screen.getByRole("button", { name: "+ Thêm lịch học" }));
    await pickOption(user, "Ngày học lịch 2", /Thứ Bảy/);
    await user.type(screen.getByLabelText("Giờ bắt đầu lịch 2"), "20:00");
    await user.click(screen.getByRole("button", { name: "Lưu thay đổi" }));

    // Shown inline and as a toast, so it is not missed after scrolling.
    expect(await screen.findAllByText(/Chỉ lưu được một phần thay đổi/)).toHaveLength(2);
    expect(onOpenChange).not.toHaveBeenCalled();
    expect(savedClass()?.course?.id).toBe(courseOptionToan.id);
  });

  describe("write access by class-staff role", () => {
    it("shows an enabled save button for the center owner", async () => {
      renderWizard();

      await screen.findByLabelText("Tên lớp *");
      expect(screen.getByRole("button", { name: "Lưu thay đổi" })).toBeEnabled();
      expect(
        screen.queryByText(/Chỉ giáo viên phụ trách hoặc chủ trung tâm mới sửa được/),
      ).not.toBeInTheDocument();
    });

    it("lets a giao_vien save but not invite teachers", async () => {
      server.use(memberCenterHandler, classWithStaffRoles(["giao_vien"]));
      renderWizard();

      await screen.findByLabelText("Tên lớp *");
      expect(screen.getByRole("button", { name: "Lưu thay đổi" })).toBeEnabled();
      expect(screen.getByRole("combobox", { name: "Giáo viên dự kiến" })).toBeDisabled();
      expect(screen.getByText("Chỉ chủ trung tâm mới mời được giáo viên.")).toBeInTheDocument();
    });

    it("disables the save button and shows a notice for a hoc_vu-only caller", async () => {
      server.use(memberCenterHandler, classWithStaffRoles(["hoc_vu"]));
      renderWizard();

      await screen.findByLabelText("Tên lớp *");
      expect(screen.getByRole("button", { name: "Lưu thay đổi" })).toBeDisabled();
      expect(
        screen.getByText("Chỉ giáo viên phụ trách hoặc chủ trung tâm mới sửa được lớp."),
      ).toBeInTheDocument();
    });
  });
});
