import { useEffect } from "react";
import { useMatches } from "react-router";

/** Route `handle` shape: `title` names the page in the browser tab. */
export interface RouteHandle {
  title?: string;
}

const APP_NAME = "Teka";

/**
 * Sets `document.title` to "<Page> · Teka" from the deepest matched route's
 * `handle.title`, so every title lives next to its route declaration. Titles
 * stay static per page: names and amounts would leak into browser history and
 * shared-screen tab bars.
 */
export function RouteDocumentTitle() {
  const matches = useMatches();
  const title = [...matches]
    .reverse()
    .map((match) => (match.handle as RouteHandle | undefined)?.title)
    .find(Boolean);

  useEffect(() => {
    document.title = title ? `${title} · ${APP_NAME}` : APP_NAME;
  }, [title]);

  return null;
}
