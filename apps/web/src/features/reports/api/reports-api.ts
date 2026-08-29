import { apiClient } from "@/lib/api/client";
import { parseList, type Paginated } from "@/lib/api/envelope";

import { reportPeriodSchema, type ReportPeriod } from "../schemas/reports-schemas";

/**
 * `GET /billing-periods` for a reports-oversight caller lists every center
 * teacher's periods (a plain member only ever gets their own — the server
 * scopes, this client just renders). `per_page=100` is the endpoint's cap;
 * newest-first so the current month leads each teacher's group.
 */
export async function listReportPeriods(): Promise<Paginated<ReportPeriod>> {
  const res = await apiClient.get<unknown>("/billing-periods", {
    params: { per_page: 100, sort: "-period_start" },
  });
  return parseList(reportPeriodSchema, res.data);
}
