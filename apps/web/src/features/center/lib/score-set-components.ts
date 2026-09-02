/** Server-side ceiling on columns per bộ điểm (mirrored in `scoreSetInputSchema`). */
export const MAX_SCORE_SET_COMPONENTS = 10;

export interface ParsedComponents {
  names: string[];
  /** True when the pasted text held more names than the ceiling allows. */
  truncated: boolean;
}

/**
 * Turn a pasted list ("Miệng, 15 phút; Giữa kỳ\nCuối kỳ") into ordered
 * column names: split on newline/comma/semicolon, trim, drop blanks, keep the
 * first ten. Duplicates are left in so the caller can point at them.
 */
export function parsePastedComponents(text: string): ParsedComponents {
  const names = text
    .split(/[\n,;]+/)
    .map((name) => name.trim())
    .filter((name) => name.length > 0);
  return {
    names: names.slice(0, MAX_SCORE_SET_COMPONENTS),
    truncated: names.length > MAX_SCORE_SET_COMPONENTS,
  };
}

/**
 * Indexes of names that repeat an earlier one, compared case-insensitively
 * after trimming. The first occurrence is never reported; blanks are ignored
 * (they fail the "không được để trống" rule on their own).
 */
export function findDuplicateIndexes(names: readonly string[]): Set<number> {
  const seen = new Set<string>();
  const duplicates = new Set<number>();
  names.forEach((name, index) => {
    const key = name.trim().toLowerCase();
    if (!key) return;
    if (seen.has(key)) {
      duplicates.add(index);
    }
    seen.add(key);
  });
  return duplicates;
}
