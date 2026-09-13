import { useState } from "react";
import { Navigate } from "react-router";

import { HvButton, HvSegmented, HvStateBlock } from "@/components/hv";
import { useAuthStore } from "@/features/auth/stores/auth-store";
import { useCenterContext } from "@/features/teaching";
import { useMediaQuery } from "@/lib/hooks/use-media-query";
import type { ColumnId, TaskId } from "@/lib/kanban";
import { useKanban } from "@/lib/kanban";

import { BoardDesktop } from "../components/board-desktop";
import { BoardMobile } from "../components/board-mobile";
import { BoardSettingsModal } from "../components/board-settings-modal";
import { TaskFormModal } from "../components/task-form-modal";
import { useMemberDirectory } from "../hooks/use-member-directory";
import { useTaskBoard } from "../hooks/use-task-board";
import { useTasksDataSource, type AppTask } from "../hooks/use-tasks-data-source";
import type { BoardScope } from "../schemas/task-schemas";

const SCOPE_OPTIONS: { value: BoardScope; label: string }[] = [
  { value: "mine", label: "Của tôi" },
  { value: "center", label: "Toàn trung tâm" },
];

/** Vietnamese copy for the headless lib's `[`/`]` keyboard-move announcement (see `src/lib/kanban/README.md` "Locale"). */
const KEYBOARD_MESSAGES = {
  moved: (columnName: string) => `Đã chuyển việc sang cột ${columnName}.`,
  moveFailed: "Không chuyển được việc.",
};

interface TaskFormTarget {
  task?: AppTask;
  defaultColumnId?: ColumnId;
}

/**
 * Task board for holders of `tasks.list`. Column moves go exclusively
 * through the move menu / keyboard shortcuts (no drag-and-drop in v1) — see
 * `use-tasks-data-source.ts` for why the optimistic update always places a
 * moved task at the top of its destination column.
 */
