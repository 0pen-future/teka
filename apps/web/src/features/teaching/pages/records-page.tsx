import { useEffect, useRef } from "react";
import { useNavigate, useSearchParams } from "react-router";
import { Download } from "lucide-react";

import { hvToast } from "@/components/hv";
import { useClassesList, useEnrollmentsList } from "@/features/roster";
import { useMediaQuery } from "@/lib/hooks/use-media-query";
import { cn } from "@/lib/utils";

import { RecordsToolbar, ResultCount } from "../components/records-toolbar";
import {
  ghostButtonClassName,
  StudentRecordsTable,
  type StudentRecordSummary,
} from "../components/student-records-table";
import { useClassMarks } from "../hooks/use-class-marks";
import { useMonthSessions } from "../hooks/use-month-sessions";
import { parseMonthParam } from "../lib/classbook-stats";
import { downloadCsv, type CsvCell } from "../lib/csv";
import { meanScore } from "../lib/classbook-stats";
import { filterStudentRows } from "../lib/student-search";
import { aggregateStudent, studentSessionRows, trendOf } from "../lib/student-stats";

/** True while the user is typing somewhere a "/" keystroke belongs to. */
function isTypingTarget(element: Element | null): boolean {
  if (!(element instanceof HTMLElement)) return false;
  return (
    element instanceof HTMLInputElement ||
    element instanceof HTMLTextAreaElement ||
    element instanceof HTMLSelectElement ||
    element.isContentEditable
  );
}

/**
 * Hồ sơ học sinh — per-student averages, trends and absence counts for the
 * selected class. NGÀY SINH renders "—" everywhere: the product stores no
 * dob, so the prototype's birthday banner is intentionally absent.
 */
