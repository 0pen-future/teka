import { apiClient } from "@/lib/api/client";
import { parseData } from "@/lib/api/envelope";

import { periodSchema, type Period } from "../schemas/billing-schemas";

/**
 * There is no `GET .../current` endpoint — `POST /billing-periods`
 * (`billing.ensurePeriod`, `apps/api/internal/features/billing/handler.go`)
 * is an idempotent create-or-get keyed on `(teacher, year, month)`, so
 * calling it with today's calendar month is the sanctioned way to resolve
 * "the current period" from the client.
 */
export async function getCurrentPeriod(): Promise<Period> {
  const now = new Date();
  const res = await apiClient.post<unknown>("/billing-periods", {
    year: now.getFullYear(),
    month: now.getMonth() + 1,
  });
  return parseData(periodSchema, res.data);
}
