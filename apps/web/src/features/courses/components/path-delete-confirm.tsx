import { HvConfirmDialog, hvToast } from "@/components/hv";
import { ApiError } from "@/lib/api/errors";

import { useDeletePath } from "../hooks/use-paths";
import type { LearningPath } from "../schemas/paths-schemas";

export interface PathDeleteConfirmProps {
  path: LearningPath;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onDeleted: (path: LearningPath) => void;
}

/**
 * Confirms a path delete. Courses are only recommended by stages, so they
 * stay in the catalog; a failure closes the prompt and surfaces as a toast.
 */
export function PathDeleteConfirm({ path, open, onOpenChange, onDeleted }: PathDeleteConfirmProps) {
  const remove = useDeletePath();
  return (
    <HvConfirmDialog
      open={open}
      onOpenChange={onOpenChange}
      title={`Xoá lộ trình "${path.name}"?`}
      description={`Xoá lộ trình cùng ${path.stage_count} chặng bên trong. Khóa học vẫn giữ nguyên trong danh mục.`}
      confirmLabel="Xoá lộ trình"
      tone="danger"
      pending={remove.isPending}
      onConfirm={() =>
        remove.mutate(path.id, {
          onSuccess: () => {
            onOpenChange(false);
            onDeleted(path);
          },
          onError: (error) => {
            onOpenChange(false);
            hvToast(error instanceof ApiError ? error.message : "Không xoá được lộ trình.", {
              variant: "danger",
            });
          },
        })
      }
    />
  );
}
