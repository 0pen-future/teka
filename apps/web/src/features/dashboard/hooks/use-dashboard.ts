import { useQueries, useQuery } from "@tanstack/react-query";

import { listClassSessions, sessionsKeys } from "@/features/attendance";
import type { Period } from "@/features/billing";
import { listStudents, studentsKeys, type Class } from "@/features/roster";

import { getPendingSessions, getPeriodPreview } from "../api/dashboard-api";

export const dashboardKeys = {
  all: ["dashboard"] as const,
  pendingSessions: () => [...dashboardKeys.all, "pending-sessions"] as const,
  periodPreview: (periodId: string) => [...dashboardKeys.all, "period-preview", periodId] as const,
};

export function usePendingSessions() {
  return useQuery({
    queryKey: dashboardKeys.pendingSessions(),
    queryFn: getPendingSessions,
  });
}

/** Roster headcount for the "HỌC SINH" stat — `meta.total`, not a full page. */
export function useStudentsTotal() {
  return useQuery({
    queryKey: studentsKeys.list({ per_page: 1 }),
    queryFn: () => listStudents({ per_page: 1 }),
    select: (page) => page.meta.total,
  });
}

/**
 * Per-class enrolled headcount, index-aligned with `classes`.
 * `StudentResponse` carries no class id, so counting one list client-side is
 * impossible — each class needs its own `class_id`-filtered `meta.total`.
 */
export function useClassStudentCounts(classes: Class[]) {
  return useQueries({
    queries: classes.map((cls) => ({
      queryKey: studentsKeys.list({ class_id: cls.id, per_page: 1 }),
      queryFn: () => listStudents({ class_id: cls.id, per_page: 1 }),
      select: (page: Awaited<ReturnType<typeof listStudents>>) => page.meta.total,
    })),
  });
}

/**
 * Every class's sessions from the current period's start through today,
 * index-aligned with `classes`. There is no cross-class sessions endpoint,
 * so the month's attendance stat and the per-class progress bars both fan
 * out per class; the shared `sessionsKeys.list` key dedupes the two
 * consumers into one request per class.
 *
 * The range deliberately stops at today, not `period_end`: "Thiếu N" /
 * "x/y buổi đã xác nhận" count only sessions already taught (mirroring the
 * server's pending predicate), and `GET /classes/:id/sessions` materializes
 * missing rows for the whole requested range — asking for the full month
 * would both mislabel future sessions as missing and generate rows the
 * dashboard never shows.
 */
export function useClassPeriodSessions(classes: Class[], period: Period | undefined) {
  const today = new Date().toISOString().slice(0, 10);
  const to = period && period.period_end < today ? period.period_end : today;
  return useQueries({
    queries: classes.map((cls) => ({
      queryKey: sessionsKeys.list(cls.id, { from: period?.period_start ?? "", to }),
      queryFn: () => listClassSessions(cls.id, { from: period!.period_start, to }),
      enabled: Boolean(period),
    })),
  });
}

/** The current period's billable totals ("PHẢI THU") via the side-effect-free preview read. */
export function usePeriodPreview(period: Period | undefined) {
  return useQuery({
    queryKey: dashboardKeys.periodPreview(period?.id ?? ""),
    queryFn: () => getPeriodPreview(period!.id),
    enabled: Boolean(period),
  });
}
