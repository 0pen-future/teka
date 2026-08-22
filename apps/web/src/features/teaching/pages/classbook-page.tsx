import { useState } from "react";
import { useSearchParams } from "react-router";

import { hvToast } from "@/components/hv";
import {
  ClassSearchEmptyNote,
  ClassSearchInput,
  useClassSearch,
  useClassesList,
  useEnrollmentsList,
} from "@/features/roster";
import { cn, formatSessionDate } from "@/lib/utils";

import { ClassStatCards, type ClassStat } from "../components/class-stat-cards";
import { CourseView } from "../components/course-view";
import { SessionDetailPanel } from "../components/session-detail-panel";
import { SessionsTable } from "../components/sessions-table";
import {
  activeHeadcount,
  classbookTotals,
  deriveSessions,
  retentionStat,
} from "../lib/classbook-stats";
import { downloadCsv, type CsvCell } from "../lib/csv";
import { SESSION_COST_VND, type TeachingState } from "../lib/teaching-store";
import { useCenterContext } from "../hooks/use-center-context";
import { useClassMarks } from "../hooks/use-class-marks";
import { useClassTeaching } from "../hooks/use-class-teaching";
import { useMonthSessions } from "../hooks/use-month-sessions";
import { vnd } from "../lib/vnd";

type ClassbookView = "sessions" | "course";

const viewTabs: { id: ClassbookView; label: string }[] = [
  { id: "sessions", label: "Buổi học & nhận xét" },
  { id: "course", label: "Chương trình & giáo án" },
];

/**
 * Quản lý lớp học — the teaching screen's default view: per-class stat
 * cards, the month's session table, and the session detail panel. Everything
 * is server data through React Query: sessions, rosters, and enrollments
 * from their features; scores, notes, and giáo án from the teaching API.
 */
