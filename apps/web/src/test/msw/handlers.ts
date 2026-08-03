import { http, HttpResponse } from "msw";

import type { User } from "@/features/users";
import type { Meta } from "@/lib/api/envelope";

/** Must match vitest.config.ts test.env.VITE_API_URL. */
export const API_URL = "http://localhost:8080/api/v1";

// --- Envelope builders (mirror the Go API's response shape exactly) ---

export function ok<T>(data: T, meta?: Meta) {
  return meta === undefined ? { success: true, data } : { success: true, data, meta };
}

export function fail(code: string, message: string, fields?: Record<string, string>) {
  return {
    success: false,
    error: fields === undefined ? { code, message } : { code, message, fields },
  };
}

export function listMeta(total: number, page = 1, perPage = 20): Meta {
  return {
    page,
    per_page: perPage,
    total,
    total_pages: Math.max(1, Math.ceil(total / perPage)),
  };
}

// --- Fixtures ---

let userCounter = 0;

export function makeUser(overrides: Partial<User> = {}): User {
  userCounter += 1;
  return {
    id: `00000000-0000-4000-8000-${String(userCounter).padStart(12, "0")}`,
    email: `user-${userCounter}@example.com`,
    name: `User ${userCounter}`,
    role: "user",
    created_at: "2026-08-01T10:00:00Z",
    updated_at: "2026-08-01T10:00:00Z",
    ...overrides,
  };
}

export const adminUser = makeUser({ email: "admin@example.com", name: "Admin", role: "admin" });
export const aliceUser = makeUser({ email: "alice@example.com", name: "Alice" });
export const bobUser = makeUser({ email: "bob@example.com", name: "Bob" });

export const defaultUsers: User[] = [adminUser, aliceUser, bobUser];

export function makeSession(user: User) {
  return {
    access_token: "test-access-token",
    token_type: "Bearer",
    expires_in: 900,
    user,
  };
}

// --- Default happy-path handlers; tests override per case with server.use() ---

export const handlers = [
  http.post(`${API_URL}/auth/login`, () => HttpResponse.json(ok(makeSession(adminUser)))),
  http.post(`${API_URL}/auth/register`, async ({ request }) => {
    const body = (await request.json()) as { name: string; email: string };
    const user = makeUser({ name: body.name, email: body.email });
    return HttpResponse.json(ok(makeSession(user)), { status: 201 });
  }),
  // No refresh cookie in tests by default: a fresh visitor has no session.
  http.post(`${API_URL}/auth/refresh`, () =>
    HttpResponse.json(fail("UNAUTHORIZED", "invalid refresh token"), { status: 401 }),
  ),
  http.post(`${API_URL}/auth/logout`, () => HttpResponse.json(ok({ message: "logged out" }))),
  http.get(`${API_URL}/users`, () =>
    HttpResponse.json(ok(defaultUsers, listMeta(defaultUsers.length))),
  ),
  http.get(`${API_URL}/users/:id`, ({ params }) => {
    const user = defaultUsers.find((candidate) => candidate.id === params.id);
    if (!user) {
      return HttpResponse.json(fail("NOT_FOUND", "user not found"), { status: 404 });
    }
    return HttpResponse.json(ok(user));
  }),
];
