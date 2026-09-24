import { ArrowLeftIcon } from "lucide-react";
import { useState } from "react";
import { Link, Navigate, useParams } from "react-router";

import { HvBadge, HvNotice, HvSelect, HvStateBlock, hvToast } from "@/components/hv";
import { Input } from "@/components/ui/input";
import { useCenterContext } from "@/features/teaching";
import { ApiError } from "@/lib/api/errors";
import { cn } from "@/lib/utils";

import {
  useAssignees,
  usePrepBoard,
  useUpdateLessonAssignment,
  type PrepCard,
} from "../hooks/use-prep";
import {
  prepStatusLabel,
  prepStatusVariant,
  versionLabel,
  versionStatusVariant,
} from "../lib/library-labels";
import type { PrepStatus } from "../schemas/library-schemas";

const headCellClassName =
  "sticky top-0 z-10 bg-cream-200 px-[18px] py-[10px] text-[12px] font-extrabold uppercase tracking-[0.4px] text-ink-500";
const cellClassName = "border-t border-line-100 px-[18px] py-[11px] align-middle";
const UNASSIGNED = "__none__";

/**
 * `/prep/:vid/assign` — who prepares each lesson of a draft and by when.
 * Holders of `prep.assign` only; everyone else lands on the board. Every
 * change PATCHes the whole assignment (assignee + due date) so the two
 * fields never drift apart across two half-saved requests.
 */
