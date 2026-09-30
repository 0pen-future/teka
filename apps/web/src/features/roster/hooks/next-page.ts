import type { Paginated } from "@/lib/api/envelope";

/**
 * Next page number for a "Xem thêm" list, or undefined once every row the
 * server counted is loaded (or a page came back empty).
 */
export function nextPageAfter<T>(pages: Paginated<T>[]): number | undefined {
  const last = pages[pages.length - 1];
  if (!last || last.items.length === 0) return undefined;
  const loaded = pages.reduce((n, p) => n + p.items.length, 0);
  return loaded < last.meta.total ? last.meta.page + 1 : undefined;
}

/**
 * Placeholder for a "Xem thêm" list keyed by `params`: while only the search
 * text changes, the previous rows stay on screen; any other change (a tab, a
 * class, a filter) starts blank, so one list's rows and row actions never show
 * under another.
 */
export function keepWhileSearching<P extends { query?: string }>(params: P) {
  return <D>(previous: D | undefined, previousQuery?: { queryKey: readonly unknown[] }) => {
    const previousParams = previousQuery?.queryKey.at(-1) as P | undefined;
    if (!previousParams) return undefined;
    const sameList =
      JSON.stringify({ ...previousParams, query: undefined }) ===
      JSON.stringify({ ...params, query: undefined });
    return sameList ? previous : undefined;
  };
}
