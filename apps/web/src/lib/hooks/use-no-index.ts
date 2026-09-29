import { useEffect } from "react";

/**
 * Appends `<meta name="robots" content="noindex, nofollow">` to
 * `document.head` while the calling component is mounted, and removes it on
 * unmount. `index.html` and nginx's `X-Robots-Tag` already keep the whole app
 * out of search results; token pages (reset, invite) add this tag as well so
 * they stay unindexed even if that site-wide policy is ever relaxed.
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
