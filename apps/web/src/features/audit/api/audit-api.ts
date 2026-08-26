import { apiClient } from "@/lib/api/client";
import { parseData } from "@/lib/api/envelope";

import {
  auditLogPageSchema,
  type AuditLogFilters,
  type AuditLogPage,
} from "../schemas/audit-schemas";

export async function listAuditLogs(
  filters: AuditLogFilters = {},
  cursor?: string,
): Promise<AuditLogPage> {
  // Empty values are dropped entirely — the API treats a present-but-empty
  // param as malformed rather than absent.
  const params: Record<string, string> = {};
  for (const [key, value] of Object.entries({ ...filters, cursor })) {
    if (value) {
      params[key] = value;
    }
  }
  const res = await apiClient.get<unknown>("/audit-logs", { params });
  return parseData(auditLogPageSchema, res.data);
}
