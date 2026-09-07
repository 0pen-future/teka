/**
 * Folds Vietnamese text for diacritic-insensitive matching: NFD splits base
 * letters from their combining marks, the marks are stripped, and đ/Đ (which
 * NFD does not decompose) map to d. Lowercased so callers compare directly.
 */
export function foldVietnamese(value: string): string {
  return value
    .normalize("NFD")
    .replace(/[̀-ͯ]/g, "")
    .replace(/đ/g, "d")
    .replace(/Đ/g, "d")
    .toLowerCase();
}

/** Half-open `[start, end)` range of UTF-16 offsets in the ORIGINAL string. */
export interface FoldedMatch {
  start: number;
  end: number;
}

function codePointLength(text: string, at: number): number {
  return (text.codePointAt(at) ?? 0) > 0xffff ? 2 : 1;
}

/**
 * Locates `query` in `text` ignoring diacritics and case, returning the match
 * as offsets into the ORIGINAL string so callers can wrap it for highlighting.
 * Each code point is folded on its own and its folded length recorded, so
 * text stored as NFD (base letter and mark as separate code points) still
 * maps back to the right original offsets — slicing by `query.length` would
 * cut through a mark. Any combining marks trailing the last matched letter
 * are pulled into the range so the highlighted slice renders as whole glyphs.
 * An empty or whitespace-only query never matches.
 */
export function findFoldedMatch(text: string, query: string): FoldedMatch | null {
  const foldedQuery = foldVietnamese(query.trim());
  if (foldedQuery === "") {
    return null;
  }
  let folded = "";
  const originalOffsetByFoldedIndex: number[] = [];
  let offset = 0;
  for (const char of text) {
    const piece = foldVietnamese(char);
    originalOffsetByFoldedIndex.push(...Array.from<number>({ length: piece.length }).fill(offset));
    folded += piece;
    offset += char.length;
  }
  const at = folded.indexOf(foldedQuery);
  if (at === -1) {
    return null;
  }
  const start = originalOffsetByFoldedIndex[at]!;
  const lastMatched = originalOffsetByFoldedIndex[at + foldedQuery.length - 1]!;
  let end = lastMatched + codePointLength(text, lastMatched);
  while (end < text.length && foldVietnamese(String.fromCodePoint(text.codePointAt(end)!)) === "") {
    end += codePointLength(text, end);
  }
  return { start, end };
}
