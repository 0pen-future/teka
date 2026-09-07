/**
 * Width-aware `window.matchMedia` for tests. The default shim in `setup.ts`
 * reports every query as not matching, so `useMediaQuery` always takes the
 * narrow branch (bottom sheets, compact rows). Call `mockViewport(width)` in
 * a test or `beforeEach` to pick a viewport: `(min-width: Npx)` and
 * `(max-width: Npx)` clauses are evaluated against it, and listeners added
 * through `addEventListener("change")` are notified when the width changes,
 * so a mounted hook re-renders like it would on a real resize.
 */
type ChangeListener = (event: MediaQueryListEvent) => void;

let viewportWidth = 0;
const subscriptions = new Set<{ query: string; listener: ChangeListener }>();

function queryMatches(query: string, width: number): boolean {
  const min = /\(min-width:\s*([\d.]+)px\)/.exec(query);
  const max = /\(max-width:\s*([\d.]+)px\)/.exec(query);
  if (!min && !max) return false;
  return (!min || width >= Number(min[1])) && (!max || width <= Number(max[1]));
}

function matchMedia(query: string): MediaQueryList {
  const list = {
    get matches() {
      return queryMatches(query, viewportWidth);
    },
    media: query,
    onchange: null,
    addEventListener(_type: string, listener: ChangeListener) {
      subscriptions.add({ query, listener });
    },
    removeEventListener(_type: string, listener: ChangeListener) {
      for (const sub of subscriptions) {
        if (sub.query === query && sub.listener === listener) subscriptions.delete(sub);
      }
    },
    addListener(listener: ChangeListener) {
      subscriptions.add({ query, listener });
    },
    removeListener(listener: ChangeListener) {
      for (const sub of subscriptions) {
        if (sub.query === query && sub.listener === listener) subscriptions.delete(sub);
      }
    },
    dispatchEvent: () => false,
  };
  return list as unknown as MediaQueryList;
}

export function mockViewport(width: number): void {
  viewportWidth = width;
  window.matchMedia = matchMedia;
  for (const { query, listener } of subscriptions) {
    listener({ matches: queryMatches(query, width), media: query } as MediaQueryListEvent);
  }
}
