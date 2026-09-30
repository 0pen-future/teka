import { screen } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { useAuthStore } from "@/features/auth";
import { API_URL, ok } from "@/test/msw/handlers";
import { server } from "@/test/msw/server";
import { renderWithProviders, signInAs, testPrimaryTeacher } from "@/test/utils";

import { ContactDetailPage } from "../pages/contact-detail-page";
import { StudentDetailPage } from "../pages/student-detail-page";
import {
  contactSingleChild,
  resetRosterStore,
  rosterHandlers,
  studentSiblingOne,
} from "./roster-handlers";

/** A member-shaped `/centers/me` holding exactly `permissions`. */
function asMember(permissions: string[]) {
  server.use(
    http.get(`${API_URL}/centers/me`, () =>
      HttpResponse.json(ok({ center_name: "Trung Tâm Bình Minh", permissions })),
    ),
  );
}

/** Records every request URL for the duration of a test. */
function recordRequests() {
  const requested: string[] = [];
  const onRequest = ({ request }: { request: Request }) => {
    requested.push(request.url);
  };
  server.events.on("request:start", onRequest);
  return {
    requested,
    stop: () => server.events.removeListener("request:start", onRequest),
  };
}

beforeEach(() => {
  resetRosterStore();
  server.use(...rosterHandlers);
  signInAs(testPrimaryTeacher);
});

afterEach(() => {
  useAuthStore.getState().clearSession();
});

describe("StudentDetailPage permissions", () => {
  function renderStudentDetail() {
    return renderWithProviders(<StudentDetailPage />, {
      route: `/students/${studentSiblingOne.id}`,
      path: "/students/:id",
    });
  }

  it("gives the owner the enroll, end, edit and delete actions", async () => {
    renderStudentDetail();

    expect(await screen.findByRole("button", { name: "Ghi danh vào lớp" })).toBeInTheDocument();
    expect(await screen.findByRole("button", { name: "Kết thúc ghi danh" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Sửa" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: studentSiblingOne.contact_name })).toHaveAttribute(
      "href",
      `/contacts/${studentSiblingOne.contact_id}`,
    );
  });

  it("shows a teacher the record read-only, with the contact as plain text", async () => {
    asMember(["students.list"]);
    renderStudentDetail();

    expect(await screen.findByText("Toán 6A")).toBeInTheDocument();
    expect(screen.getByText(studentSiblingOne.contact_name)).toBeInTheDocument();
    expect(
      screen.queryByRole("link", { name: studentSiblingOne.contact_name }),
    ).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Ghi danh vào lớp" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Kết thúc ghi danh" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Sửa" })).not.toBeInTheDocument();
  });
});

describe("ContactDetailPage permissions", () => {
  function renderContactDetail() {
    return renderWithProviders(<ContactDetailPage />, {
      route: `/contacts/${contactSingleChild.id}`,
      path: "/contacts/:id",
      extraRoutes: [{ path: "/students", element: <div>Trang học sinh</div> }],
    });
  }

  it("redirects a member without contacts.view_all before any contact request", async () => {
    asMember(["students.list"]);
    const recorder = recordRequests();
    try {
      renderContactDetail();

      expect(await screen.findByText("Trang học sinh")).toBeInTheDocument();
      expect(
        recorder.requested.filter((url) => url.includes("/contacts") || url.includes("/students")),
      ).toEqual([]);
    } finally {
      recorder.stop();
    }
  });

  it("shows the Zalo mapping read-only to a contacts.view_all member without reports.send", async () => {
    asMember(["students.list", "contacts.view_all"]);
    const recorder = recordRequests();
    try {
      renderContactDetail();

      expect(await screen.findByText("Chưa liên kết bạn Zalo nào.")).toBeInTheDocument();
      expect(screen.getByText(contactSingleChild.full_name)).toBeInTheDocument();
      // The read-only card never asks for the viewer's own Zalo session.
      expect(recorder.requested.filter((url) => url.includes("/me/zalo"))).toEqual([]);
    } finally {
      recorder.stop();
    }
  });
});
