import { useRef, useState } from "react";
import { Link, useNavigate, useParams, useSearchParams } from "react-router";
import { z } from "zod";

import { HvBadge, HvButton, HvStateBlock, hvToast } from "@/components/hv";
import { actorLabel, useAuditLogs, type AuditLog } from "@/features/audit";
import {
  materialFormatLabel,
  materialKindLabel,
  useLessons,
  useTemplatesList,
  useVersionDetail,
  useVersions,
  versionStatusLabel,
  type TemplateVersion,
} from "@/features/library";
import { formatScheduleSummary, useClassesList } from "@/features/roster";
import { useCenterContext } from "@/features/teaching";
import { ApiError } from "@/lib/api/errors";
import { cn, formatDateTime } from "@/lib/utils";

import { CourseDialog } from "../components/course-dialog";
import {
  useArchiveCourse,
  useCourse,
  useSetTuitionPacks,
  useUpdateCourse,
} from "../hooks/use-courses";
import { useCoursePaths } from "../hooks/use-paths";
import { copyCourseCode, courseStatusLabel, courseStatusVariant } from "../lib/course-labels";
import {
  cellClassName,
  formatVnd,
  headCellClassName,
  tableCardClassName,
} from "../lib/table-classes";
import { courseToInput, type Course, type CourseInput } from "../schemas/courses-schemas";

function apiMessage(error: unknown, fallback: string): string {
  return error instanceof ApiError ? error.message : fallback;
}

/** "setup" is the Thiết lập sub-view of the info tab, reached from its rows. */
const tabs = [
  "info",
  "template",
  "classes",
  "report",
  "students",
  "homework",
  "eval",
  "tasks",
  "setup",
] as const;
type Tab = (typeof tabs)[number];
const tabSchema = z.enum(tabs).catch("info");

const tabLabels: [Exclude<Tab, "setup">, string][] = [
  ["info", "Thông tin chung"],
  ["template", "Chương trình mẫu"],
  ["classes", "Lớp học"],
  ["report", "Báo cáo"],
  ["students", "Học viên"],
  ["homework", "Bài tập"],
  ["eval", "Đánh giá"],
  ["tasks", "Công việc"],
];

/** Operations tabs the prototype sketches without a screen of their own yet. */
const opsHints: Partial<Record<Tab, string>> = {
  report: "Doanh thu, chuyên cần, tiến độ theo phiên bản template — join qua phiên bản.",
  students: "Học viên đang theo học tất cả lớp của khóa; chuyển lớp trong cùng khóa giữ lịch sử.",
  homework: "Bài nộp của học viên theo từng lớp — thuộc instance, không thuộc template.",
  eval: "Điểm tổng kết theo cấu trúc điểm của phiên bản lớp đang dùng.",
  tasks: "Việc chuẩn bị tài liệu cho khóa.",
};

const cardClassName = "rounded-[24px] bg-white px-5 py-[18px] shadow-soft-md";
const cardTitleClassName = "font-display text-[19px] font-bold text-ink-900";
const cardNoteClassName = "text-[12.5px] text-ink-400";
const linkButtonClassName =
  "cursor-pointer border-none bg-transparent p-0 font-extrabold text-sky-500 hover:text-sky-600";
const outlineSmallClassName =
  "rounded-[10px] border-[1.5px] border-line-200 bg-transparent px-2.5 py-[5px] text-[12px] font-extrabold text-ink-500 hover:border-mint-400 hover:text-mint-600";
const openTemplateClassName =
  "rounded-[12px] border-[1.5px] border-line-200 bg-transparent px-3 py-[7px] text-[12.5px] font-extrabold text-sky-500 hover:border-sky-300";
const inputClassName =
  "w-full rounded-[14px] border-2 border-line-200 bg-white px-3 py-2.5 text-[14.5px] outline-none focus:border-mint-400";
const fieldLabelClassName = "block text-[13px] font-extrabold text-ink-500";

/**
 * `/courses/:id` — the prototype's "Chi tiết khóa học": a header card with
 * the status toggle, then Thông tin chung (facts, change history, the
 * Thiết lập list), the bound program template and the course's classes.
 */
export function CourseDetailPage() {
  const { id = "" } = useParams<{ id: string }>();
  const { isResolved } = useCenterContext();
  const course = useCourse(id);

  // Wait for the permission set too, so editing controls never pop in late.
  if (course.isPending || !isResolved) {
    return <HvStateBlock state="loading" title="Đang tải khóa học" />;
  }
  if (course.isError) {
    const notFound = course.error instanceof ApiError && course.error.status === 404;
    return (
      <HvStateBlock
        state="error"
        title={notFound ? "Không tìm thấy khóa học" : "Không tải được khóa học"}
        description={
          notFound ? "Khóa học có thể đã bị xoá hoặc đường dẫn không đúng." : "Thử tải lại trang."
        }
        action={
          <Link
            to="/courses"
            className="font-display text-[13px] font-bold text-mint-600 hover:underline"
          >
            Về danh sách khóa học
          </Link>
        }
      />
    );
  }

  return <CourseWorkspace course={course.data} />;
}

