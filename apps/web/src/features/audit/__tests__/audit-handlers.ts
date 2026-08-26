import { http, HttpResponse } from "msw";

import { API_URL, ok } from "@/test/msw/handlers";

export const auditOwnerId = "aa000000-0000-4000-8000-000000000001";
export const auditMemberId = "aa000000-0000-4000-8000-000000000002";

/** Owner-shaped /centers/me with a member list so the actor filter has options. */
export const auditCenterMembers = [
  { id: auditOwnerId, full_name: "Cô Lan", phone: "+84901234567", is_owner: true },
  { id: auditMemberId, full_name: "Thầy Minh", phone: "+84907654321", is_owner: false },
];

export const logClassCreate = {
  id: "b0000000-0000-4000-8000-000000000001",
  occurred_at: "2026-08-26T10:30:00Z",
  actor_user_id: auditOwnerId,
  actor_name: "Cô Lan",
  actor_role: "owner",
  action: "class.create",
  method: "POST",
  path: "/api/v1/classes",
  entity_type: "class",
  entity_id: "c0000000-0000-4000-8000-000000000001",
  status_code: 201,
  ip: "203.0.113.10",
  user_agent: "Mozilla/5.0 (X11; Linux x86_64)",
  metadata: { class_name: "Toán 6A" },
};

export const logAuthLogin = {
  id: "b0000000-0000-4000-8000-000000000002",
  occurred_at: "2026-08-26T09:15:00Z",
  actor_user_id: auditMemberId,
  actor_name: "Thầy Minh",
  actor_role: "member",
  action: "auth.login",
  method: "POST",
  path: "/api/v1/auth/login",
  entity_type: "session",
  entity_id: "",
  status_code: 200,
  ip: "203.0.113.24",
  user_agent: "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X)",
  metadata: null,
};

/** Actor since removed from teachers — empty actor_name renders "(đã xóa)". */
export const logStudentDelete = {
  id: "b0000000-0000-4000-8000-000000000003",
  occurred_at: "2026-08-25T16:00:00Z",
  actor_user_id: "aa000000-0000-4000-8000-000000000009",
  actor_name: "",
  actor_role: "member",
  action: "student.delete",
  method: "DELETE",
  path: "/api/v1/students/d0000000-0000-4000-8000-000000000001",
  entity_type: "student",
  entity_id: "d0000000-0000-4000-8000-000000000001",
  status_code: 403,
  ip: "203.0.113.24",
  user_agent: "Mozilla/5.0 (X11; Linux x86_64)",
  metadata: null,
};

export const logStatementFail = {
  id: "b0000000-0000-4000-8000-000000000004",
  occurred_at: "2026-08-25T08:00:00Z",
  actor_user_id: auditOwnerId,
  actor_name: "Cô Lan",
  actor_role: "owner",
  action: "statement.generate",
  method: "POST",
  path: "/api/v1/statements",
  entity_type: "statement",
  entity_id: "",
  status_code: 500,
  ip: "203.0.113.10",
  user_agent: "Mozilla/5.0 (X11; Linux x86_64)",
  metadata: null,
};

/** Newest-first, matching the API's occurred_at DESC ordering. */
const allLogs = [logClassCreate, logAuthLogin, logStudentDelete, logStatementFail];

/** Two per page so four fixtures exercise a full second page and the last-page state. */
export const AUDIT_PAGE_SIZE = 2;

/** Every /audit-logs request URL, for asserting query params. */
export const auditRequests: URL[] = [];

export function resetAuditStore(): void {
  auditRequests.length = 0;
}

export const auditHandlers = [
  http.get(`${API_URL}/centers/me`, () =>
    HttpResponse.json(
      ok({
        center: {
          id: "30000000-0000-4000-8000-000000000001",
          name: "Trung Tâm Bình Minh",
          is_owner: true,
        },
        members: auditCenterMembers,
      }),
    ),
  ),
  http.get(`${API_URL}/audit-logs`, ({ request }) => {
    const url = new URL(request.url);
    auditRequests.push(url);

    const actorId = url.searchParams.get("actor_id");
    const action = url.searchParams.get("action");
    let rows = allLogs;
    if (actorId) {
      rows = rows.filter((row) => row.actor_user_id === actorId);
    }
    if (action) {
      rows = rows.filter((row) => row.action.startsWith(action));
    }

    // Offset cursor stands in for the API's opaque keyset cursor: the client
    // must treat it as a token, so the fake shape is free to differ.
    const cursor = url.searchParams.get("cursor");
    const offset = cursor ? Number(cursor.replace("offset-", "")) : 0;
    const items = rows.slice(offset, offset + AUDIT_PAGE_SIZE);
    const nextOffset = offset + AUDIT_PAGE_SIZE;
    return HttpResponse.json(
      ok({
        items,
        next_cursor: nextOffset < rows.length ? `offset-${nextOffset}` : "",
      }),
    );
  }),
];
