import { useState } from "react";
import { useNavigate, useSearchParams } from "react-router";
import { z } from "zod";

import { HvBadge, HvButton, HvChip, HvStateBlock, hvToast } from "@/components/hv";
import { useCenterContext } from "@/features/teaching";
import { cn } from "@/lib/utils";

import { CourseDialog } from "../components/course-dialog";
import { useCoursesList } from "../hooks/use-courses";
import { usePathsList } from "../hooks/use-paths";
import {
  copyCourseCode,
  courseStatusLabel,
  courseStatusVariant,
  templateLabel,
} from "../lib/course-labels";
import {
  cellClassName,
  formatVnd,
  headCellClassName,
  mintActionClassName,
  skyActionClassName,
  tableCardClassName,
} from "../lib/table-classes";
import type { Course } from "../schemas/courses-schemas";
import type { LearningPath } from "../schemas/paths-schemas";

/** The prototype's strip: drafts only show under "Tất cả". */
type StatusFilter = "all" | "active" | "archived";

const statusFilterSchema = z.enum(["all", "active", "archived"]).catch("all");

const filters: { value: StatusFilter; label: string }[] = [
  { value: "all", label: "Tất cả" },
  { value: "active", label: courseStatusLabel.active },
  { value: "archived", label: courseStatusLabel.archived },
];

/** Course id → the stage names recommending it, across every path. */
function stageNamesByCourse(paths: LearningPath[]): Map<string, string[]> {
  const names = new Map<string, string[]>();
  for (const path of paths) {
    for (const stage of path.stages) {
      for (const course of stage.courses) {
        const held = names.get(course.id) ?? [];
        if (!held.includes(stage.name)) held.push(stage.name);
        names.set(course.id, held);
      }
    }
  }
  return names;
}

/**
 * `/courses` — the prototype's "Khóa học" table. The catalog loads once so
 * the chips carry counts and the search filters as you type; the CHẶNG
 * column is read off the learning paths, which embed their stage courses.
 */