export function RecordsPage() {
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();
  const activeClassId = searchParams.get("class_id") ?? "";
  const query = searchParams.get("q") ?? "";
  const wide = useMediaQuery("(min-width: 768px)");
  const searchRef = useRef<HTMLInputElement>(null);

  const { data: classesPage } = useClassesList({ status: "active", per_page: 100 });
  const classes = classesPage?.items ?? [];
  const effectiveClassId = activeClassId || (classes[0]?.id ?? "");
  const selectedClass = classes.find((klass) => klass.id === effectiveClassId);
  const selectedClassId = selectedClass?.id;

  const { month, sessions, rosters, sessionsPending } = useMonthSessions(
    selectedClassId,
    parseMonthParam(null),
  );
  const { sessionScores } = useClassMarks(selectedClassId, month.from.slice(0, 7));

  const { data: enrollmentsPage } = useEnrollmentsList(
    { class_id: selectedClassId, active: true, per_page: 100 },
    { enabled: Boolean(selectedClassId) },
  );
  const enrollments = selectedClassId ? (enrollmentsPage?.items ?? []) : [];

  const rows: StudentRecordSummary[] = enrollments.map((enrollment) => {
    const aggregate = aggregateStudent(
      studentSessionRows(sessions, rosters, sessionScores, enrollment.student_id),
    );
    return {
      studentId: enrollment.student_id,
      name: enrollment.student_name,
      average: meanScore(aggregate.scores),
      scoreCount: aggregate.scores.length,
      trend: trendOf(aggregate.scores),
      absences: aggregate.absences,
    };
  });

  const filteredRows = filterStudentRows(rows, query);
  const total = sessionsPending && selectedClassId ? null : rows.length;

  // "/" jumps to the student search from anywhere on the page, except while
  // typing in another field (the class filter inside the picker included).
  useEffect(() => {
    function onKeyDown(event: KeyboardEvent) {
      if (event.key !== "/" || event.ctrlKey || event.metaKey || event.altKey) return;
      if (isTypingTarget(document.activeElement)) return;
      event.preventDefault();
      searchRef.current?.focus();
    }
    document.addEventListener("keydown", onKeyDown);
    return () => document.removeEventListener("keydown", onKeyDown);
  }, []);

  // The query lives on the URL so a filtered list survives reload and can be
  // shared; replace keeps typing from flooding history.
  function setQuery(nextQuery: string) {
    setSearchParams(
      (prev) => {
        const next = new URLSearchParams(prev);
        if (nextQuery === "") next.delete("q");
        else next.set("q", nextQuery);
        return next;
      },
      { replace: true },
    );
  }

  // Switching class starts a fresh search: class_id and q change in the same
  // navigation so history holds one entry, not two. Re-picking the current
  // class only pins it into the URL and keeps the query.
  function selectClass(classId: string) {
    setSearchParams(
      (prev) => {
        const next = new URLSearchParams(prev);
        if (classId !== effectiveClassId) next.delete("q");
        next.set("class_id", classId);
        return next;
      },
      { replace: true },
    );
  }

  function exportCsv() {
    if (!selectedClass) {
      return;
    }
    const enrollmentByStudent = new Map(enrollments.map((item) => [item.student_id, item]));
    const csvRows: CsvCell[][] = [
      ["Họ tên", "Ngày sinh", "Lớp", "Nhập học", "Điểm TB", "Xu hướng", "Số buổi vắng"],
      ...rows.map((row) => [
        row.name,
        "—",
        selectedClass.name,
        enrollmentByStudent.get(row.studentId)?.started_on ?? "",
        row.average === null ? "" : row.average.toFixed(1),
        row.trend.label,
        row.absences,
      ]),
    ];
    const fileName = `HocSinh_${selectedClass.name.replace(/ /g, "_")}.csv`;
    downloadCsv(fileName, csvRows);
    hvToast(`Đã tải ${fileName}`);
  }

  return (
    <div className="flex flex-col gap-3.5">
      <div className="flex flex-wrap items-start gap-3">
        <div className="min-w-[260px] flex-1">
          <h1 className="font-display text-[26px] font-extrabold text-ink-900">Hồ sơ học sinh</h1>
          <p className="mt-1 text-[14px] text-ink-500">
            Điểm từng buổi và xu hướng — bằng chứng để trao đổi với phụ huynh.
          </p>
        </div>
        {wide ? (
          <button type="button" onClick={exportCsv} className={ghostButtonClassName}>
            <Download className="size-4" aria-hidden="true" />
            Tải danh sách (CSV)
          </button>
        ) : null}
      </div>

      <RecordsToolbar
        classes={classes}
        selectedClassId={effectiveClassId}
        onSelectClass={selectClass}
        query={query}
        onQueryChange={setQuery}
        matched={filteredRows.length}
        total={total}
        searchRef={searchRef}
        compact={!wide}
      />

      {sessionsPending && selectedClassId ? (
        <StudentRecordsTable
          rows={[]}
          onOpen={() => undefined}
          loading
          loadingLabel={`Đang tải dữ liệu tháng ${Number(month.label)}…`}
          compact={!wide}
        />
      ) : rows.length === 0 ? (
        <div className="rounded-[24px] bg-white p-6 text-center text-[13px] text-ink-400 shadow-soft-md">
          Lớp chưa có học sinh đang học.
        </div>
      ) : (
        <StudentRecordsTable
          rows={filteredRows}
          query={query}
          onClearSearch={() => setQuery("")}
          compact={!wide}
          onOpen={(studentId) => void navigate(`/records/${studentId}`)}
        />
      )}

      {wide ? null : (
        <div className="flex items-center justify-between px-1">
          <ResultCount matched={filteredRows.length} total={total} query={query} />
          <button
            type="button"
            onClick={exportCsv}
            aria-label="Tải danh sách (CSV)"
            className={cn(ghostButtonClassName, "min-h-10")}
          >
            <Download className="size-4" aria-hidden="true" />
            CSV
          </button>
        </div>
      )}
    </div>
  );
}
