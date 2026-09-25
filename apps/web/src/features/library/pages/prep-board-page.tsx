import { ArrowLeftIcon, CalendarIcon, UserRoundIcon } from "lucide-react";
import { useMemo, useState } from "react";
import { Link, useNavigate, useParams } from "react-router";

import { HvBadge, HvNotice, HvStateBlock, hvToast } from "@/components/hv";
import { ActionsMenu } from "@/features/tasks";
import { useCenterContext } from "@/features/teaching";
import { ApiError } from "@/lib/api/errors";
import type { ColumnId, KanbanColumn, KanbanDataSource, TaskId, TaskProps } from "@/lib/kanban";
import { useKanban } from "@/lib/kanban";
import { cn } from "@/lib/utils";
import { formatDayMonth } from "@/lib/utils/format";

import { usePrepBoard, usePrepDataSource, type PrepCard } from "../hooks/use-prep";
import { versionLabel, versionStatusVariant } from "../lib/library-labels";
import type { TemplateVersion } from "../schemas/library-schemas";

/** Vietnamese copy for the headless lib's `[`/`]` keyboard-move announcement. */
const KEYBOARD_MESSAGES = {
  moved: (columnName: string) => `Đã chuyển buổi sang cột ${columnName}.`,
  moveFailed: "Không chuyển được buổi.",
};

const MOVE_FAILED_TOAST = "Không chuyển được buổi, vui lòng thử lại.";

function lockedNotice(version: TemplateVersion, canEdit: boolean): string | null {
  if (version.status === "published") {
    return `Phiên bản v${version.version_no} đã phát hành, trạng thái chuẩn bị được khoá.`;
  }
  if (version.status === "archived") {
    return `Phiên bản v${version.version_no} đã lưu trữ, trạng thái chuẩn bị được khoá.`;
  }
  return canEdit ? null : "Bạn không có quyền đổi trạng thái chuẩn bị.";
}

/**
 * `/prep/:vid/board` — one version's lessons as cards across the four prep
 * statuses. A card moves through its menu or the lib's `[`/`]` shortcut;
 * both end in `kanban.moveTask`, which PATCHes the lesson's status. On a
 * locked version, or for a viewer without `library.edit`, the data source
 * refuses every move so the keyboard path announces the failure instead of
 * silently doing nothing.
 */
