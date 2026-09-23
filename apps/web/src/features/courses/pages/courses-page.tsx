import { SearchIcon } from "lucide-react";
import { useEffect, useState } from "react";
import { Link, useNavigate, useSearchParams } from "react-router";
import { z } from "zod";

import {
  HvBadge,
  HvButton,
  HvSegmented,
  HvStateBlock,
  hvToast,
  type HvSegmentedOption,
} from "@/components/hv";
import { Input } from "@/components/ui/input";
import { useCenterContext } from "@/features/teaching";
import { cn } from "@/lib/utils";

import { CourseDialog } from "../components/course-dialog";
import { useCoursesList } from "../hooks/use-courses";
import { courseStatusLabel, courseStatusVariant } from "../lib/course-labels";
import { courseStatusSchema, type Course } from "../schemas/courses-schemas";

type StatusFilter = "all" | z.infer<typeof courseStatusSchema>;

const statusFilterSchema = z.union([z.literal("all"), courseStatusSchema]).catch("all");

const statusOptions: HvSegmentedOption<StatusFilter>[] = [
  { value: "all", label: "Tất cả" },
  { value: "draft", label: courseStatusLabel.draft },
  { value: "active", label: courseStatusLabel.active },
  { value: "archived", label: courseStatusLabel.archived },
];

const STATUS_ID_BASE = "courses-status";

const headCellClassName =
  "sticky top-0 z-10 bg-cream-200 px-[18px] py-[10px] text-[12px] font-extrabold uppercase tracking-[0.4px] text-ink-500";
const cellClassName = "border-t border-line-100 px-[18px] py-[11px] align-middle";

function classesSummary(course: Course): string | null {
  const parts: string[] = [];
  if (course.classes_running > 0) parts.push(`${course.classes_running} đang học`);
  if (course.classes_upcoming > 0) parts.push(`${course.classes_upcoming} sắp mở`);
  return parts.length > 0 ? parts.join(" · ") : null;
}

/**
 * `/courses` — the center's course catalog: what it sells, at what price,
 * built on which program template. Classes hang off a course; the class
 * dialog reads the active ones from the same endpoint.
 */
export function CoursesPage() {
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();
  const status: StatusFilter = statusFilterSchema.parse(searchParams.get("status") ?? undefined);
  const { has, isResolved } = useCenterContext();
  const canEdit = has("courses.edit");

  const [query, setQuery] = useState("");
  const [q, setQ] = useState("");
  const [creating, setCreating] = useState(false);

  useEffect(() => {
    const timer = setTimeout(() => setQ(query.trim()), 300);
    return () => clearTimeout(timer);
  }, [query]);

  const list = useCoursesList(
    {
      status: status === "all" ? undefined : status,
      q: q || undefined,
      per_page: 100,
      sort: "name",
    },
    isResolved,
  );
  const courses = list.data?.items ?? [];

  function selectStatus(next: StatusFilter) {
    const params = new URLSearchParams(searchParams);
    if (next === "all") {
      params.delete("status");
    } else {
      params.set("status", next);
    }
    setSearchParams(params, { replace: true });
  }

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h1 className="font-display text-[26px] font-extrabold text-ink-900">
            Danh mục khóa học
          </h1>
          <p className="mt-1 text-[14px] text-ink-500">
            Khóa học là sản phẩm trung tâm bán: giá mỗi buổi, gói học phí và chương trình mẫu đi
            kèm. Mỗi lớp mở ra gắn với một khóa.
          </p>
        </div>
        {canEdit ? (
          <HvButton size="sm" onClick={() => setCreating(true)}>
            Tạo khóa học
          </HvButton>
        ) : null}
      </div>

      <HvSegmented
        variant="tabs"
        idBase={STATUS_ID_BASE}
        aria-label="Lọc theo trạng thái"
        options={statusOptions}
        value={status}
        onValueChange={selectStatus}
      />

      <div className="relative">
        <SearchIcon
          aria-hidden="true"
          className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-ink-400"
        />
        <Input
          type="search"
          aria-label="Tìm khóa học"
          placeholder="Tìm theo tên hoặc mã…"
          value={query}
          onChange={(event) => setQuery(event.target.value)}
          className="pl-9"
        />
      </div>

      {!isResolved || list.isPending ? (
        <HvStateBlock state="loading" title="Đang tải danh mục khóa học" />
      ) : list.isError ? (
        <HvStateBlock
          state="error"
          title="Không tải được danh mục khóa học"
          action={
            <HvButton size="sm" variant="ghost" onClick={() => void list.refetch()}>
              Thử lại
            </HvButton>
          }
        />
      ) : courses.length === 0 ? (
        <HvStateBlock
          state="empty"
          title={
            q
              ? "Không có khóa học nào khớp từ khoá."
              : status !== "all"
                ? "Không có khóa học nào ở trạng thái này."
                : "Chưa có khóa học nào."
          }
          description={
            q
              ? "Đổi từ khoá hoặc xoá ô tìm."
              : status !== "all"
                ? "Chọn Tất cả để xem toàn bộ danh mục."
                : canEdit
                  ? "Tạo khóa đầu tiên bằng nút Tạo khóa học."
                  : "Người có quyền quản lý khóa học sẽ thêm khóa tại đây."
          }
        />
      ) : (
        <div className="overflow-x-auto rounded-[var(--radius-lg)] border border-line-200 bg-white">
          <table className="w-full min-w-[840px] border-collapse text-left text-[14px]">
            <thead>
              <tr>
                <th className={headCellClassName}>Mã</th>
                <th className={headCellClassName}>Tên khóa học</th>
                <th className={headCellClassName}>Môn</th>
                <th className={headCellClassName}>Cấp</th>
                <th className={headCellClassName}>Chương trình mẫu</th>
                <th className={headCellClassName}>Lớp</th>
                <th className={headCellClassName}>Trạng thái</th>
              </tr>
            </thead>
            <tbody>
              {courses.map((course) => {
                const classes = classesSummary(course);
                return (
                  <tr key={course.id} className="transition-colors hover:bg-cream-100">
                    <td className={cn(cellClassName, "font-mono text-[13px] text-ink-700")}>
                      {course.code}
                    </td>
                    <td className={cn(cellClassName, "font-extrabold text-ink-900")}>
                      <Link to={`/courses/${course.id}`} className="hover:text-mint-600">
                        {course.name}
                      </Link>
                    </td>
                    <td className={cn(cellClassName, "text-ink-700")}>
                      {course.subject ?? <span className="text-ink-400">—</span>}
                    </td>
                    <td className={cn(cellClassName, "text-ink-700")}>
                      {course.level ?? <span className="text-ink-400">—</span>}
                    </td>
                    <td className={cn(cellClassName, "text-ink-700")}>
                      {course.default_template ? (
                        `${course.default_template.name} · v${course.default_template.version_no}`
                      ) : (
                        <span className="text-ink-400">—</span>
                      )}
                    </td>
                    <td className={cn(cellClassName, "text-ink-700")}>
                      {classes ?? <span className="text-ink-400">Chưa có lớp</span>}
                    </td>
                    <td className={cellClassName}>
                      <HvBadge variant={courseStatusVariant[course.status]} size="sm" dot>
                        {courseStatusLabel[course.status]}
                      </HvBadge>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}

      {canEdit ? (
        <CourseDialog
          mode="create"
          open={creating}
          onOpenChange={setCreating}
          onCreated={(course) => {
            hvToast(`Đã tạo khóa học ${course.code}`);
            void navigate(`/courses/${course.id}`);
          }}
        />
      ) : null}
    </div>
  );
}
