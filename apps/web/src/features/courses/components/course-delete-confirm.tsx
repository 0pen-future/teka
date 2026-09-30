import { HvConfirmDialog, hvToast } from "@/components/hv";
import { ApiError } from "@/lib/api/errors";

import { useDeleteCourse } from "../hooks/use-courses";
import type { Course } from "../schemas/courses-schemas";

export interface CourseDeleteConfirmProps {
  course: Course;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onDeleted: (course: Course) => void;
}

/**
 * Confirms a course delete. The API refuses while classes or learning path
 * stages still point at the course; that refusal closes the prompt and
 * surfaces as a toast carrying the server's advice (archive instead).
 */
export function CourseDeleteConfirm({
  course,
  open,
  onOpenChange,
  onDeleted,
}: CourseDeleteConfirmProps) {
  const remove = useDeleteCourse();
  return (
    <HvConfirmDialog
      open={open}
      onOpenChange={onOpenChange}
      title={`Xoá khóa "${course.name}"?`}
      description={`Xoá vĩnh viễn khóa ${course.code} cùng lịch sử và gói học phí. Chương trình mẫu không bị ảnh hưởng.`}
      confirmLabel="Xoá khóa"
      tone="danger"
      pending={remove.isPending}
      onConfirm={() =>
        remove.mutate(course.id, {
          onSuccess: () => {
            onOpenChange(false);
            onDeleted(course);
          },
          onError: (error) => {
            onOpenChange(false);
            hvToast(error instanceof ApiError ? error.message : "Không xoá được khóa học.", {
              variant: "danger",
            });
          },
        })
      }
    />
  );
}
