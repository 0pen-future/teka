import { useState } from "react";
import { Outlet, useMatches } from "react-router";

import { HvButton, HvCard } from "@/components/hv";
import { cn, formatSessionDate } from "@/lib/utils";
import { useClassesList } from "@/features/roster";

import { CreateSessionDialog } from "../components/create-session-dialog";
import { SessionListItem } from "../components/session-list-item";
import { useSessionsList } from "../hooks/use-sessions";
import type { Session } from "../schemas/attendance-schemas";

function today(): string {
  return new Date().toISOString().slice(0, 10);
}

function daysAgo(count: number): string {
  const date = new Date();
  date.setDate(date.getDate() - count);
  return date.toISOString().slice(0, 10);
}

function groupByDateDescending(sessions: Session[]): [string, Session[]][] {
  const map = new Map<string, Session[]>();
  for (const session of sessions) {
    const bucket = map.get(session.session_date) ?? [];
    bucket.push(session);
    map.set(session.session_date, bucket);
  }
  return Array.from(map.entries()).sort(([a], [b]) => (a < b ? 1 : a > b ? -1 : 0));
}

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
  // `null` means "no explicit choice yet" — falls back to the first active
  // class below, without needing an effect to seed it once classes load.
  const [explicitClassId, setExplicitClassId] = useState<string | null>(null);
  const [range, setRange] = useState(() => ({ from: daysAgo(13), to: today() }));
  const [createDialogOpen, setCreateDialogOpen] = useState(false);

  const { data: classesPage } = useClassesList({ status: "active", per_page: 100 });
  const classes = classesPage?.items ?? [];
  const selectedClassId = explicitClassId ?? classes[0]?.id ?? null;

  const { data: sessions, isPending } = useSessionsList(selectedClassId ?? undefined, range);
  const allSessions = sessions ?? [];
  const todayStr = today();

  const unconfirmedPast = allSessions
    .filter(
      (session) =>
        session.status !== "cancelled" &&
        !session.attendance_confirmed_at &&
        session.session_date < todayStr,
    )
    .sort((a, b) => (a.session_date < b.session_date ? -1 : 1));
  const unconfirmedPastIds = new Set(unconfirmedPast.map((session) => session.id));
  const remainingGroups = groupByDateDescending(
    allSessions.filter((session) => !unconfirmedPastIds.has(session.id)),
  );

  const hasSelection = Boolean(selectedId);

  return (
    <div className="flex flex-col gap-4 lg:flex-row lg:items-start">
      <div
        className={cn(
          "flex flex-col gap-4",
          hasSelection
            ? "hidden lg:flex lg:w-[360px] lg:shrink-0"
            : "flex lg:w-[360px] lg:shrink-0",
        )}
      >
        <div>
          <div className="flex flex-wrap items-center gap-3">
            <h1 className="flex-1 font-display text-[26px] font-extrabold text-ink-900">
              Điểm danh
            </h1>
            <HvButton
              size="sm"
              disabled={!selectedClassId}
              onClick={() => setCreateDialogOpen(true)}
            >
              Thêm buổi học
            </HvButton>
          </div>
          <p className="mt-1 text-[13.5px] text-ink-500">
            Mặc định cả lớp có mặt — chỉ chạm vào bạn vắng, rồi xác nhận.
          </p>
        </div>

        {classes.length === 0 ? (
          <HvCard variant="flat" className="text-center text-[13px] text-ink-400">
            Chưa có lớp nào đang hoạt động. Tạo một lớp trước khi điểm danh.
          </HvCard>
        ) : (
          <div role="tablist" aria-label="Lớp" className="flex flex-wrap gap-2">
            {classes.map((klass) => (
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
        )}

        <div className="flex items-center gap-2 text-[13px] text-ink-500">
          <label className="flex items-center gap-1">
            Từ
            <input
              type="date"
              value={range.from}
              max={range.to}
              onChange={(event) => setRange((prev) => ({ ...prev, from: event.target.value }))}
              className="rounded-md border border-line-200 px-2 py-1 text-[13px]"
            />
          </label>
          <label className="flex items-center gap-1">
            Đến
            <input
              type="date"
              value={range.to}
              min={range.from}
              onChange={(event) => setRange((prev) => ({ ...prev, to: event.target.value }))}
              className="rounded-md border border-line-200 px-2 py-1 text-[13px]"
            />
          </label>
        </div>

        {isPending ? <p className="text-[13px] text-ink-400">Đang tải…</p> : null}

        {!isPending && selectedClassId && allSessions.length === 0 ? (
          <HvCard variant="flat" className="text-center text-[13px] text-ink-400">
            Không có buổi học nào trong khoảng thời gian này.
          </HvCard>
        ) : null}

        {/* Prototype session-list card: one white rounded-20 surface holding
            every group, section labels in the muted 12.5px/800 band style. */}
        {allSessions.length > 0 ? (
          <div className="flex flex-col gap-1 rounded-[20px] bg-white p-[14px] shadow-soft-md">
            {unconfirmedPast.length > 0 ? (
              <>
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
              </>
            ) : null}

            {remainingGroups.map(([date, group]) => (
              <div key={date} className="flex flex-col gap-[6px]">
                <h2 className="px-2 pt-2 text-[12.5px] font-extrabold uppercase tracking-[0.4px] text-ink-400">
                  {formatSessionDate(date)}
                </h2>
                {group.map((session) => (
                  <SessionListItem
                    key={session.id}
                    session={session}
                    unconfirmedPast={false}
                    selected={session.id === selectedId}
                  />
                ))}
              </div>
            ))}
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

      {selectedClassId ? (
        <CreateSessionDialog
          open={createDialogOpen}
          onOpenChange={setCreateDialogOpen}
          classId={selectedClassId}
        />
      ) : null}
    </div>
  );
}
