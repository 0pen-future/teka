import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { useAuthStore } from "@/features/auth";
import {
  libraryHandlers,
  resetLibraryStore,
  templateToan6,
  versionToan6Published,
} from "@/features/library/__tests__/library-handlers";
import { API_URL, fail, ok } from "@/test/msw/handlers";
import { server } from "@/test/msw/server";
import { renderWithProviders, signInAs, testPrimaryTeacher } from "@/test/utils";

import { ClassDetailPage } from "../pages/class-detail-page";
import {
  auditLogClassUpdate,
  classParentToan5,
  classWithSchedule,
  courseOptionToan,
  getRosterStore,
  messageFromLan,
  messageFromMinh,
  programLessonsToan6,
  programToan6,
  resetRosterStore,
  rosterHandlers,
} from "./roster-handlers";

function renderPage(tab?: string) {
  const route = tab
    ? `/classes/${classWithSchedule.id}?tab=${tab}`
    : `/classes/${classWithSchedule.id}`;
  return renderWithProviders(<ClassDetailPage />, {
    route,
    path: "/classes/:id",
    extraRoutes: [
      { path: "/classes", element: <div>class-list-stub</div> },
      { path: "/library/templates/:id", element: <div>template-stub</div> },
    ],
  });
}

/** Non-owner center member: reads the class, chats, but never applies programs nor reads audit. */
const memberCenterHandler = http.get(`${API_URL}/centers/me`, () =>
  HttpResponse.json(
    ok({
      center_name: "Trung Tâm Bình Minh",
      permissions: ["classes.read", "class_messages.post"],
    }),
  ),
);

const memberWithLibraryHandler = http.get(`${API_URL}/centers/me`, () =>
  HttpResponse.json(
    ok({
      center_name: "Trung Tâm Bình Minh",
      permissions: ["classes.read", "class_messages.post", "library.read"],
    }),
  ),
);

const memberWithoutPostHandler = http.get(`${API_URL}/centers/me`, () =>
  HttpResponse.json(ok({ center_name: "Trung Tâm Bình Minh", permissions: ["classes.read"] })),
);

function applyProgramToStore() {
  const store = getRosterStore();
  store.program = { ...programToan6 };
  store.programLessons = programLessonsToan6.map((lesson) => ({ ...lesson }));
}

