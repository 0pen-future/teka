import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { useAuthStore } from "@/features/auth";
import { mockZaloStatus } from "@/features/profile/__tests__/zalo-handlers";
import { API_URL, fail, listMeta, ok } from "@/test/msw/handlers";
import { server } from "@/test/msw/server";
import { renderWithProviders, signInAs, testPrimaryTeacher } from "@/test/utils";

import { ContactsPage } from "../pages/contacts-page";
import {
  contactSingleChild,
  contactTwoChildren,
  getRosterStore,
  resetRosterStore,
  rosterHandlers,
} from "./roster-handlers";

/** One row of `POST /me/zalo/friends/match`, keyed by the phone as sent. */
interface MatchRow {
  phone: string;
  matched: boolean;
  user_id?: string;
  display_name?: string;
  zalo_name?: string;
  avatar?: string;
  is_friend: boolean;
}

/**
 * Serves the match endpoint from a phone→row table; phones without an entry
 * answer `matched: false` like the real API. Returns the recorded request
 * bodies so tests can assert exactly which phones traveled.
 */
function mockZaloMatch(rowsByPhone: Record<string, Omit<MatchRow, "phone">>) {
  const calls: { phones: string[][] } = { phones: [] };
  server.use(
    http.post(`${API_URL}/me/zalo/friends/match`, async ({ request }) => {
      const body = (await request.json()) as { phones: string[] };
      calls.phones.push(body.phones);
      const rows: MatchRow[] = body.phones.map((phone) => ({
        phone,
        matched: false,
        is_friend: false,
        ...rowsByPhone[phone],
      }));
      return HttpResponse.json(ok(rows));
    }),
  );
  return calls;
}

/** Serves the friend-request endpoint, recording each body. */
function mockFriendRequest(status = 204) {
  const calls: { bodies: { user_id?: string }[] } = { bodies: [] };
  server.use(
    http.post(`${API_URL}/me/zalo/friends/request`, async ({ request }) => {
      calls.bodies.push((await request.json()) as { user_id?: string });
      if (status === 204) {
        return new HttpResponse(null, { status: 204 });
      }
      return HttpResponse.json(fail("BAD_GATEWAY", "zalo refused"), { status });
    }),
  );
  return calls;
}

const matchedFriendRow = {
  matched: true,
  user_id: "zl-user-101",
  display_name: "Mẹ Lan",
  avatar: "https://example.com/me-lan.png",
  is_friend: true,
};

const foundNotFriendRow = {
  matched: true,
  user_id: "zl-user-102",
  display_name: "Bố Hùng",
  is_friend: false,
};

/** A third unmapped contact so every group has a row. */
function addUnmatchedContact() {
  getRosterStore().contacts.push({
    id: "40000000-0000-4000-8000-000000000003",
    full_name: "Cô Ba",
    phone: "+84911222333",
    student_count: 0,
    created_at: "2026-01-01T08:00:00Z",
  });
}

function renderContacts() {
  signInAs(testPrimaryTeacher);
  return renderWithProviders(<ContactsPage />, {
    route: "/contacts",
    path: "/contacts",
    extraRoutes: [{ path: "/profile", element: <div>Trang cá nhân</div> }],
  });
}

async function openAutoMap(user: ReturnType<typeof userEvent.setup>) {
  const trigger = await screen.findByRole("button", { name: "Tự động ghép Zalo" });
  await waitFor(() => expect(trigger).toBeEnabled());
  await user.click(trigger);
  return screen.findByRole("dialog");
}

/** The list row (rendered as a listitem) holding the given contact name. */
function rowFor(dialog: HTMLElement, name: string) {
  const row = within(dialog).getByText(name).closest("li");
  expect(row).not.toBeNull();
  return within(row as HTMLElement);
}

beforeEach(() => {
  resetRosterStore();
  server.use(...rosterHandlers);
});

afterEach(() => {
  useAuthStore.getState().clearSession();
});

describe("auto-map trigger on the contacts page", () => {
  it("stays disabled while Zalo is unlinked and explains why inline", async () => {
    // The shared default `GET /me/zalo` answers "not linked".
    renderContacts();
    const trigger = await screen.findByRole("button", { name: "Tự động ghép Zalo" });
    await screen.findByText("Nguyễn Thị Lan");
    expect(trigger).toBeDisabled();
    // Tooltips never surface on touch, so the reason lives in the page.
    expect(await screen.findByRole("link", { name: "Kết nối Zalo ở trang Hồ sơ" })).toHaveAttribute(
      "href",
      "/profile",
    );
  });

  it("stays disabled while the stored session expired", async () => {
    mockZaloStatus({ linked: true, status: "expired", display_name: "Cô Lan" });
    renderContacts();
    const trigger = await screen.findByRole("button", { name: "Tự động ghép Zalo" });
    await screen.findByText("Nguyễn Thị Lan");
    expect(trigger).toBeDisabled();
  });
});

