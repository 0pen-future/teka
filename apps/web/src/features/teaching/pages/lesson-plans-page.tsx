import { Navigate } from "react-router";

import { useCenterContext } from "../hooks/use-center-context";

/**
 * Duyệt giáo án — owner-only review queue, filled out in phase 6. Nav hiding
 * alone is not a guard: non-owners landing here (deep link, stale bookmark)
 * are routed back to the classbook once the role resolves.
 */
export function LessonPlansPage() {
  const { isOwner, isResolved, isError } = useCenterContext();

  if (!isResolved && !isError) {
    return null;
  }
  // Unresolvable role (query failed) degrades like non-owner — a redirect,
  // never a permanently blank page.
  if (!isOwner) {
    return <Navigate to="/classbook" replace />;
  }
  return (
    <header>
      <h1 className="font-display text-[26px] font-extrabold text-ink-900">Duyệt giáo án</h1>
      <p className="mt-1 text-[13.5px] text-ink-500">
        Giáo án giáo viên đã gửi, chờ duyệt hoặc cần chỉnh sửa.
      </p>
    </header>
  );
}