function CourseWorkspace({ course }: { course: Course }) {
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();
  const tab: Tab = tabSchema.parse(searchParams.get("tab") ?? undefined);
  const { has } = useCenterContext();
  const canEdit = has("courses.edit");
  const historyRef = useRef<HTMLDivElement>(null);

  const paths = useCoursePaths(course.id, has("paths.read"));
  const pathRows = paths.data ?? [];
  const archive = useArchiveCourse(course.id);
  const update = useUpdateCourse(course.id);
  const [editing, setEditing] = useState(false);

  function selectTab(next: Tab) {
    const params = new URLSearchParams(searchParams);
    if (next === "info") {
      params.delete("tab");
    } else {
      params.set("tab", next);
    }
    setSearchParams(params, { replace: true });
  }

  function showHistory() {
    selectTab("info");
    // The card mounts with the tab; scroll once it is on screen.
    requestAnimationFrame(() => historyRef.current?.scrollIntoView({ behavior: "smooth" }));
  }

  const stopped = course.status === "archived";
  const openClasses = course.classes_running + course.classes_upcoming;

  function toggleStatus() {
    if (stopped) {
      update.mutate(
        { ...courseToInput(course), status: "active" },
        {
          onSuccess: () => hvToast("Đã kích hoạt lại"),
          onError: (error) =>
            hvToast(apiMessage(error, "Không kích hoạt lại được khóa học."), {
              variant: "danger",
            }),
        },
      );
      return;
    }
    if (openClasses > 0) {
      hvToast(`Khóa còn ${openClasses} lớp đang mở — kết thúc lớp trước khi dừng`, {
        variant: "danger",
      });
      return;
    }
    archive.mutate(undefined, {
      onSuccess: () => hvToast("Đã dừng hoạt động — không mở lớp mới từ khóa này"),
      onError: (error) =>
        hvToast(apiMessage(error, "Không dừng hoạt động được khóa học."), { variant: "danger" }),
    });
  }

  const meta = pathRows.map((row) => `${row.name} → ${row.stage_name}`).join(", ");

  return (
    <div className="flex flex-col">
      <button
        type="button"
        onClick={() => void navigate("/courses")}
        className={cn(linkButtonClassName, "self-start text-[13.5px]")}
      >
        ← Danh sách khóa học
      </button>

      <div className={cn(cardClassName, "mt-2.5")}>
        <div className="flex flex-wrap items-start gap-4">
          <div
            aria-hidden="true"
            className="flex size-16 flex-none items-center justify-center rounded-[18px] bg-sky-100 font-display text-[22px] font-extrabold text-sky-600"
          >
            {course.name.slice(0, 1).toUpperCase()}
          </div>
          <div className="min-w-[240px] flex-1">
            <div className="text-[12.5px] font-bold text-ink-400">Khóa học</div>
            <div className="flex flex-wrap items-center gap-2.5">
              <h1 className="m-0 font-display text-[28px] font-extrabold text-ink-900">
                {course.name}
              </h1>
              <HvBadge variant={courseStatusVariant[course.status]} size="sm">
                {courseStatusLabel[course.status]}
              </HvBadge>
            </div>
            <div className="mt-1.5 flex flex-wrap items-center gap-2.5 text-[13.5px] text-ink-500">
              <span className="whitespace-nowrap">
                Mã khóa học: <b className="text-ink-900">{course.code}</b>
              </span>
              <button
                type="button"
                title="Sao chép mã"
                onClick={() => void copyCourseCode(course.code)}
                className="rounded-[8px] bg-cream-200 px-2 py-[3px] text-[11.5px] font-extrabold text-ink-500 hover:bg-sky-100 hover:text-sky-600"
              >
                Sao chép
              </button>
              <span aria-hidden="true" className="text-line-300">
                ·
              </span>
              <button
                type="button"
                onClick={showHistory}
                className={cn(linkButtonClassName, "text-[13px]")}
              >
                Lịch sử
              </button>
              {meta ? (
                <>
                  <span aria-hidden="true" className="text-line-300">
                    ·
                  </span>
                  <span>{meta}</span>
                </>
              ) : null}
            </div>
          </div>
          {canEdit ? (
            <div className="flex gap-2 self-start">
              <button
                type="button"
                onClick={() => setEditing(true)}
                className="rounded-[14px] border-2 border-line-200 bg-transparent px-4 py-2.5 text-[13.5px] font-extrabold text-ink-700 hover:border-mint-400 hover:text-mint-600"
              >
                Sửa khóa
              </button>
              <button
                type="button"
                onClick={toggleStatus}
                disabled={archive.isPending || update.isPending}
                className={cn(
                  "rounded-[14px] border-2 bg-transparent px-4 py-2.5 text-[13.5px] font-extrabold disabled:opacity-50",
                  stopped ? "border-mint-300 text-mint-600" : "border-sun-300 text-sun-600",
                )}
              >
                {stopped ? "Kích hoạt lại" : "Dừng hoạt động"}
              </button>
            </div>
          ) : null}
        </div>
      </div>

      <div
        role="tablist"
        aria-label="Các mục của khóa học"
        className="mt-3.5 flex flex-wrap gap-1 border-b-[1.5px] border-line-100 pb-1.5"
      >
        {tabLabels.map(([value, label]) => {
          const on = tab === value || (value === "info" && tab === "setup");
          return (
            <button
              key={value}
              type="button"
              role="tab"
              aria-selected={on}
              onClick={() => selectTab(value)}
              className={cn(
                "-mb-[7.5px] border-b-[2.5px] bg-transparent px-3.5 py-2 text-[13.5px] font-extrabold",
                on ? "border-mint-400 text-ink-900" : "border-transparent text-ink-500",
              )}
            >
              {label}
            </button>
          );
        })}
      </div>

      <div role="tabpanel" aria-label={tabLabels.find(([value]) => value === tab)?.[1]}>
        {tab === "info" ? (
          <InfoTab
            course={course}
            canEdit={canEdit}
            historyRef={historyRef}
            onSetup={() => selectTab("setup")}
            stageLabel={pathRows.map((row) => row.stage_name).join(", ") || "—"}
            pathLabel={pathRows.map((row) => row.name).join(", ") || "—"}
          />
        ) : tab === "setup" ? (
          <SetupView course={course} canEdit={canEdit} onBack={() => selectTab("info")} />
        ) : tab === "template" ? (
          <TemplateTab course={course} onSetup={() => selectTab("setup")} />
        ) : tab === "classes" ? (
          <ClassesTab course={course} />
        ) : (
          <div className={cn(cardClassName, "mt-3 px-5 py-[34px] text-center")}>
            <div className={cardTitleClassName}>
              {tabLabels.find(([value]) => value === tab)?.[1]} — chưa có màn hình chi tiết
            </div>
            <div className="mx-auto mt-1 max-w-[480px] text-[13.5px] text-ink-400">
              {opsHints[tab]}
            </div>
          </div>
        )}
      </div>

      {canEdit ? (
        <CourseDialog
          mode="edit"
          course={course}
          open={editing}
          onOpenChange={setEditing}
          onSaved={() => hvToast("Đã lưu khóa học")}
        />
      ) : null}
    </div>
  );
}

function templateHref(course: Course, tab?: string): string | null {
  const template = course.default_template;
  if (!template) return null;
  const params = new URLSearchParams({ v: String(template.version_no) });
  if (tab) params.set("tab", tab);
  return `/library/templates/${template.template_id}?${params.toString()}`;
}

