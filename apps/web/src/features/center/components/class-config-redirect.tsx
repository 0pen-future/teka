import { Navigate } from "react-router";

/** Old class-config bookmarks still open now that score sets live on program templates. */
export function ClassConfigRedirect() {
  return <Navigate to="/center" replace />;
}