export function PrepBoardPage() {
  const { vid = "" } = useParams<{ vid: string }>();
  const { has, isResolved, isError } = useCenterContext();
  const canEdit = has("library.edit");
  const canAssign = has("prep.assign");
  const boardQuery = usePrepBoard(vid);
  const baseDataSource = usePrepDataSource(vid);

  const version = boardQuery.data?.version;
  const editable = canEdit && version?.status === "draft";

  const dataSource = useMemo<KanbanDataSource<PrepCard>>(
    () =>
      editable
        ? baseDataSource
        : {
            ...baseDataSource,
            moveTask: () =>
              // eslint-disable-next-line @typescript-eslint/prefer-promise-reject-errors -- the lib's plain `KanbanError` union, as in use-prep.ts
              Promise.reject({ kind: "forbidden" } as const),
          },
    [baseDataSource, editable],
  );

  const kanban = useKanban<PrepCard, KanbanColumn>({
    board: boardQuery.data?.board ?? { columns: [], tasks: [] },
    dataSource,
    messages: KEYBOARD_MESSAGES,
  });
  const [announcement, setAnnouncement] = useState("");
  const navigate = useNavigate();

  if (!isResolved && !isError) {
    return <HvStateBlock state="loading" title="Đang tải bảng chuẩn bị" />;
  }
  if (!has("library.read")) {
    return <HvStateBlock state="error" title="Bạn không có quyền xem mục này" />;
  }
  if (boardQuery.isPending) {
    return <HvStateBlock state="loading" title="Đang tải bảng chuẩn bị" />;
  }
  if (boardQuery.isError || !boardQuery.data) {
    const notFound = boardQuery.error instanceof ApiError && boardQuery.error.status === 404;
    return (
      <HvStateBlock
        state="error"
        title={notFound ? "Không tìm thấy phiên bản" : "Không tải được bảng chuẩn bị"}
        description={
          notFound ? "Phiên bản có thể đã bị xoá hoặc đường dẫn không đúng." : "Thử tải lại trang."
        }
        action={
          <Link
            to="/prep"
            className="font-display text-[13px] font-bold text-mint-600 hover:underline"
          >
            Về danh sách chuẩn bị
          </Link>
        }
      />
    );
  }

  const { template } = boardQuery.data;
  const currentVersion = boardQuery.data.version;
  const notice = lockedNotice(currentVersion, canEdit);

  const moveAndAnnounce = (taskId: TaskId, columnId: ColumnId) => {
    const card = kanban.board.tasks.find((candidate) => candidate.id === taskId);
    const target = kanban.board.columns.find((column) => column.id === columnId);
    void kanban.moveTask(taskId, columnId, 0).then(
      () => {
        if (card) {
          setAnnouncement(`Đã chuyển "${card.title}" sang cột ${target?.name ?? ""}.`);
        }
      },
      () => {
        hvToast(MOVE_FAILED_TOAST, { variant: "danger" });
        setAnnouncement(KEYBOARD_MESSAGES.moveFailed);
      },
    );
  };

  const openLesson = (card: PrepCard) => {
    void navigate(`/library/templates/${template.id}/lessons/${String(card.id)}`);
  };

  const columns = [...kanban.board.columns].sort((a, b) => a.order - b.order);

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-col gap-3">
        <Link
          to="/prep"
          className="inline-flex items-center gap-1 self-start font-display text-[13px] font-bold text-ink-500 hover:text-mint-600"
        >
          <ArrowLeftIcon aria-hidden="true" className="size-4" />
          Chuẩn bị tài liệu
        </Link>
        <div className="flex flex-wrap items-center justify-between gap-2">
          <div className="flex flex-wrap items-center gap-2">
            <h1 className="font-display text-[26px] font-extrabold text-ink-900">
              Bảng chuẩn bị · {template.name}
            </h1>
            <HvBadge variant={versionStatusVariant[currentVersion.status]} dot>
              {versionLabel(currentVersion)}
            </HvBadge>
          </div>
          <div className="flex gap-3">
            <Link
              to={`/library/templates/${template.id}`}
              className="font-display text-[13px] font-bold text-mint-600 hover:underline"
            >
              Chương trình mẫu
            </Link>
            {canAssign ? (
              <Link
                to={`/prep/${vid}/assign`}
                className="font-display text-[13px] font-bold text-mint-600 hover:underline"
              >
                Phân công
              </Link>
            ) : null}
          </div>
        </div>
      </div>

      {notice ? <HvNotice tone="info">{notice}</HvNotice> : null}

      <div className="grid gap-3 md:grid-cols-4">
        {columns.map((column) => {
          const cards = kanban.tasksByColumn.get(column.id) ?? [];
          return (
            <section
              key={column.id}
              className="flex flex-col gap-2 rounded-[var(--radius-lg)] border border-line-200 bg-cream-100 p-2"
            >
              <h2 className="flex items-center justify-between px-1 font-display text-[13px] font-extrabold uppercase tracking-[0.4px] text-ink-500">
                {column.name}
                <span className="rounded-full bg-white px-2 py-0.5 text-[12px] text-ink-700">
                  {cards.length}
                </span>
              </h2>
              <div
                {...kanban.getColumnProps(column.id)}
                className="flex min-h-[80px] flex-col gap-2"
              >
                {cards.length === 0 ? (
                  <p className="px-1 py-3 text-center text-[13px] text-ink-400">Trống</p>
                ) : null}
                {cards.map((card) => (
                  <PrepCardView
                    key={card.id}
                    card={card}
                    columns={columns}
                    moveDisabled={!editable}
                    onMove={(columnId) => moveAndAnnounce(card.id, columnId)}
                    onOpen={() => openLesson(card)}
                    taskProps={kanban.getTaskProps(card.id)}
                  />
                ))}
              </div>
            </section>
          );
        })}
      </div>

      {/* Two live regions, one per announcement owner: the page's own
          menu-move outcome and the lib's keyboard-move announcement. */}
      <div aria-live="polite" role="status" className="sr-only">
        {announcement}
      </div>
      <div aria-live="polite" role="status" className="sr-only">
        {kanban.announcement}
      </div>
    </div>
  );
}

interface PrepCardViewProps {
  card: PrepCard;
  columns: KanbanColumn[];
  moveDisabled: boolean;
  onMove: (columnId: ColumnId) => void;
  onOpen: () => void;
  taskProps: TaskProps;
}

function PrepCardView({
  card,
  columns,
  moveDisabled,
  onMove,
  onOpen,
  taskProps,
}: PrepCardViewProps) {
  const title = `Buổi ${card.lessonPosition} · ${card.title}`;
  return (
    <div
      {...taskProps}
      aria-label={title}
      className={cn(
        "group flex flex-col gap-1.5 rounded-[var(--radius-md)] border border-line-200 bg-white p-3",
        "focus-visible:outline-none focus-visible:ring-4 focus-visible:ring-mint-100",
      )}
    >
      <div className="flex items-start justify-between gap-2">
        <p className="font-display text-[14px] font-extrabold text-ink-900">{title}</p>
        <ActionsMenu
          currentColumnId={card.columnId}
          columns={columns}
          onMove={onMove}
          onOpen={onOpen}
          moveDisabled={moveDisabled}
        />
      </div>
      <p className="text-[12px] text-ink-500">
        {card.checklistDone}/{card.checklistTotal} việc
      </p>
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-[12px] text-ink-700">
        <span className="inline-flex items-center gap-1">
          <UserRoundIcon aria-hidden className="size-3.5 text-ink-400" />
          {card.assigneeName ?? <span className="text-ink-400">Chưa phân công</span>}
        </span>
        {card.dueDate ? (
          <span className="inline-flex items-center gap-1">
            <CalendarIcon aria-hidden className="size-3.5 text-ink-400" />
            {formatDayMonth(card.dueDate)}
          </span>
        ) : null}
      </div>
    </div>
  );
}