describe("ClassDetailPage — program, lineage and the read-through tabs", () => {
  beforeEach(() => {
    resetRosterStore();
    resetLibraryStore();
    server.use(...rosterHandlers, ...libraryHandlers);
    signInAs(testPrimaryTeacher);
  });

  afterEach(() => {
    useAuthStore.getState().clearSession();
  });

  it("enables the Chat, Bài tập and Tài liệu tabs", async () => {
    renderPage();
    await screen.findByRole("heading", { name: "Toán 6A" });
    for (const name of ["Chat", "Bài tập", "Tài liệu"]) {
      const tab = screen.getByRole("tab", { name });
      expect(tab).toBeEnabled();
      expect(tab).not.toHaveAttribute("title");
    }
  });

  describe("Chương trình học card", () => {
    it("lets the owner pick a published template version and apply it", async () => {
      const user = userEvent.setup();
      renderPage();
      const card = await screen.findByRole("region", { name: "Chương trình học" });
      expect(within(card).getByText("Lớp chưa áp dụng chương trình mẫu.")).toBeInTheDocument();

      await user.click(within(card).getByRole("button", { name: "Thiết lập chương trình" }));
      const templatePicker = within(card).getByRole("combobox", { name: "Chương trình mẫu" });
      await waitFor(() => expect(templatePicker).toBeEnabled());
      await user.click(templatePicker);
      const listbox = await screen.findByRole("listbox");
      // Only templates with a published version are offered.
      expect(within(listbox).queryByRole("option", { name: /Văn 9/ })).not.toBeInTheDocument();
      await user.click(within(listbox).getByRole("option", { name: /Toán 6 cơ bản/ }));

      const versionPicker = within(card).getByRole("combobox", { name: "Phiên bản" });
      await waitFor(() => expect(versionPicker).toHaveTextContent("v1 · Đã phát hành"));
      await user.click(within(card).getByRole("button", { name: "Áp dụng" }));

      expect(await screen.findByText("Đã áp dụng chương trình mẫu")).toBeInTheDocument();
      expect(getRosterStore().program?.template_version_id).toBe(versionToan6Published.id);
      expect(within(card).getByText("Toán 6 cơ bản · v1 · 2 buổi")).toBeInTheDocument();
      expect(within(card).getByRole("link", { name: "Mở chương trình mẫu" })).toHaveAttribute(
        "href",
        `/library/templates/${templateToan6.id}`,
      );
    });

    it("applies the course's default version from one button", async () => {
      const user = userEvent.setup();
      getRosterStore().classes[0]!.course = {
        id: courseOptionToan.id,
        code: courseOptionToan.code,
        name: courseOptionToan.name,
      };
      getRosterStore().courseDefaultVersion[courseOptionToan.id] = versionToan6Published.id;
      renderPage();
      const card = await screen.findByRole("region", { name: "Chương trình học" });

      await user.click(await within(card).findByRole("button", { name: "Áp dụng từ khóa mẫu" }));

      expect(await screen.findByText("Đã áp dụng chương trình mẫu")).toBeInTheDocument();
      expect(getRosterStore().program?.template_version_id).toBe(versionToan6Published.id);
    });

    it("asks before overwriting a curriculum that differs, then resends with confirm", async () => {
      const user = userEvent.setup();
      getRosterStore().curriculumDiffers = true;
      getRosterStore().classes[0]!.course = {
        id: courseOptionToan.id,
        code: courseOptionToan.code,
        name: courseOptionToan.name,
      };
      getRosterStore().courseDefaultVersion[courseOptionToan.id] = versionToan6Published.id;
      renderPage();
      const card = await screen.findByRole("region", { name: "Chương trình học" });
      await user.click(await within(card).findByRole("button", { name: "Áp dụng từ khóa mẫu" }));

      const dialog = await screen.findByRole("dialog");
      expect(dialog).toHaveTextContent("Lớp đang có 3 buổi trong sổ đầu bài");
      expect(dialog).toHaveTextContent("chương trình mẫu có 2 buổi");
      expect(getRosterStore().program).toBeNull();

      await user.click(within(dialog).getByRole("button", { name: "Vẫn áp dụng" }));

      expect(await screen.findByText("Đã áp dụng chương trình mẫu")).toBeInTheDocument();
      expect(getRosterStore().program?.template_version_id).toBe(versionToan6Published.id);
    });

    it("removes the program after a confirmation that names what stays", async () => {
      const user = userEvent.setup();
      applyProgramToStore();
      renderPage();
      const card = await screen.findByRole("region", { name: "Chương trình học" });
      await within(card).findByText("Toán 6 cơ bản · v1 · 2 buổi");

      await user.click(within(card).getByRole("button", { name: "Gỡ chương trình" }));
      const dialog = await screen.findByRole("dialog");
      expect(dialog).toHaveTextContent("Sổ đầu bài và giáo án giữ nguyên");
      await user.click(within(dialog).getByRole("button", { name: "Gỡ" }));

      expect(await screen.findByText("Đã gỡ chương trình mẫu")).toBeInTheDocument();
      expect(getRosterStore().program).toBeNull();
      expect(
        await within(card).findByText("Lớp chưa áp dụng chương trình mẫu."),
      ).toBeInTheDocument();
    });

    it("shows a member the applied program without any action", async () => {
      server.use(memberWithLibraryHandler);
      applyProgramToStore();
      renderPage();
      const card = await screen.findByRole("region", { name: "Chương trình học" });
      expect(await within(card).findByText("Toán 6 cơ bản · v1 · 2 buổi")).toBeInTheDocument();
      expect(within(card).getByRole("link", { name: "Mở chương trình mẫu" })).toBeInTheDocument();
      expect(within(card).queryByRole("button")).not.toBeInTheDocument();
    });

    it("keeps the template link from a member without library.read", async () => {
      server.use(memberCenterHandler);
      applyProgramToStore();
      renderPage();
      const card = await screen.findByRole("region", { name: "Chương trình học" });
      expect(await within(card).findByText("Toán 6 cơ bản · v1 · 2 buổi")).toBeInTheDocument();
      expect(
        within(card).queryByRole("link", { name: "Mở chương trình mẫu" }),
      ).not.toBeInTheDocument();
    });

    it("flags a program whose version the library has since archived", async () => {
      applyProgramToStore();
      getRosterStore().program!.version_status = "archived";
      renderPage();
      const card = await screen.findByRole("region", { name: "Chương trình học" });
      expect(await within(card).findByText("Toán 6 cơ bản · v1 · 2 buổi")).toBeInTheDocument();
      expect(within(card).getByText("Đã lưu trữ")).toBeInTheDocument();
      expect(within(card).getByRole("note")).toHaveTextContent(
        "Phiên bản này đã được lưu trữ trong thư viện; lớp vẫn xem được bài, nhưng nên đổi sang phiên bản mới hơn.",
      );
    });

    it("toasts the API's reason when applying fails for a reason other than a curriculum mismatch", async () => {
      const user = userEvent.setup();
      getRosterStore().classes[0]!.course = {
        id: courseOptionToan.id,
        code: courseOptionToan.code,
        name: courseOptionToan.name,
      };
      getRosterStore().courseDefaultVersion[courseOptionToan.id] = versionToan6Published.id;
      server.use(
        http.put(`${API_URL}/classes/:id/program`, () =>
          HttpResponse.json(fail("VERSION_NOT_PUBLISHED", "Phiên bản chưa được phát hành"), {
            status: 409,
          }),
        ),
      );
      renderPage();
      const card = await screen.findByRole("region", { name: "Chương trình học" });
      await user.click(await within(card).findByRole("button", { name: "Áp dụng từ khóa mẫu" }));

      expect(await screen.findByText("Phiên bản chưa được phát hành")).toBeInTheDocument();
      expect(getRosterStore().program).toBeNull();
    });

    it("toasts the API's reason when removing the program fails", async () => {
      const user = userEvent.setup();
      applyProgramToStore();
      server.use(
        http.delete(`${API_URL}/classes/:id/program`, () =>
          HttpResponse.json(fail("FORBIDDEN", "Bạn không có quyền gỡ chương trình"), {
            status: 403,
          }),
        ),
      );
      renderPage();
      const card = await screen.findByRole("region", { name: "Chương trình học" });
      await within(card).findByText("Toán 6 cơ bản · v1 · 2 buổi");
      await user.click(within(card).getByRole("button", { name: "Gỡ chương trình" }));
      const dialog = await screen.findByRole("dialog");
      await user.click(within(dialog).getByRole("button", { name: "Gỡ" }));

      expect(await screen.findByText("Bạn không có quyền gỡ chương trình")).toBeInTheDocument();
      expect(getRosterStore().program).not.toBeNull();
    });
  });

  describe("Lịch sử lớp card", () => {
    it("shows the parent chain and lets the owner set the parent and note", async () => {
      const user = userEvent.setup();
      getRosterStore().classes.push({ ...classParentToan5 });
      renderPage();
      const card = await screen.findByRole("region", { name: "Lịch sử lớp" });
      expect(within(card).getByText("Lớp không tách từ lớp nào.")).toBeInTheDocument();

      await user.click(within(card).getByRole("button", { name: "Sửa lịch sử lớp" }));
      const parentPicker = within(card).getByRole("combobox", { name: "Lớp gốc" });
      await waitFor(() => expect(parentPicker).toBeEnabled());
      await user.click(parentPicker);
      const listbox = await screen.findByRole("listbox");
      // A class can never be its own parent.
      expect(within(listbox).queryByRole("option", { name: /Toán 6A/ })).not.toBeInTheDocument();
      await user.click(within(listbox).getByRole("option", { name: /Toán 5B/ }));
      await user.type(
        within(card).getByRole("textbox", { name: "Ghi chú lịch sử" }),
        "Tách từ lớp 5B sau kỳ hè",
      );
      await user.click(within(card).getByRole("button", { name: "Lưu" }));

      expect(await screen.findByText("Đã cập nhật lớp")).toBeInTheDocument();
      const saved = getRosterStore().classes[0]!;
      expect(saved.parent_class_id).toBe(classParentToan5.id);
      expect(saved.lineage_note).toBe("Tách từ lớp 5B sau kỳ hè");
      expect(await within(card).findByRole("link", { name: "Toán 5B" })).toHaveAttribute(
        "href",
        `/classes/${classParentToan5.id}`,
      );
      expect(within(card).getByText("Tách từ lớp 5B sau kỳ hè")).toBeInTheDocument();
    });

    it("lists the classes split off from this one", async () => {
      getRosterStore().classes.push({
        ...classParentToan5,
        id: "70000000-0000-4000-8000-000000000003",
        name: "Toán 6A2",
        code: "TOAN6A2",
        parent_class_id: classWithSchedule.id,
      });
      renderPage();
      const card = await screen.findByRole("region", { name: "Lịch sử lớp" });
      expect(await within(card).findByText("Lớp tách ra")).toBeInTheDocument();
      expect(within(card).getByRole("link", { name: "Toán 6A2" })).toHaveAttribute(
        "href",
        "/classes/70000000-0000-4000-8000-000000000003",
      );
    });

    it("reveals the change history to a reader of the audit trail", async () => {
      const user = userEvent.setup();
      getRosterStore().auditLogs.push({ ...auditLogClassUpdate });
      const seen: URLSearchParams[] = [];
      server.use(
        http.get(`${API_URL}/audit-logs`, ({ request }) => {
          seen.push(new URL(request.url).searchParams);
          return HttpResponse.json(ok({ items: getRosterStore().auditLogs, next_cursor: "" }));
        }),
      );
      renderPage();
      const card = await screen.findByRole("region", { name: "Lịch sử lớp" });
      expect(seen).toHaveLength(0);

      await user.click(within(card).getByRole("button", { name: "Lịch sử thay đổi" }));

      expect(await within(card).findByText("class.update")).toBeInTheDocument();
      expect(within(card).getByText("Cô Lan")).toBeInTheDocument();
      expect(seen).toHaveLength(1);
      expect(seen[0]!.get("entity_type")).toBe("class");
      expect(seen[0]!.get("entity_id")).toBe(classWithSchedule.id);
    });

    it("hides failed requests and class_message actions from the change history", async () => {
      const user = userEvent.setup();
      getRosterStore().auditLogs.push(
        { ...auditLogClassUpdate },
        {
          ...auditLogClassUpdate,
          id: "77000000-0000-4000-8000-000000000002",
          action: "class.update",
          status_code: 403,
        },
        {
          ...auditLogClassUpdate,
          id: "77000000-0000-4000-8000-000000000003",
          action: "class_message.post",
          entity_type: "class",
        },
        {
          ...auditLogClassUpdate,
          id: "77000000-0000-4000-8000-000000000004",
          action: "class_message.delete",
          entity_type: "class_message",
          actor_user_id: null,
          actor_name: "",
        },
      );
      server.use(
        http.get(`${API_URL}/audit-logs`, () =>
          HttpResponse.json(ok({ items: getRosterStore().auditLogs, next_cursor: "" })),
        ),
      );
      renderPage();
      const card = await screen.findByRole("region", { name: "Lịch sử lớp" });

      await user.click(within(card).getByRole("button", { name: "Lịch sử thay đổi" }));

      expect(await within(card).findByText("class.update")).toBeInTheDocument();
      expect(within(card).getAllByText("class.update")).toHaveLength(1);
      expect(within(card).queryByText("class_message.post")).not.toBeInTheDocument();
      expect(within(card).queryByText("class_message.delete")).not.toBeInTheDocument();
      expect(within(card).getByText(testPrimaryTeacher.full_name)).toBeInTheDocument();
    });

    it("hides the change history from a member without audit.read", async () => {
      server.use(memberCenterHandler);
      renderPage();
      const card = await screen.findByRole("region", { name: "Lịch sử lớp" });
      await within(card).findByText("Lớp không tách từ lớp nào.");
      expect(
        within(card).queryByRole("button", { name: "Lịch sử thay đổi" }),
      ).not.toBeInTheDocument();
      expect(
        within(card).queryByRole("button", { name: "Sửa lịch sử lớp" }),
      ).not.toBeInTheDocument();
    });
  });

  describe("Bài tập tab", () => {
    it("groups the exercises by template lesson", async () => {
      applyProgramToStore();
      renderPage("homework");
      expect(
        await screen.findByRole("heading", { name: "Buổi 1 · Số tự nhiên" }),
      ).toBeInTheDocument();
      expect(screen.getByRole("heading", { name: "Buổi 2 · Phân số" })).toBeInTheDocument();
      expect(screen.getByText("Bài 1: Tập hợp")).toBeInTheDocument();
      expect(screen.getByText("Mức 2")).toBeInTheDocument();
      expect(screen.getByText("Bài 2: So sánh phân số")).toBeInTheDocument();
    });

    it("points at the info tab while no program is applied", async () => {
      renderPage("homework");
      expect(await screen.findByText("Lớp chưa áp dụng chương trình mẫu")).toBeInTheDocument();
      expect(
        screen.getByText("Áp dụng chương trình ở tab Thông tin để xem bài tập theo buổi."),
      ).toBeInTheDocument();
    });
  });

  describe("Tài liệu tab", () => {
    it("shows only the materials shared with students until 'Hiện tất cả' is on", async () => {
      const user = userEvent.setup();
      applyProgramToStore();
      renderPage("documents");
      expect(await screen.findByRole("link", { name: "Video phân số" })).toHaveAttribute(
        "href",
        "https://example.com/video-phan-so",
      );
      expect(screen.queryByText("Slide số tự nhiên")).not.toBeInTheDocument();

      await user.click(screen.getByRole("switch", { name: "Hiện tất cả" }));

      expect(await screen.findByRole("link", { name: "Slide số tự nhiên" })).toBeInTheDocument();
      expect(screen.getByRole("link", { name: "Video phân số" })).toBeInTheDocument();
      expect(screen.getByText("Chia sẻ với học sinh")).toBeInTheDocument();
    });
  });

  describe("Buổi học tab with a program", () => {
    it("zips the countable sessions with the template lessons and flags the mismatch", async () => {
      applyProgramToStore();
      renderPage("sessions");
      const table = await screen.findByRole("table");
      await screen.findByText("Số tự nhiên");
      const rows = within(table).getAllByRole("row").slice(1);
      const lessonCells = rows.map((row) => within(row).getAllByRole("cell").at(-2)!.textContent);
      // Day 8 is cancelled and never consumes a template lesson.
      expect(lessonCells).toEqual(["Số tự nhiên", "—", "Phân số", "—", "—"]);
      expect(screen.getByRole("note")).toHaveTextContent(
        "Lớp có 4 buổi đã lên lịch (tính trên các buổi đã tạo, không tính buổi huỷ) nhưng chương trình mẫu có 2 buổi.",
      );
      expect(screen.getByRole("link", { name: "Sổ đầu bài" })).toHaveAttribute(
        "href",
        `/classbook?class_id=${classWithSchedule.id}`,
      );
    });

    it("hides the mismatch warning while an open-ended class still trails the template", async () => {
      applyProgramToStore();
      // Only the first (non-cancelled) session survives: 1 created session
      // against a 2-lesson template, on a class with no end_date yet — the
      // gap is only because sessions aren't all generated yet, not a real
      // mismatch, so the warning would be noise here.
      getRosterStore().sessions = [getRosterStore().sessions[0]!];
      renderPage("sessions");
      await screen.findByRole("table");
      // Wait for the lesson data (not just the sessions) to land before
      // asserting the warning stays absent, or the check could pass simply
      // because the mismatch hasn't been computed yet.
      await screen.findByText("Số tự nhiên");
      expect(screen.queryByRole("note")).not.toBeInTheDocument();
    });

    it("keeps the table usable, with a blank Buổi mẫu column, when the lesson list fails to load", async () => {
      applyProgramToStore();
      server.use(
        http.get(`${API_URL}/classes/:id/program/lessons`, () =>
          HttpResponse.json(fail("INTERNAL", "Lỗi máy chủ"), { status: 500 }),
        ),
      );
      renderPage("sessions");
      const table = await screen.findByRole("table");
      const rows = within(table).getAllByRole("row").slice(1);
      const lessonCells = rows.map((row) => within(row).getAllByRole("cell").at(-2)!.textContent);
      expect(lessonCells).toEqual(["—", "—", "—", "—", "—"]);
      expect(screen.queryByRole("note")).not.toBeInTheDocument();
    });
  });

  describe("Chat tab", () => {
    it("lists the thread oldest first with the internal-only note", async () => {
      getRosterStore().messages.push({ ...messageFromMinh }, { ...messageFromLan });
      renderPage("chat");
      const items = await screen.findAllByRole("listitem");
      expect(items[0]).toHaveTextContent("Thầy Minh");
      expect(items[0]).toHaveTextContent("Tuần này kiểm tra 15 phút nhé.");
      expect(items[1]).toHaveTextContent("Cô Lan");
      expect(screen.getByText("Tin nhắn nội bộ, không đồng bộ Zalo")).toBeInTheDocument();
    });

    it("shows the empty copy when nothing was posted yet", async () => {
      renderPage("chat");
      expect(await screen.findByText("Chưa có tin nhắn nào.")).toBeInTheDocument();
    });

    it("shows the API's reason when the caller no longer works the class", async () => {
      server.use(
        http.get(`${API_URL}/classes/:id/messages`, () =>
          HttpResponse.json(fail("FORBIDDEN", "Bạn không còn phụ trách lớp này"), {
            status: 403,
          }),
        ),
      );
      renderPage("chat");
      expect(await screen.findByRole("alert")).toHaveTextContent("Bạn không còn phụ trách lớp này");
    });

    it("hides the composer from a member without class_messages.post", async () => {
      server.use(memberWithoutPostHandler);
      renderPage("chat");
      expect(await screen.findByText("Chưa có tin nhắn nào.")).toBeInTheDocument();
      expect(screen.queryByRole("textbox", { name: "Nội dung tin nhắn" })).not.toBeInTheDocument();
      expect(screen.getByText("Bạn không có quyền nhắn tin trong lớp này.")).toBeInTheDocument();
    });

    it("posts a message and shows it in the thread", async () => {
      const user = userEvent.setup();
      renderPage("chat");
      await screen.findByText("Chưa có tin nhắn nào.");
      const composer = screen.getByRole("textbox", { name: "Nội dung tin nhắn" });
      await user.type(composer, "Nhớ mang vở bài tập.");
      await user.click(screen.getByRole("button", { name: "Gửi" }));

      const item = await screen.findByRole("listitem");
      expect(item).toHaveTextContent("Nhớ mang vở bài tập.");
      expect(item).toHaveTextContent("Cô Lan");
      expect(composer).toHaveValue("");
      expect(getRosterStore().messages).toHaveLength(1);
    });

    it("lets the author delete their own message, after confirming, but not someone else's", async () => {
      const user = userEvent.setup();
      server.use(memberCenterHandler);
      getRosterStore().messages.push({ ...messageFromMinh }, { ...messageFromLan });
      renderPage("chat");
      const items = await screen.findAllByRole("listitem");
      expect(
        within(items[0]!).queryByRole("button", { name: /^Xoá tin nhắn/ }),
      ).not.toBeInTheDocument();

      // The label names the message so screen readers can tell the buttons apart.
      await user.click(
        within(items[1]!).getByRole("button", { name: /^Xoá tin nhắn của Cô Lan lúc / }),
      );
      const dialog = await screen.findByRole("dialog");
      expect(dialog).toHaveTextContent("Xoá tin nhắn này?");
      expect(getRosterStore().messages).toHaveLength(2);

      await user.click(within(dialog).getByRole("button", { name: "Xoá" }));

      await waitFor(() => expect(getRosterStore().messages).toHaveLength(1));
      expect(getRosterStore().messages[0]!.id).toBe(messageFromMinh.id);
    });

    it("keeps the message when the deletion is cancelled", async () => {
      const user = userEvent.setup();
      getRosterStore().messages.push({ ...messageFromLan });
      renderPage("chat");
      const item = await screen.findByRole("listitem");
      await user.click(within(item).getByRole("button", { name: /^Xoá tin nhắn của Cô Lan/ }));
      const dialog = await screen.findByRole("dialog");

      await user.click(within(dialog).getByRole("button", { name: "Hủy" }));

      await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
      expect(getRosterStore().messages).toHaveLength(1);
      expect(screen.getByRole("listitem")).toHaveTextContent("Đã nhận, cảm ơn thầy.");
    });

    it("loads older messages page by page", async () => {
      const user = userEvent.setup();
      getRosterStore().messagesPageSize = 1;
      getRosterStore().messages.push({ ...messageFromMinh }, { ...messageFromLan });
      renderPage("chat");
      expect(await screen.findAllByRole("listitem")).toHaveLength(1);

      await user.click(screen.getByRole("button", { name: "Tải tin cũ hơn" }));

      await waitFor(() => expect(screen.getAllByRole("listitem")).toHaveLength(2));
      expect(screen.queryByRole("button", { name: "Tải tin cũ hơn" })).not.toBeInTheDocument();
    });
  });
});
