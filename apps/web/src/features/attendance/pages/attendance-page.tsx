import { useCallback, useMemo, useRef, useState } from "react";
import { useBlocker, useNavigate, useParams } from "react-router";

import { HvButton, HvModal, hvToast } from "@/components/hv";
import { formatSessionDate } from "@/lib/utils";

import { AttendanceRow } from "../components/attendance-row";
import { CancelSessionDialog } from "../components/cancel-session-dialog";
import { ClosedPeriodWarning } from "../components/closed-period-warning";
import { ConfirmAttendanceBar } from "../components/confirm-attendance-bar";
import {
  useSaveAttendance,
  useSession,
  useSessionRoster,
  usePeriodForDate,
} from "../hooks/use-sessions";
import type { AttendanceRow as AttendanceRowData } from "../schemas/attendance-schemas";

export interface AttendancePageProps {
  /**
   * Set when mounted as the two-pane panel inside `SessionsPage` (`lg+`);
   * falls back to the route param when mounted standalone at
   * `/sessions/:id/attendance` under `lg`. Same component, two mounts.
   */
  sessionId?: string;
}

function absentIdsFromRows(rows: AttendanceRowData[]): Set<string> {
  return new Set(rows.filter((row) => row.status === "absent").map((row) => row.student_id));
}

/** Mint band atop the panel — shared by the live and cancelled branches. */
function PanelHeader({
  session,
  subtitle,
}: {
  session: { class_name: string; session_date: string; start_time?: string | null };
  subtitle?: string;
}) {
  return (
    <div className="rounded-t-[28px] bg-mint-400 px-5 py-4 text-white">
      <p className="font-display text-[19px] font-bold">
        {session.class_name} — {formatSessionDate(session.session_date)}
        {session.start_time ? ` · ${session.start_time}` : ""}
      </p>
      {subtitle ? <p className="text-[13px] opacity-90">{subtitle}</p> : null}
    </div>
  );
}

function setsEqual(a: Set<string>, b: Set<string>): boolean {
  if (a.size !== b.size) {
    return false;
  }
  for (const value of a) {
    if (!b.has(value)) {
      return false;
    }
  }
  return true;
}

/**
 * The one-touch điểm danh screen (PRD North Star G4). Everyone defaults to
 * present; only absentees are tapped, purely client-side (`Set<string>`, no
 * network per row). "Xác nhận" is the single request that writes the whole
 * roster server-side.
 */