describe("auto-map review dialog", () => {
  beforeEach(() => {
    mockZaloStatus({ linked: true, status: "linked", display_name: "Cô Lan" });
  });

  it("groups rows into matched, not-friend, and not-found", async () => {
    addUnmatchedContact();
    mockZaloMatch({
      [contactSingleChild.phone]: matchedFriendRow,
      [contactTwoChildren.phone]: foundNotFriendRow,
    });
    const user = userEvent.setup();
    renderContacts();
    const dialog = await openAutoMap(user);

    // Matched friend: default-checked checkbox plus the Zalo name.
    const matchedRow = rowFor(dialog, "Nguyễn Thị Lan");
    expect(matchedRow.getByRole("checkbox", { name: /Nguyễn Thị Lan/ })).toBeChecked();
    expect(matchedRow.getByText("Mẹ Lan")).toBeInTheDocument();

    // Found but not a friend: labeled, no checkbox, per-row Kết bạn button.
    const notFriendRow = rowFor(dialog, "Phạm Văn Hùng");
    expect(notFriendRow.getByText("Chưa kết bạn")).toBeInTheDocument();
    expect(notFriendRow.queryByRole("checkbox")).not.toBeInTheDocument();
    expect(notFriendRow.getByRole("button", { name: "Kết bạn" })).toBeInTheDocument();

    // Not found: display-only.
    const notFoundRow = rowFor(dialog, "Cô Ba");
    expect(notFoundRow.getByText("Không tìm thấy")).toBeInTheDocument();
    expect(notFoundRow.queryByRole("checkbox")).not.toBeInTheDocument();
    expect(notFoundRow.queryByRole("button", { name: "Kết bạn" })).not.toBeInTheDocument();

    // One friend request per explicit click — no bulk control anywhere.
    expect(within(dialog).queryByRole("button", { name: /tất cả/i })).not.toBeInTheDocument();
  });

  it("excludes already-mapped contacts from the lookup", async () => {
    const mapped = getRosterStore().contacts.find((item) => item.id === contactSingleChild.id);
    if (mapped) {
      mapped.zalo_user_id = "zl-user-900";
      mapped.zalo_name = "Đã ghép";
    }
    const calls = mockZaloMatch({ [contactTwoChildren.phone]: foundNotFriendRow });
    const user = userEvent.setup();
    renderContacts();
    const dialog = await openAutoMap(user);

    await within(dialog).findByText("Phạm Văn Hùng");
    expect(calls.phones).toEqual([[contactTwoChildren.phone]]);
  });

  it("says so when every contact is already mapped", async () => {
    for (const contact of getRosterStore().contacts) {
      contact.zalo_user_id = `zl-user-${contact.id.slice(-2)}`;
      contact.zalo_name = "Đã ghép";
    }
    const calls = mockZaloMatch({});
    const user = userEvent.setup();
    renderContacts();
    const dialog = await openAutoMap(user);

    await within(dialog).findByText("Tất cả người liên hệ đã được ghép.");
    expect(calls.phones).toHaveLength(0);
  });

  it("writes only the checked rows on confirm and reports the split", async () => {
    mockZaloMatch({
      [contactSingleChild.phone]: matchedFriendRow,
      [contactTwoChildren.phone]: {
        matched: true,
        user_id: "zl-user-103",
        display_name: "Dì Đào",
        is_friend: true,
      },
    });
    const user = userEvent.setup();
    renderContacts();
    const dialog = await openAutoMap(user);

    // Both matched rows arrive checked; unchecking one must exclude it.
    await within(dialog).findByRole("checkbox", { name: /Nguyễn Thị Lan/ });
    await user.click(within(dialog).getByRole("checkbox", { name: /Phạm Văn Hùng/ }));
    await user.click(within(dialog).getByRole("button", { name: "Ghép 1 đã chọn" }));

    await within(dialog).findByText("Đã ghép 1/1");
    const stored = getRosterStore().contacts;
    const lan = stored.find((item) => item.id === contactSingleChild.id);
    const hung = stored.find((item) => item.id === contactTwoChildren.id);
    expect(lan?.zalo_user_id).toBe("zl-user-101");
    expect(lan?.zalo_name).toBe("Mẹ Lan");
    expect(hung?.zalo_user_id).toBeUndefined();
  });

  it("caps the lookup at 200 phones and says so", async () => {
    for (let i = 0; i < 200; i += 1) {
      getRosterStore().contacts.push({
        id: `40000000-0000-4000-8000-9${String(i).padStart(11, "0")}`,
        full_name: `Phụ huynh ${i}`,
        phone: `+8490${String(1000000 + i)}`,
        student_count: 0,
        created_at: "2026-01-01T08:00:00Z",
      });
    }
    const calls = mockZaloMatch({});
    const user = userEvent.setup();
    renderContacts();
    const dialog = await openAutoMap(user);

    await within(dialog).findByText(/200 người liên hệ đầu tiên/);
    // The capped 200 phones travel in paced-endpoint-sized requests.
    expect(calls.phones.map((batch) => batch.length)).toEqual([100, 100]);
    expect(new Set(calls.phones.flat()).size).toBe(200);
  });

  it("pages through the list and dedupes a row straddling a page boundary", async () => {
    const extras = Array.from({ length: 148 }, (_, i) => ({
      id: `40000000-0000-4000-8000-8${String(i).padStart(11, "0")}`,
      full_name: `Phụ huynh ${i}`,
      phone: `+8491${String(1000000 + i)}`,
      student_count: 0,
      created_at: "2026-01-01T08:00:00Z",
    }));
    const all = [...getRosterStore().contacts, ...extras];
    server.use(
      http.get(`${API_URL}/contacts`, ({ request }) => {
        const url = new URL(request.url);
        const page = Number(url.searchParams.get("page") ?? "1");
        const perPage = Number(url.searchParams.get("per_page") ?? "20");
        // Page 2 re-serves page 1's last row, like a name-sorted list with no
        // tiebreaker can when rows shift between page reads.
        const start = page === 1 ? 0 : (page - 1) * perPage - 1;
        const items = all.slice(start, page * perPage);
        return HttpResponse.json(ok(items, listMeta(all.length, page, perPage)));
      }),
    );
    const calls = mockZaloMatch({});
    const user = userEvent.setup();
    renderContacts();
    const dialog = await openAutoMap(user);

    await within(dialog).findByText(/Không tìm thấy trên Zalo — 150/);
    expect(calls.phones.map((batch) => batch.length)).toEqual([100, 50]);
    expect(new Set(calls.phones.flat()).size).toBe(150);
  });

  it("writes one mapping when two checked contacts share a phone", async () => {
    getRosterStore().contacts.push({
      id: "40000000-0000-4000-8000-000000000004",
      full_name: "Ông Tư",
      phone: contactSingleChild.phone,
      student_count: 1,
      created_at: "2026-01-01T08:00:00Z",
    });
    mockZaloMatch({ [contactSingleChild.phone]: matchedFriendRow });
    const user = userEvent.setup();
    renderContacts();
    const dialog = await openAutoMap(user);

    await within(dialog).findByRole("checkbox", { name: /Ông Tư/ });
    await user.click(within(dialog).getByRole("button", { name: "Ghép 2 đã chọn" }));

    // One Zalo user maps to one contact, so the duplicate is skipped, counted
    // in the split, and never sent as a doomed 409 write.
    await within(dialog).findByText("Đã ghép 1/2");
    await within(dialog).findByText("Các dòng còn lại chưa được ghép.");
    const holders = getRosterStore().contacts.filter(
      (contact) => contact.zalo_user_id === "zl-user-101",
    );
    expect(holders).toHaveLength(1);
  });

  it.each([
    [409, "CONFLICT", "zalo session expired"],
    [404, "NOT_FOUND", "no zalo session"],
  ] as const)(
    "routes to the profile page when the lookup answers %i",
    async (status, code, message) => {
      server.use(
        http.post(`${API_URL}/me/zalo/friends/match`, () =>
          HttpResponse.json(fail(code, message), { status }),
        ),
      );
      const user = userEvent.setup();
      renderContacts();
      const dialog = await openAutoMap(user);

      await within(dialog).findByText(/Phiên Zalo không còn hiệu lực/);
      expect(within(dialog).getByRole("link", { name: "Quét lại mã" })).toHaveAttribute(
        "href",
        "/profile",
      );
    },
  );
});

