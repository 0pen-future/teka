import { useMemo, useSyncExternalStore } from "react";

const noopUnsubscribe = () => undefined;

/**
 * Subscribes to a CSS media query so a component can branch on viewport
 * width the same way the stylesheet does (`sm`, `md` breakpoints). Backed by
 * `useSyncExternalStore`, so the first render already reflects the real
 * viewport instead of flashing the wrong branch. Without a `window` (SSR,
 * server snapshot) the query is reported as not matching.
 */
export function useMediaQuery(query: string): boolean {
  const store = useMemo(
    () => ({
      subscribe: (onChange: () => void) => {
        if (typeof window === "undefined") return noopUnsubscribe;
        const list = window.matchMedia(query);
        list.addEventListener("change", onChange);
        return () => list.removeEventListener("change", onChange);
      },
      getSnapshot: () => (typeof window === "undefined" ? false : window.matchMedia(query).matches),
    }),
    [query],
  );
  return useSyncExternalStore(store.subscribe, store.getSnapshot, () => false);
}
