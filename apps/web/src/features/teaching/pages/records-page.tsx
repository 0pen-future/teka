import { useNavigate, useSearchParams } from "react-router";

import { hvToast } from "@/components/hv";
import {
  ClassSearchEmptyNote,
  ClassSearchInput,
  useClassSearch,
  useClassesList,
  useEnrollmentsList,
} from "@/features/roster";
import { cn } from "@/lib/utils";

import {
  StudentRecordsTable,
  type StudentRecordSummary,
} from "../components/student-records-table";
import { useClassMarks } from "../hooks/use-class-marks";
import { useMonthSessions } from "../hooks/use-month-sessions";
import { downloadCsv, type CsvCell } from "../lib/csv";
import { meanScore } from "../lib/classbook-stats";
import { aggregateStudent, studentSessionRows, trendOf } from "../lib/student-stats";

/**
 * Hồ sơ học sinh — per-student averages, trends and absence counts for the
 * selected class. NGÀY SINH renders "—" everywhere: the product stores no
 * dob, so the prototype's birthday banner is intentionally absent.
 */
export function RecordsPage() {
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();
  const activeClassId = searchParams.get("class_id") ?? "";

  const { data: classesPage } = useClassesList({ status: "active", per_page: 100 });
  const classes = classesPage?.items ?? [];
  const classSearch = useClassSearch(classes);
  const effectiveClassId = activeClassId || (classes[0]?.id ?? "");
  const selectedClass = classes.find((klass) => klass.id === effectiveClassId);
  const selectedClassId = selectedClass?.id;

  const { month, sessions, rosters, sessionsPending } = useMonthSessions(selectedClassId);
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

  function selectClass(classId: string) {
    setSearchParams(
      (prev) => {
        const next = new URLSearchParams(prev);
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
        <button
          type="button"
          onClick={exportCsv}
          className="flex items-center gap-2 rounded-[14px] border-2 border-line-200 bg-white px-4 py-[9px] text-[13px] font-extrabold text-ink-700 transition-colors hover:border-mint-400 hover:text-mint-600 focus-visible:ring-4 focus-visible:outline-none"
        >
          ⬇ Tải danh sách (CSV)
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

      {sessionsPending && selectedClassId ? (
        <p className="text-[13px] text-ink-400">Đang tải dữ liệu tháng {Number(month.label)}…</p>
      ) : rows.length === 0 ? (
        <div className="rounded-[24px] bg-white p-6 text-center text-[13px] text-ink-400 shadow-soft-md">
          Lớp chưa có học sinh đang học.
        </div>
      ) : (
        <StudentRecordsTable
          rows={rows}
          onOpen={(studentId) => void navigate(`/records/${studentId}`)}
        />
      )}
    </div>
  );
}