interface InfoTabProps {
  course: Course;
  canEdit: boolean;
  historyRef: React.RefObject<HTMLDivElement | null>;
  onSetup: () => void;
  stageLabel: string;
  pathLabel: string;
}

function InfoTab({ course, canEdit, historyRef, onSetup, stageLabel, pathLabel }: InfoTabProps) {
  return (
    <div className="mt-3.5 grid grid-cols-[repeat(auto-fit,minmax(300px,1fr))] items-start gap-3.5">
      <div className="flex flex-col gap-3.5">
        <GeneralCard
          key={`${course.duration_min}-${course.default_unit_price}`}
          course={course}
          canEdit={canEdit}
          stageLabel={stageLabel}
          pathLabel={pathLabel}
        />
        <div ref={historyRef} className={cardClassName}>
          <div className={cardTitleClassName}>Lịch sử thay đổi</div>
          <div className={cn(cardNoteClassName, "mb-1.5")}>
            Ai đổi gì, khi nào — kể cả đổi phiên bản template.
          </div>
          <CourseHistory courseId={course.id} />
        </div>
      </div>
      <SettingsCard course={course} onSetup={onSetup} />
    </div>
  );
}

function Fact({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div>
      <div className="text-[12px] font-bold text-ink-400">{label}</div>
      <div className="mt-0.5 text-[15px] font-extrabold text-ink-900">{children}</div>
    </div>
  );
}

interface GeneralCardProps {
  course: Course;
  canEdit: boolean;
  stageLabel: string;
  pathLabel: string;
}

/** Thông tin chung: the two commercial facts editable in place, plus where the course sits. */
function GeneralCard({ course, canEdit, stageLabel, pathLabel }: GeneralCardProps) {
  const update = useUpdateCourse(course.id);
  const [open, setOpen] = useState(false);
  const [duration, setDuration] = useState(String(course.duration_min ?? ""));
  const [price, setPrice] = useState(String(course.default_unit_price));

  function close() {
    setOpen(false);
    setDuration(String(course.duration_min ?? ""));
    setPrice(String(course.default_unit_price));
  }

  function save() {
    const nextDuration = duration.trim() === "" ? null : Number(duration);
    const nextPrice = Number(price);
    if (
      (nextDuration !== null && (!Number.isInteger(nextDuration) || nextDuration <= 0)) ||
      !Number.isInteger(nextPrice) ||
      nextPrice < 0
    ) {
      hvToast("Thời lượng và giá phải là số nguyên hợp lệ", { variant: "danger" });
      return;
    }
    if (nextDuration === course.duration_min && nextPrice === course.default_unit_price) {
      hvToast("Không có gì thay đổi");
      return;
    }
    const input: CourseInput = {
      ...courseToInput(course),
      duration_min: nextDuration,
      default_unit_price: nextPrice,
    };
    update.mutate(input, {
      onSuccess: () => {
        setOpen(false);
        hvToast("Đã lưu — áp dụng cho lớp mở mới");
      },
      onError: (error) =>
        hvToast(apiMessage(error, "Không lưu được thông tin khóa học."), { variant: "danger" }),
    });
  }

  return (
    <div className={cardClassName}>
      <div className="flex items-center gap-2.5">
        <div className={cn(cardTitleClassName, "flex-1")}>Thông tin chung</div>
        {canEdit ? (
          <button
            type="button"
            onClick={() => (open ? close() : setOpen(true))}
            className={outlineSmallClassName}
          >
            {open ? "Đóng" : "Chỉnh sửa"}
          </button>
        ) : null}
      </div>
      {open ? (
        <>
          <div className={cn(cardNoteClassName, "mt-1")}>
            Áp dụng cho lớp mở mới; lớp đang chạy giữ giá đã ghi danh.
          </div>
          <div className="mt-3 flex gap-2.5">
            <div className="flex-1">
              <label htmlFor="course-duration" className={fieldLabelClassName}>
                Thời lượng buổi (phút)
              </label>
              <input
                id="course-duration"
                type="number"
                step={15}
                min={0}
                value={duration}
                onChange={(event) => setDuration(event.target.value)}
                className={cn(inputClassName, "mt-1.5")}
              />
            </div>
            <div className="flex-1">
              <label htmlFor="course-price" className={fieldLabelClassName}>
                Giá / buổi (đ)
              </label>
              <input
                id="course-price"
                type="number"
                step={5000}
                min={0}
                value={price}
                onChange={(event) => setPrice(event.target.value)}
                className={cn(inputClassName, "mt-1.5")}
              />
            </div>
          </div>
          <div className="mt-3.5 flex flex-wrap items-center gap-2.5">
            <HvButton onClick={save} disabled={update.isPending}>
              Lưu thay đổi
            </HvButton>
            <button
              type="button"
              onClick={close}
              className="rounded-[14px] border-2 border-line-200 bg-transparent px-4 py-2.5 font-extrabold text-ink-500"
            >
              Hủy
            </button>
          </div>
        </>
      ) : (
        <dl className="mt-3.5 grid grid-cols-[repeat(auto-fit,minmax(140px,1fr))] gap-3.5 border-t border-line-100 pt-3.5">
          <Fact label="Thời lượng buổi học">
            {course.duration_min === null ? "—" : `${course.duration_min} phút`}
          </Fact>
          <Fact label="Giá tiền / buổi học">{formatVnd(course.default_unit_price)}</Fact>
          <Fact label="Chặng">{stageLabel}</Fact>
          <Fact label="Lộ trình">{pathLabel}</Fact>
        </dl>
      )}
    </div>
  );
}

const historyVerbs: Record<string, string> = {
  "course.create": "tạo khóa học",
  "course.update": "cập nhật thông tin khóa",
  "course.archive": "dừng hoạt động khóa",
  "course.delete": "xoá khóa học",
  "course.set_tuition_packs": "cập nhật gói học phí",
};

function historyText(log: AuditLog): string {
  return historyVerbs[log.action] ?? log.action;
}

/**
 * The course's audit trail. Only successful writes count as changes; the
 * trail is owner-only, so other roles see a note instead of an empty list.
 */
