import { useState } from "react";

import { HvButton, HvCard, HvSelect, hvToast } from "@/components/hv";
import { Field, FieldLabel } from "@/components/ui/field";
import type { CenterMember } from "@/features/center";
import { ApiError } from "@/lib/api/errors";

import { useReassignTeacher } from "../hooks/use-classes";
import type { Class } from "../schemas/roster-schemas";

/** Owner-only handoff keeps the teacher's scheduled future sessions in sync server-side. */
export function TeacherHandoffCard({ klass, members }: { klass: Class; members: CenterMember[] }) {
  const [targetId, setTargetId] = useState("");
  const [arming, setArming] = useState(false);
  const reassign = useReassignTeacher(klass.id);

  const currentTeacher = members.find((member) => member.id === klass.teacher_id);
  const targets = members.filter((member) => member.id !== klass.teacher_id);
  const errorMessage =
    reassign.error instanceof ApiError
      ? reassign.error.message
      : reassign.error
        ? "Không bàn giao được lớp. Thử lại sau."
        : null;

  function handleTargetChange(next: string) {
    if (next === targetId) return;
    setTargetId(next);
    // A changed target requires a new explicit confirmation.
    setArming(false);
  }

  function confirm() {
    if (!targetId) return;
    reassign.mutate(targetId, {
      onSuccess: (result) => {
        const name = members.find((member) => member.id === result.teacher_id)?.full_name;
        hvToast(name ? `Đã bàn giao lớp cho ${name}` : "Đã bàn giao lớp");
        setArming(false);
        setTargetId("");
      },
    });
  }

  return (
    <HvCard id="teacher-handoff">
      <p className="font-display text-[16px] font-bold text-ink-900">Giáo viên phụ trách</p>
      <p className="mt-0.5 text-[13px] text-ink-400">
        Giáo viên hiện tại:{" "}
        <span className="font-bold text-ink-700">
          {currentTeacher ? currentTeacher.full_name : "Không rõ"}
        </span>
        . Bàn giao sẽ chuyển lớp, lịch học và các buổi <em>đã lên lịch</em> từ hôm nay trở đi sang
        giáo viên mới. Buổi đã dạy, đã hủy và học phí đã chốt vẫn giữ nguyên.
      </p>
      {targets.length === 0 ? (
        <p className="mt-3 text-[13px] text-ink-400">
          Chưa có giáo viên khác trong trung tâm để bàn giao.
        </p>
      ) : (
        <div className="mt-3 flex flex-col gap-3">
          <Field className="max-w-[320px]">
            <FieldLabel htmlFor="handoff-teacher">Bàn giao cho</FieldLabel>
            <HvSelect
              id="handoff-teacher"
              options={targets.map((member) => ({
                value: member.id,
                label: member.is_owner ? `${member.full_name} (chủ trung tâm)` : member.full_name,
              }))}
              value={targetId}
              onValueChange={handleTargetChange}
              placeholder="— Chọn giáo viên —"
              disabled={reassign.isPending}
              searchNoun="giáo viên"
              sheetTitle="Bàn giao cho"
              className="w-full"
            />
          </Field>
          {errorMessage ? <p className="text-[13px] text-coral-600">{errorMessage}</p> : null}
          {arming ? (
            <div className="flex flex-wrap items-center gap-2">
              <span className="text-[13px] font-bold text-ink-700">
                Bàn giao lớp cho{" "}
                {targets.find((member) => member.id === targetId)?.full_name ?? "giáo viên này"}?
              </span>
              <HvButton size="sm" onClick={confirm} disabled={reassign.isPending}>
                {reassign.isPending ? "Đang bàn giao…" : "Xác nhận bàn giao"}
              </HvButton>
              <HvButton
                size="sm"
                variant="ghost"
                onClick={() => setArming(false)}
                disabled={reassign.isPending}
              >
                Hủy
              </HvButton>
            </div>
          ) : (
            <div>
              <HvButton
                variant="secondary"
                size="sm"
                onClick={() => setArming(true)}
                disabled={!targetId || reassign.isPending}
              >
                Bàn giao lớp
              </HvButton>
            </div>
          )}
        </div>
      )}
    </HvCard>
  );
}
