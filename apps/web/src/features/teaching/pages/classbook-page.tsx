import { useCallback, useRef, useState } from "react";
import { useNavigate, useSearchParams } from "react-router";

import {
  HvButton,
  HvIcon,
  HvSegmented,
  HvStateBlock,
  hvToast,
  type HvSegmentedOption,
} from "@/components/hv";
import { canWriteClass, useClassesList, useEnrollmentsList } from "@/features/roster";
import { formatSessionDate } from "@/lib/utils";

import { ClassKpiStrip, type ClassKpi } from "../components/class-kpi-strip";
import { ClassSelect } from "../components/class-select";
import { CourseView } from "../components/course-view";
import { MonthStepper } from "../components/month-stepper";
import { SessionExpandRow, type SessionExpandRowHandle } from "../components/session-expand-row";
import { SessionsTable } from "../components/sessions-table";
import { UnsavedScoresGuard } from "../components/unsaved-scores-guard";
import {
  activeHeadcount,
  classbookTotals,
  deriveSessions,
  formatLedgerScore,
  parseMonthParam,
  retentionStat,
  scoredStudentCount,
  todayIso,
} from "../lib/classbook-stats";
import { downloadCsv, type CsvCell } from "../lib/csv";
import { SESSION_COST_VND, type TeachingState } from "../lib/teaching-store";
import { useCenterContext } from "../hooks/use-center-context";
import { useClassMarks } from "../hooks/use-class-marks";
import { useClassTeaching } from "../hooks/use-class-teaching";
import { useClassScoreComponents } from "../hooks/use-component-scores";
import { useMonthSessions } from "../hooks/use-month-sessions";
import { useSessionScoreCounts } from "../hooks/use-session-score-counts";
import { vnd } from "../lib/vnd";

type ClassbookView = "sessions" | "course";

/** Any change that would unmount the open row waits on the unsaved-scores guard. */
type Navigation =
  | { kind: "session"; sessionId: string | null }
  | { kind: "class"; classId: string }
  | { kind: "month"; month: string }
  | { kind: "view"; view: ClassbookView };

const viewOptions: HvSegmentedOption<ClassbookView>[] = [
  { value: "sessions", label: "Buổi học", icon: <HvIcon name="table" size={16} /> },
  {
    value: "course",
    label: "Chương trình & giáo án",
    icon: <HvIcon name="file" size={16} />,
  },
];

/**
 * Quản lý lớp học — the teaching screen's default view: one toolbar (class,
 * month, view), a four-figure KPI strip, and the month's session ledger
 * with the open session expanded in place. Everything is server data
 * through React Query: sessions, rosters, and enrollments from their
 * features; scores, notes, and giáo án from the teaching API.
 */
