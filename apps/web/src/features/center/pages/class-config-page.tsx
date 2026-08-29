import { useState } from "react";
import { Navigate } from "react-router";

import { HvButton, HvCard, hvToast } from "@/components/hv";
import { useClassesList, type Class } from "@/features/roster";
import { useCenterContext } from "@/features/teaching";

import { AssignScoreSetDialog } from "../components/assign-score-set-dialog";
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
        <h1 className="font-display text-[26px] font-extrabold text-ink-900">Cấu hình lớp học</h1>
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
  const [armedDeleteId, setArmedDeleteId] = useState<string | null>(null);

  function openCreate() {
    setEditing(undefined);
    setEditorOpen(true);
  }

  function openEdit(scoreSet: ScoreSet) {
    setEditing(scoreSet);
    setEditorOpen(true);
  }

  function confirmDelete(scoreSet: ScoreSet) {
    deleteMutation.mutate(scoreSet.id, {
      onSuccess: () => {
        hvToast(`Đã xóa bộ điểm ${scoreSet.name}`, { variant: "success" });
        setArmedDeleteId(null);
      },
      onError: () => {
        hvToast("Có lỗi xảy ra, thử lại sau", { variant: "danger" });
        setArmedDeleteId(null);
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
        <p className="mt-3 text-[13px] text-ink-400">Đang tải…</p>
      ) : isError || !scoreSets ? (
        <p className="mt-3 text-[13px] text-ink-500">Không tải được danh sách bộ điểm.</p>
      ) : scoreSets.length === 0 ? (
        <p className="mt-3 text-[13px] text-ink-400">Chưa có bộ điểm nào.</p>
      ) : (
        <ul className="mt-3 flex flex-col gap-2">
          {scoreSets.map((scoreSet) => (
            <li
              key={scoreSet.id}
              className="flex flex-wrap items-center justify-between gap-2 rounded-[var(--radius-md)] border border-line-200 px-3 py-2.5"
            >
              <div>
                <p className="text-[13.5px] font-bold text-ink-900">{scoreSet.name}</p>
                <p className="text-[12.5px] text-ink-400">{scoreSet.components.join(", ")}</p>
              </div>
              <div className="flex items-center gap-2">
                {armedDeleteId === scoreSet.id ? (
                  <>
                    <span className="text-[12.5px] font-bold text-ink-700">Xóa bộ điểm này?</span>
                    <HvButton
                      size="sm"
                      variant="danger"
                      disabled={deleteMutation.isPending}
                      onClick={() => confirmDelete(scoreSet)}
                    >
                      {deleteMutation.isPending ? "Đang xóa…" : "Xác nhận xóa"}
                    </HvButton>
                    <HvButton
                      size="sm"
                      variant="ghost"
                      disabled={deleteMutation.isPending}
                      onClick={() => setArmedDeleteId(null)}
                    >
                      Hủy
                    </HvButton>
                  </>
                ) : (
                  <>
                    <HvButton size="sm" variant="ghost" onClick={() => openEdit(scoreSet)}>
                      Sửa
                    </HvButton>
                    <HvButton
                      size="sm"
                      variant="ghost"
                      className="text-coral-500"
                      onClick={() => setArmedDeleteId(scoreSet.id)}
                    >
                      Xóa
                    </HvButton>
                  </>
                )}
              </div>
            </li>
          ))}
        </ul>
      )}

      <ScoreSetEditorModal open={editorOpen} onOpenChange={setEditorOpen} scoreSet={editing} />
    </HvCard>
  );
}

function ClassAssignmentSection() {
  const { data: classesPage, isPending, isError } = useClassesList({ per_page: 100 });
  const { data: scoreSets } = useScoreSets();
  const [assigningClass, setAssigningClass] = useState<Class | null>(null);

  const classes = classesPage?.items ?? [];
  const canAssign = Boolean(scoreSets && scoreSets.length > 0);

  return (
    <HvCard>
      <p className="font-display text-[16px] font-bold text-ink-900">Gán bộ điểm cho lớp</p>
      <p className="mt-0.5 text-[12.5px] text-ink-400">
        Lớp đã có điểm được ghi nhận không thể đổi hoặc xóa bộ điểm đang gán.
      </p>

      {isPending ? (
        <p className="mt-3 text-[13px] text-ink-400">Đang tải…</p>
      ) : isError ? (
        <p className="mt-3 text-[13px] text-ink-500">Không tải được danh sách lớp.</p>
      ) : classes.length === 0 ? (
        <p className="mt-3 text-[13px] text-ink-400">Chưa có lớp nào.</p>
      ) : (
        <div className="mt-3 overflow-x-auto">
          <table className="w-full min-w-[420px] border-collapse text-[13.5px]">
            <thead>
              <tr>
                <th className="py-2 pr-3 text-left font-bold text-ink-500">Lớp</th>
                <th className="py-2 pr-3 text-right font-bold text-ink-500">Bộ điểm</th>
              </tr>
            </thead>
            <tbody>
              {classes.map((klass) => (
                <tr key={klass.id} className="border-t border-line-200">
                  <td className="py-2 pr-3 text-ink-900">{klass.name}</td>
                  <td className="py-2 pr-3 text-right">
                    <HvButton
                      size="sm"
                      variant="ghost"
                      disabled={!canAssign}
                      title={canAssign ? undefined : "Tạo ít nhất một bộ điểm trước"}
                      onClick={() => setAssigningClass(klass)}
                    >
                      Gán bộ điểm
                    </HvButton>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {assigningClass ? (
        <AssignScoreSetDialog
          open={Boolean(assigningClass)}
          onOpenChange={(open) => {
            if (!open) {
              setAssigningClass(null);
            }
          }}
          classId={assigningClass.id}
          className={assigningClass.name}
          scoreSets={scoreSets ?? []}
        />
      ) : null}
    </HvCard>
  );
}
