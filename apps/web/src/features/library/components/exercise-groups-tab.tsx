import { useState } from "react";

import { HvButton, HvConfirmDialog, HvStateBlock, hvToast } from "@/components/hv";
import { Input } from "@/components/ui/input";
import { ApiError } from "@/lib/api/errors";

import {
  useCreateExerciseGroup,
  useDeleteExerciseGroup,
  useExerciseGroups,
} from "../hooks/use-library";
import type { ExerciseGroup, TemplateVersion } from "../schemas/library-schemas";

function apiMessage(error: unknown, fallback: string): string {
  return error instanceof ApiError ? error.message : fallback;
}

interface ExerciseGroupsTabProps {
  version: TemplateVersion;
  templateId: string;
  authoring: boolean;
}

/** Named buckets exercises attach to within one version, used to group a class's grading rows. */
export function ExerciseGroupsTab({ version, templateId, authoring }: ExerciseGroupsTabProps) {
  const groups = useExerciseGroups(version.id);
  const create = useCreateExerciseGroup(version.id, templateId);
  const remove = useDeleteExerciseGroup(version.id, templateId);
  const [name, setName] = useState("");
  const [deleting, setDeleting] = useState<ExerciseGroup | null>(null);

  function submit() {
    const trimmed = name.trim();
    if (trimmed === "") return;
    create.mutate(
      { name: trimmed },
      {
        onSuccess: () => {
          setName("");
          hvToast("Đã thêm nhóm bài tập");
        },
        onError: (error) =>
          hvToast(apiMessage(error, "Không thêm được nhóm bài tập."), { variant: "danger" }),
      },
    );
  }

  function requestDelete(group: ExerciseGroup) {
    if (group.exercise_count > 0) {
      setDeleting(group);
      return;
    }
    remove.mutate(group.id, {
      onSuccess: () => hvToast("Đã xoá nhóm bài tập"),
      onError: (error) =>
        hvToast(apiMessage(error, "Không xoá được nhóm bài tập."), { variant: "danger" }),
    });
  }

  if (groups.isPending) return <HvStateBlock state="loading" title="Đang tải nhóm bài tập" />;
  if (groups.isError) return <HvStateBlock state="error" title="Không tải được nhóm bài tập" />;

  const rows = groups.data ?? [];

  return (
    <div className="flex flex-col gap-3">
      <p className="text-[13px] text-ink-500">
        Mỗi bài tập trong buổi mẫu gán vào một nhóm. Nhóm gắn theo phiên bản.
      </p>
      {rows.length === 0 ? (
        <p className="text-[14px] text-ink-400">Chưa có nhóm nào.</p>
      ) : (
        <ul className="flex flex-col rounded-[var(--radius-lg)] border border-line-200 bg-white">
          {rows.map((group) => (
            <li
              key={group.id}
              className="flex items-center justify-between gap-2 border-t border-line-100 px-4 py-3 first:border-t-0"
            >
              <span className="font-bold text-ink-900">{group.name}</span>
              <span className="text-[13px] text-ink-500">
                Dùng trong {group.exercise_count} bài
              </span>
              {authoring ? (
                <HvButton
                  type="button"
                  size="sm"
                  variant="ghost"
                  disabled={remove.isPending}
                  onClick={() => requestDelete(group)}
                >
                  Xoá
                </HvButton>
              ) : null}
            </li>
          ))}
        </ul>
      )}
      {authoring ? (
        <div className="flex gap-2">
          <Input
            aria-label="Tên nhóm mới…"
            placeholder="Tên nhóm mới…"
            value={name}
            onChange={(event) => setName(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === "Enter") {
                event.preventDefault();
                submit();
              }
            }}
          />
          <HvButton type="button" size="sm" disabled={create.isPending} onClick={submit}>
            + Thêm nhóm
          </HvButton>
        </div>
      ) : null}
      <HvConfirmDialog
        open={deleting != null}
        onOpenChange={(open) => {
          if (!open) setDeleting(null);
        }}
        title={`Xoá nhóm "${deleting?.name ?? ""}"?`}
        description="Bài tập trong nhóm sẽ về 'Chưa phân nhóm'."
        confirmLabel="Xoá nhóm"
        tone="danger"
        pending={remove.isPending}
        onConfirm={() => {
          if (!deleting) return;
          remove.mutate(deleting.id, {
            onSuccess: () => {
              setDeleting(null);
              hvToast("Đã xoá nhóm bài tập");
            },
            onError: (error) => {
              setDeleting(null);
              hvToast(apiMessage(error, "Không xoá được nhóm bài tập."), { variant: "danger" });
            },
          });
        }}
      />
    </div>
  );
}
