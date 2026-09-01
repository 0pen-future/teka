import { useState } from "react";
import { Outlet, useMatches, useNavigate, useSearchParams } from "react-router";

import { HvCard } from "@/components/hv";
import { cn } from "@/lib/utils";
import {
  ClassSearchEmptyNote,
  ClassSearchInput,
  useClassesList,
  useClassSearch,
} from "@/features/roster";

import { MonthCalendarModal } from "../components/month-calendar-modal";
import { SessionListItem } from "../components/session-list-item";
import { SessionTrioPicker } from "../components/session-trio-picker";
import { useSessionsList } from "../hooks/use-sessions";
import { addDaysIso, bySessionOrder, monthOf, resolveAnchor, todayIso } from "../lib/session-dates";
import type { Session } from "../schemas/attendance-schemas";

/**
 * Half-width of the session window queried around the anchor. Wide enough
 * that even a once-a-week class keeps a prev and next session in range.
 */
const WINDOW_RADIUS_DAYS = 45;

/**
 * Finds the `:id` param of the `sessions/:id/attendance` child route from an
 * ancestor. `useParams()` only sees the current route's own params, not a
 * descendant's — `useMatches()` walks every matched route in the tree, so
 * this is the only way `SessionsPage` can tell whether a panel is open.
 */
function useSelectedSessionId(): string | undefined {
  const matches = useMatches();
  for (const match of matches) {
    const params = match.params as Record<string, string | undefined>;
    if (params.id) {
      return params.id;
    }
  }
  return undefined;
}

/**
 * `/sessions` — grouped session list plus the attendance panel. One route
 * tree serves two layouts: side-by-side at `lg+` (list always visible, panel
 * rendered through `<Outlet/>`) and a standalone panel under `lg` (list
 * hidden while a session is open), per the Responsive Strategy.
 */
