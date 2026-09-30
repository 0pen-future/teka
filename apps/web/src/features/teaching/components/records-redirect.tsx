import { Navigate, useParams, useSearchParams } from "react-router";

/** Old `/records` bookmarks open the students page, keeping the class filter and search. */
export function RecordsRedirect() {
  const [searchParams] = useSearchParams();
  const kept = new URLSearchParams();
  for (const key of ["class_id", "q"]) {
    const value = searchParams.get(key);
    if (value) kept.set(key, value);
  }
  const qs = kept.toString();
  return <Navigate to={`/students${qs ? `?${qs}` : ""}`} replace />;
}

/** Old `/records/:studentId` bookmarks open the same student's detail page. */
export function StudentRecordRedirect() {
  const { studentId } = useParams();
  return <Navigate to={`/students/${studentId ?? ""}`} replace />;
}
