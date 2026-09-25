import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { useAuthStore } from "@/features/auth";
import { API_URL, fail, ok } from "@/test/msw/handlers";
import { server } from "@/test/msw/server";
import {
  renderWithProviders,
  signInAs,
  testPrimaryTeacher,
  testSecondaryTeacher,
} from "@/test/utils";

import { ExerciseGroupsTab } from "../components/exercise-groups-tab";
import { MaterialsBank } from "../components/materials-bank";
import { TemplateLessonPage } from "../pages/template-lesson-page";
import {
  exerciseBai1,
  getLibraryStore,
  lessonDraftSoTuNhien,
  lessonPublishedSoTuNhien,
  libraryHandlers,
  materialSlide,
  materialVideo,
  resetLibraryStore,
  templateToan6,
  versionToan6Draft,
} from "./library-handlers";

function renderPage(lessonId = lessonDraftSoTuNhien.id) {
  return renderWithProviders(<TemplateLessonPage />, {
    route: `/library/templates/${templateToan6.id}/lessons/${lessonId}`,
    path: "/library/templates/:id/lessons/:lessonId",
    extraRoutes: [{ path: "/library/templates/:id", element: <div>template-stub</div> }],
  });
}

function memberWith(...permissions: string[]) {
  return http.get(`${API_URL}/centers/me`, () =>
    HttpResponse.json(ok({ center_name: "Trung Tâm Bình Minh", permissions })),
  );
}

async function openExercisesTab(user: ReturnType<typeof userEvent.setup>) {
  await user.click(await screen.findByRole("tab", { name: "Bài tập" }));
}

function materialLinksOf(lessonId: string) {
  return getLibraryStore()
    .materialLinks.filter((l) => l.lesson_id === lessonId)
    .sort((a, b) => a.position - b.position);
}

function exerciseLinksOf(lessonId: string) {
  return getLibraryStore()
    .exerciseLinks.filter((l) => l.lesson_id === lessonId)
    .sort((a, b) => a.position - b.position);
}

beforeEach(() => {
  resetLibraryStore();
  server.use(...libraryHandlers);
  signInAs(testPrimaryTeacher);
});

afterEach(() => {
  useAuthStore.getState().clearSession();
});