function CourseHistory({ courseId }: { courseId: string }) {
  const { has } = useCenterContext();
  const canRead = has("audit.read");
  const logs = useAuditLogs({ entity_type: "course", entity_id: courseId }, canRead);

  if (!canRead) {
    return (
      <p className="border-t border-line-100 py-2.5 text-[13px] text-ink-400">
        Chỉ chủ trung tâm xem được lịch sử thay đổi.
      </p>
    );
  }
  if (logs.isPending) {
    return <p className="border-t border-line-100 py-2.5 text-[13px] text-ink-400">Đang tải…</p>;
  }
  if (logs.isError) {
    return (
      <p className="border-t border-line-100 py-2.5 text-[13px] text-ink-400">
        Không tải được lịch sử thay đổi.
      </p>
    );
  }
  const rows = logs.data.pages.flatMap((page) => page.items).filter((log) => log.status_code < 400);
  if (rows.length === 0) {
    return (
      <p className="border-t border-line-100 py-2.5 text-[13px] text-ink-400">
        Chưa có thay đổi nào.
      </p>
    );
  }
  return (
    <ul>
      {rows.map((log) => (
        <li key={log.id} className="flex gap-3 border-t border-line-100 py-[9px] text-[13px]">
          <div className="w-[78px] flex-none font-bold text-ink-400">
            {formatDateTime(log.occurred_at)}
          </div>
          <div className="flex-1">
            <span className="font-extrabold text-ink-900">{actorLabel(log)}</span>{" "}
            <span className="text-ink-700">{historyText(log)}</span>
          </div>
        </li>
      ))}
      {logs.hasNextPage ? (
        <li className="border-t border-line-100 pt-2">
          <button
            type="button"
            disabled={logs.isFetchingNextPage}
            onClick={() => void logs.fetchNextPage()}
            className={cn(linkButtonClassName, "text-[13px]")}
          >
            Xem thêm
          </button>
        </li>
      ) : null}
    </ul>
  );
}

const NOT_SET = "Chưa thiết lập";

/**
 * Thiết lập khóa học: what the bound template version brings (program,
 * score components, exercise groups, the session log) and the course's
 * own tuition packs. Score components live in named groups; a group whose
 * title names "bài tập" scores exercises, every other one scores sessions.
 */
function SettingsCard({ course, onSetup }: { course: Course; onSetup: () => void }) {
  const navigate = useNavigate();
  const { has } = useCenterContext();
  const versionId = has("library.read")
    ? (course.default_template?.version_id ?? undefined)
    : undefined;
  const detail = useVersionDetail(versionId);
  const template = course.default_template;

  const scoreSet = detail.data?.score_set ?? [];
  const isExerciseGroup = (title: string) => /bài tập/i.test(title);
  const count = (groups: typeof scoreSet) =>
    groups.reduce((sum, group) => sum + group.components.length, 0);
  const sessionScores = count(scoreSet.filter((group) => !isExerciseGroup(group.title)));
  const exerciseScores = count(scoreSet.filter((group) => isExerciseGroup(group.title)));
  const exerciseGroups = new Set(
    (detail.data?.lessons ?? []).flatMap((lesson) =>
      lesson.exercises.map((exercise) => exercise.group_id).filter(Boolean),
    ),
  ).size;
  const logFields = detail.data?.log_fields.length ?? 0;
  const applied = (n: number) => (template && n > 0 ? `${n} mục đang áp dụng` : NOT_SET);

  function openTemplate(tab: string) {
    const href = templateHref(course, tab);
    if (!href) {
      hvToast("Khóa chưa gắn chương trình mẫu — thiết lập ở mục Chương trình học");
      onSetup();
      return;
    }
    void navigate(href);
  }

  const rows: { label: string; sub: string; onClick: () => void }[] = [
    {
      label: "Chương trình học",
      sub: template ? `${template.name} · v${template.version_no}` : NOT_SET,
      onClick: onSetup,
    },
    {
      label: "Điểm thành phần buổi học",
      sub: applied(sessionScores),
      onClick: () => openTemplate("scores"),
    },
    {
      label: "Điểm thành phần bài tập",
      sub: applied(exerciseScores),
      onClick: () => openTemplate("scores"),
    },
    { label: "Nhóm bài tập", sub: applied(exerciseGroups), onClick: () => openTemplate("groups") },
    {
      label: "Template Nhật ký buổi học",
      sub: template && logFields > 0 ? `${logFields} trường` : NOT_SET,
      onClick: () => openTemplate("logs"),
    },
    {
      label: "Gói học phí",
      sub: course.tuition_packs.length > 0 ? `${course.tuition_packs.length} gói` : NOT_SET,
      onClick: onSetup,
    },
  ];

  return (
    <div className={cardClassName}>
      <div className={cardTitleClassName}>Thiết lập khóa học</div>
      <div className={cardNoteClassName}>
        Chương trình, cấu trúc điểm, nhóm bài tập lấy từ phiên bản chương trình mẫu đang gắn. Gói
        học phí thuộc khóa.
      </div>
      <div className="mt-2.5 flex flex-col">
        {rows.map((row) => (
          <button
            key={row.label}
            type="button"
            onClick={row.onClick}
            className="flex w-full items-center gap-3 border-t border-line-100 bg-transparent px-1 py-3 text-left hover:bg-cream-100"
          >
            <div className="min-w-0 flex-1">
              <div className="text-[14.5px] font-extrabold text-ink-900">{row.label}</div>
              <div
                className={cn(
                  "mt-0.5 text-[12.5px]",
                  row.sub.startsWith("Chưa") ? "text-ink-300" : "text-ink-500",
                )}
              >
                {row.sub}
              </div>
            </div>
            <span aria-hidden="true" className="text-[16px] font-extrabold text-ink-300">
              ›
            </span>
          </button>
        ))}
      </div>
    </div>
  );
}

/** Which version each class runs, read off the versions' own class lists. */
function versionByClass(versions: TemplateVersion[]): Map<string, TemplateVersion> {
  const byClass = new Map<string, TemplateVersion>();
  for (const version of versions) {
    for (const klass of version.classes) byClass.set(klass.id, version);
  }
  return byClass;
}

function versionPill(inherited: boolean) {
  return inherited ? "bg-mint-50 text-mint-600" : "bg-sun-100 text-sun-600";
}

const pillClassName = "inline-block rounded-full px-2.5 py-[3px] text-[12px] font-extrabold";