export function SessionsPage() {
  const selectedId = useSelectedSessionId();
  const navigate = useNavigate();
  // `null` means "no explicit choice yet" — falls back to the first active
  // class below, without needing an effect to seed it once classes load.
  const [explicitClassId, setExplicitClassId] = useState<string | null>(null);
  // The query window is centered here, not on the anchor: recentering happens
  // only in navigation handlers (arrow/calendar taps at a window edge), never
  // during render, so a widening window can't feed back into itself.
  const [windowCenter, setWindowCenter] = useState(todayIso);
  const [calendarOpen, setCalendarOpen] = useState(false);

  const { data: classesPage } = useClassesList({ status: "active", per_page: 100 });
  const classes = classesPage?.items ?? [];
  // Search narrows which pills render; selection still falls back to the
  // full list so filtering can never change the selected class.
  const classSearch = useClassSearch(classes);
  // Read-only deep link from the dashboard's class cards (`?class_id=`);
  // an id not in the active list (stale link, other teacher) is ignored so
  // the sessions query never fires against a class this account can't read.
  const [searchParams] = useSearchParams();
  const urlClassId = searchParams.get("class_id");
  const linkedClassId =
    urlClassId && classes.some((cls) => cls.id === urlClassId) ? urlClassId : null;
  const selectedClassId = explicitClassId ?? linkedClassId ?? classes[0]?.id ?? null;

  const { data: sessions, isPending } = useSessionsList(selectedClassId ?? undefined, {
    from: addDaysIso(windowCenter, -WINDOW_RADIUS_DAYS),
    to: addDaysIso(windowCenter, WINDOW_RADIUS_DAYS),
  });
  const allSessions = sessions ?? [];
  const todayStr = todayIso();

  const unconfirmedPast = allSessions
    .filter(
      (session) =>
        session.status !== "cancelled" &&
        !session.attendance_confirmed_at &&
        session.session_date < todayStr,
    )
    .sort((a, b) => (a.session_date < b.session_date ? -1 : 1));

  const sorted = [...allSessions].sort(bySessionOrder);
  const anchor = resolveAnchor(sorted, selectedId, todayStr);
  const anchorIndex = anchor ? sorted.findIndex((session) => session.id === anchor.id) : -1;
  const prev = anchorIndex > 0 ? sorted[anchorIndex - 1]! : null;
  const next =
    anchorIndex >= 0 && anchorIndex < sorted.length - 1 ? sorted[anchorIndex + 1]! : null;

  const goToSession = (session: Session) => {
    // Landing on the window's edge session means its own neighbor may lie
    // outside the current query — recenter so the next render can see it.
    if (session.id === sorted[0]?.id || session.id === sorted.at(-1)?.id) {
      setWindowCenter(session.session_date);
    }
    void navigate(`/sessions/${session.id}/attendance`);
  };

  const hasSelection = Boolean(selectedId);

  return (
    <div className="flex flex-col gap-4">
      {/* Prototype places the header and the class picker above the
          list/panel row at full page width, so the pills get the whole
          line instead of wrapping inside the 360px list column. Under lg
          the open panel takes over the screen, so this block hides with
          the list. */}
      <div className={cn("flex-col gap-4", hasSelection ? "hidden lg:flex" : "flex")}>
        <div>
          <h1 className="font-display text-[26px] font-extrabold text-ink-900">Điểm danh</h1>
          <p className="mt-1 text-[14px] text-ink-500">
            Mặc định cả lớp đúng giờ — chỉ chạm vào bạn muộn hoặc vắng, rồi xác nhận.
          </p>
        </div>

        {classes.length === 0 ? (
          <HvCard variant="flat" className="text-center text-[13px] text-ink-400">
            Chưa có lớp nào đang hoạt động. Tạo một lớp trước khi điểm danh.
          </HvCard>
        ) : (
          <div className="flex flex-col gap-2">
            <h2 className="text-[12px] font-extrabold tracking-[var(--tracking-wide)] text-ink-400">
              CHỌN LỚP
            </h2>
            <div className="flex flex-wrap items-center gap-2">
              {classSearch.showSearch ? (
                <ClassSearchInput value={classSearch.query} onChange={classSearch.setQuery} />
              ) : null}
              {/* A tablist must own at least one tab, so the container leaves
                  the tree entirely when the filter matches nothing.
                  `contents` dissolves its box so each tab wraps individually
                  in the row shared with the search pill and empty note —
                  otherwise the whole tab strip drops to its own line. */}
              {classSearch.filtered.length > 0 ? (
                <div role="tablist" aria-label="Lớp" className="contents">
                  {classSearch.filtered.map((klass) => (
                    <button
                      key={klass.id}
                      type="button"
                      role="tab"
                      aria-selected={selectedClassId === klass.id}
                      onClick={() => setExplicitClassId(klass.id)}
                      className={cn(
                        // The shadow utilities override the base :focus-visible
                        // box-shadow ring, so the ring must be re-added explicitly
                        // (same trap HvButton guards against).
                        "min-h-11 rounded-full px-[18px] font-display text-[14px] font-extrabold transition-[background-color,color,box-shadow] focus-visible:outline-none focus-visible:ring-4",
                        selectedClassId === klass.id
                          ? "bg-mint-400 text-white shadow-press-mint"
                          : "bg-white text-ink-500 shadow-soft-sm hover:bg-cream-100",
                      )}
                    >
                      {klass.name}
                    </button>
                  ))}
                </div>
              ) : null}
              {classSearch.emptyNote ? <ClassSearchEmptyNote note={classSearch.emptyNote} /> : null}
            </div>
          </div>
        )}
      </div>

      <div className="flex flex-col gap-4 lg:flex-row lg:items-start">
        <div
          className={cn(
            "flex flex-col gap-4",
            hasSelection
              ? "hidden lg:flex lg:w-[360px] lg:shrink-0"
              : "flex lg:w-[360px] lg:shrink-0",
          )}
        >
          {isPending ? <p className="text-[13px] text-ink-400">Đang tải…</p> : null}

          {!isPending && selectedClassId && allSessions.length === 0 ? (
            <HvCard variant="flat" className="text-center text-[13px] text-ink-400">
              Không có buổi học nào quanh thời điểm này.
            </HvCard>
          ) : null}

          {allSessions.length > 0 ? (
            <div className="flex flex-col gap-3 rounded-[20px] bg-white p-[14px] shadow-soft-md">
              <SessionTrioPicker
                prev={prev}
                anchor={anchor}
                next={next}
                today={todayStr}
                onNavigate={goToSession}
              />
              <button
                type="button"
                onClick={() => setCalendarOpen(true)}
                className="min-h-11 self-start rounded-full bg-white px-[18px] font-display text-[13px] font-extrabold text-ink-500 shadow-soft-sm transition-colors hover:bg-cream-100 focus-visible:outline-none focus-visible:ring-4"
              >
                Mở lịch tháng
              </button>
            </div>
          ) : null}

          {/* Quick entry into overdue sessions, kept from the previous list
            layout — the trio only ever shows three cards at a time. */}
          {unconfirmedPast.length > 0 ? (
            <div className="flex flex-col gap-1 rounded-[20px] bg-white p-[14px] shadow-soft-md">
              <h2 className="px-2 py-1 text-[12.5px] font-extrabold uppercase tracking-[0.4px] text-coral-600">
                Cần điểm danh
              </h2>
              <div className="flex flex-col gap-[6px]">
                {unconfirmedPast.map((session) => (
                  <SessionListItem
                    key={session.id}
                    session={session}
                    unconfirmedPast
                    selected={session.id === selectedId}
                  />
                ))}
              </div>
            </div>
          ) : null}
        </div>

        <div
          className={cn(
            "flex-1",
            hasSelection
              ? "flex flex-col"
              : "hidden lg:flex lg:min-h-[240px] lg:items-center lg:justify-center",
          )}
        >
          {hasSelection ? (
            <Outlet />
          ) : (
            <p className="text-[13px] text-ink-400">Chọn một buổi học để điểm danh.</p>
          )}
        </div>
      </div>

      {/* Mounted only while open so each opening starts fresh on the anchor's
          month instead of wherever the last browse ended. */}
      {calendarOpen && selectedClassId ? (
        <MonthCalendarModal
          open
          onOpenChange={setCalendarOpen}
          classId={selectedClassId}
          initialMonth={monthOf(anchor?.session_date ?? windowCenter)}
          today={todayStr}
          onPickSession={(session) => {
            setCalendarOpen(false);
            setWindowCenter(session.session_date);
            void navigate(`/sessions/${session.id}/attendance`);
          }}
        />
      ) : null}
    </div>
  );
}
