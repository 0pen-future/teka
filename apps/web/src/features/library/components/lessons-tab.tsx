import { SearchIcon } from "lucide-react";
import { useState } from "react";
import { useQueryClient } from "@tanstack/react-query";

import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  HvButton,
  HvConfirmDialog,
  HvIcon,
  HvModal,
  HvSegmented,
  HvStateBlock,
  hvToast,
  type HvSegmentedOption,
} from "@/components/hv";
import { Input } from "@/components/ui/input";
import { ApiError } from "@/lib/api/errors";

import { createLesson } from "../api/library-api";
import {
  invalidateVersionContent,
  useClearLessons,
  useDeleteLesson,
  useDuplicateLesson,
  useLessons,
  useReorderLessons,
} from "../hooks/use-library";
import { useSearch } from "../hooks/use-search";
import { LessonDialog } from "./lesson-form";
import { LessonsTable } from "./lessons-table";
import { LessonsTree } from "./lessons-tree";
import type { TemplateLesson, TemplateVersion } from "../schemas/library-schemas";

function apiMessage(error: unknown, fallback: string): string {
  return error instanceof ApiError ? error.message : fallback;
}

type ViewMode = "table" | "tree";

const viewOptions: HvSegmentedOption<ViewMode>[] = [
  { value: "table", label: "Bảng" },
  { value: "tree", label: "Cây" },
];

const MIN_BULK = 1;
const MAX_BULK = 50;

interface LessonsTabProps {
  version: TemplateVersion;
  templateId: string;
  authoring: boolean;
}