/**
 * Thiết lập: the course's default template version with the classes that
 * run it, and the tuition packs. Only a published version can be bound — a
 * draft still changes and an archived one is retired.
 */
function SetupView({
  course,
  canEdit,
  onBack,
}: {
  course: Course;
  canEdit: boolean;
  onBack: () => void;
}) {
  const navigate = useNavigate();
  const { has } = useCenterContext();
  const canPick = canEdit && has("library.read");
  const update = useUpdateCourse(course.id);
  const template = course.default_template;
  const templates = useTemplatesList({ per_page: 100, sort: "name" }, canPick && !template);
  const versions = useVersions(has("library.read") ? template?.template_id : undefined);
  const classes = useClassesList({ course_id: course.id, status: "all", per_page: 100 });
  const byClass = versionByClass(versions.data ?? []);
  const sortedVersions = [...(versions.data ?? [])].sort((a, b) => a.version_no - b.version_no);

  function bind(versionId: string | null, toast: string) {
    update.mutate(
      { ...courseToInput(course), default_template_version_id: versionId },
      {
        onSuccess: () => hvToast(toast),
        onError: (error) =>
          hvToast(apiMessage(error, "Không lưu được chương trình mẫu."), { variant: "danger" }),
      },
    );
  }

  function pickVersion(id: string) {
    const version = sortedVersions.find((row) => row.id === id);
    if (!version || version.id === course.default_template_version_id) return;
    if (version.status !== "published") {
      hvToast(
        version.status === "draft"
          ? `v${version.version_no} còn nháp — kích hoạt trước khi gắn`
          : `v${version.version_no} đã lưu trữ — chọn phiên bản đã phát hành`,
        { variant: "danger" },
      );
      return;
    }
    bind(version.id, `Khóa dùng v${version.version_no} — lớp đang chạy giữ phiên bản cũ`);
  }

  const href = templateHref(course);
  const classRows = classes.data?.items ?? [];

  return (
    <>
      <button
        type="button"
        onClick={onBack}
        className={cn(linkButtonClassName, "mt-3.5 self-start text-[13px]")}
      >
        ← Thông tin chung
      </button>
      <div className={cn(cardClassName, "mt-2.5")}>
        <div className="flex flex-wrap items-center gap-3">
          <div className="min-w-[220px] flex-1">
            <div className={cardTitleClassName}>Phiên bản chương trình mẫu</div>
            <div className={cardNoteClassName}>
              Thứ tự kế thừa: template → khóa học (mặc định) → lớp học (override). Lớp mới mở nhận
              phiên bản của khóa; lớp đang chạy giữ phiên bản cũ cho tới khi nâng thủ công.
            </div>
          </div>
          <button
            type="button"
            onClick={() => (href ? void navigate(href) : hvToast("Khóa chưa gắn chương trình mẫu"))}
            className={openTemplateClassName}
          >
            Mở chương trình mẫu →
          </button>
        </div>
        <div className="mt-3 flex flex-wrap items-end gap-2.5">
          <div className="min-w-[220px] flex-1">
            <label htmlFor="course-template" className={fieldLabelClassName}>
              Chương trình mẫu
            </label>
            {template || !canPick ? (
              <div
                id="course-template"
                className="mt-1.5 rounded-[14px] border-2 border-dashed border-line-200 bg-cream-50 px-3 py-2.5 text-[14.5px] font-extrabold text-ink-700"
              >
                {template?.name ?? "Chưa gắn chương trình mẫu"}
              </div>
            ) : (
              <TemplatePicker
                templates={templates.data?.items ?? []}
                disabled={templates.isPending || update.isPending}
                onPick={(versionId) => bind(versionId, "Đã gắn chương trình mẫu")}
              />
            )}
          </div>
          <div className="min-w-[220px] flex-1">
            <label htmlFor="course-version" className={fieldLabelClassName}>
              Phiên bản mặc định cho khóa
            </label>
            <select
              id="course-version"
              value={course.default_template_version_id ?? ""}
              disabled={!canPick || !template || update.isPending}
              onChange={(event) => pickVersion(event.target.value)}
              className="mt-1.5 w-full rounded-[14px] border-2 border-line-200 bg-white px-3 py-2.5 text-[14.5px] font-bold disabled:opacity-60"
            >
              {sortedVersions.length === 0 ? <option value="">—</option> : null}
              {sortedVersions.map((version) => (
                <option key={version.id} value={version.id}>
                  v{version.version_no} — {versionStatusLabel[version.status]}
                  {version.status !== "published" ? " (không thể gắn)" : ""}
                </option>
              ))}
            </select>
          </div>
        </div>
        {course.default_template_version_id && !template ? (
          <p className="mt-2 text-[12.5px] font-bold text-coral-600">
            Chương trình mẫu đã gắn không còn trong kho; chọn chương trình khác.
          </p>
        ) : template?.status === "archived" ? (
          <p className="mt-2 text-[12.5px] font-bold text-sun-600">
            Phiên bản này đã lưu trữ; chọn phiên bản đã phát hành khác cho lớp mới.
          </p>
        ) : null}
        {canEdit && !has("library.read") ? (
          <p className="mt-2 text-[12.5px] text-ink-400">
            Cần quyền xem Kho học liệu để chọn chương trình mẫu.
          </p>
        ) : null}
        {canEdit && course.default_template_version_id ? (
          <button
            type="button"
            disabled={update.isPending}
            onClick={() => bind(null, "Đã bỏ chương trình mẫu")}
            className="mt-2 text-[12.5px] font-extrabold text-coral-400 hover:text-coral-600"
          >
            Bỏ chương trình mẫu
          </button>
        ) : null}

        <div className="mt-4 text-[12px] font-extrabold tracking-[0.4px] text-ink-400">
          LỚP THUỘC KHÓA — PHIÊN BẢN ĐANG DÙNG
        </div>
        {classRows.map((klass) => {
          const version = byClass.get(klass.id);
          const inherited = !version || version.id === course.default_template_version_id;
          return (
            <div
              key={klass.id}
              className="flex flex-wrap items-center gap-3 border-t border-line-100 py-2.5 text-[13.5px]"
            >
              <div className="w-[180px] font-extrabold text-ink-900">{klass.name}</div>
              <span className={cn(pillClassName, versionPill(inherited))}>
                {version
                  ? `${inherited ? "Kế thừa" : "Override →"} v${version.version_no}`
                  : "Chưa áp dụng chương trình"}
              </span>
              <div className="flex-1 text-[12.5px] text-ink-400">
                {inherited
                  ? "Theo phiên bản mặc định của khóa."
                  : "Lớp tự chọn phiên bản khác khóa. Điểm đã chấm giữ theo cấu trúc cũ."}
              </div>
              <button
                type="button"
                onClick={() => void navigate(`/classes/${klass.id}`)}
                className={outlineSmallClassName}
              >
                {inherited ? "Đổi phiên bản" : "Về kế thừa"}
              </button>
            </div>
          );
        })}
        {!classes.isPending && classRows.length === 0 ? (
          <div className="py-3.5 text-[13px] font-bold text-ink-400">
            Chưa có lớp nào mở từ khóa này.
          </div>
        ) : null}
      </div>

      <TuitionPacksCard
        key={course.tuition_packs.map((pack) => `${pack.id}:${pack.price}`).join(",")}
        course={course}
        canEdit={canEdit}
      />
    </>
  );
}

