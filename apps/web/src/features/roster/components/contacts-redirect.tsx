import { Navigate, useSearchParams } from "react-router";

/** Old `/contacts` bookmarks open the contacts tab of the students page, keeping the search. */
export function ContactsRedirect() {
  const [searchParams] = useSearchParams();
  const q = searchParams.get("q");
  return (
    <Navigate to={`/students?tab=contacts${q ? `&q=${encodeURIComponent(q)}` : ""}`} replace />
  );
}