/** The 7-tab page's default tab: a version's lessons, table or tree, with bulk authoring actions. */
export function LessonsTab({ version, templateId, authoring }: LessonsTabProps) {
  const lessons = useLessons(version.id);
  const reorder = useReorderLessons(version.id, templateId);
  const removeLesson = useDeleteLesson(version.id, templateId);
  const duplicate = useDuplicateLesson(version.id, templateId);
  const clearAll = useClearLessons(version.id, templateId);
  const queryClient = useQueryClient();

  const [view, setView] = useState<ViewMode>("table");
  const { query, setQuery, q } = useSearch();
  const [addingLesson, setAddingLesson] = useState(false);
  const [addingMany, setAddingMany] = useState(false);
  const [bulkCount, setBulkCount] = useState("5");
  const [bulkPending, setBulkPending] = useState(false);
  const [clearing, setClearing] = useState(false);
  const [lessonToDelete, setLessonToDelete] = useState<TemplateLesson | null>(null);

  function moveLesson(lesson: TemplateLesson, direction: -1 | 1) {
    const ordered = lessons.data ?? [];
    const index = ordered.findIndex((row) => row.id === lesson.id);
    const target = index + direction;
    if (index < 0 || target < 0 || target >= ordered.length) return;
    const ids = ordered.map((row) => row.id);
    [ids[index], ids[target]] = [ids[target]!, ids[index]!];
    reorder.mutate(ids, {
      onError: (error) =>
        hvToast(apiMessage(error, "Không đổi được thứ tự buổi học."), { variant: "danger" }),
    });
  }

  async function submitBulk() {
    const count = Number(bulkCount);
    if (!Number.isInteger(count) || count < MIN_BULK || count > MAX_BULK) {
      hvToast(`Số buổi phải từ ${MIN_BULK} đến ${MAX_BULK}`, { variant: "danger" });
      return;
    }
    setBulkPending(true);
    try {
      for (let position = 1; position <= count; position += 1) {
        // Lessons must append in order, so each POST waits for the previous one.
        await createLesson(version.id, {
          title: `Buổi ${position}`,
          mode: "scheduled",
          unit: null,
          objectives: null,
          duration_min: null,
          homework_note: null,
        });
      }
      hvToast(`Đã thêm ${count} buổi`);
      setAddingMany(false);
      setBulkCount("5");
    } catch (error) {
      hvToast(apiMessage(error, "Không thêm được buổi học."), { variant: "danger" });
    } finally {
      setBulkPending(false);
      invalidateVersionContent(queryClient, version.id, templateId);
    }
  }

  if (lessons.isPending) return <HvStateBlock state="loading" title="Đang tải buổi học" />;
  if (lessons.isError) {
    return (
      <HvStateBlock
        state="error"
        title="Không tải được buổi học"
        action={
          <HvButton size="sm" variant="ghost" onClick={() => void lessons.refetch()}>
            Thử lại
          </HvButton>
        }
      />
    );
  }

  const allRows = lessons.data ?? [];
  const filtered =
    q === "" ? allRows : allRows.filter((row) => row.title.toLowerCase().includes(q.toLowerCase()));
  const totalMinutes = filtered.reduce((sum, row) => sum + (row.duration_min ?? 0), 0);

  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="flex flex-wrap items-center gap-3">
          <HvSegmented
            aria-label="Chế độ xem buổi học"
            idBase="lessons-view"
            options={viewOptions}
            value={view}
            onValueChange={setView}
          />
          <span className="text-[13px] text-ink-500">
            {filtered.length} buổi · {totalMinutes} phút
          </span>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <div className="relative w-[220px]">
            <SearchIcon
              aria-hidden="true"
              className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-ink-400"
            />
            <Input
              type="search"
              aria-label="Tìm buổi theo tiêu đề…"
              placeholder="Tìm buổi theo tiêu đề…"
              value={query}
              onChange={(event) => setQuery(event.target.value)}
              className="pl-9"
            />
          </div>
          {authoring && allRows.length > 0 ? (
            <HvButton size="sm" variant="ghost" onClick={() => setClearing(true)}>
              Xoá tất cả
            </HvButton>
          ) : null}
          {authoring ? (
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <HvButton size="sm">
                  + Buổi học <HvIcon name="chevron-down" size={16} aria-hidden="true" />
                </HvButton>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end">
                <DropdownMenuItem onSelect={() => setAddingLesson(true)}>
                  Thêm một buổi
                </DropdownMenuItem>
                <DropdownMenuItem onSelect={() => setAddingMany(true)}>
                  Thêm nhiều buổi
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          ) : null}
        </div>
      </div>

      {allRows.length === 0 ? (
        <HvStateBlock state="empty" title="Chưa có buổi mẫu nào — thêm buổi hoặc nhập từ file." />
      ) : filtered.length === 0 ? (
        <HvStateBlock state="empty" title="Không tìm thấy buổi học khớp." />
      ) : view === "table" ? (
        <LessonsTable
          lessons={filtered}
          templateId={templateId}
          editing={
            authoring
              ? {
                  pending: reorder.isPending || removeLesson.isPending || duplicate.isPending,
                  onMove: moveLesson,
                  onDelete: setLessonToDelete,
                  onDuplicate: (lesson) =>
                    duplicate.mutate(lesson.id, {
                      onSuccess: () => hvToast("Đã nhân bản buổi học"),
                      onError: (error) =>
                        hvToast(apiMessage(error, "Không nhân bản được buổi học."), {
                          variant: "danger",
                        }),
                    }),
                }
              : undefined
          }
        />
      ) : (
        <LessonsTree lessons={filtered} templateId={templateId} />
      )}

      <LessonDialog
        open={addingLesson}
        onOpenChange={setAddingLesson}
        versionId={version.id}
        templateId={templateId}
        onCreated={(lesson) => hvToast(`Đã thêm buổi ${lesson.position}`)}
      />

      <HvModal
        open={addingMany}
        onOpenChange={setAddingMany}
        title="Thêm nhiều buổi"
        description="Tạo liên tiếp các buổi mang tiêu đề Buổi 1, Buổi 2… theo số lượng nhập."
        footer={
          <>
            <HvButton type="button" variant="ghost" onClick={() => setAddingMany(false)}>
              Hủy
            </HvButton>
            <HvButton type="button" disabled={bulkPending} onClick={() => void submitBulk()}>
              {bulkPending ? "Đang tạo…" : "Thêm"}
            </HvButton>
          </>
        }
      >
        <Input
          type="number"
          min={MIN_BULK}
          max={MAX_BULK}
          aria-label="Số buổi"
          value={bulkCount}
          onChange={(event) => setBulkCount(event.target.value)}
        />
      </HvModal>

      <HvConfirmDialog
        open={clearing}
        onOpenChange={setClearing}
        title="Xoá tất cả buổi học?"
        description="Toàn bộ buổi học của phiên bản này sẽ bị xoá."
        confirmLabel="Xoá tất cả"
        tone="danger"
        pending={clearAll.isPending}
        onConfirm={() =>
          clearAll.mutate(undefined, {
            onSuccess: () => {
              setClearing(false);
              hvToast("Đã xoá tất cả buổi học");
            },
            onError: (error) => {
              setClearing(false);
              hvToast(apiMessage(error, "Không xoá được buổi học."), { variant: "danger" });
            },
          })
        }
      />

      <HvConfirmDialog
        open={lessonToDelete != null}
        onOpenChange={(open) => {
          if (!open) setLessonToDelete(null);
        }}
        title={`Xoá buổi "${lessonToDelete?.title ?? ""}"?`}
        description="Các buổi phía sau sẽ được đánh số lại."
        confirmLabel="Xoá buổi"
        tone="danger"
        pending={removeLesson.isPending}
        onConfirm={() => {
          if (!lessonToDelete) return;
          removeLesson.mutate(lessonToDelete.id, {
            onSuccess: () => {
              setLessonToDelete(null);
              hvToast("Đã xoá buổi học");
            },
            onError: (error) => {
              setLessonToDelete(null);
              hvToast(apiMessage(error, "Không xoá được buổi học."), { variant: "danger" });
            },
          });
        }}
      />
    </div>
  );
}