/** Two-step pick for an unbound course: the template, then one of its published versions. */
function TemplatePicker({
  templates,
  disabled,
  onPick,
}: {
  templates: { id: string; name: string }[];
  disabled: boolean;
  onPick: (versionId: string) => void;
}) {
  const [templateId, setTemplateId] = useState("");
  const versions = useVersions(templateId || undefined);
  const published = (versions.data ?? [])
    .filter((version) => version.status === "published")
    .sort((a, b) => b.version_no - a.version_no);

  return (
    <div className="mt-1.5 flex flex-col gap-1.5">
      <select
        id="course-template"
        value={templateId}
        disabled={disabled}
        onChange={(event) => setTemplateId(event.target.value)}
        className="w-full rounded-[14px] border-2 border-line-200 bg-white px-3 py-2.5 text-[14.5px] font-bold"
      >
        <option value="">Chọn chương trình mẫu…</option>
        {templates.map((template) => (
          <option key={template.id} value={template.id}>
            {template.name}
          </option>
        ))}
      </select>
      {templateId && !versions.isPending ? (
        published.length === 0 ? (
          <span className="text-[12.5px] text-ink-400">
            Chương trình này chưa có phiên bản đã phát hành.
          </span>
        ) : (
          <HvButton
            size="sm"
            variant="secondary"
            disabled={disabled}
            onClick={() => onPick(published[0]!.id)}
          >
            Gắn v{published[0]!.version_no}
          </HvButton>
        )
      ) : null}
    </div>
  );
}

const MAX_PACKS = 20;

/** Gói học phí: saved wholesale on every add or remove, in display order. */
function TuitionPacksCard({ course, canEdit }: { course: Course; canEdit: boolean }) {
  const save = useSetTuitionPacks(course.id);
  const [name, setName] = useState("");
  const [sessions, setSessions] = useState("");
  const [price, setPrice] = useState("");
  const packs = course.tuition_packs;

  function write(next: { name: string; sessions: number; price: number }[], toast: string) {
    save.mutate(next, {
      onSuccess: () => hvToast(toast),
      onError: (error) =>
        hvToast(apiMessage(error, "Không lưu được gói học phí."), { variant: "danger" }),
    });
  }

  function add() {
    const n = Number(sessions);
    const p = Number(price);
    if (!name.trim() || !Number.isInteger(n) || n < 1 || !Number.isInteger(p) || p <= 0) {
      hvToast("Điền tên gói, số buổi và giá", { variant: "danger" });
      return;
    }
    if (packs.length >= MAX_PACKS) {
      hvToast(`Mỗi khóa tối đa ${MAX_PACKS} gói`, { variant: "danger" });
      return;
    }
    write(
      [
        ...packs.map(({ name, sessions, price }) => ({ name, sessions, price })),
        { name: name.trim(), sessions: n, price: p },
      ],
      "Đã thêm gói",
    );
    setName("");
    setSessions("");
    setPrice("");
  }

  return (
    <div className={cn(cardClassName, "mt-3.5")}>
      <div className={cardTitleClassName}>Gói học phí</div>
      <div className={cn(cardNoteClassName, "mb-1.5")}>
        Thuộc khóa học, không thuộc template. Giá/buổi hiện tại:{" "}
        <b>{formatVnd(course.default_unit_price)}</b>.
      </div>
      {packs.map((pack) => {
        const full = pack.sessions * course.default_unit_price;
        return (
          <div
            key={pack.id}
            className="flex items-center gap-3 border-t border-line-100 py-2.5 text-[13.5px]"
          >
            <div className="flex-1 font-extrabold text-ink-900">{pack.name}</div>
            <div className="text-ink-500">{pack.sessions} buổi</div>
            <div className="w-[110px] text-right font-extrabold">{formatVnd(pack.price)}</div>
            <div className="w-[90px] text-right text-[12px] font-extrabold text-mint-600">
              {full > pack.price ? `−${Math.round((1 - pack.price / full) * 100)}%` : ""}
            </div>
            {canEdit ? (
              <button
                type="button"
                aria-label={`Xóa ${pack.name}`}
                disabled={save.isPending}
                onClick={() =>
                  write(
                    packs
                      .filter((row) => row.id !== pack.id)
                      .map(({ name, sessions, price }) => ({ name, sessions, price })),
                    `Đã xóa ${pack.name}`,
                  )
                }
                className="text-[12.5px] font-extrabold text-coral-400 hover:text-coral-600 disabled:opacity-50"
              >
                Xóa
              </button>
            ) : null}
          </div>
        );
      })}
      {packs.length === 0 ? (
        <div className="border-t border-line-100 py-2.5 text-[13px] font-bold text-ink-400">
          Chưa có gói học phí.
        </div>
      ) : null}
      {canEdit ? (
        <div className="mt-2.5 flex flex-wrap items-center gap-2">
          <input
            type="text"
            aria-label="Tên gói"
            placeholder="Tên gói (VD: Gói 36 buổi)"
            value={name}
            onChange={(event) => setName(event.target.value)}
            className="min-w-[160px] flex-[1.5] rounded-[14px] border-2 border-line-200 px-3 py-[9px] text-[13.5px] outline-none focus:border-mint-400"
          />
          <input
            type="number"
            aria-label="Số buổi"
            placeholder="Số buổi"
            value={sessions}
            onChange={(event) => setSessions(event.target.value)}
            className="w-[100px] rounded-[14px] border-2 border-line-200 px-3 py-[9px] text-[13.5px] outline-none focus:border-mint-400"
          />
          <input
            type="number"
            step={10000}
            aria-label="Giá gói"
            placeholder="Giá gói (đ)"
            value={price}
            onChange={(event) => setPrice(event.target.value)}
            className="w-[140px] rounded-[14px] border-2 border-line-200 px-3 py-[9px] text-[13.5px] outline-none focus:border-mint-400"
          />
          <HvButton variant="secondary" onClick={add} disabled={save.isPending}>
            + Thêm gói
          </HvButton>
        </div>
      ) : null}
    </div>
  );
}

