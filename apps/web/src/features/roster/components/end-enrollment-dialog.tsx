import { zodResolver } from "@hookform/resolvers/zod";
import { useForm } from "react-hook-form";

import { HvButton, HvModal } from "@/components/hv";
import { Field, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";

import { useEndEnrollment } from "../hooks/use-enrollments";
import {
  endEnrollmentInputSchema,
  type EndEnrollmentInput,
  type Enrollment,
} from "../schemas/roster-schemas";

export interface EndEnrollmentDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  enrollment: Enrollment;
  onSuccess?: (enrollment: Enrollment) => void;
}

const today = () => new Date().toISOString().slice(0, 10);

/**
 * `POST /enrollments/:id/end`. Fee calculation still runs through the last
 * attended session and any existing debt on the student is left untouched —
 * ending an enrollment never writes off a balance.
 */
export function EndEnrollmentDialog({
  open,
  onOpenChange,
  enrollment,
  onSuccess,
}: EndEnrollmentDialogProps) {
  const form = useForm<EndEnrollmentInput>({
    resolver: zodResolver(endEnrollmentInputSchema(enrollment.started_on)),
    defaultValues: { ended_on: today() },
  });
  const mutation = useEndEnrollment();
  const { errors } = form.formState;

  const onSubmit = form.handleSubmit((values) => {
    // An empty string means "clear the field", which server-side means
    // "use today" — distinct from omitting the key entirely, so this can't
    // be a plain `??` fallback.
    const endedOn = values.ended_on === "" ? undefined : values.ended_on;
    mutation.mutate(
      { id: enrollment.id, endedOn },
      {
        onSuccess: (result) => {
          onOpenChange(false);
          onSuccess?.(result);
        },
        onError: (error) => {
          form.setError("root", {
            type: "server",
            message: error instanceof Error ? error.message : "Không thể kết thúc ghi danh",
          });
        },
      },
    );
  });

  return (
    <HvModal
      open={open}
      onOpenChange={onOpenChange}
      title="Kết thúc ghi danh"
      footer={
        <>
          <HvButton type="button" variant="ghost" onClick={() => onOpenChange(false)}>
            Huỷ
          </HvButton>
          <HvButton
            type="submit"
            form="end-enrollment-form"
            variant="danger"
            disabled={mutation.isPending}
          >
            {mutation.isPending ? "Đang lưu…" : "Kết thúc"}
          </HvButton>
        </>
      }
    >
      <p className="mb-4">Học phí được tính tới buổi cuối cùng. Nợ cũ (nếu có) vẫn được giữ.</p>
      <form id="end-enrollment-form" onSubmit={(event) => void onSubmit(event)} noValidate>
        <FieldGroup>
          <Field data-invalid={Boolean(errors.ended_on)}>
            <FieldLabel htmlFor="end-enrollment-date">Ngày kết thúc</FieldLabel>
            <Input
              id="end-enrollment-date"
              type="date"
              aria-invalid={Boolean(errors.ended_on)}
              {...form.register("ended_on")}
            />
            <FieldError errors={[errors.ended_on]} />
          </Field>
          <FieldError errors={[errors.root]} />
        </FieldGroup>
      </form>
    </HvModal>
  );
}
