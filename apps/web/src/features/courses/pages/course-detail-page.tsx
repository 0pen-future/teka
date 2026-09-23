import { ArrowLeftIcon } from "lucide-react";
import { useState } from "react";
import { Link, useNavigate, useParams, useSearchParams } from "react-router";
import { z } from "zod";

import {
  HvBadge,
  HvButton,
  HvConfirmDialog,
  HvSegmented,
  HvSelect,
  HvStateBlock,
  hvToast,
  type HvSegmentedOption,
} from "@/components/hv";
import { useTemplatesList, useVersions, versionLabel } from "@/features/library";
import { phaseLabel, phaseVariant, useClassesList } from "@/features/roster";
import { useCenterContext } from "@/features/teaching";
import { ApiError } from "@/lib/api/errors";
import { cn, formatMoney } from "@/lib/utils";

import { CourseDialog } from "../components/course-dialog";
import { TuitionPacksEditor, TuitionPacksReadOnly } from "../components/tuition-packs-editor";
import {
  useArchiveCourse,
  useCourse,
  useDeleteCourse,
  useUpdateCourse,
} from "../hooks/use-courses";
import { useCoursePaths } from "../hooks/use-paths";
import { courseStatusLabel, courseStatusVariant } from "../lib/course-labels";
import { pathStatusLabel, pathStatusVariant } from "../lib/path-labels";
import { courseToInput, type Course } from "../schemas/courses-schemas";

function apiMessage(error: unknown, fallback: string): string {
  return error instanceof ApiError ? error.message : fallback;
}

const tabs = ["info", "template", "settings", "classes"] as const;
type Tab = (typeof tabs)[number];
const tabSchema = z.enum(tabs).catch("info");

const tabOptions: HvSegmentedOption<Tab>[] = [
  { value: "info", label: "Thông tin" },
  { value: "template", label: "Chương trình mẫu" },
  { value: "settings", label: "Gói học phí" },
  { value: "classes", label: "Vận hành" },
];

const TAB_ID_BASE = "course";