const subTabs = [
  ["lessons", "Buổi học"],
  ["exercises", "Bài tập mẫu"],
  ["materials", "Tài liệu mẫu"],
] as const;
type SubTab = (typeof subTabs)[number][0];

/** Chương trình mẫu: a read-only look at the bound version's lessons, exercises and materials. */
function TemplateTab({ course, onSetup }: { course: Course; onSetup: () => void }) {
  const navigate = useNavigate();
  const { has } = useCenterContext();
  const [sub, setSub] = useState<SubTab>("lessons");
  const template = course.default_template;
  const versionId = has("library.read") ? template?.version_id : undefined;
  const lessons = useLessons(versionId);
  const detail = useVersionDetail(sub === "lessons" ? undefined : versionId);

  if (!template) {
    return (
      <div className={cn(cardClassName, "mt-3.5 px-5 py-[34px] text-center")}>
        <div className={cardTitleClassName}>Khóa chưa gắn chương trình mẫu</div>
        <div className="mt-1 text-[13.5px] text-ink-400">
          Gắn một phiên bản chương trình mẫu ở mục Thiết lập → Chương trình học.
        </div>
        <div className="mt-3.5 flex justify-center">
          <HvButton variant="secondary" onClick={onSetup}>
            Mở thiết lập
          </HvButton>
        </div>
      </div>
    );
  }

  const lessonRows = lessons.data ?? [];
  const href = templateHref(course);

  return (
    <>
      <div className="mt-3.5 flex flex-wrap items-center gap-2">
        <div
          role="tablist"
          aria-label="Nội dung chương trình mẫu"
          className="flex gap-0.5 rounded-[14px] bg-white p-[3px] shadow-soft-sm"
        >
          {subTabs.map(([value, label]) => (
            <button
              key={value}
              type="button"
              role="tab"
              aria-selected={sub === value}
              onClick={() => setSub(value)}
              className={cn(
                "rounded-[11px] px-3.5 py-[7px] text-[13px] font-extrabold",
                sub === value ? "bg-mint-50 text-mint-600" : "text-ink-500",
              )}
            >
              {label}
            </button>
          ))}
        </div>
        <div className="text-[13px] font-bold text-ink-500">
          {template.name} · v{template.version_no}
          {lessons.data ? ` · ${lessonRows.length} buổi` : ""}
        </div>
        {href ? (
          <button
            type="button"
            onClick={() => void navigate(href)}
            className={cn(openTemplateClassName, "ml-auto")}
          >
            Mở chương trình mẫu →
          </button>
        ) : null}
      </div>

      {!versionId ? (
        <HvStateBlock state="empty" title="Cần quyền xem Kho học liệu để xem chương trình mẫu." />
      ) : sub === "lessons" ? (
        lessons.isPending ? (
          <HvStateBlock state="loading" title="Đang tải buổi học" />
        ) : lessons.isError ? (
          <HvStateBlock state="error" title="Không tải được buổi học" />
        ) : (
          <div className={cn(tableCardClassName, "mt-3 max-h-[64vh]")}>
            <table className="w-full min-w-[640px] border-collapse text-left text-[13.5px]">
              <thead className="sticky top-0">
                <tr>
                  <th className={cn(headCellClassName, "w-[56px]")}>STT</th>
                  <th className={headCellClassName}>Buổi học</th>
                  <th className={headCellClassName}>Thời lượng (phút)</th>
                  <th className={headCellClassName}>Số lượng bài tập</th>
                  <th className={headCellClassName}>Số lượng nội dung</th>
                </tr>
              </thead>
              <tbody>
                {lessonRows.map((lesson, index) => (
                  <tr
                    key={lesson.id}
                    className="cursor-pointer hover:bg-cream-100"
                    onClick={() =>
                      void navigate(
                        `/library/templates/${template.template_id}/lessons/${lesson.id}`,
                      )
                    }
                  >
                    <td className={cn(cellClassName, "font-extrabold text-ink-400")}>
                      {index + 1}
                    </td>
                    <td className={cn(cellClassName, "font-extrabold text-ink-900")}>
                      Buổi {index + 1} {lesson.title}
                    </td>
                    <td className={cellClassName}>{lesson.duration_min ?? "—"}</td>
                    <td className={cn(cellClassName, "font-bold")}>{lesson.exercise_count}</td>
                    <td className={cn(cellClassName, "font-bold")}>{lesson.material_count}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )
      ) : detail.isPending ? (
        <HvStateBlock state="loading" title="Đang tải chương trình mẫu" />
      ) : detail.isError ? (
        <HvStateBlock state="error" title="Không tải được chương trình mẫu" />
      ) : sub === "exercises" ? (
        <TemplateExercises lessons={detail.data.lessons} />
      ) : (
        <TemplateMaterials lessons={detail.data.lessons} />
      )}
    </>
  );
}

type VersionLessons = NonNullable<ReturnType<typeof useVersionDetail>["data"]>["lessons"];

function usedLabel(count: number): string {
  return `${count} buổi`;
}

function TemplateExercises({ lessons }: { lessons: VersionLessons }) {
  const rows = new Map<
    string,
    { exercise: VersionLessons[number]["exercises"][number]; used: number }
  >();
  for (const lesson of lessons) {
    for (const exercise of lesson.exercises) {
      const row = rows.get(exercise.id) ?? { exercise, used: 0 };
      row.used += 1;
      rows.set(exercise.id, row);
    }
  }
  return (
    <div className={cn(tableCardClassName, "mt-3")}>
      <table className="w-full min-w-[700px] border-collapse text-left text-[13.5px]">
        <thead>
          <tr>
            <th className={cn(headCellClassName, "w-[56px]")}>STT</th>
            <th className={cn(headCellClassName, "w-[90px]")}>Mã</th>
            <th className={headCellClassName}>Bài tập</th>
            <th className={headCellClassName}>Kỹ năng</th>
            <th className={headCellClassName}>Cấp độ</th>
            <th className={headCellClassName}>Dùng trong</th>
          </tr>
        </thead>
        <tbody>
          {rows.size === 0 ? (
            <tr>
              <td
                colSpan={6}
                className="border-t border-line-100 p-[30px] text-center font-bold text-ink-400"
              >
                Chưa có bài tập mẫu.
              </td>
            </tr>
          ) : null}
          {[...rows.values()].map(({ exercise, used }, index) => (
            <tr key={exercise.id} className="hover:bg-cream-100">
              <td className={cn(cellClassName, "font-extrabold text-ink-400")}>{index + 1}</td>
              <td className={cellClassName}>
                <span className="rounded-[8px] bg-cream-200 px-2 py-1 text-[12px] font-extrabold text-ink-500">
                  {exercise.code}
                </span>
              </td>
              <td className={cn(cellClassName, "font-extrabold text-ink-900")}>{exercise.title}</td>
              <td className={cellClassName}>
                {exercise.skill ? (
                  <span className={cn(pillClassName, "bg-sky-100 text-[11.5px] text-sky-600")}>
                    {exercise.skill}
                  </span>
                ) : (
                  "—"
                )}
              </td>
              <td className={cn(cellClassName, "text-ink-500")}>{exercise.level ?? "—"}</td>
              <td className={cn(cellClassName, "font-bold")}>{usedLabel(used)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function TemplateMaterials({ lessons }: { lessons: VersionLessons }) {
  const rows = new Map<
    string,
    { material: VersionLessons[number]["materials"][number]; used: number }
  >();
  for (const lesson of lessons) {
    for (const material of lesson.materials) {
      const row = rows.get(material.id) ?? { material, used: 0 };
      row.used += 1;
      rows.set(material.id, row);
    }
  }
  return (
    <div className={cn(tableCardClassName, "mt-3")}>
      <table className="w-full min-w-[660px] border-collapse text-left text-[13.5px]">
        <thead>
          <tr>
            <th className={cn(headCellClassName, "w-[56px]")}>STT</th>
            <th className={cn(headCellClassName, "w-[120px]")}>Loại</th>
            <th className={headCellClassName}>Tài liệu</th>
            <th className={headCellClassName}>Định dạng</th>
            <th className={headCellClassName}>Dùng trong</th>
          </tr>
        </thead>
        <tbody>
          {rows.size === 0 ? (
            <tr>
              <td
                colSpan={5}
                className="border-t border-line-100 p-[30px] text-center font-bold text-ink-400"
              >
                Chưa có tài liệu mẫu.
              </td>
            </tr>
          ) : null}
          {[...rows.values()].map(({ material, used }, index) => (
            <tr key={material.id} className="hover:bg-cream-100">
              <td className={cn(cellClassName, "font-extrabold text-ink-400")}>{index + 1}</td>
              <td className={cellClassName}>
                <span className={cn(pillClassName, "bg-cream-200 text-[11.5px] text-ink-700")}>
                  {materialKindLabel[material.kind]}
                </span>
              </td>
              <td className={cn(cellClassName, "font-extrabold text-ink-900")}>{material.title}</td>
              <td className={cn(cellClassName, "text-[12.5px] text-ink-500")}>
                {materialFormatLabel(material.url)}
              </td>
              <td className={cn(cellClassName, "font-bold")}>{usedLabel(used)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

/** Lớp học: every class opened on the course, with the template version it runs. */
function ClassesTab({ course }: { course: Course }) {
  const navigate = useNavigate();
  const { has } = useCenterContext();
  const list = useClassesList({ course_id: course.id, status: "all", per_page: 100 });
  const versions = useVersions(
    has("library.read") ? course.default_template?.template_id : undefined,
  );
  const byClass = versionByClass(versions.data ?? []);
  const today = new Date().toISOString().slice(0, 10);

  if (list.isPending) {
    return <HvStateBlock state="loading" title="Đang tải lớp của khóa" />;
  }
  if (list.isError) {
    return (
      <HvStateBlock
        state="error"
        title="Không tải được lớp của khóa"
        action={
          <HvButton size="sm" variant="ghost" onClick={() => void list.refetch()}>
            Thử lại
          </HvButton>
        }
      />
    );
  }
  const classes = list.data.items;

  return (
    <div className={cn(tableCardClassName, "mt-3")}>
      <table className="w-full min-w-[560px] border-collapse text-left text-[13.5px]">
        <thead>
          <tr>
            <th className={headCellClassName}>Lớp</th>
            <th className={headCellClassName}>Phiên bản</th>
            <th className={headCellClassName}>Lịch</th>
            <th className={headCellClassName}>Học viên</th>
          </tr>
        </thead>
        <tbody>
          {classes.length === 0 ? (
            <tr>
              <td
                colSpan={4}
                className="border-t border-line-100 p-[30px] text-center font-bold text-ink-400"
              >
                Chưa có lớp nào mở từ khóa này.
              </td>
            </tr>
          ) : null}
          {classes.map((klass) => {
            const version = byClass.get(klass.id);
            const inherited = !version || version.id === course.default_template_version_id;
            return (
              <tr
                key={klass.id}
                className="cursor-pointer hover:bg-cream-100"
                onClick={() => void navigate(`/classes/${klass.id}`)}
              >
                <td className={cn(cellClassName, "font-extrabold text-ink-900")}>{klass.name}</td>
                <td className={cellClassName}>
                  {version ? (
                    <span className={cn(pillClassName, versionPill(inherited))}>
                      v{version.version_no}
                      {inherited ? "" : " (override)"}
                    </span>
                  ) : (
                    <span className="text-ink-300">—</span>
                  )}
                </td>
                <td className={cn(cellClassName, "text-ink-500")}>
                  {formatScheduleSummary(klass.schedules, today) || "—"}
                </td>
                <td className={cn(cellClassName, "font-bold")}>{klass.student_count} HV</td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}
