import { Link, useParams } from "react-router";

import { hvToast } from "@/components/hv";
import { useEnrollmentsList } from "@/features/roster";
import { formatSessionDate, nameInitial } from "@/lib/utils";

import { ScoreBarChart } from "../components/score-bar-chart";
import { StudentSessionsTable } from "../components/student-sessions-table";
import { useClassMarks } from "../hooks/use-class-marks";
import { useClassTeaching } from "../hooks/use-class-teaching";
import { useMonthSessions } from "../hooks/use-month-sessions";
import { useSaveMarks } from "../hooks/use-teaching-mutations";
import { downloadCsv, type CsvCell } from "../lib/csv";
import { meanScore } from "../lib/classbook-stats";
import { aggregateStudent, studentSessionRows, trendOf } from "../lib/student-stats";
import { personalNoteKey } from "../lib/teaching-store";

function formatFullDate(isoDate: string): string {
  const [year, month, day] = isoDate.split("-");
  return year && month && day ? `${day}/${month}/${year}` : isoDate;
}

/**
 * Hồ sơ một học sinh: stat cards, per-session score bars and the inline
 * personal-note column. Class context comes from the student's own
 * enrollment (never the list page's tab state) so a stale link still
 * resolves the right class.
 */
export function StudentRecordPage() {
  const { studentId } = useParams();

  const { data: enrollmentsPage } = useEnrollmentsList(
    { student_id: studentId, per_page: 100 },
    { enabled: Boolean(studentId) },
  );
  const enrollments = enrollmentsPage?.items ?? [];
  // Prefer the open enrollment; an ended student still shows their last class.
  const enrollment =
    enrollments.find((item) => !item.ended_on) ??
    [...enrollments].sort((a, b) => a.started_on.localeCompare(b.started_on)).at(-1);

  const { month, sessions, rosters, sessionsPending } = useMonthSessions(enrollment?.class_id);
  const monthNumber = Number(month.label);
  const monthKey = month.from.slice(0, 7);
  const { sessionScores, personalNotes } = useClassMarks(enrollment?.class_id, monthKey);
  const { curriculum } = useClassTeaching(enrollment?.class_id);
  const saveMarks = useSaveMarks(enrollment?.class_id ?? "", monthKey);

  const rows = studentId ? studentSessionRows(sessions, rosters, sessionScores, studentId) : [];
  const aggregate = aggregateStudent(rows);
  const average = meanScore(aggregate.scores);
  const trend = trendOf(aggregate.scores);
  const attendancePct =
    aggregate.held === 0
      ? null
      : Math.round(((aggregate.held - aggregate.absences) / aggregate.held) * 100);

  const notes: Record<string, string> = {};
  for (const row of rows) {
    const note = studentId
      ? personalNotes[personalNoteKey(row.session.id, studentId)]
      : undefined;
    if (note !== undefined) {
      notes[row.session.id] = note;
    }
  }

  function saveNote(sessionId: string, text: string) {
    if (!studentId || !enrollment) {
      return;
    }
    // Blur-save with an empty field clears the stored note (tri-state wire:
    // `null` deletes, a value sets). The success toast waits for the server —
    // on failure the mutation's onError already surfaces the danger toast.
    const studentName = enrollment.student_name;
    saveMarks.mutate(
      {
        sessionId,
        entries: [{ student_id: studentId, personal_note: text === "" ? null : text }],
      },
      {
        onSuccess: () => hvToast(`Đã lưu nhận xét cho ${studentName}`),
      },
    );
  }

  function exportCsv() {
    if (!enrollment) {
      return;
    }
    const csvRows: CsvCell[][] = [
      ["Buổi", "Bài học", "Trạng thái", "Điểm", "Nhận xét"],
      ...rows.map((row) => [
        formatSessionDate(row.session.session_date),
        row.lessonIndex === null
          ? ""
          : `Bài ${row.lessonIndex + 1}${
              curriculum?.lessons[row.lessonIndex]
                ? ` - ${curriculum.lessons[row.lessonIndex]}`
                : ""
            }`,
        row.absent ? "Vắng" : "Có mặt",
        row.score === null ? "" : row.score.toFixed(1),
        notes[row.session.id] ?? "",
      ]),
    ];
    const fileName = `${enrollment.student_name.replace(/ /g, "_")}_ky${month.label}.csv`;
    downloadCsv(fileName, csvRows);
    hvToast(`Đã tải ${fileName}`);
  }

  const stats = [
    {
      label: `ĐIỂM TB THÁNG ${monthNumber}`,
      value: average === null ? "—" : average.toFixed(1),
      sub: `${aggregate.scores.length} bài kiểm tra buổi`,
    },
    {
      label: "XU HƯỚNG",
      value: `${trend.arrow} ${trend.label}`,
      sub: "so với đầu tháng",
    },
    {
      label: "CHUYÊN CẦN",
      value: attendancePct === null ? "—" : `${attendancePct}%`,
      sub: `${aggregate.absences} buổi vắng`,
    },
  ];

  return (
    <div className="flex flex-col gap-4">
      <div>
        <Link
          to={enrollment ? `/records?class_id=${enrollment.class_id}` : "/records"}
          className="p-0 text-[13.5px] font-extrabold text-sky-500 hover:text-sky-600"
        >
          ← Hồ sơ học sinh
        </Link>
      </div>

      {!enrollment ? (
        sessionsPending || !enrollmentsPage ? (
          <p className="text-[13px] text-ink-400">Đang tải hồ sơ…</p>
        ) : (
          <div className="rounded-[24px] bg-white p-6 text-center text-[13px] text-ink-400 shadow-soft-md">
            Không tìm thấy hồ sơ học sinh này.
          </div>
        )
      ) : (
        <>
          <div className="flex flex-wrap items-center gap-4">
            <div className="flex size-[54px] items-center justify-center rounded-full bg-sky-100 font-display text-[24px] font-extrabold text-sky-600">
              {nameInitial(enrollment.student_name)}
            </div>
            <div className="min-w-[240px] flex-1">
              <h1 className="font-display text-[24px] font-extrabold text-ink-900">
                {enrollment.student_name}
              </h1>
              <p className="text-[13.5px] text-ink-500">
                {enrollment.class_name} · Nhập học {formatFullDate(enrollment.started_on)}
              </p>
            </div>
            <button
              type="button"
              onClick={exportCsv}
              className="rounded-[14px] border-2 border-line-200 bg-white px-4 py-[9px] text-[13px] font-extrabold text-ink-700 transition-colors hover:border-mint-400 hover:text-mint-600 focus-visible:ring-4 focus-visible:outline-none"
            >
              Tải hồ sơ (CSV)
            </button>
          </div>

          <div className="grid max-w-[620px] grid-cols-3 gap-3">
            {stats.map((stat) => (
              <div key={stat.label} className="rounded-[20px] bg-white px-4 py-3.5 shadow-soft-md">
                <div className="text-[11.5px] font-extrabold tracking-[0.3px] text-ink-400">
                  {stat.label}
                </div>
                <div className="mt-0.5 font-display text-[21px] font-extrabold text-ink-900">
                  {stat.value}
                </div>
                <div className="text-[12px] text-ink-500">{stat.sub}</div>
              </div>
            ))}
          </div>

          <div className="flex flex-wrap items-start gap-4">
            <ScoreBarChart rows={rows} monthNumber={monthNumber} />
            <StudentSessionsTable rows={rows} notes={notes} onSaveNote={saveNote} />
          </div>
        </>
      )}
    </div>
  );
}