export function TaskBoardPage() {
  const { has, isResolved, isError } = useCenterContext();
  const currentUserId = useAuthStore((state) => state.user)?.id;
  const isDesktop = useMediaQuery("(min-width: 768px)");

  const canList = has("tasks.list");
  const canViewAll = has("tasks.view_all");
  const canCreate = has("tasks.create");
  const canEdit = has("tasks.edit");
  const canDelete = has("tasks.delete");
  const canManageBoard = has("tasks.manage_board");

  const [requestedScope, setRequestedScope] = useState<BoardScope>("mine");
  const scope: BoardScope = canViewAll ? requestedScope : "mine";

  const boardQuery = useTaskBoard({ scope });
  const membersQuery = useMemberDirectory();
  const dataSourceResult = useTasksDataSource({ scope });
  const { dataSource, ...mutations } = dataSourceResult;

  const [announcement, setAnnouncement] = useState("");
  const [formTarget, setFormTarget] = useState<TaskFormTarget | undefined>(undefined);
  const [settingsOpen, setSettingsOpen] = useState(false);

  const board = boardQuery.data?.board;
  const effectiveScope = boardQuery.data?.scope ?? scope;

  const kanban = useKanban<AppTask>({
    board: board ?? { columns: [], tasks: [] },
    dataSource,
    messages: KEYBOARD_MESSAGES,
  });

  if (!isResolved && !isError) {
    return null;
  }
  if (!canList) {
    return <Navigate to="/" replace />;
  }

  const members = membersQuery.data ?? [];
  const assigneeNameFor = (assigneeId: string | null): string | null => {
    if (!assigneeId) return null;
    return members.find((member) => member.teacher_id === assigneeId)?.display_name ?? null;
  };

  const handleOpenTask = (taskId: TaskId) => {
    const task = kanban.board.tasks.find((candidate) => candidate.id === taskId);
    if (task) {
      setFormTarget({ task });
    }
  };

  const handleCreateTask = (columnId: ColumnId) => {
    setFormTarget({ defaultColumnId: columnId });
  };

  const handleMoveTask = (taskId: TaskId, columnId: ColumnId) => {
    const task = kanban.board.tasks.find((candidate) => candidate.id === taskId);
    void kanban.moveTask(taskId, columnId, 0).then(() => {
      if (task) {
        const target = kanban.board.columns.find((column) => column.id === columnId);
        setAnnouncement(`Đã chuyển "${task.title}" sang cột ${target?.name ?? ""}.`);
      }
    });
  };

  const boardContent = boardQuery.isPending ? (
    <HvStateBlock state="loading" title="Đang tải bảng công việc…" />
  ) : boardQuery.isError ? (
    <HvStateBlock state="error" title="Không tải được bảng công việc" description="Thử lại sau." />
  ) : !board || board.columns.length === 0 ? (
    <HvStateBlock
      state="empty"
      title="Chưa có cột nào"
      description={
        canManageBoard ? "Mở cấu hình cột để tạo cột đầu tiên." : "Chưa có cột nào được tạo."
      }
    />
  ) : isDesktop ? (
    <BoardDesktop
      columns={board.columns}
      tasksByColumn={kanban.tasksByColumn}
      assigneeNameFor={assigneeNameFor}
      currentUserId={currentUserId}
      canCreate={canCreate}
      canMove={canEdit}
      onOpenTask={handleOpenTask}
      onCreateTask={handleCreateTask}
      onMoveTask={handleMoveTask}
      getColumnProps={kanban.getColumnProps}
      getTaskProps={kanban.getTaskProps}
    />
  ) : (
    <BoardMobile
      columns={board.columns}
      tasksByColumn={kanban.tasksByColumn}
      assigneeNameFor={assigneeNameFor}
      currentUserId={currentUserId}
      canCreate={canCreate}
      canMove={canEdit}
      onOpenTask={handleOpenTask}
      onCreateTask={handleCreateTask}
      onMoveTask={handleMoveTask}
      getColumnProps={kanban.getColumnProps}
      getTaskProps={kanban.getTaskProps}
    />
  );

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h1 className="font-display text-[22px] font-extrabold text-ink-900">Công việc</h1>
        <div className="flex items-center gap-2">
          {canViewAll ? (
            <HvSegmented
              aria-label="Phạm vi bảng công việc"
              value={effectiveScope}
              onValueChange={setRequestedScope}
              options={SCOPE_OPTIONS}
            />
          ) : null}
          {canManageBoard ? (
            <HvButton
              type="button"
              variant="secondary"
              size="sm"
              onClick={() => setSettingsOpen(true)}
            >
              Cấu hình cột
            </HvButton>
          ) : null}
        </div>
      </div>

      {boardContent}

      <div aria-live="polite" role="status" className="sr-only">
        {announcement || kanban.announcement}
      </div>

      {formTarget ? (
        <TaskFormModal
          open
          onOpenChange={(open) => {
            if (!open) setFormTarget(undefined);
          }}
          columns={board?.columns ?? []}
          members={members}
          currentUserId={currentUserId}
          canDelete={canDelete}
          task={formTarget.task}
          defaultColumnId={formTarget.defaultColumnId}
          createTaskMutation={mutations.createTaskMutation}
          updateTaskMutation={mutations.updateTaskMutation}
          deleteTaskMutation={mutations.deleteTaskMutation}
          onAnnounce={setAnnouncement}
        />
      ) : null}

      {canManageBoard && board ? (
        <BoardSettingsModal
          open={settingsOpen}
          onOpenChange={setSettingsOpen}
          columns={board.columns}
          tasksByColumn={kanban.tasksByColumn}
          createColumnMutation={mutations.createColumnMutation}
          updateColumnMutation={mutations.updateColumnMutation}
          deleteColumnMutation={mutations.deleteColumnMutation}
          reorderColumn={kanban.reorderColumn}
          onAnnounce={setAnnouncement}
        />
      ) : null}
    </div>
  );
}