/**
 * `/courses/:id` — one course: its commercial facts, the program template
 * classes inherit, the tuition packs and the classes attached to it.
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
            Về danh mục khóa học
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

  const archive = useArchiveCourse(course.id);
  const remove = useDeleteCourse();

  const [editing, setEditing] = useState(false);
  const [archiving, setArchiving] = useState(false);
  const [deleting, setDeleting] = useState(false);

  function selectTab(next: Tab) {
    const params = new URLSearchParams(searchParams);
    if (next === "info") {
      params.delete("tab");
    } else {
      params.set("tab", next);
    }
    setSearchParams(params, { replace: true });
  }

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-col gap-3">
        <Link
          to="/courses"
          className="inline-flex items-center gap-1 self-start font-display text-[13px] font-bold text-ink-500 hover:text-mint-600"
        >
          <ArrowLeftIcon aria-hidden="true" className="size-4" />
          Danh mục khóa học
        </Link>
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div className="min-w-0">
            <h1 className="font-display text-[26px] font-extrabold text-ink-900">{course.name}</h1>
            <div className="mt-1 flex flex-wrap items-center gap-2 text-[14px] text-ink-500">
              <span className="font-mono text-[13px]">Mã: {course.code}</span>
              <HvBadge variant={courseStatusVariant[course.status]} size="sm" dot>
                {courseStatusLabel[course.status]}
              </HvBadge>
            </div>
          </div>
          {canEdit ? (
            <div className="flex flex-wrap gap-2">
              <HvButton size="sm" variant="secondary" onClick={() => setEditing(true)}>
                Sửa
              </HvButton>
              {course.status !== "archived" ? (
                <HvButton size="sm" variant="ghost" onClick={() => setArchiving(true)}>
                  Ngừng tuyển
                </HvButton>
              ) : null}
              <HvButton size="sm" variant="ghost" onClick={() => setDeleting(true)}>
                Xoá khóa học
              </HvButton>
            </div>
          ) : null}
        </div>
      </div>

      <HvSegmented
        variant="tabs"
        idBase={TAB_ID_BASE}
        aria-label="Các mục của khóa học"
        options={tabOptions}
        value={tab}
        onValueChange={selectTab}
      />

      <div
        role="tabpanel"
        id={`${TAB_ID_BASE}-panel-${tab}`}
        aria-labelledby={`${TAB_ID_BASE}-tab-${tab}`}
        className="flex flex-col gap-4"
      >
        {tab === "info" ? (
          <InfoTab course={course} />
        ) : tab === "template" ? (
          <TemplateTab course={course} canEdit={canEdit} />
        ) : tab === "settings" ? (
          canEdit ? (
            <TuitionPacksEditor
              key={course.tuition_packs.map((pack) => pack.id).join(",")}
              courseId={course.id}
              packs={course.tuition_packs}
            />
          ) : (
            <TuitionPacksReadOnly packs={course.tuition_packs} />
          )
        ) : (
          <ClassesTab courseId={course.id} />
        )}
      </div>

      {canEdit ? (
        <>
          <CourseDialog
            mode="edit"
            course={course}
            open={editing}
            onOpenChange={setEditing}
            onSaved={() => hvToast("Đã lưu khóa học")}
          />
          <HvConfirmDialog
            open={archiving}
            onOpenChange={setArchiving}
            title={`Ngừng tuyển khóa "${course.name}"?`}
            description="Khóa học rời khỏi danh sách chọn khi tạo lớp mới. Lớp đang gắn vẫn hoạt động bình thường."
            confirmLabel="Ngừng tuyển"
            pending={archive.isPending}
            onConfirm={() =>
              archive.mutate(undefined, {
                onSuccess: () => {
                  setArchiving(false);
                  hvToast(`Đã ngừng tuyển khóa ${course.code}`);
                },
                onError: (error) =>
                  hvToast(apiMessage(error, "Không ngừng tuyển được khóa học."), {
                    variant: "danger",
                  }),
              })
            }
          />
          <HvConfirmDialog
            open={deleting}
            onOpenChange={setDeleting}
            title={`Xoá khóa "${course.name}"?`}
            description="Chỉ xoá được khóa chưa có lớp nào gắn vào. Gói học phí của khóa bị xoá theo."
            confirmLabel="Xoá khóa học"
            tone="danger"
            pending={remove.isPending}
            onConfirm={() =>
              remove.mutate(course.id, {
                onSuccess: () => {
                  hvToast(`Đã xoá khóa học ${course.code}`);
                  void navigate("/courses");
                },
                onError: (error) => {
                  setDeleting(false);
                  hvToast(apiMessage(error, "Không xoá được khóa học."), { variant: "danger" });
                },
              })
            }
          />
        </>
      ) : null}
    </div>
  );
}

const factLabelClassName = "text-[12px] font-extrabold uppercase tracking-[0.4px] text-ink-500";
const factValueClassName = "mt-1 text-[15px] text-ink-900";

function Fact({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div>
      <div className={factLabelClassName}>{label}</div>
      <div className={factValueClassName}>{children}</div>
    </div>
  );
}

function Dash() {
  return <span className="text-ink-400">—</span>;
}

function InfoTab({ course }: { course: Course }) {
  const { has } = useCenterContext();
  return (
    <>
      <div className="flex flex-col gap-4 rounded-[var(--radius-lg)] border border-line-200 bg-white p-4">
        <dl className="grid grid-cols-2 gap-4 sm:grid-cols-3 lg:grid-cols-4">
          <Fact label="Môn học">{course.subject ?? <Dash />}</Fact>
          <Fact label="Cấp / trình độ">{course.level ?? <Dash />}</Fact>
          <Fact label="Đơn giá / buổi">{formatMoney(course.default_unit_price)}</Fact>
          <Fact label="Tổng số buổi">
            {course.total_sessions === null ? <Dash /> : `${course.total_sessions} buổi`}
          </Fact>
          <Fact label="Thời lượng mỗi buổi">
            {course.duration_min === null ? <Dash /> : `${course.duration_min} phút`}
          </Fact>
          <Fact label="Chương trình mẫu">
            {course.default_template ? (
              <Link
                to={`/library/templates/${course.default_template.template_id}`}
                className="text-mint-600 hover:underline"
              >
                {course.default_template.name} · v{course.default_template.version_no}
              </Link>
            ) : (
              <Dash />
            )}
          </Fact>
          {/* Catalog facts count every class of the center, not only the ones
            the reader may open on the Lớp học tab. */}
          <Fact label="Lớp đang học (toàn trung tâm)">{course.classes_running}</Fact>
          <Fact label="Lớp sắp mở (toàn trung tâm)">{course.classes_upcoming}</Fact>
        </dl>
        {course.description ? (
          <p className="max-w-[720px] text-[14px] whitespace-pre-line text-ink-700">
            {course.description}
          </p>
        ) : null}
      </div>
      {has("paths.read") ? <CoursePathsSection courseId={course.id} /> : null}
    </>
  );
}

