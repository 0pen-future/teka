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
