import { apiClient } from "@/lib/api/client";
import { parseList, type Paginated } from "@/lib/api/envelope";

import { reportPeriodSchema, type ReportPeriod } from "../schemas/reports-schemas";

/**
 * `GET /billing-periods` for a reports-oversight caller lists every center
 * teacher's periods (a plain member only ever gets their own — the server
 * scopes, this client just renders). With `classId` it switches to class
 * discovery instead: only periods carrying that class's charges, opened by
 * any class_staff stint on the class (how a hoc_vu finds WHICH period to
 * send, since the class may bill under another teacher's period). The server
 * answers 404 for a caller without a stint. `per_page=100` is the endpoint's
 * cap; newest-first so the current month leads each teacher's group.
 */
export async function listReportPeriods(classId?: string): Promise<Paginated<ReportPeriod>> {
  const res = await apiClient.get<unknown>("/billing-periods", {
    params: {
      per_page: 100,
      sort: "-period_start",
      ...(classId ? { class_id: classId } : {}),
    },
  });
  return parseList(reportPeriodSchema, res.data);
}
