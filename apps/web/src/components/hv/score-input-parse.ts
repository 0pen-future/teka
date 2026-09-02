/**
 * Result of parsing a raw score string: a number on the 0–10 half-point
 * scale, `null` for an intentionally empty cell, or `"invalid"` when the
 * text cannot be read as a score at all.
 */
export type ParsedScore = number | null | "invalid";

/** One or two leading digits, optionally followed by a decimal part using `.` or `,`. */
const SCORE_PATTERN = /^\d{1,2}([.,]\d+)?$/;

/**
 * Parses a teacher-typed score. Empty/whitespace means "no score". Accepts
 * Vietnamese decimal commas, clamps to 0–10 and rounds to the nearest 0.5 so
 * the stored value always matches what the grading UI can display.
 */
export function parseScoreInput(raw: string): ParsedScore {
  const trimmed = raw.trim();
  if (trimmed === "") return null;
  if (!SCORE_PATTERN.test(trimmed)) return "invalid";

  const value = Number(trimmed.replace(",", "."));
  if (!Number.isFinite(value)) return "invalid";

  const clamped = Math.min(10, Math.max(0, value));
  return Math.round(clamped * 2) / 2;
}
