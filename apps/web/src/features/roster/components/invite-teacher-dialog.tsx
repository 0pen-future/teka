import { useId, useState } from "react";

import { HvButton, HvModal, HvSelect, hvToast } from "@/components/hv";
import { Field, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field";
import { useAuthStore } from "@/features/auth";
import { useMemberDirectory } from "@/features/tasks";
import { ApiError } from "@/lib/api/errors";
import { cn } from "@/lib/utils";

import { useSendClassInvitation } from "../hooks/use-class-invitations";
import {
  invitableStaffRoleKeys,
  type Class,
  type InvitableStaffRoleKey,
} from "../schemas/roster-schemas";

const ROLE_LABELS: Record<InvitableStaffRoleKey, string> = {
  giao_vien: "Giáo viên",
  tro_giang: "Trợ giảng",
  hoc_vu: "Học vụ",
};

const MESSAGE_MAX = 500;

const textareaClassName = cn(
  "min-h-20 w-full rounded-lg border border-input bg-transparent px-2.5 py-2 text-base transition-colors outline-none",
  "placeholder:text-muted-foreground focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50",
  "aria-invalid:border-destructive aria-invalid:ring-3 aria-invalid:ring-destructive/20 md:text-sm",
);

/**
 * Owner-only "Mời thành viên vào lớp". The picker offers the center
 * directory minus the signed-in owner (the API refuses a self-invite) and
 * minus anyone already active on the class (one active stint per person).
 * Sending writes nothing to class_staff — the stint lands when the owner
 * confirms the invitation later.
 */
export function InviteTeacherDialog({
  klass,
  activeTeacherIds,
  onOpenChange,
}: {
  klass: Class;
  activeTeacherIds: string[];
  onOpenChange: (open: boolean) => void;
}) {
  const memberLabelId = useId();
  const roleLabelId = useId();
  const messageId = useId();
  const selfId = useAuthStore((state) => state.user)?.id;
  const directory = useMemberDirectory();
  const send = useSendClassInvitation(klass.id);
  const [teacherId, setTeacherId] = useState("");
  const [roleKey, setRoleKey] = useState<InvitableStaffRoleKey>("giao_vien");
  const [message, setMessage] = useState("");
  const [touched, setTouched] = useState(false);

  const excluded = new Set([...activeTeacherIds, ...(selfId ? [selfId] : [])]);
  const candidates = (directory.data ?? []).filter((entry) => !excluded.has(entry.teacher_id));
  const teacherError = touched && teacherId === "" ? "Bắt buộc chọn thành viên" : undefined;
  const trimmed = message.trim();
  const errorMessage = send.error
    ? send.error instanceof ApiError
      ? send.error.message
      : "Không gửi được lời mời. Thử lại sau."
    : null;

  function handleSubmit(event: React.FormEvent) {
    event.preventDefault();
    setTouched(true);
    if (teacherId === "" || trimmed.length > MESSAGE_MAX) {
      return;
    }
    const picked = candidates.find((entry) => entry.teacher_id === teacherId);
    send.mutate(
      {
        teacher_id: teacherId,
        role_key: roleKey,
        ...(trimmed === "" ? {} : { message: trimmed }),
      },
      {
        onSuccess: (invitation) => {
          hvToast(`Đã gửi lời mời cho ${picked?.display_name ?? invitation.teacher_name}`);
          onOpenChange(false);
        },
      },
    );
  }

  const formId = `${messageId}-form`;

  return (
    <HvModal
      open
      onOpenChange={(open) => {
        if (send.isPending && !open) return;
        onOpenChange(open);
      }}
      title="Mời thành viên vào lớp"
      description={`Lớp ${klass.name} — người được mời sẽ thấy lời mời ở mục Lời mời nhận lớp.`}
      footer={
        <>
          <HvButton
            type="button"
            variant="ghost"
            size="sm"
            onClick={() => onOpenChange(false)}
            disabled={send.isPending}
          >
            Huỷ
          </HvButton>
          <HvButton type="submit" form={formId} size="sm" disabled={send.isPending}>
            {send.isPending ? "Đang gửi…" : "Gửi lời mời"}
          </HvButton>
        </>
      }
    >
      <form id={formId} onSubmit={handleSubmit} noValidate>
        <FieldGroup>
          <Field data-invalid={Boolean(teacherError)}>
            <FieldLabel id={memberLabelId}>Thành viên</FieldLabel>
            <HvSelect
              labelId={memberLabelId}
              options={candidates.map((entry) => ({
                value: entry.teacher_id,
                label: entry.role_name
                  ? `${entry.display_name} · ${entry.role_name}`
                  : entry.display_name,
              }))}
              value={teacherId}
              onValueChange={(value) => {
                setTeacherId(value);
                setTouched(true);
              }}
              placeholder={directory.isPending ? "Đang tải thành viên…" : "— Chọn thành viên —"}
              disabled={send.isPending || directory.isPending}
              searchNoun="thành viên"
              sheetTitle="Chọn thành viên"
              aria-invalid={Boolean(teacherError)}
              className="w-full"
            />
            {directory.isError ? (
              <p className="text-[13px] text-coral-600">Không tải được danh sách thành viên.</p>
            ) : candidates.length === 0 && directory.isSuccess ? (
              <p className="text-[13px] text-ink-400">Không còn thành viên nào để mời.</p>
            ) : null}
            <FieldError errors={teacherError ? [{ message: teacherError }] : []} />
          </Field>
          <Field>
            <FieldLabel id={roleLabelId}>Vai trò</FieldLabel>
            <HvSelect
              labelId={roleLabelId}
              options={invitableStaffRoleKeys.map((key) => ({
                value: key,
                label: ROLE_LABELS[key],
              }))}
              value={roleKey}
              onValueChange={(value) => setRoleKey(value as InvitableStaffRoleKey)}
              sheetTitle="Chọn vai trò"
              searchThreshold={Infinity}
              disabled={send.isPending}
              className="w-full"
            />
          </Field>
          <Field data-invalid={trimmed.length > MESSAGE_MAX}>
            <FieldLabel htmlFor={messageId}>Lời nhắn</FieldLabel>
            <textarea
              id={messageId}
              className={textareaClassName}
              placeholder="Ví dụ: nhờ cô hỗ trợ lớp tối thứ ba…"
              value={message}
              onChange={(event) => setMessage(event.target.value)}
              aria-invalid={trimmed.length > MESSAGE_MAX}
              disabled={send.isPending}
            />
            <FieldError
              errors={
                trimmed.length > MESSAGE_MAX ? [{ message: `Tối đa ${MESSAGE_MAX} ký tự` }] : []
              }
            />
          </Field>
        </FieldGroup>
        {errorMessage ? <p className="mt-2 text-[13px] text-coral-600">{errorMessage}</p> : null}
      </form>
    </HvModal>
  );
}
