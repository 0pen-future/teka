import { useState } from "react";
import { Navigate } from "react-router";

import { HvButton, HvCard, HvConfirmDialog, HvStateBlock, hvToast } from "@/components/hv";
import { useClassesList, type Class } from "@/features/roster";
import { useCenterContext } from "@/features/teaching";

import { AssignScoreSetDialog } from "../components/assign-score-set-dialog";
import { ClassScoreSetTable, type ClassScoreSetRow } from "../components/class-score-set-table";
import { ScoreSetCard } from "../components/score-set-card";
import { ScoreSetEditorModal } from "../components/score-set-editor-modal";
import { useDeleteScoreSet, useScoreSets } from "../hooks/use-score-sets";
import type { ScoreSet } from "../schemas/grading";

/**
 * Owner-only "Cấu hình lớp học" page: manage the center's bộ điểm (score
 * sets) and assign one to each class. The score-set read model and the class
 * snapshot mutations are owner-only server-side, so a non-owner deep-linking
 * here redirects to the dashboard before any request could 403 — same gate
 * shape as the audit and permissions pages.
 */
export function ClassConfigPage() {
  const { isOwner, isResolved, isError } = useCenterContext();

  if (!isResolved && !isError) {
    return null;
  }
  if (!isOwner) {
    return <Navigate to="/" replace />;
  }

  return (
    <div className="flex flex-col gap-5">
      <div>
        <h1 className="font-display text-[length:var(--text-xl)] font-bold text-ink-900">
          Cấu hình lớp học
        </h1>
        <p className="mt-1 text-[14px] text-ink-500">
          Tạo bộ điểm dùng chung cho trung tâm rồi gán từng lớp vào một bộ điểm để nhập điểm theo
          đúng cột.
        </p>
      </div>
      <ScoreSetsSection />
      <ClassAssignmentSection />
    </div>
  );
}

function ScoreSetsSection() {
  const { data: scoreSets, isPending, isError } = useScoreSets();
  const deleteMutation = useDeleteScoreSet();
  const [editorOpen, setEditorOpen] = useState(false);
  const [editing, setEditing] = useState<ScoreSet | undefined>(undefined);
  const [deletingSet, setDeletingSet] = useState<ScoreSet | null>(null);

  function openCreate() {
    setEditing(undefined);
    setEditorOpen(true);
  }

  function openEdit(scoreSet: ScoreSet) {
    setEditing(scoreSet);
    setEditorOpen(true);
  }

  function confirmDelete() {
    if (!deletingSet) return;
    const scoreSet = deletingSet;
    deleteMutation.mutate(scoreSet.id, {
      onSuccess: () => {
        hvToast(`Đã xóa bộ điểm ${scoreSet.name}`, { variant: "success" });
        setDeletingSet(null);
      },
      onError: () => {
        hvToast("Có lỗi xảy ra, thử lại sau", { variant: "danger" });
        setDeletingSet(null);
      },
    });
  }

  return (
    <HvCard>
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <p className="font-display text-[16px] font-bold text-ink-900">Bộ điểm</p>
          <p className="text-[12.5px] text-ink-400">Mỗi bộ điểm gồm tối đa 10 cột, theo thứ tự.</p>
        </div>
        <HvButton size="sm" onClick={openCreate}>
          + Tạo bộ điểm
        </HvButton>
      </div>

      {isPending ? (
        <HvStateBlock state="loading" compact className="mt-3" title="Đang tải bộ điểm" />
      ) : isError || !scoreSets ? (
        <HvStateBlock
          state="error"
          compact
          className="mt-3"
          title="Không tải được danh sách bộ điểm."
        />
      ) : scoreSets.length === 0 ? (
        <HvStateBlock
          state="empty"
          compact
          className="mt-3"
          title="Chưa có bộ điểm nào."
          description="Tạo bộ điểm đầu tiên để gán cho các lớp."
          action={
            <HvButton size="sm" variant="secondary" onClick={openCreate}>
              Tạo bộ điểm
            </HvButton>
          }
        />
      ) : (
        <ul className="mt-3 flex flex-col gap-3">
          {scoreSets.map((scoreSet) => (
            <ScoreSetCard
              key={scoreSet.id}
              scoreSet={scoreSet}
              onEdit={() => openEdit(scoreSet)}
              onDelete={() => setDeletingSet(scoreSet)}
            />
          ))}
        </ul>
      )}

      <ScoreSetEditorModal open={editorOpen} onOpenChange={setEditorOpen} scoreSet={editing} />
      <HvConfirmDialog
        open={deletingSet !== null}
        onOpenChange={(open) => {
          if (!open) setDeletingSet(null);
        }}
        title={deletingSet ? `Xóa bộ điểm ${deletingSet.name}?` : "Xóa bộ điểm?"}
        description="Lớp đang dùng bộ điểm này vẫn giữ nguyên cột điểm đã gán."
        confirmLabel="Xác nhận xóa"
        tone="danger"
        pending={deleteMutation.isPending}
        onConfirm={confirmDelete}
      />
    </HvCard>
  );
}

function ClassAssignmentSection() {
  const { data: classesPage, isPending, isError } = useClassesList({ per_page: 100 });
  const { data: scoreSets } = useScoreSets();
  const [assigningClass, setAssigningClass] = useState<ClassScoreSetRow | null>(null);
  // Classes that answered 409 this page session: reopening the dialog for one
  // of them locks immediately instead of letting the owner retry into the
  // same conflict. Cleared on reload; the API has no `has_scores` flag yet.
  const [lockedClassIds, setLockedClassIds] = useState<ReadonlySet<string>>(() => new Set());

  const rows: ClassScoreSetRow[] = (classesPage?.items ?? []).map((klass: Class) => ({
    classId: klass.id,
    className: klass.name,
  }));
  const canAssign = Boolean(scoreSets && scoreSets.length > 0);

  function markLocked(classId: string) {
    setLockedClassIds((prev) => {
      if (prev.has(classId)) return prev;
      const next = new Set(prev);
      next.add(classId);
      return next;
    });
  }

  return (
    <HvCard>
      <p className="font-display text-[16px] font-bold text-ink-900">Gán bộ điểm cho lớp</p>
      <p className="mt-0.5 text-[12.5px] text-ink-400">
        Lớp đã có điểm được ghi nhận không thể đổi hoặc xóa bộ điểm đang gán.
      </p>
      {!canAssign ? (
        <p className="mt-1 text-[12.5px] font-bold text-sun-600">Tạo ít nhất một bộ điểm trước</p>
      ) : null}

      {isPending ? (
        <HvStateBlock state="loading" compact className="mt-3" title="Đang tải danh sách lớp" />
      ) : isError ? (
        <HvStateBlock
          state="error"
          compact
          className="mt-3"
          title="Không tải được danh sách lớp."
        />
      ) : rows.length === 0 ? (
        <HvStateBlock state="empty" compact className="mt-3" title="Chưa có lớp nào." />
      ) : (
        <ClassScoreSetTable rows={rows} canAssign={canAssign} onAssign={setAssigningClass} />
      )}

      {assigningClass ? (
        <AssignScoreSetDialog
          open={Boolean(assigningClass)}
          onOpenChange={(open) => {
            if (!open) {
              setAssigningClass(null);
            }
          }}
          classId={assigningClass.classId}
          className={assigningClass.className}
          scoreSets={scoreSets ?? []}
          locked={lockedClassIds.has(assigningClass.classId)}
          onLocked={() => markLocked(assigningClass.classId)}
        />
      ) : null}
    </HvCard>
  );
}