export function ClassbookPage() {
  const [searchParams, setSearchParams] = useSearchParams();
  const navigate = useNavigate();
  const today = todayIso();
  const activeClassId = searchParams.get("class_id") ?? "";
  const month = parseMonthParam(searchParams.get("month"), today);
  const [view, setView] = useState<ClassbookView>("sessions");
  const [selectedSessionId, setSelectedSessionId] = useState<string | null>(null);
  // The expand row is keyed by session, so switching sessions unmounts it
  // before it could object: the page holds the intended navigation and asks
  // about unsaved scores first.
  const rowRef = useRef<SessionExpandRowHandle>(null);
  const [rowDirtyCount, setRowDirtyCount] = useState(0);
  const [rowInvalidCount, setRowInvalidCount] = useState(0);
  const [pendingNavigation, setPendingNavigation] = useState<Navigation | null>(null);
  const [pendingSaving, setPendingSaving] = useState(false);

  const { centerId, isOwner } = useCenterContext();

  // per_page mirrors the students page: the class-search empty note asserts
  // over the full active-class list, so it must not be a truncated page.
  const { data: classesPage, isPending: classesPending } = useClassesList({
    status: "active",
    per_page: 100,
  });
  const classes = classesPage?.items ?? [];
  const effectiveClassId = activeClassId || (classes[0]?.id ?? "");
  // A stale class_id in the URL matches no class — render the page frame
  // with no per-class queries instead of fetching garbage.
  const selectedClass = classes.find((klass) => klass.id === effectiveClassId);
  const selectedClassId = selectedClass?.id;
  const canWrite = selectedClass ? canWriteClass(isOwner, selectedClass) : false;

  const {
    month: monthWin,
    sessions,
    heldSessions,
    rosters,
    sessionsPending,
    sessionsError,
    refetchSessions,
  } = useMonthSessions(selectedClassId, month);
  const monthNumber = Number(monthWin.label);
  const previousMonthNumber = monthNumber === 1 ? 12 : monthNumber - 1;

  // Server-backed teaching data, reassembled into the store-shaped slice the
  // table/CSV code and the child components were written against.
  const { curriculum, lessonPlans } = useClassTeaching(selectedClassId);
  const classMarks = useClassMarks(selectedClassId, month);
  const teaching: TeachingState = {
    curricula: selectedClassId && curriculum ? { [selectedClassId]: curriculum } : {},
    lessonPlans,
    sessionNotes: classMarks.sessionNotes,
    sessionScores: classMarks.sessionScores,
    personalNotes: classMarks.personalNotes,
  };

  // CHẤM ĐIỂM counts: component classes need one scores query per held
  // session; general-score classes read the month marks batch instead.
  const scoreComponentsQuery = useClassScoreComponents(selectedClassId);
  const hasScoreComponents = (scoreComponentsQuery.data?.components.length ?? 0) > 0;
  const componentCounts = useSessionScoreCounts(
    heldSessions.map((session) => session.id),
    hasScoreComponents,
  );
  const scoredCounts: Record<string, number> = {};
  for (const session of heldSessions) {
    scoredCounts[session.id] = hasScoreComponents
      ? (componentCounts[session.id] ?? 0)
      : scoredStudentCount(teaching.sessionScores[session.id]);
  }

  // No `active` filter: ended enrollments still price past sessions and feed
  // the retention windows; `activeHeadcount` filters for the SĨ SỐ figure.
  const { data: enrollmentsPage } = useEnrollmentsList(
    { class_id: selectedClassId, per_page: 100 },
    { enabled: Boolean(selectedClassId) },
  );
  // keepPreviousData holds the previous class's page while switching;
  // gate on the driving id so its prices never touch this class's rows.
  const enrollments = selectedClassId ? (enrollmentsPage?.items ?? []) : [];
  const headcount = activeHeadcount(enrollments);

  // The picker's second line names the class's giáo viên; a class between
  // handoffs simply shows no name.
  const unitPriceByEnrollmentId = new Map(
    enrollments.map((enrollment) => [enrollment.id, enrollment.unit_price]),
  );
  const derived = deriveSessions(
    sessions,
    rosters,
    teaching.sessionScores,
    unitPriceByEnrollmentId,
  );
  const totals = classbookTotals(derived);
  const retention = retentionStat(enrollments, monthWin.from);

  const kpis: ClassKpi[] = [
    {
      label: "SĨ SỐ",
      value: String(headcount),
      sub: `tái tục ${retention.pct}%`,
    },
    {
      label: "CHUYÊN CẦN",
      value: `${totals.attendancePct}%`,
      sub: `${totals.presentTotal}/${totals.eligibleTotal} lượt`,
    },
    {
      label: "ĐIỂM TB",
      value: formatLedgerScore(totals.classAverage),
      sub: `${totals.scoredSessionCount} buổi`,
    },
    {
      label: `LÃI/LỖ T${monthNumber}`,
      value: vnd(totals.monthNet),
      sub: `thu ${vnd(totals.monthGross)} · chi ${vnd(totals.monthCost)}`,
      tone: totals.monthNet < 0 ? "negative" : "default",
    },
  ];

  const handleRowDirtyChange = useCallback((dirty: number, invalid: number) => {
    setRowDirtyCount(dirty);
    setRowInvalidCount(invalid);
  }, []);

  function resetRowCounts() {
    setRowDirtyCount(0);
    setRowInvalidCount(0);
  }

  function setParams(entries: Record<string, string>) {
    setSearchParams(
      (prev) => {
        const next = new URLSearchParams(prev);
        for (const [key, value] of Object.entries(entries)) next.set(key, value);
        return next;
      },
      { replace: true },
    );
  }

  // The row unmounts in the same commit as these state changes, so its own
  // "dirty count → 0" effect never runs; the page clears the count here.
  function applyNavigation(target: Navigation) {
    resetRowCounts();
    switch (target.kind) {
      case "session":
        setSelectedSessionId(target.sessionId);
        break;
      case "class":
        setSelectedSessionId(null);
        setParams({ class_id: target.classId });
        break;
      case "month":
        setSelectedSessionId(null);
        setParams({ month: target.month });
        break;
      case "view":
        setSelectedSessionId(null);
        setView(target.view);
        break;
    }
  }

  function requestNavigation(target: Navigation) {
    if (rowDirtyCount > 0 && selectedSessionId !== null) {
      setPendingNavigation(target);
      return;
    }
    applyNavigation(target);
  }

  function toggleSession(sessionId: string) {
    requestNavigation({
      kind: "session",
      sessionId: sessionId === selectedSessionId ? null : sessionId,
    });
  }

  function closeSession() {
    requestNavigation({ kind: "session", sessionId: null });
  }

  async function savePendingNavigation() {
    if (!pendingNavigation) return;
    setPendingSaving(true);
    const saved = (await rowRef.current?.flush()) ?? true;
    setPendingSaving(false);
    if (!saved) return;
    applyNavigation(pendingNavigation);
    setPendingNavigation(null);
  }

  function discardPendingNavigation() {
    if (!pendingNavigation) return;
    rowRef.current?.discard();
    applyNavigation(pendingNavigation);
    setPendingNavigation(null);
  }

  function exportCsv() {
    if (!selectedClass) {
      return;
    }
    const classCurriculum = teaching.curricula[selectedClass.id];
    const rows: CsvCell[][] = [
      [
        "Buổi",
        "Trạng thái",
        "Bài học",
        "Có mặt",
        "Sĩ số",
        "Điểm TB",
        "Thu (đ)",
        "Chi (đ)",
        "Lãi/lỗ (đ)",
        "Nhận xét chung",
      ],
      ...derived.map((row) => {
        const { session } = row;
        const held = session.status === "held";
        const lesson =
          row.lessonIndex === null
            ? ""
            : `Bài ${row.lessonIndex + 1}${
                classCurriculum?.lessons[row.lessonIndex]
                  ? ` - ${classCurriculum.lessons[row.lessonIndex]}`
                  : ""
              }`;
        return [
          formatSessionDate(session.session_date),
          held ? "Đã dạy" : session.status === "planned" ? "Chưa điểm danh" : "Hủy",
          lesson,
          held ? (row.present ?? "") : "",
          row.eligible ?? "",
          held && row.average !== null ? row.average.toFixed(1) : "",
          held ? (row.gross ?? 0) : 0,
          held && row.gross !== null ? SESSION_COST_VND : 0,
          held ? (row.net ?? 0) : 0,
          teaching.sessionNotes[session.id]?.text ??
            (session.status === "cancelled" ? (session.cancel_reason ?? "") : ""),
        ];
      }),
    ];
    const fileName = `${selectedClass.name.replace(/ /g, "_")}_ky${monthWin.label}.csv`;
    downloadCsv(fileName, rows);
    hvToast(`Đã tải ${fileName}`);
  }

  const noClasses = !classesPending && classes.length === 0;
  // A `class_id` that matches nothing gets an explicit state instead of a bare frame.
  const fallbackClass = classes[0];

  return (
    <div className="flex flex-col gap-4">
      <header className="flex flex-col gap-3">
        <h1 className="font-display text-[26px] font-extrabold text-ink-900">Quản lý lớp học</h1>
        <div className="flex flex-wrap items-center gap-3 sm:flex-nowrap">
          {classes.length > 0 ? (
            <ClassSelect
              classes={classes}
              selected={selectedClass}
              today={today}
              onSelect={(classId) => requestNavigation({ kind: "class", classId })}
            />
          ) : null}
          <MonthStepper
            month={month}
            onChange={(next) => requestNavigation({ kind: "month", month: next })}
          />
          <div className="ml-auto flex shrink-0 items-center gap-2">
            <HvSegmented<ClassbookView>
              variant="tabs"
              idBase="classbook-view"
              aria-label="Chế độ xem"
              options={viewOptions}
              value={view}
              onValueChange={(next) => {
                if (next !== view) requestNavigation({ kind: "view", view: next });
              }}
            />
            <HvButton
              type="button"
              variant="ghost"
              size="sm"
              aria-label="Tải dữ liệu lớp (CSV)"
              title="Tải dữ liệu lớp (CSV)"
              icon={<HvIcon name="arrow-down" />}
              onClick={exportCsv}
              disabled={!selectedClass}
              className="w-11 px-0"
            />
          </div>
        </div>
      </header>

      {noClasses ? (
        <HvStateBlock
          state="empty"
          title="Chưa có lớp đang hoạt động"
          description="Tạo lớp trước, rồi quay lại đây để ghi sổ từng buổi."
          action={
            isOwner ? (
              <HvButton type="button" size="sm" onClick={() => void navigate("/center/classes")}>
                Tạo lớp
              </HvButton>
            ) : undefined
          }
        />
      ) : null}

      {fallbackClass && !selectedClass ? (
        <HvStateBlock
          state="empty"
          title="Không tìm thấy lớp"
          description="Lớp trong đường dẫn đã đóng hoặc không thuộc trung tâm này."
          action={
            <HvButton
              type="button"
              size="sm"
              onClick={() => requestNavigation({ kind: "class", classId: fallbackClass.id })}
            >
              Mở {fallbackClass.name}
            </HvButton>
          }
        />
      ) : null}

      {selectedClass ? <ClassKpiStrip items={kpis} /> : null}

      <div
        role="tabpanel"
        id={`classbook-view-panel-${view}`}
        aria-labelledby={`classbook-view-tab-${view}`}
      >
        {view === "sessions" ? (
          <>
            {sessionsPending && selectedClassId ? (
              <HvStateBlock state="loading" title="Đang tải buổi học…" />
            ) : sessionsError && selectedClassId ? (
              <HvStateBlock
                state="error"
                title="Không tải được buổi học."
                action={
                  <HvButton type="button" variant="ghost" size="sm" onClick={refetchSessions}>
                    Thử lại
                  </HvButton>
                }
              />
            ) : selectedClass && derived.length === 0 ? (
              <HvStateBlock
                state="empty"
                title={`Chưa có buổi học nào trong tháng ${monthNumber}.`}
              />
            ) : selectedClass ? (
              <SessionsTable
                rows={derived}
                classId={selectedClass.id}
                curriculum={teaching.curricula[selectedClass.id]}
                lessonPlans={teaching.lessonPlans}
                notes={teaching.sessionNotes}
                scoredCounts={scoredCounts}
                selectedId={selectedSessionId}
                onSelect={toggleSession}
                onClose={closeSession}
                renderExpanded={(row) => (
                  <SessionExpandRow
                    ref={rowRef}
                    key={row.session.id}
                    centerId={centerId}
                    classId={selectedClass.id}
                    classTitle={selectedClass.name}
                    derived={row}
                    canWrite={canWrite}
                    onClose={closeSession}
                    onDirtyChange={handleRowDirtyChange}
                  />
                )}
              />
            ) : null}
          </>
        ) : selectedClass ? (
          <CourseView
            centerId={centerId}
            classId={selectedClass.id}
            classTitle={selectedClass.name}
            doneCount={heldSessions.length}
            enrollments={enrollments}
            monthStart={monthWin.from}
            monthNumber={monthNumber}
            previousMonthNumber={previousMonthNumber}
            canWrite={canWrite}
          />
        ) : null}
      </div>

      <UnsavedScoresGuard
        open={pendingNavigation !== null}
        dirtyCount={rowDirtyCount}
        invalidCount={rowInvalidCount}
        pending={pendingSaving}
        onSave={() => void savePendingNavigation()}
        onDiscard={discardPendingNavigation}
        onStay={() => setPendingNavigation(null)}
      />
    </div>
  );
}
