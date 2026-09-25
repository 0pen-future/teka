import { ApiError } from "@/lib/api/errors";

export interface RowErrors {
  /** Row index → field → message, from `<index>.<field>` validation keys. */
  rows: Record<number, Record<string, string>>;
  /** Everything the row map cannot express: a non-indexed field, a 409, a network failure. */
  general: string | null;
}

/**
 * Splits an array-body validation failure back onto its rows. The API keys
 * each entry's errors as `<index>.<field>`; anything else is shown once
 * under the whole editor.
 */
export function rowErrorsFromApi(error: unknown, fallback: string): RowErrors {
  if (!(error instanceof ApiError)) {
    return { rows: {}, general: fallback };
  }
  const rows: RowErrors["rows"] = {};
  const general: string[] = [];
  for (const [key, message] of Object.entries(error.fields ?? {})) {
    const match = /^(\d+)\.(\w+)$/.exec(key);
    if (match) {
      const index = Number(match[1]);
      rows[index] = { ...rows[index], [match[2]!]: message };
    } else {
      general.push(message);
    }
  }
  if (general.length === 0 && Object.keys(rows).length === 0) {
    general.push(error.message);
  }
  return { rows, general: general.length ? general.join(" · ") : null };
}
