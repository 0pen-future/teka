import { findFoldedMatch } from "@/lib/utils";

/**
 * Narrows the Hồ sơ học sinh rows to those whose name contains `query`,
 * ignoring diacritics and case, preserving the incoming order. A blank query
 * returns the input array itself so memoised consumers keep their identity.
 */
export function filterStudentRows<T extends { name: string }>(rows: T[], query: string): T[] {
  if (query.trim() === "") {
    return rows;
  }
  return rows.filter((row) => findFoldedMatch(row.name, query) !== null);
}
