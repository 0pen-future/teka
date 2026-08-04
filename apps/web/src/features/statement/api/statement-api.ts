import { publicApiClient } from "@/lib/api/public-client";
import { parseData } from "@/lib/api/envelope";

import { statementSchema } from "../schemas/statement-schemas";
import type { Statement } from "../types/statement-types";

/**
 * `GET /public/statements/:token` — no authentication. The server returns
 * `404` for every failure mode (unknown, malformed, revoked, expired,
 * already-paid, or soft-deleted token); there is no other error code to
 * branch on, so callers treat any non-200 identically.
 */
export async function getStatement(token: string): Promise<Statement> {
  const res = await publicApiClient.get<unknown>(`/public/statements/${encodeURIComponent(token)}`);
  return parseData(statementSchema, res.data);
}