describe("TemplateLessonPage", () => {
  it("shows the not-found block for an unknown lesson", async () => {
    renderPage("82000000-0000-4000-8000-00000000dead");

    expect(await screen.findByText("Không tìm thấy buổi học mẫu")).toBeInTheDocument();
  });

  describe("thông tin chung", () => {
    it("shows the locked banner and hides every write control when the version was published", async () => {
      renderPage(lessonPublishedSoTuNhien.id);

      expect(
        await screen.findByRole("heading", { name: "Buổi 1 · Số tự nhiên" }),
      ).toBeInTheDocument();
      expect(
        await screen.findByText(
          "Phiên bản v1 đã phát hành (0 lớp đang gắn) — chỉ xem. Muốn sửa, tạo bản nháp mới ở màn chương trình mẫu.",
        ),
      ).toBeInTheDocument();
      expect(screen.queryByRole("button", { name: "Sửa" })).not.toBeInTheDocument();

      const info = screen.getByRole("region", { name: "Thông tin chung" });
      expect(
        within(screen.getByRole("group", { name: "Hình thức" })).getByText("Buổi học có lịch"),
      ).toBeInTheDocument();
      expect(
        within(screen.getByRole("group", { name: "Thời lượng" })).getByText("90 phút"),
      ).toBeInTheDocument();
      expect(within(info).queryByRole("textbox")).not.toBeInTheDocument();
    });

    it("is read-only for a member without library.edit, keeping the field values visible", async () => {
      server.use(memberWith("library.read"));
      signInAs(testSecondaryTeacher);
      renderPage();

      expect(await screen.findByText("Bạn không có quyền soạn buổi học mẫu.")).toBeInTheDocument();
      expect(screen.queryByRole("button", { name: "Sửa" })).not.toBeInTheDocument();
      expect(
        within(screen.getByRole("group", { name: "Mô tả ngắn" })).getByText("Nhận biết tập N"),
      ).toBeInTheDocument();
    });

    it("switches to self-study mode and saves the duration in a step of 15 minutes", async () => {
      const user = userEvent.setup();
      renderPage();
      await screen.findByRole("heading", { name: "Buổi 1 · Số tự nhiên" });
      const info = screen.getByRole("region", { name: "Thông tin chung" });

      await user.click(within(info).getByRole("button", { name: "Sửa" }));
      expect(screen.getByLabelText("Tên buổi")).toHaveValue("Số tự nhiên");

      await user.click(screen.getByRole("radio", { name: "Không lịch" }));
      await user.clear(screen.getByLabelText("Thời lượng (phút)"));
      await user.type(screen.getByLabelText("Thời lượng (phút)"), "105");
      await user.click(screen.getByRole("button", { name: "Lưu" }));

      expect(await screen.findByText("Đã lưu buổi học")).toBeInTheDocument();
      expect(screen.queryByLabelText("Tên buổi")).not.toBeInTheDocument();
      await waitFor(() => {
        expect(
          getLibraryStore().lessons.find((l) => l.id === lessonDraftSoTuNhien.id),
        ).toMatchObject({ mode: "self_study", duration_min: 105 });
      });
      expect(
        within(screen.getByRole("group", { name: "Hình thức" })).getByText("Không lịch"),
      ).toBeInTheDocument();
    });

    it("rejects a duration that is not a multiple of 15 minutes before calling the API", async () => {
      const user = userEvent.setup();
      renderPage();
      await screen.findByRole("heading", { name: "Buổi 1 · Số tự nhiên" });
      const info = screen.getByRole("region", { name: "Thông tin chung" });
      await user.click(within(info).getByRole("button", { name: "Sửa" }));

      await user.clear(screen.getByLabelText("Thời lượng (phút)"));
      await user.type(screen.getByLabelText("Thời lượng (phút)"), "100");
      await user.click(screen.getByRole("button", { name: "Lưu" }));

      expect(
        await screen.findByText("Thời lượng là bội số của 15 phút, từ 15 đến 1440 phút"),
      ).toBeInTheDocument();
      expect(
        getLibraryStore().lessons.find((l) => l.id === lessonDraftSoTuNhien.id)?.duration_min,
      ).toBe(90);
    });

    it("locks the form when the save is refused because the version was published meanwhile", async () => {
      const user = userEvent.setup();
      renderPage();
      await screen.findByRole("heading", { name: "Buổi 1 · Số tự nhiên" });
      const info = screen.getByRole("region", { name: "Thông tin chung" });
      await user.click(within(info).getByRole("button", { name: "Sửa" }));

      server.use(
        http.put(`${API_URL}/library/lessons/:lid`, () => {
          const version = getLibraryStore().versions.find((v) => v.id === versionToan6Draft.id);
          if (version) version.status = "published";
          return HttpResponse.json(fail("VERSION_LOCKED", "Phiên bản đã phát hành"), {
            status: 409,
          });
        }),
      );
      await user.click(screen.getByRole("button", { name: "Lưu" }));

      expect(await screen.findByText(/Phiên bản v2 đã phát hành/)).toBeInTheDocument();
      expect(screen.queryByRole("button", { name: "Sửa" })).not.toBeInTheDocument();
    });
  });

  describe("nội dung buổi học", () => {
    it("creates new content from the menu, posting the material then attaching it with a PUT", async () => {
      const user = userEvent.setup();
      renderPage();
      await screen.findByRole("heading", { name: "Buổi 1 · Số tự nhiên" });

      await user.click(screen.getByRole("button", { name: "Thêm nội dung" }));
      await user.click(await screen.findByRole("menuitem", { name: "Video" }));

      const dialog = await screen.findByRole("dialog", { name: "Thêm học liệu" });
      await user.type(within(dialog).getByLabelText("Tên học liệu"), "Slide chương 2");
      await user.type(within(dialog).getByLabelText("Đường dẫn"), "https://example.com/slide-2");
      await user.click(within(dialog).getByRole("button", { name: "Thêm" }));

      expect(await screen.findByText("Đã thêm nội dung vào buổi học")).toBeInTheDocument();
      const created = getLibraryStore().materials.find((m) => m.title === "Slide chương 2");
      expect(created).toMatchObject({ kind: "video", url: "https://example.com/slide-2" });
      expect(materialLinksOf(lessonDraftSoTuNhien.id).map((l) => l.material_id)).toEqual([
        materialSlide.id,
        created?.id,
      ]);
    });

    it("adds an existing material from the bank picker, offering only active items", async () => {
      const user = userEvent.setup();
      getLibraryStore().materials.push({
        ...materialVideo,
        id: "83000000-0000-4000-8000-000000000099",
        title: "Ảnh minh hoạ cũ",
        active: false,
      });
      renderPage();
      await screen.findByRole("heading", { name: "Buổi 1 · Số tự nhiên" });

      await user.click(screen.getByRole("button", { name: "Thêm nội dung" }));
      await user.click(await screen.findByRole("menuitem", { name: "Chọn từ ngân hàng nội dung" }));

      const picker = await screen.findByRole("dialog", { name: "Chọn từ ngân hàng nội dung" });
      await within(picker).findByRole("checkbox", { name: "Video phân số" });
      expect(
        within(picker).queryByRole("checkbox", { name: "Ảnh minh hoạ cũ" }),
      ).not.toBeInTheDocument();

      await user.click(within(picker).getByRole("checkbox", { name: "Video phân số" }));
      await user.click(within(picker).getByRole("button", { name: "Thêm 1 nội dung" }));

      expect(await screen.findByText("Đã thêm nội dung vào buổi học")).toBeInTheDocument();
      expect(materialLinksOf(lessonDraftSoTuNhien.id).map((l) => l.material_id)).toEqual([
        materialSlide.id,
        materialVideo.id,
      ]);
    });

    it("toggles the share flag with an immediate save", async () => {
      const user = userEvent.setup();
      renderPage();
      const checkbox = await screen.findByRole("checkbox", {
        name: "Chia sẻ Slide số tự nhiên với học viên",
      });
      expect(checkbox).toBeChecked();

      await user.click(checkbox);

      expect(await screen.findByText("Đã cập nhật chia sẻ nội dung")).toBeInTheDocument();
      expect(materialLinksOf(lessonDraftSoTuNhien.id)).toEqual([
        expect.objectContaining({ material_id: materialSlide.id, shared_with_students: false }),
      ]);
    });

    it("reorders with the up/down arrows and saves once sorting is turned off", async () => {
      const user = userEvent.setup();
      getLibraryStore().materialLinks.push({
        lesson_id: lessonDraftSoTuNhien.id,
        material_id: materialVideo.id,
        shared_with_students: false,
        position: 2,
      });
      renderPage();
      await screen.findByText("Video phân số");

      await user.click(screen.getByRole("button", { name: "Sắp xếp thứ tự" }));
      await user.click(screen.getByRole("button", { name: "Đưa Slide số tự nhiên xuống" }));
      await user.click(screen.getByRole("button", { name: "Xong sắp xếp" }));

      expect(await screen.findByText("Đã lưu thứ tự nội dung")).toBeInTheDocument();
      expect(materialLinksOf(lessonDraftSoTuNhien.id).map((l) => l.material_id)).toEqual([
        materialVideo.id,
        materialSlide.id,
      ]);
    });

    it("removes a material from the lesson with an immediate save", async () => {
      const user = userEvent.setup();
      renderPage();
      await screen.findByText("Slide số tự nhiên");

      await user.click(screen.getByRole("button", { name: "Gỡ khỏi buổi" }));

      expect(await screen.findByText("Đã gỡ nội dung khỏi buổi học")).toBeInTheDocument();
      expect(materialLinksOf(lessonDraftSoTuNhien.id)).toEqual([]);
    });

    it("hides every write control on a published lesson but still lets a reader open the link", async () => {
      renderPage(lessonPublishedSoTuNhien.id);
      await screen.findByText(/Phiên bản v1 đã phát hành/);

      const content = screen.getByRole("region", { name: "Nội dung buổi học" });
      expect(within(content).getByText("Slide số tự nhiên")).toBeInTheDocument();
      expect(
        within(content).queryByRole("button", { name: "Thêm nội dung" }),
      ).not.toBeInTheDocument();
      expect(within(content).queryByRole("checkbox")).not.toBeInTheDocument();
      expect(
        within(content).queryByRole("button", { name: "Gỡ khỏi buổi" }),
      ).not.toBeInTheDocument();

      const user = userEvent.setup();
      await user.click(within(content).getByRole("button", { name: "Xem thêm Slide số tự nhiên" }));
      expect(within(content).getByRole("link", { name: "Mở liên kết" })).toHaveAttribute(
        "href",
        materialSlide.url,
      );
    });

    it("refreshes the content bank's usage column once a material is attached from the picker", async () => {
      const user = userEvent.setup();

      // Both views share one query client, as they would in the app: the
      // content bank and a lesson's content tab are separate routes that
      // stay mounted at once here so the attach's cache invalidation, not a
      // remount, is what the bank picks up.
      renderWithProviders(
        <>
          <MaterialsBank canEdit={false} />
          <TemplateLessonPage />
        </>,
        {
          route: `/library/templates/${templateToan6.id}/lessons/${lessonDraftSoTuNhien.id}`,
          path: "/library/templates/:id/lessons/:lessonId",
          extraRoutes: [{ path: "/library/templates/:id", element: <div>template-stub</div> }],
        },
      );
      await screen.findByRole("heading", { name: "Buổi 1 · Số tự nhiên" });
      expect(
        within(await screen.findByRole("row", { name: /Video phân số/ })).getByText("—"),
      ).toBeInTheDocument();

      await user.click(screen.getByRole("button", { name: "Thêm nội dung" }));
      await user.click(await screen.findByRole("menuitem", { name: "Chọn từ ngân hàng nội dung" }));
      const picker = await screen.findByRole("dialog", { name: "Chọn từ ngân hàng nội dung" });
      await user.click(within(picker).getByRole("checkbox", { name: "Video phân số" }));
      await user.click(within(picker).getByRole("button", { name: "Thêm 1 nội dung" }));
      expect(await screen.findByText("Đã thêm nội dung vào buổi học")).toBeInTheDocument();

      await waitFor(() => {
        expect(
          within(screen.getByRole("row", { name: /Video phân số/ })).getByText("1 buổi · 1 CT mẫu"),
        ).toBeInTheDocument();
      });
    });
  });

  describe("bài tập", () => {
    it("creates a self-authored exercise, posting it then attaching it with a PUT", async () => {
      const user = userEvent.setup();
      renderPage();
      await openExercisesTab(user);

      await user.click(screen.getByRole("button", { name: "Bài tập tự soạn" }));
      const dialog = await screen.findByRole("dialog", { name: "Thêm bài tập" });
      await user.type(within(dialog).getByLabelText("Tên bài tập"), "Bài 3: Phép cộng");
      await user.click(within(dialog).getByRole("button", { name: "Thêm" }));

      expect(await screen.findByText("Đã thêm bài tập vào buổi học")).toBeInTheDocument();
      const created = getLibraryStore().exercises.find((e) => e.title === "Bài 3: Phép cộng");
      expect(created).toBeDefined();
      expect(exerciseLinksOf(lessonDraftSoTuNhien.id).map((l) => l.exercise_id)).toEqual([
        exerciseBai1.id,
        created?.id,
      ]);
      expect(
        exerciseLinksOf(lessonDraftSoTuNhien.id).find((l) => l.exercise_id === created?.id)
          ?.group_id,
      ).toBeNull();
    });

    it("moves an exercise into a group with an immediate save", async () => {
      const user = userEvent.setup();
      getLibraryStore().exerciseGroups.push({
        id: "86000000-0000-4000-8000-000000000001",
        version_id: versionToan6Draft.id,
        name: "Nhóm A",
        position: 1,
      });
      renderPage();
      await openExercisesTab(user);

      await user.click(screen.getByRole("combobox", { name: "Nhóm bài tập cho Bài 1: Tập hợp" }));
      await user.click(
        within(await screen.findByRole("listbox")).getByRole("option", { name: "Nhóm A" }),
      );

      expect(await screen.findByText("Đã cập nhật nhóm bài tập")).toBeInTheDocument();
      expect(exerciseLinksOf(lessonDraftSoTuNhien.id)).toEqual([
        expect.objectContaining({
          exercise_id: exerciseBai1.id,
          group_id: "86000000-0000-4000-8000-000000000001",
        }),
      ]);
    });

    it("removes an exercise from the lesson with an immediate save", async () => {
      const user = userEvent.setup();
      renderPage();
      await openExercisesTab(user);
      await screen.findByText("Bài 1: Tập hợp");

      await user.click(screen.getByRole("button", { name: "Gỡ" }));

      expect(await screen.findByText("Đã gỡ bài tập khỏi buổi học")).toBeInTheDocument();
      expect(exerciseLinksOf(lessonDraftSoTuNhien.id)).toEqual([]);
    });

    it("links each exercise row to its own entry in the exercise bank", async () => {
      const user = userEvent.setup();
      renderPage();
      await openExercisesTab(user);

      expect(await screen.findByRole("link", { name: "Xem trong ngân hàng" })).toHaveAttribute(
        "href",
        `/library/exercises?q=${exerciseBai1.code}`,
      );
    });

    it("hides every write control on a published lesson", async () => {
      const user = userEvent.setup();
      renderPage(lessonPublishedSoTuNhien.id);
      await screen.findByText(/Phiên bản v1 đã phát hành/);
      await openExercisesTab(user);

      expect(screen.getByText("Chưa có bài tập nào trong buổi.")).toBeInTheDocument();
      expect(screen.queryByRole("button", { name: "Bài tập tự soạn" })).not.toBeInTheDocument();
      expect(screen.queryByRole("button", { name: "Bài tập có sẵn" })).not.toBeInTheDocument();
    });

    it("clears a lesson's stale group selection once its exercise group is deleted elsewhere", async () => {
      const user = userEvent.setup();
      getLibraryStore().exerciseGroups.push({
        id: "86000000-0000-4000-8000-000000000002",
        version_id: versionToan6Draft.id,
        name: "Nhóm B",
        position: 1,
      });
      getLibraryStore().exerciseLinks.find(
        (l) => l.lesson_id === lessonDraftSoTuNhien.id && l.exercise_id === exerciseBai1.id,
      )!.group_id = "86000000-0000-4000-8000-000000000002";

      // Both views share one query client, as they would in the app: the
      // groups tab and the lesson's exercises tab are separate routes that
      // stay mounted at once here so the delete's cache invalidation, not a
      // remount, is what the lesson view picks up.
      renderWithProviders(
        <>
          <ExerciseGroupsTab version={versionToan6Draft} templateId={templateToan6.id} authoring />
          <TemplateLessonPage />
        </>,
        {
          route: `/library/templates/${templateToan6.id}/lessons/${lessonDraftSoTuNhien.id}`,
          path: "/library/templates/:id/lessons/:lessonId",
          extraRoutes: [{ path: "/library/templates/:id", element: <div>template-stub</div> }],
        },
      );
      await openExercisesTab(user);
      expect(
        await screen.findByRole("combobox", { name: "Nhóm bài tập cho Bài 1: Tập hợp" }),
      ).toHaveTextContent("Nhóm B");

      await user.click(await screen.findByRole("button", { name: "Xoá" }));
      await user.click(
        within(await screen.findByRole("dialog")).getByRole("button", { name: "Xoá nhóm" }),
      );
      expect(await screen.findByText("Đã xoá nhóm bài tập")).toBeInTheDocument();

      await waitFor(() => {
        expect(
          screen.getByRole("combobox", { name: "Nhóm bài tập cho Bài 1: Tập hợp" }),
        ).toHaveTextContent("Chưa phân nhóm");
      });
    });
  });
});