export function AttendancePage({ sessionId: sessionIdProp }: AttendancePageProps) {
  const params = useParams<{ id: string }>();
  const sessionId = sessionIdProp ?? params.id;
  const navigate = useNavigate();

  const { data: session, isPending: sessionPending, isError: sessionError } = useSession(sessionId);
  const {
    data: roster,
    isPending: rosterPending,
    isError: rosterError,
  } = useSessionRoster(sessionId);
  const { data: period } = usePeriodForDate(session?.session_date);
  const saveMutation = useSaveAttendance(sessionId ?? "");

  const [absentIds, setAbsentIds] = useState<Set<string>>(new Set());
  const [baselineIds, setBaselineIds] = useState<Set<string> | null>(null);
  const [cancelDialogOpen, setCancelDialogOpen] = useState(false);
  // Tracks which session's roster the local `absentIds`/`baselineIds` state
  // was seeded from, so re-seeding only happens once per session (not on
  // every roster refetch, which would wipe an in-flight edit). Adjusting
  // state during render (React's documented pattern for "state that depends
  // on a changed prop") avoids an extra render pass a `useEffect` would add.
  const [seededForSessionId, setSeededForSessionId] = useState<string | undefined>(undefined);
  if (roster && seededForSessionId !== sessionId) {
    const seeded = absentIdsFromRows(roster.rows);
    setSeededForSessionId(sessionId);
    setAbsentIds(seeded);
    setBaselineIds(seeded);
  }

  const dirty = baselineIds !== null && !setsEqual(absentIds, baselineIds);
  // Confirmed server-side with no local edits — the single source of truth
  // for both the button label and the confirm handler's no-op branch.
  const settled = Boolean(session?.attendance_confirmed_at) && !dirty;
  // A successful save navigates away in the same callback that resets the
  // baseline; React batches those state writes, so at navigate() time the
  // committed render still reads `dirty === true`. Without this ref the
  // dirty-guard blocker would intercept its own post-save navigation and
  // strand the teacher on the attendance screen. The ref flips synchronously
  // before navigate(), so the blocker function sees the save is done.
  const savedRef = useRef(false);
  const blocker = useBlocker(useCallback(() => dirty && !savedRef.current, [dirty]));

  const duplicateNames = useMemo(() => {
    const counts = new Map<string, number>();
    for (const row of roster?.rows ?? []) {
      counts.set(row.student_name, (counts.get(row.student_name) ?? 0) + 1);
    }
    return counts;
  }, [roster]);

  function toggleAbsent(studentId: string) {
    setAbsentIds((prev) => {
      const next = new Set(prev);
      if (next.has(studentId)) {
        next.delete(studentId);
      } else {
        next.add(studentId);
      }
      return next;
    });
  }

  function handleConfirm() {
    // Prototype behavior: a confirmed session with no edits only reports its
    // state — no redundant roster write, no navigation.
    if (settled) {
      hvToast("Buổi này đã xác nhận rồi");
      return;
    }
    saveMutation.mutate(
      { absent_student_ids: Array.from(absentIds) },
      {
        onSuccess: (response) => {
          const savedAbsentIds = absentIdsFromRows(response.rows);
          setAbsentIds(savedAbsentIds);
          setBaselineIds(savedAbsentIds);
          const presentCount = response.rows.length - savedAbsentIds.size;
          hvToast(`Đã điểm danh ${presentCount} có mặt, ${savedAbsentIds.size} vắng`);
          if (response.warning) {
            hvToast(response.warning, { variant: "danger", duration: 6000 });
          }
          savedRef.current = true;
          void navigate("/sessions");
        },
      },
    );
  }

  if (!sessionId) {
    return null;
  }

  if (sessionPending || rosterPending) {
    return <p className="p-4 text-[13px] text-ink-400">Đang tải…</p>;
  }

  if (sessionError || rosterError || !session || !roster) {
    return (
      <p className="p-4 text-[14px] font-semibold text-coral-600">Không tải được buổi học này.</p>
    );
  }

  if (session.status === "cancelled") {
    return (
      <div className="flex flex-col rounded-[28px] bg-white shadow-soft-lg">
        <PanelHeader session={session} />
        <div className="flex flex-col items-center gap-1 px-5 py-[26px] text-center text-[14.5px] text-ink-500">
          <span aria-hidden className="text-[30px]">
            🚫
          </span>
          <p className="font-bold text-ink-900">Buổi học đã huỷ</p>
          {session.cancel_reason ? <p>{session.cancel_reason}</p> : null}
          <p className="mt-1 font-bold text-mint-600">Không tính tiền cho học sinh nào.</p>
        </div>
      </div>
    );
  }

  const presentCount = roster.rows.length - absentIds.size;
  const closedPeriod = period?.status === "closed";
  const confirmed = Boolean(session.attendance_confirmed_at);

  return (
    <div className="flex flex-col rounded-[28px] bg-white shadow-soft-lg">
      <PanelHeader
        session={session}
        subtitle={
          confirmed
            ? "Đã xác nhận — chạm để sửa lại"
            : "Buổi chưa điểm danh — mặc định cả lớp có mặt"
        }
      />

      <div className="flex items-center gap-[10px] border-b border-line-100 px-4 py-3">
        <span className="rounded-full bg-mint-50 px-3 py-[5px] text-[13px] font-extrabold text-mint-600">
          Có mặt {presentCount}
        </span>
        <span className="rounded-full bg-coral-100 px-3 py-[5px] text-[13px] font-extrabold text-coral-600">
          Vắng {absentIds.size}
        </span>
        {confirmed ? (
          <span className="ml-auto text-[12.5px] text-ink-400">Sửa được cả buổi đã qua</span>
        ) : null}
      </div>

      <div className="flex flex-col gap-3 px-3 pt-3">
        {closedPeriod ? <ClosedPeriodWarning /> : null}
        <div className="flex justify-end">
          <HvButton
            type="button"
            variant="ghost"
            size="sm"
            onClick={() => setCancelDialogOpen(true)}
          >
            Huỷ buổi học
          </HvButton>
        </div>
      </div>

      {/* Prototype list viewport, two-pane only: at lg+ rows scroll inside
          430px so the confirm bar never leaves reach. Under lg the document
          already scrolls and the confirm bar is sticky — a nested scroller
          there would just fight touch scrolling. */}
      <div className="flex flex-col gap-[6px] px-3 py-[10px] lg:max-h-[430px] lg:overflow-auto">
        {roster.rows.map((row) => (
          <AttendanceRow
            key={row.student_id}
            row={row}
            absent={absentIds.has(row.student_id)}
            duplicateName={(duplicateNames.get(row.student_name) ?? 0) > 1}
            onToggle={toggleAbsent}
          />
        ))}
      </div>

      <ConfirmAttendanceBar
        absentCount={absentIds.size}
        pending={saveMutation.isPending}
        closedPeriod={closedPeriod}
        settled={settled}
        onConfirm={handleConfirm}
      />

      <CancelSessionDialog
        open={cancelDialogOpen}
        onOpenChange={setCancelDialogOpen}
        sessionId={sessionId}
        onCancelled={() => void navigate("/sessions")}
      />

      <HvModal
        open={blocker.state === "blocked"}
        onOpenChange={(open) => {
          if (!open && blocker.state === "blocked") {
            blocker.reset();
          }
        }}
        title="Chưa lưu điểm danh"
        footer={
          <>
            <HvButton
              type="button"
              variant="ghost"
              onClick={() => blocker.state === "blocked" && blocker.reset()}
            >
              Ở lại
            </HvButton>
            <HvButton
              type="button"
              variant="danger"
              onClick={() => blocker.state === "blocked" && blocker.proceed()}
            >
              Rời khỏi trang
            </HvButton>
          </>
        }
      >
        <p className="text-[14px] text-ink-600">
          Điểm danh chưa được lưu. Nếu rời khỏi trang bây giờ, các thay đổi sẽ mất.
        </p>
      </HvModal>
    </div>
  );
}
