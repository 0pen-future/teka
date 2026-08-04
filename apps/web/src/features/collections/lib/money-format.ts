/**
 * Parses a thousands-separated đồng string back to an integer, stripping
 * every non-digit character. Local to `features/collections`, mirroring
 * `features/roster/lib/roster-format.ts#parseMoney` — kept feature-local
 * rather than shared since it is the only other consumer. Lives outside
 * `money-field.tsx` so that component file only exports the component
 * (fast-refresh boundary).
 */
export function parseMoney(value: string): number {
  const digits = value.replace(/[^\d]/g, "");
  return digits === "" ? 0 : Number.parseInt(digits, 10);
}