export function PrepAssignPage() {
  const { vid = "" } = useParams<{ vid: string }>();
  const { has, isResolved, isError } = useCenterContext();
  const canAssign = has("prep.assign");
  const boardQuery = usePrepBoard(vid);
  const assigneesQuery = useAssignees(isResolved && canAssign);

  if (!isResolved && !isError) {
    return <HvStateBlock state="loading" title="Đang tải phân công" />;
  }
  if (!canAssign) {
    return <Navigate to={`/prep/${vid}/board`} replace />;
  }
  if (boardQuery.isPending) {
    return <HvStateBlock state="loading" title="Đang tải phân công" />;
  }
  if (boardQuery.isError || !boardQuery.data) {
    const notFound = boardQuery.error instanceof ApiError && boardQuery.error.status === 404;
    return (
      <HvStateBlock
        state="error"
        title={notFound ? "Không tìm thấy phiên bản" : "Không tải được phân công"}
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

  const { template, version, board } = boardQuery.data;
  const locked = version.status !== "draft";
  const lessons = [...board.tasks].sort((a, b) => a.lessonPosition - b.lessonPosition);
  const memberOptions = [
    { value: UNASSIGNED, label: "Chưa phân công" },
    ...(assigneesQuery.data ?? []).map((entry) => ({
      value: entry.id,
      label: entry.full_name,
    })),
  ];

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-col gap-3">
        <Link
          to={`/prep/${vid}/board`}
          className="inline-flex items-center gap-1 self-start font-display text-[13px] font-bold text-ink-500 hover:text-mint-600"
        >
          <ArrowLeftIcon aria-hidden="true" className="size-4" />
          Bảng chuẩn bị
        </Link>
        <div className="flex flex-wrap items-center gap-2">
          <h1 className="font-display text-[26px] font-extrabold text-ink-900">
            Phân công · {template.name}
          </h1>
          <HvBadge variant={versionStatusVariant[version.status]} dot>
            {versionLabel(version)}
          </HvBadge>
        </div>
      </div>

      {locked ? (
        <HvNotice tone="info">
          Phiên bản v{version.version_no} không còn là bản nháp, phân công được khoá.
        </HvNotice>
      ) : null}
      {assigneesQuery.isError ? (
        <HvNotice tone="warning">Không tải được danh sách thành viên.</HvNotice>
      ) : null}

      {lessons.length === 0 ? (
        <HvStateBlock state="empty" title="Phiên bản này chưa có buổi học nào." />
      ) : (
        <div className="overflow-x-auto rounded-[var(--radius-lg)] border border-line-200 bg-white">
          <table className="w-full min-w-[720px] border-collapse text-[14px]">
            <thead>
              <tr>
                <th className={headCellClassName}>Buổi</th>
                <th className={headCellClassName}>Trạng thái</th>
                <th className={headCellClassName}>Việc</th>
                <th className={cn(headCellClassName, "min-w-[220px]")}>Người phụ trách</th>
                <th className={headCellClassName}>Hạn hoàn thành</th>
              </tr>
            </thead>
            <tbody>
              {lessons.map((card) => (
                <AssignRow
                  key={card.id}
                  card={card}
                  versionId={vid}
                  locked={locked}
                  memberOptions={memberOptions}
                />
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}

interface AssignRowProps {
  card: PrepCard;
  versionId: string;
  locked: boolean;
  memberOptions: { value: string; label: string; meta?: string }[];
}

function AssignRow({ card, versionId, locked, memberOptions }: AssignRowProps) {
  const lessonId = String(card.id);
  const mutation = useUpdateLessonAssignment(lessonId, versionId);
  const status = card.columnId as string as PrepStatus;
  const rowLabel = `Buổi ${card.lessonPosition}`;
  const selectId = `assignee-${lessonId}`;
  const dueId = `due-${lessonId}`;
  // The row owns its two fields between saves: a due date typed right after
  // picking an assignee must send that assignee, not the one the board
  // still shows while its refetch is in flight.
  const [assigneeId, setAssigneeId] = useState(card.assigneeId);
  const [dueDate, setDueDate] = useState(card.dueDate);
  // The date input keeps its own draft so a keystroke mid-date never sends a
  // half-typed value; it commits on blur or once the value is a complete
  // YYYY-MM-DD (or cleared), never on every keystroke. `save` is the only
  // place `dueDate` changes, so it resets the draft alongside it directly
  // instead of syncing them through an effect.
  const [dueDraft, setDueDraft] = useState(card.dueDate ?? "");

  function save(next: { assigneeId: string | null; dueDate: string | null }) {
    setAssigneeId(next.assigneeId);
    setDueDate(next.dueDate);
    setDueDraft(next.dueDate ?? "");
    mutation.mutate(
      { assignee_id: next.assigneeId, due_date: next.dueDate },
      {
        onSuccess: (saved) => {
          setAssigneeId(saved.assignee_id);
          setDueDate(saved.due_date);
          setDueDraft(saved.due_date ?? "");
          hvToast("Đã lưu phân công");
        },
        onError: () => {
          setAssigneeId(card.assigneeId);
          setDueDate(card.dueDate);
          setDueDraft(card.dueDate ?? "");
          hvToast("Không lưu được phân công, vui lòng thử lại.", { variant: "danger" });
        },
      },
    );
  }

  function commitDue(raw: string) {
    const nextDue = raw === "" ? null : raw;
    if (nextDue === dueDate) {
      return;
    }
    save({ assigneeId, dueDate: nextDue });
  }

  return (
    <tr>
      <td className={cn(cellClassName, "whitespace-nowrap")}>
        <span className="font-display font-extrabold text-ink-900">{rowLabel}</span>
        <span className="ml-2 text-ink-700">{card.title}</span>
      </td>
      <td className={cellClassName}>
        <HvBadge variant={prepStatusVariant[status]} dot>
          {prepStatusLabel[status]}
        </HvBadge>
      </td>
      <td className={cn(cellClassName, "text-ink-500 whitespace-nowrap")}>
        {card.checklistDone}/{card.checklistTotal} việc
      </td>
      <td className={cellClassName}>
        <HvSelect
          id={selectId}
          aria-label={`Người phụ trách ${rowLabel}`}
          sheetTitle={`Người phụ trách ${rowLabel}`}
          searchNoun="thành viên"
          options={memberOptions}
          value={assigneeId ?? UNASSIGNED}
          disabled={locked || mutation.isPending}
          onValueChange={(value) =>
            save({ assigneeId: value === UNASSIGNED ? null : value, dueDate })
          }
        />
      </td>
      <td className={cellClassName}>
        <label htmlFor={dueId} className="sr-only">
          Hạn hoàn thành
        </label>
        <Input
          id={dueId}
          type="date"
          value={dueDraft}
          disabled={locked}
          onChange={(event) => {
            const value = event.target.value;
            setDueDraft(value);
            if (value === "" || value.length === 10) {
              commitDue(value);
            }
          }}
          onBlur={(event) => commitDue(event.target.value)}
          className="w-[160px]"
        />
      </td>
    </tr>
  );
}
