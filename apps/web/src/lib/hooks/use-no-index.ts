import { useEffect } from "react";

/**
 * Appends `<meta name="robots" content="noindex, nofollow">` to
 * `document.head` while the calling component is mounted, and removes it on
 * unmount. The app is a single client-rendered `index.html`
 * (`apps/web/index.html`), so a static tag would apply to every route; this
 * keeps the rest of the app indexable while an unguessable parent-statement
 * link stays out of search results.
 *
 * Each mount creates and owns its own `<meta>` element, so a React strict
 * mode double-invoke (mount → cleanup → mount) leaves exactly one tag behind,
 * not zero or two.
 */
export function useNoIndex(): void {
  useEffect(() => {
    const meta = document.createElement("meta");
    meta.name = "robots";
    meta.content = "noindex, nofollow";
    document.head.appendChild(meta);
    return () => {
      document.head.removeChild(meta);
    };
  }, []);
}