export function CoursesPage() {
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();
  const status: StatusFilter = statusFilterSchema.parse(searchParams.get("status") ?? undefined);
  const { has, isResolved } = useCenterContext();
  const canEdit = has("courses.edit");

  const [query, setQuery] = useState("");
  const [creating, setCreating] = useState(false);
  const [editing, setEditing] = useState<Course | null>(null);

  const list = useCoursesList({ per_page: 100, sort: "name" }, isResolved);
  const paths = usePathsList({ per_page: 100 }, isResolved && has("paths.read"));
  const stages = stageNamesByCourse(paths.data?.items ?? []);
  const all = list.data?.items ?? [];
  const q = query.trim().toLowerCase();
  const rows = all.filter(
    (course) =>
      (status === "all" || course.status === status) &&
      (!q || course.name.toLowerCase().includes(q) || course.code.toLowerCase().includes(q)),
  );

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
      <div className="flex flex-wrap items-start gap-3">
        <div className="min-w-[260px] flex-1">
          <h1 className="font-display text-[26px] font-extrabold text-ink-900">Khóa học</h1>
          <p className="mt-1 text-[14px] text-ink-500">
            Khóa học gắn một phiên bản chương trình mẫu làm mặc định. Lịch dạy, giáo viên, học phí
            thuộc khóa/lớp — không thuộc template.
          </p>
        </div>
        {canEdit ? <HvButton onClick={() => setCreating(true)}>+ Tạo khóa học</HvButton> : null}
      </div>

      <div className="flex flex-wrap items-center gap-2">
        <div role="radiogroup" aria-label="Lọc theo trạng thái" className="flex flex-wrap gap-2">
          {filters.map((filter) => (
            <HvChip
              key={filter.value}
              role="radio"
              size="sm"
              pressed={filter.value === status}
              count={
                list.data
                  ? filter.value === "all"
                    ? all.length
                    : all.filter((course) => course.status === filter.value).length
                  : undefined
              }
              onClick={() => selectStatus(filter.value)}
            >
              {filter.label}
            </HvChip>
          ))}
        </div>
        <input
          type="search"
          aria-label="Tìm khóa học"
          placeholder="Tìm theo tên, mã khóa học…"
          value={query}
          onChange={(event) => setQuery(event.target.value)}
          className="ml-auto w-[240px] rounded-[14px] border-2 border-line-200 bg-white px-3 py-[9px] text-[13.5px] outline-none focus:border-mint-400 max-sm:ml-0 max-sm:w-full"
        />
      </div>

      {!isResolved || list.isPending ? (
        <HvStateBlock state="loading" title="Đang tải khóa học" />
      ) : list.isError ? (
        <HvStateBlock
          state="error"
          title="Không tải được khóa học"
          action={
            <HvButton size="sm" variant="ghost" onClick={() => void list.refetch()}>
              Thử lại
            </HvButton>
          }
        />
      ) : (
        <div className={tableCardClassName}>
          <table className="w-full min-w-[1000px] table-fixed border-collapse text-left text-[13.5px]">
            <colgroup>
              <col className="w-[44px]" />
              <col className="w-[120px]" />
              <col className="w-[17%]" />
              <col className="w-[9%]" />
              <col className="w-[8%]" />
              <col className="w-[9%]" />
              <col className="w-[9%]" />
              <col className="w-[13%]" />
              <col className="w-[9%]" />
              <col className="w-[130px]" />
            </colgroup>
            <thead>
              <tr>
                <th className={headCellClassName}>STT</th>
                <th className={headCellClassName}>Mã</th>
                <th className={headCellClassName}>Khóa học</th>
                <th className={headCellClassName}>Chặng</th>
                <th className={headCellClassName}>Thời lượng</th>
                <th className={headCellClassName}>Giá/buổi học</th>
                <th className={headCellClassName}>Lớp đang diễn ra</th>
                <th className={headCellClassName}>Chương trình mẫu</th>
                <th className={headCellClassName}>Trạng thái</th>
                <th className={headCellClassName}>
                  <span className="sr-only">Thao tác</span>
                </th>
              </tr>
            </thead>
            <tbody>
              {rows.length === 0 ? (
                <tr>
                  <td
                    colSpan={10}
                    className="border-t border-line-100 p-[30px] text-center font-bold text-ink-400"
                  >
                    Không có khóa nào khớp.
                  </td>
                </tr>
              ) : null}
              {rows.map((course, index) => (
                <tr
                  key={course.id}
                  className="cursor-pointer transition-colors hover:bg-cream-100"
                  onClick={() => void navigate(`/courses/${course.id}`)}
                >
                  <td className={cn(cellClassName, "font-extrabold text-ink-400")}>{index + 1}</td>
                  <td className={cn(cellClassName, "text-[12.5px] font-extrabold text-ink-500")}>
                    {course.code}
                  </td>
                  <td className={cn(cellClassName, "font-extrabold text-ink-900")}>
                    {course.name}
                  </td>
                  <td className={cn(cellClassName, "text-ink-500")}>
                    {stages.get(course.id)?.join(", ") ?? "—"}
                  </td>
                  <td className={cn(cellClassName, "text-ink-700")}>
                    {course.duration_min === null ? "—" : `${course.duration_min} phút`}
                  </td>
                  <td className={cn(cellClassName, "font-bold")}>
                    {formatVnd(course.default_unit_price)}
                  </td>
                  <td
                    className={cn(
                      cellClassName,
                      "font-extrabold",
                      course.classes_running > 0 ? "text-mint-600" : "text-ink-300",
                    )}
                  >
                    {course.classes_running > 0 ? `${course.classes_running} lớp` : "—"}
                  </td>
                  <td className={cn(cellClassName, "text-[12.5px] text-ink-500")}>
                    {templateLabel(course)}
                  </td>
                  <td className={cellClassName}>
                    <HvBadge variant={courseStatusVariant[course.status]} size="sm">
                      {courseStatusLabel[course.status]}
                    </HvBadge>
                  </td>
                  <td className={cellClassName}>
                    <div className="flex justify-end gap-1.5">
                      <button
                        type="button"
                        title="Sao chép mã"
                        aria-label={`Sao chép mã ${course.code}`}
                        className={skyActionClassName}
                        onClick={(event) => {
                          event.stopPropagation();
                          void copyCourseCode(course.code);
                        }}
                      >
                        Mã
                      </button>
                      {canEdit ? (
                        <button
                          type="button"
                          aria-label={`Sửa ${course.name}`}
                          className={mintActionClassName}
                          onClick={(event) => {
                            event.stopPropagation();
                            setEditing(course);
                          }}
                        >
                          Sửa
                        </button>
                      ) : null}
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {canEdit ? (
        <>
          <CourseDialog
            mode="create"
            open={creating}
            onOpenChange={setCreating}
            onCreated={(course) => {
              hvToast(`Đã tạo khóa học ${course.code}`);
              void navigate(`/courses/${course.id}`);
            }}
          />
          {editing ? (
            <CourseDialog
              mode="edit"
              course={editing}
              open
              onOpenChange={(next) => {
                if (!next) setEditing(null);
              }}
              onSaved={() => hvToast("Đã lưu khóa học")}
            />
          ) : null}
        </>
      ) : null}
    </div>
  );
}
