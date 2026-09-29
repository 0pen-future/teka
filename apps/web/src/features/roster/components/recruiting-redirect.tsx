import { Navigate, useSearchParams } from "react-router";

/**
 * Old `/classes/recruiting` bookmarks open the catalog on its "Cần tuyển sinh"
 * chip, keeping the search and schedule filters. The old page's own chip and
 * phase params are dropped: the catalog holds a single view.
 */
export function RecruitingRedirect() {
  const [searchParams] = useSearchParams();
  const params = new URLSearchParams({ view: "recruiting" });
  for (const key of ["q", "weekday", "shift"]) {
    const value = searchParams.get(key);
    if (value) {
      params.set(key, value);
    }
  }
  return <Navigate to={`/classes?${params.toString()}`} replace />;
}