/** The learning paths that recommend this course, and at which stage. */
function CoursePathsSection({ courseId }: { courseId: string }) {
  const paths = useCoursePaths(courseId);
  const rows = paths.data ?? [];
  return (
    <section
      aria-labelledby="course-paths-heading"
      className="flex flex-col gap-3 rounded-[var(--radius-lg)] border border-line-200 bg-white p-4"
    >
      <h2 id="course-paths-heading" className={factLabelClassName}>
        Lộ trình học
      </h2>
      {paths.isPending ? (
        <p className="text-[14px] text-ink-500">Đang tải lộ trình…</p>
      ) : paths.isError ? (
        <p className="text-[14px] text-ink-500">Không tải được lộ trình của khóa.</p>
      ) : rows.length === 0 ? (
        <p className="text-[14px] text-ink-500">Chưa nằm trong lộ trình nào.</p>
      ) : (
        <ul className="flex flex-col gap-2">
          {rows.map((row) => (
            <li
              key={`${row.id}-${row.stage_id}`}
              className="flex flex-wrap items-center gap-2 text-[14px]"
            >
              <Link
                to={`/paths/${row.id}`}
                className="font-extrabold text-ink-900 hover:text-mint-600"
              >
                {row.name}
              </Link>
              <span className="text-ink-500">
                Giai đoạn {row.stage_position} · {row.stage_name}
              </span>
              <HvBadge variant={pathStatusVariant[row.status]} size="sm" dot>
                {pathStatusLabel[row.status]}
              </HvBadge>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}

/**
 * Binds the course to one PUBLISHED template version: classes opened on the
 * course inherit it, so a draft (still changing) or archived version would
 * hand them a moving or retired plan.
 */
function TemplateTab({ course, canEdit }: { course: Course; canEdit: boolean }) {
  const { has } = useCenterContext();
  // The pickers read the library, which has its own permission; an editor
  // without it can still drop the current binding.
  const canPick = canEdit && has("library.read");
  const update = useUpdateCourse(course.id);
  const [templateId, setTemplateId] = useState(course.default_template?.template_id ?? "");
  const [versionId, setVersionId] = useState(course.default_template_version_id ?? "");

  const templates = useTemplatesList({ per_page: 100, sort: "name" }, canPick);
  const versions = useVersions(canPick && templateId ? templateId : undefined);
  const published = (versions.data ?? []).filter((version) => version.status === "published");

  const templateOptions = (templates.data?.items ?? []).map((template) => ({
    value: template.id,
    label: template.name,
    meta: template.code,
  }));
  const versionOptions = [...published]
    .sort((a, b) => b.version_no - a.version_no)
    .map((version) => ({ value: version.id, label: versionLabel(version) }));

  function pickTemplate(nextId: string) {
    setTemplateId(nextId);
    setVersionId("");
  }

  function save(nextVersionId: string | null) {
    update.mutate(
      { ...courseToInput(course), default_template_version_id: nextVersionId },
      {
        onSuccess: (saved) => {
          setTemplateId(saved.default_template?.template_id ?? "");
          setVersionId(saved.default_template_version_id ?? "");
          hvToast(nextVersionId ? "Đã gắn chương trình mẫu" : "Đã bỏ chương trình mẫu");
        },
        onError: (error) =>
          hvToast(apiMessage(error, "Không lưu được chương trình mẫu."), { variant: "danger" }),
      },
    );
  }

  const unchanged = (versionId || null) === course.default_template_version_id;
  // The stored id outlives its template: the API keeps it and drops the embed.
  const templateGone = course.default_template_version_id !== null && !course.default_template;
  const versionRetired = course.default_template?.status === "archived";

  return (
    <div className="flex flex-col gap-4 rounded-[var(--radius-lg)] border border-line-200 bg-white p-4">
      <div>
        <div className={factLabelClassName}>Đang gắn</div>
        <div className={factValueClassName}>
          {course.default_template ? (
            <Link
              to={`/library/templates/${course.default_template.template_id}?v=${course.default_template.version_no}`}
              className="text-mint-600 hover:underline"
            >
              {course.default_template.name} · {versionLabel(course.default_template)}
            </Link>
          ) : templateGone ? (
            "Chương trình mẫu đã gắn không còn trong kho; chọn chương trình khác."
          ) : (
            "Chưa gắn chương trình mẫu."
          )}
        </div>
        <p className="mt-1 text-[13px] text-ink-500">
          {versionRetired
            ? "Phiên bản này đã lưu trữ; chọn phiên bản đã phát hành khác cho lớp mới."
            : "Lớp mở trên khóa này dùng phiên bản đã phát hành ở đây làm lộ trình mặc định."}
        </p>
      </div>

      {canEdit ? (
        <div className="flex flex-col gap-3 border-t border-line-100 pt-4">
          {!canPick ? (
            <p className="text-[14px] text-ink-500">
              Cần quyền xem Kho học liệu để chọn chương trình mẫu.
            </p>
          ) : null}
          {canPick ? (
            <div className="flex flex-wrap items-center gap-3">
              <HvSelect
                options={templateOptions}
                value={templateId}
                onValueChange={pickTemplate}
                sheetTitle="Chọn chương trình mẫu"
                placeholder="Chọn chương trình mẫu…"
                searchNoun="chương trình"
                aria-label="Chương trình mẫu"
                disabled={templates.isPending}
                className="min-w-[260px] max-sm:w-full"
              />
              {templateId && !versions.isPending && versionOptions.length > 0 ? (
                <HvSelect
                  options={versionOptions}
                  value={versionId}
                  onValueChange={setVersionId}
                  sheetTitle="Chọn phiên bản"
                  placeholder="Chọn phiên bản…"
                  aria-label="Phiên bản"
                  searchThreshold={Infinity}
                  className="min-w-[200px]"
                />
              ) : null}
            </div>
          ) : null}
          {canPick && templateId && !versions.isPending && versionOptions.length === 0 ? (
            <p className="text-[14px] text-ink-500">
              Chương trình này chưa có phiên bản đã phát hành.
            </p>
          ) : null}
          <div className="flex flex-wrap gap-2">
            {canPick ? (
              <HvButton
                size="sm"
                disabled={versionId === "" || unchanged || update.isPending}
                onClick={() => save(versionId)}
              >
                Lưu chương trình mẫu
              </HvButton>
            ) : null}
            {course.default_template_version_id ? (
              <HvButton
                size="sm"
                variant="ghost"
                disabled={update.isPending}
                onClick={() => save(null)}
              >
                Bỏ chương trình mẫu
              </HvButton>
            ) : null}
          </div>
        </div>
      ) : null}
    </div>
  );
}

const headCellClassName =
  "bg-cream-200 px-[18px] py-[10px] text-[12px] font-extrabold uppercase tracking-[0.4px] text-ink-500";
const cellClassName = "border-t border-line-100 px-[18px] py-[11px] align-middle";

function ClassesTab({ courseId }: { courseId: string }) {
  const list = useClassesList({ course_id: courseId, status: "all", per_page: 100 });
  const classes = list.data?.items ?? [];

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
  if (classes.length === 0) {
    return (
      <HvStateBlock
        state="empty"
        title="Chưa có lớp nào gắn với khóa này."
        description="Chọn khóa học trong hộp thoại Tạo lớp để gắn."
      />
    );
  }

  return (
    <div className="overflow-x-auto rounded-[var(--radius-lg)] border border-line-200 bg-white">
      <table className="w-full min-w-[560px] border-collapse text-left text-[14px]">
        <thead>
          <tr>
            <th className={headCellClassName}>Lớp</th>
            <th className={headCellClassName}>Mã lớp</th>
            <th className={headCellClassName}>Khai giảng</th>
            <th className={headCellClassName}>Học sinh</th>
            <th className={headCellClassName}>Giai đoạn</th>
          </tr>
        </thead>
        <tbody>
          {classes.map((klass) => (
            <tr key={klass.id} className="transition-colors hover:bg-cream-100">
              <td className={cn(cellClassName, "font-extrabold text-ink-900")}>
                <Link to={`/classes/${klass.id}`} className="hover:text-mint-600">
                  {klass.name}
                </Link>
              </td>
              <td className={cn(cellClassName, "font-mono text-[13px] text-ink-700")}>
                {klass.code}
              </td>
              <td className={cn(cellClassName, "text-ink-700")}>{klass.start_date}</td>
              <td className={cn(cellClassName, "text-ink-700")}>{klass.student_count}</td>
              <td className={cellClassName}>
                <HvBadge variant={phaseVariant[klass.phase]} size="sm" dot>
                  {phaseLabel[klass.phase]}
                </HvBadge>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
