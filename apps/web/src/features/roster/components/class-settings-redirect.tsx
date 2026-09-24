import { Navigate, useParams } from "react-router";

/** Old class-settings bookmarks open the detail's edit dialog in place. */
export function ClassSettingsRedirect() {
  const { id } = useParams<{ id: string }>();
  return <Navigate to={`/classes/${id ?? ""}?edit=1`} replace />;
}
