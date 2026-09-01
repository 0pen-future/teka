import { useCallback, useMemo, useRef, useState } from "react";
import { useBlocker, useNavigate, useParams } from "react-router";

import { HvButton, HvModal, hvToast } from "@/components/hv";
import { canRecordAttendance, canWriteClass, useClass } from "@/features/roster";
import { useCenterContext } from "@/features/teaching";
import { formatSessionDate } from "@/lib/utils";

import { AttendanceRow } from "../components/attendance-row";
import {
  ATTENDANCE_STATUSES,
  type AttendanceMarkStatus,
  type AttendanceStatus,
} from "../components/attendance-status-meta";
import { AttendanceTableHeader } from "../components/attendance-table-header";
import { CancelSessionDialog } from "../components/cancel-session-dialog";
import { ClosedPeriodWarning } from "../components/closed-period-warning";
import { ConfirmAttendanceBar } from "../components/confirm-attendance-bar";
import { StatusCountChips } from "../components/status-count-chips";
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

/**
 * One student's local exception from the all-present default. `note` is kept
 * as a plain string (`""` = none) so the excused input stays controlled.
 */
interface LocalMark {
  status: AttendanceMarkStatus;
  note: string;
}

function marksFromRows(rows: AttendanceRowData[]): Map<string, LocalMark> {
  const marks = new Map<string, LocalMark>();
  for (const row of rows) {
    if (row.status === "late" || row.status === "absent" || row.status === "excused") {
      marks.set(row.student_id, { status: row.status, note: row.note ?? "" });
    }
  }
  return marks;
}