export function ClassbookPage() {
  const [searchParams, setSearchParams] = useSearchParams();
  const activeClassId = searchParams.get("class_id") ?? "";
  const [view, setView] = useState<ClassbookView>("sessions");
  const [selectedSessionId, setSelectedSessionId] = useState<string | null>(null);

  const { centerId } = useCenterContext();

  // per_page mirrors the students page: the class-search empty note asserts
  // over the full active-class list, so it must not be a truncated page.
  const { data: classesPage } = useClassesList({ status: "active", per_page: 100 });
  const classes = classesPage?.items ?? [];
  const classSearch = useClassSearch(classes);
  const effectiveClassId = activeClassId || (classes[0]?.id ?? "");
  // A stale class_id in the URL matches no class — render the page frame
  // with no per-class queries instead of fetching garbage.
  const selectedClass = classes.find((klass) => klass.id === effectiveClassId);
  const selectedClassId = selectedClass?.id;

  const { month, sessions, heldSessions, rosters, sessionsPending } =
    useMonthSessions(selectedClassId);
  const monthNumber = Number(month.label);
  const previousMonthNumber = monthNumber === 1 ? 12 : monthNumber - 1;

  // Server-backed teaching data, reassembled into the store-shaped slice the
  // table/CSV code and the child components were written against.
  const { curriculum, lessonPlans } = useClassTeaching(selectedClassId);
  const classMarks = useClassMarks(selectedClassId, month.from.slice(0, 7));
  const teaching: TeachingState = {
    curricula: selectedClassId && curriculum ? { [selectedClassId]: curriculum } : {},
    lessonPlans,
    sessionNotes: classMarks.sessionNotes,
    sessionScores: classMarks.sessionScores,
    personalNotes: classMarks.personalNotes,
  };

  // No `active` filter: ended enrollments still price past sessions and feed
  // the retention windows; `activeHeadcount` filters for the SĨ SỐ card.
  const { data: enrollmentsPage } = useEnrollmentsList(
    { class_id: selectedClassId, per_page: 100 },
    { enabled: Boolean(selectedClassId) },
  );
  // keepPreviousData holds the previous class's page while switching tabs;
  // gate on the driving id so its prices never touch this class's rows.
  const enrollments = selectedClassId ? (enrollmentsPage?.items ?? []) : [];

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
  const retention = retentionStat(enrollments, month.from);

  const stats: ClassStat[] = [
    {
      label: "SĨ SỐ HIỆN TẠI",
      value: String(activeHeadcount(enrollments)),
      sub: "học sinh đang học",
    },
    {
      label: `CHUYÊN CẦN THÁNG ${monthNumber}`,
      value: `${totals.attendancePct}%`,
      sub: `${totals.presentTotal}/${totals.eligibleTotal} lượt có mặt`,
    },
    {
      label: `TÁI TỤC T${previousMonthNumber}→T${monthNumber}`,
      value: `${retention.pct}%`,
      sub: `${retention.previous} → ${retention.continuing} học sinh`,
    },
    {
      label: "ĐIỂM TB LỚP",
      value: totals.classAverage === null ? "—" : totals.classAverage.toFixed(1),
      sub: `trung bình ${totals.scoredSessionCount} buổi`,
    },
    {
      label: `LÃI/LỖ THÁNG ${monthNumber}`,
      value: vnd(totals.monthNet),
      sub: `thu ${vnd(totals.monthGross)} − chi ${vnd(totals.monthCost)}`,
    },
  ];

  const selectedDerived = derived.find((row) => row.session.id === selectedSessionId);

  function selectClass(classId: string) {
    setSearchParams(
      (prev) => {
        const next = new URLSearchParams(prev);
        next.set("class_id", classId);
        return next;
      },
      { replace: true },
    );
    setSelectedSessionId(null);
  }

  function exportCsv() {
    if (!selectedClass) {
      return;
    }
    const curriculum = teaching.curricula[selectedClass.id];
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
                curriculum?.lessons[row.lessonIndex]
                  ? ` - ${curriculum.lessons[row.lessonIndex]}`
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
    const fileName = `${selectedClass.name.replace(/ /g, "_")}_ky${month.label}.csv`;
    downloadCsv(fileName, rows);
    hvToast(`Đã tải ${fileName}`);
  }

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-start gap-3">
        <div className="min-w-[260px] flex-1">
          <h1 className="font-display text-[26px] font-extrabold text-ink-900">Quản lý lớp học</h1>
          <p className="mt-1 text-[14px] text-ink-500">
            Sĩ số, điểm trung bình, doanh thu và giáo án từng buổi — số liệu cho họp tuần nằm hết ở
            đây.
          </p>
        </div>
        <button
          type="button"
          onClick={exportCsv}
          className="flex items-center gap-2 rounded-[14px] border-2 border-line-200 bg-white px-4 py-[9px] text-[13px] font-extrabold text-ink-700 transition-colors hover:border-mint-400 hover:text-mint-600 focus-visible:ring-4 focus-visible:outline-none"
        >
          ⬇ Tải dữ liệu lớp (CSV)
        </button>
      </div>

      <div className="flex flex-wrap items-center gap-2">
        {classSearch.showSearch ? (
          <ClassSearchInput value={classSearch.query} onChange={classSearch.setQuery} />
        ) : null}
        <div role="tablist" aria-label="Lớp" className="contents">
          {classSearch.filtered.map((klass) => (
            <button
              key={klass.id}
              type="button"
              role="tab"
              aria-selected={effectiveClassId === klass.id}
              onClick={() => selectClass(klass.id)}
              className={cn(
                "min-h-11 rounded-full px-[18px] font-display text-[14px] font-extrabold transition-[background-color,color,box-shadow] focus-visible:ring-4 focus-visible:outline-none",
                effectiveClassId === klass.id
                  ? "bg-mint-400 text-white shadow-press-mint"
                  : "bg-white text-ink-500 shadow-soft-sm hover:bg-cream-100",
              )}
            >
              {klass.name}
            </button>
          ))}
        </div>
        {classSearch.emptyNote ? <ClassSearchEmptyNote note={classSearch.emptyNote} /> : null}
      </div>

      <ClassStatCards stats={stats} />

      <div
        className="flex gap-[22px] border-b-[1.5px] border-line-200"
        role="tablist"
        aria-label="Chế độ xem"
      >
        {viewTabs.map((tab) => (
          <button
            key={tab.id}
            type="button"
            role="tab"
            aria-selected={view === tab.id}
            onClick={() => setView(tab.id)}
            className={cn(
              "border-b-[3px] px-0.5 py-2.5 text-[14.5px] font-extrabold focus-visible:ring-4 focus-visible:outline-none",
              view === tab.id ? "border-mint-400 text-ink-900" : "border-transparent text-ink-400",
            )}
          >
            {tab.label}
          </button>
        ))}
      </div>

      {view === "sessions" ? (
        <div className="flex flex-wrap items-start gap-4">
          <div className="min-w-0 flex-[1.6] basis-[460px]">
            {sessionsPending && selectedClassId ? (
              <p className="text-[13px] text-ink-400">Đang tải buổi học…</p>
            ) : derived.length === 0 ? (
              <div className="rounded-[24px] bg-white p-6 text-center text-[13px] text-ink-400 shadow-soft-md">
                Chưa có buổi học nào trong tháng {monthNumber}.
              </div>
            ) : (
              <SessionsTable
                classId={effectiveClassId}
                derived={derived}
                teaching={teaching}
                selectedSessionId={selectedSessionId}
                onSelect={(sessionId) =>
                  setSelectedSessionId((current) => (current === sessionId ? null : sessionId))
                }
              />
            )}
          </div>
          {selectedDerived && selectedClass ? (
            <SessionDetailPanel
              key={selectedDerived.session.id}
              centerId={centerId}
              classId={selectedClass.id}
              classTitle={selectedClass.name}
              derived={selectedDerived}
              onClose={() => setSelectedSessionId(null)}
            />
          ) : null}
        </div>
      ) : selectedClass ? (
        <CourseView
          centerId={centerId}
          classId={selectedClass.id}
          classTitle={selectedClass.name}
          doneCount={heldSessions.length}
          enrollments={enrollments}
          monthStart={month.from}
          monthNumber={monthNumber}
          previousMonthNumber={previousMonthNumber}
        />
      ) : null}
    </div>
  );
}