describe("Kết bạn action", () => {
  beforeEach(() => {
    mockZaloStatus({ linked: true, status: "linked", display_name: "Cô Lan" });
    mockZaloMatch({ [contactTwoChildren.phone]: foundNotFriendRow });
  });

  it("sends exactly one request for the row's UID and flips to Đã gửi", async () => {
    const calls = mockFriendRequest();
    const user = userEvent.setup();
    renderContacts();
    const dialog = await openAutoMap(user);

    const row = rowFor(dialog, "Phạm Văn Hùng");
    await user.click(await row.findByRole("button", { name: "Kết bạn" }));

    const sent = await row.findByRole("button", { name: "Đã gửi" });
    expect(sent).toBeDisabled();
    expect(calls.bodies).toEqual([{ user_id: "zl-user-102" }]);
  });

  it("surfaces a failed request and re-enables the button", async () => {
    const calls = mockFriendRequest(502);
    const user = userEvent.setup();
    renderContacts();
    const dialog = await openAutoMap(user);

    const row = rowFor(dialog, "Phạm Văn Hùng");
    await user.click(await row.findByRole("button", { name: "Kết bạn" }));

    await within(dialog).findByText("Không gửi được lời mời. Thử lại.");
    expect(row.getByRole("button", { name: "Kết bạn" })).toBeEnabled();
    expect(calls.bodies).toHaveLength(1);
  });
});
