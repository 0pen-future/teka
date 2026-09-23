import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { useAuthStore } from "@/features/auth";
import { API_URL, ok } from "@/test/msw/handlers";
import { server } from "@/test/msw/server";
import { renderWithProviders, signInAs, testPrimaryTeacher } from "@/test/utils";

import { TemplateCreateWizardPage } from "../pages/template-create-wizard-page";
import { getLibraryStore, libraryHandlers, resetLibraryStore } from "./library-handlers";

function renderPage() {
  return renderWithProviders(<TemplateCreateWizardPage />, {
    route: "/library/templates/new",
    path: "/library/templates/new",
    extraRoutes: [
      { path: "/prep/:vid/board", element: <div>board-stub</div> },
      { path: "/library", element: <div>library-stub</div> },
    ],
  });
}

function memberWith(...permissions: string[]) {
  return http.get(`${API_URL}/centers/me`, () =>
    HttpResponse.json(ok({ center_name: "Trung Tâm Bình Minh", permissions })),
  );
}

beforeEach(() => {
  resetLibraryStore();
  server.use(...libraryHandlers);
  signInAs(testPrimaryTeacher);
});

afterEach(() => {
  useAuthStore.getState().clearSession();
});

describe("TemplateCreateWizardPage", () => {
  it("walks the three steps and lands on the new draft's board", async () => {
    const user = userEvent.setup();
    const { router } = renderPage();

    // Step 1: the template's own fields, validated before moving on.
    expect(await screen.findByRole("heading", { name: /Bước 1/ })).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Tiếp tục" }));
    expect(await screen.findByText("Bắt buộc nhập tên")).toBeInTheDocument();
    await user.type(screen.getByLabelText("Mã chương trình"), "hoa-8");
    await user.type(screen.getByLabelText("Tên chương trình"), "Hoá 8 cơ bản");
    await user.type(screen.getByLabelText("Môn học"), "Hoá");
    await user.click(screen.getByRole("button", { name: "Tiếp tục" }));

    // Step 2: how many lessons to seed.
    expect(await screen.findByRole("heading", { name: /Bước 2/ })).toBeInTheDocument();
    const count = screen.getByLabelText("Số buổi");
    await user.clear(count);
    await user.type(count, "3");
    await user.click(screen.getByRole("button", { name: "Tiếp tục" }));

    // Step 3: preview, then create.
    expect(await screen.findByRole("heading", { name: /Bước 3/ })).toBeInTheDocument();
    expect(screen.getByText("HOA-8")).toBeInTheDocument();
    expect(screen.getByText("Buổi 1")).toBeInTheDocument();
    expect(screen.getByText("Buổi 3")).toBeInTheDocument();
    expect(screen.queryByText("Buổi 4")).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Tạo chương trình" }));

    await waitFor(() => expect(screen.getByText("board-stub")).toBeInTheDocument());
    const store = getLibraryStore();
    const template = store.templates.find((t) => t.code === "HOA-8");
    expect(template?.subject).toBe("Hoá");
    const draft = store.versions.find((v) => v.template_id === template?.id);
    expect(store.lessons.filter((l) => l.version_id === draft?.id).map((l) => l.title)).toEqual([
      "Buổi 1",
      "Buổi 2",
      "Buổi 3",
    ]);
    expect(router.state.location.pathname).toBe(`/prep/${draft?.id}/board`);
  });

  it("rejects a lesson count outside 1–100 and lets the user step back", async () => {
    const user = userEvent.setup();
    renderPage();

    await user.type(await screen.findByLabelText("Mã chương trình"), "SU9");
    await user.type(screen.getByLabelText("Tên chương trình"), "Sử 9");
    await user.click(screen.getByRole("button", { name: "Tiếp tục" }));
    const count = await screen.findByLabelText("Số buổi");
    await user.clear(count);
    await user.type(count, "0");
    await user.click(screen.getByRole("button", { name: "Tiếp tục" }));
    expect(await screen.findByText("Số buổi từ 1 đến 100")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Quay lại" }));
    expect(await screen.findByLabelText("Tên chương trình")).toHaveValue("Sử 9");
  });

  it("surfaces a duplicate code on the code field", async () => {
    const user = userEvent.setup();
    renderPage();

    await user.type(await screen.findByLabelText("Mã chương trình"), "toan6");
    await user.type(screen.getByLabelText("Tên chương trình"), "Trùng mã");
    await user.click(screen.getByRole("button", { name: "Tiếp tục" }));
    await user.click(await screen.findByRole("button", { name: "Tiếp tục" }));
    await user.click(await screen.findByRole("button", { name: "Tạo chương trình" }));

    expect(await screen.findByLabelText("Mã chương trình")).toBeInTheDocument();
    expect(screen.getByLabelText("Mã chương trình")).toHaveAttribute("aria-invalid", "true");
  });

  it("blocks a member without library.edit", async () => {
    server.use(memberWith("library.read"));
    renderPage();

    expect(await screen.findByText("Bạn không có quyền tạo chương trình mẫu")).toBeInTheDocument();
  });
});
