import type { Period } from "@/features/billing";
import { periodSchema } from "@/features/billing";
import { apiClient } from "@/lib/api/client";
import { parseData } from "@/lib/api/envelope";

import {
  attendanceResponseSchema,
  sessionSchema,
  type AttendanceResponse,
  type CancelSessionInput,
  type ConfirmAttendanceInput,
  type Session,
} from "../schemas/attendance-schemas";

/**
 * There is no top-level `GET /sessions?...` list — sessions are always listed
 * per class (`sessions.listRange`, `apps/api/internal/features/sessions/
 * handler.go`), which conveniently matches the Design Spec's class-pill-tabs
 * filter. Generates any missing rows for `[from, to]` from the class's
 * schedules before returning, capped at a 400-day range server-side.
 */
export async function listClassSessions(
  classId: string,
  params: { from: string; to: string },
): Promise<Session[]> {
  const res = await apiClient.get<unknown>(`/classes/${classId}/sessions`, { params });
  return parseData(sessionSchema.array(), res.data);
}

/** `GET /sessions/:id` — one session's header fields (class name, date, status). */
export async function getSession(sessionId: string): Promise<Session> {
  const res = await apiClient.get<unknown>(`/sessions/${sessionId}`);
  return parseData(sessionSchema, res.data);
}

/**
 * `GET /sessions/:id/attendance` — the roster + attendance sheet in one
 * call. `status` is null on every row until the session is first confirmed.
 */
export async function getSessionRoster(sessionId: string): Promise<AttendanceResponse> {
  const res = await apiClient.get<unknown>(`/sessions/${sessionId}/attendance`);
  return parseData(attendanceResponseSchema, res.data);
}

/**
 * `POST /sessions/:id/attendance` (`attendance.confirm`) — NOT `PUT`, despite
 * the idempotent-replace semantics: send the full exception list (`marks`)
 * every time, first confirm or a later edit alike, so the server can write
 * present rows for every unlisted student and transition the session to held
 * in one call.
 */
export async function confirmAttendance(
  sessionId: string,
  input: ConfirmAttendanceInput,
): Promise<AttendanceResponse> {
  const res = await apiClient.post<unknown>(`/sessions/${sessionId}/attendance`, input);
  return parseData(attendanceResponseSchema, res.data);
}

/** `POST /sessions/:id/cancel` — `reason` is required; refuses (409) an already-confirmed session. */
export async function cancelSession(
  sessionId: string,
  input: CancelSessionInput,
): Promise<Session> {
  const res = await apiClient.post<unknown>(`/sessions/${sessionId}/cancel`, input);
  return parseData(sessionSchema, res.data);
}

/**
 * The attendance API carries no `period_status` field on either the session
 * or the attendance sheet — the closed-period signal only exists as
 * `confirm`'s post-hoc `warning` string, after the write already happened.
 * To warn the teacher *before* they commit, this reuses `POST
 * /billing-periods` (`billing.ensurePeriod`), the same idempotent
 * create-or-get `features/billing` already exposes as `getCurrentPeriod`,
 * keyed on the session's own year/month instead of today's.
 */
export async function getPeriodForDate(sessionDate: string): Promise<Period> {
  const [year, month] = sessionDate.split("-").map(Number);
  const res = await apiClient.post<unknown>("/billing-periods", { year, month });
  return parseData(periodSchema, res.data);
}