function marksEqual(a: Map<string, LocalMark>, b: Map<string, LocalMark>): boolean {
  if (a.size !== b.size) {
    return false;
  }
  for (const [studentId, mark] of a) {
    const other = b.get(studentId);
    if (other?.status !== mark.status || other.note.trim() !== mark.note.trim()) {
      return false;
    }
  }
  return true;
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

/**
 * The one-touch điểm danh screen (PRD North Star G4), now the 4-column sheet
 * (Đúng giờ / Muộn / Vắng / Có lý do). Everyone defaults to Đúng giờ; only
 * exceptions are tapped, purely client-side (`Map<studentId, LocalMark>`, no
 * network per row). "Xác nhận" is the single request that writes the whole
 * roster server-side — and every status stays billable.
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
  const { data: klass } = useClass(session?.class_id);
  const { isOwner } = useCenterContext();
  // Defaults closed while the class is still loading so hoc_vu/tro_giang
  // staff never see write controls flash enabled before the role check lands;
  // accessResolved keeps the denial label from flashing at real writers in
  // the meantime (the notifications page's accessResolved precedent).
  const accessResolved = Boolean(klass);
  const canWrite = klass ? canRecordAttendance(isOwner, klass) : false;
  // Cancelling a session is a lifecycle write (sessions.write): giao_vien or
  // owner only — narrower than the attendance confirm the trợ giảng holds.
  const canCancel = klass ? canWriteClass(isOwner, klass) : false;

  const [marks, setMarks] = useState<Map<string, LocalMark>>(new Map());
  const [baselineMarks, setBaselineMarks] = useState<Map<string, LocalMark> | null>(null);
  const [cancelDialogOpen, setCancelDialogOpen] = useState(false);
  // Tracks which session's roster the local `marks`/`baselineMarks` state
  // was seeded from, so re-seeding only happens once per session (not on
  // every roster refetch, which would wipe an in-flight edit). Adjusting
  // state during render (React's documented pattern for "state that depends
  // on a changed prop") avoids an extra render pass a `useEffect` would add.
  const [seededForSessionId, setSeededForSessionId] = useState<string | undefined>(undefined);
  if (roster && seededForSessionId !== sessionId) {
    const seeded = marksFromRows(roster.rows);
    setSeededForSessionId(sessionId);
    setMarks(seeded);
    setBaselineMarks(seeded);
  }

  const dirty = baselineMarks !== null && !marksEqual(marks, baselineMarks);
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

  function selectStatus(studentId: string, status: AttendanceStatus) {
    // Read-only viewers (hoc_vu, a handed-off teacher) must not accumulate
    // local edits they can never save — a dirty sheet would trap them in the
    // unsaved-changes blocker on the way out.
    if (!canWrite) {
      return;
    }
    setMarks((prev) => {
      const next = new Map(prev);
      const current = next.get(studentId);
      // Tapping Đúng giờ — or the already-selected exception — clears the
      // exception back to the default.
      if (status === "present" || current?.status === status) {
        next.delete(studentId);
      } else {
        // A status switch resets the note: the input only shows for excused,
        // and what the teacher sees must be exactly what gets sent.
        next.set(studentId, { status, note: "" });
      }
      return next;
    });
  }

  function setMarkNote(studentId: string, note: string) {
    setMarks((prev) => {
      const current = prev.get(studentId);
      if (!current) {
        return prev;
      }
      const next = new Map(prev);
      next.set(studentId, { ...current, note });
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
      {
        marks: Array.from(marks, ([studentId, mark]) => ({
          student_id: studentId,
          status: mark.status,
          ...(mark.note.trim() ? { note: mark.note.trim() } : {}),
        })),
      },
      {
        onSuccess: (response) => {
          const savedMarks = marksFromRows(response.rows);
          setMarks(savedMarks);
          setBaselineMarks(savedMarks);
          const savedCounts = countByStatus(response.rows.length, savedMarks);
          const parts = ATTENDANCE_STATUSES.filter(
            (status) => status.value === "present" || savedCounts[status.value] > 0,
          ).map((status) => `${savedCounts[status.value]} ${status.label.toLowerCase()}`);
          hvToast(`Đã điểm danh ${parts.join(", ")}`);
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

  const counts = countByStatus(roster.rows.length, marks);
  const closedPeriod = period?.status === "closed";
  const confirmed = Boolean(session.attendance_confirmed_at);

  return (
    <div className="flex flex-col rounded-[28px] bg-white shadow-soft-lg">
      <PanelHeader
        session={session}
        subtitle={
          confirmed
            ? "Đã xác nhận — chạm để sửa lại"
            : "Buổi chưa điểm danh — mặc định cả lớp đúng giờ"
        }
      />

      <div className="flex items-center gap-[10px] border-b border-line-100 px-4 py-3">
        <StatusCountChips counts={counts} />
        {confirmed ? (
          <span className="ml-auto shrink-0 text-[12.5px] text-ink-400">
            Sửa được cả buổi đã qua
          </span>
        ) : null}
      </div>

      <div className="flex flex-col gap-3 px-3 pt-3">
        {closedPeriod ? <ClosedPeriodWarning /> : null}
        {canCancel ? (
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
        ) : null}
      </div>

      {/* Prototype list viewport, two-pane only: at lg+ rows scroll inside
          430px so the confirm bar never leaves reach. Under lg the document
          already scrolls and the confirm bar is sticky — a nested scroller
          there would just fight touch scrolling. The column header sticks to
          the top of whichever scroll context applies. */}
      <div className="flex flex-col gap-[6px] px-3 py-[10px] lg:max-h-[430px] lg:overflow-auto">
        <AttendanceTableHeader />
        {roster.rows.map((row) => {
          const mark = marks.get(row.student_id);
          return (
            <AttendanceRow
              key={row.student_id}
              row={row}
              status={mark?.status ?? "present"}
              note={mark?.note ?? ""}
              duplicateName={(duplicateNames.get(row.student_name) ?? 0) > 1}
              canWrite={canWrite}
              onSelect={selectStatus}
              onNoteChange={setMarkNote}
            />
          );
        })}
      </div>

      <ConfirmAttendanceBar
        absentCount={counts.absent}
        lateCount={counts.late}
        pending={saveMutation.isPending}
        closedPeriod={closedPeriod}
        settled={settled}
        canWrite={canWrite}
        accessResolved={accessResolved}
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

function countByStatus(
  rosterSize: number,
  marks: Map<string, LocalMark>,
): Record<AttendanceStatus, number> {
  const counts: Record<AttendanceStatus, number> = {
    present: rosterSize - marks.size,
    late: 0,
    absent: 0,
    excused: 0,
  };
  for (const mark of marks.values()) {
    counts[mark.status] += 1;
  }
  return counts;
}
