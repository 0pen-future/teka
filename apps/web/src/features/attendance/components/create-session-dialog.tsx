import { zodResolver } from "@hookform/resolvers/zod";
import { useEffect } from "react";
import { useForm } from "react-hook-form";

import { HvButton, HvModal } from "@/components/hv";
import { Field, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { useApiFormErrors } from "@/lib/forms/use-api-form-errors";

import { useCreateSession } from "../hooks/use-sessions";
import {
  createSessionInputSchema,
  type CreateSessionInput,
  type Session,
} from "../schemas/attendance-schemas";

export interface CreateSessionDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  classId: string;
  onCreated?: (session: Session) => void;
}

const today = () => new Date().toISOString().slice(0, 10);

/**
 * Ad-hoc make-up session outside any weekly schedule (PRD §5 edge case).
 * Kept as a secondary action on `SessionsPage` — the normal path is
 * server-generated sessions from the class's schedule.
 */
export function CreateSessionDialog({
  open,
  onOpenChange,
  classId,
  onCreated,
}: CreateSessionDialogProps) {
  const form = useForm<CreateSessionInput>({
    resolver: zodResolver(createSessionInputSchema),
    defaultValues: { session_date: today(), start_time: "" },
  });
  const createMutation = useCreateSession(classId);
  const handleApiError = useApiFormErrors(form);

  useEffect(() => {
    if (open) {
      form.reset({ session_date: today(), start_time: "" });
    }
    // form is stable from react-hook-form and not a meaningful dependency here.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open]);

  const onSubmit = form.handleSubmit((values) => {
    createMutation.mutate(values, {
      onSuccess: (session) => {
        onOpenChange(false);
        onCreated?.(session);
      },
      onError: handleApiError,
    });
  });

  const { errors } = form.formState;

  return (
    <HvModal
      open={open}
      onOpenChange={onOpenChange}
      title="Thêm buổi học"
      footer={
        <>
          <HvButton type="button" variant="ghost" onClick={() => onOpenChange(false)}>
            Huỷ
          </HvButton>
          <HvButton type="submit" form="create-session-form" disabled={createMutation.isPending}>
            {createMutation.isPending ? "Đang lưu…" : "Thêm buổi"}
          </HvButton>
        </>
      }
    >
      <form id="create-session-form" onSubmit={(event) => void onSubmit(event)} noValidate>
        <FieldGroup>
          <Field data-invalid={Boolean(errors.session_date)}>
            <FieldLabel htmlFor="session-date">Ngày học</FieldLabel>
            <Input
              id="session-date"
              type="date"
              aria-invalid={Boolean(errors.session_date)}
              {...form.register("session_date")}
            />
            <FieldError errors={[errors.session_date]} />
          </Field>
          <Field data-invalid={Boolean(errors.start_time)}>
            <FieldLabel htmlFor="session-start-time">Giờ học (không bắt buộc)</FieldLabel>
            <Input
              id="session-start-time"
              type="time"
              aria-invalid={Boolean(errors.start_time)}
              {...form.register("start_time")}
            />
            <FieldError errors={[errors.start_time]} />
          </Field>
          <FieldError errors={[errors.root]} />
        </FieldGroup>
      </form>
    </HvModal>
  );
}
