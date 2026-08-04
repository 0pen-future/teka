import { zodResolver } from "@hookform/resolvers/zod";
import { useEffect } from "react";
import { useForm } from "react-hook-form";

import { HvButton, HvModal } from "@/components/hv";
import { Field, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field";
import { useApiFormErrors } from "@/lib/forms/use-api-form-errors";
import { cn } from "@/lib/utils";

import { useCancelSession } from "../hooks/use-sessions";
import { cancelSessionInputSchema, type CancelSessionInput } from "../schemas/attendance-schemas";

export interface CancelSessionDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  sessionId: string;
  onCancelled?: () => void;
}

/**
 * No shared `Textarea` primitive exists yet (`apps/web/src/components/ui`
 * only has single-line `Input`), so the reason field reuses `Input`'s token
 * classes on a raw `<textarea>` rather than introducing a new shared component
 * for a single caller.
 */
const textareaClassName = cn(
  "min-h-20 w-full rounded-lg border border-input bg-transparent px-2.5 py-2 text-base transition-colors outline-none",
  "placeholder:text-muted-foreground focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50",
  "aria-invalid:border-destructive aria-invalid:ring-3 aria-invalid:ring-destructive/20 md:text-sm",
);

/** `CancelSessionDialog` (prototype `modalWarn`-adjacent). Cancelling bills nobody for the session. */
export function CancelSessionDialog({
  open,
  onOpenChange,
  sessionId,
  onCancelled,
}: CancelSessionDialogProps) {
  const form = useForm<CancelSessionInput>({
    resolver: zodResolver(cancelSessionInputSchema),
    defaultValues: { reason: "" },
  });
  const cancelMutation = useCancelSession(sessionId);
  const handleApiError = useApiFormErrors(form);

  useEffect(() => {
    if (open) {
      form.reset({ reason: "" });
    }
    // form is stable from react-hook-form and not a meaningful dependency here.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open]);

  const onSubmit = form.handleSubmit((values) => {
    cancelMutation.mutate(values, {
      onSuccess: () => {
        onOpenChange(false);
        onCancelled?.();
      },
      onError: handleApiError,
    });
  });

  const { errors } = form.formState;

  return (
    <HvModal
      open={open}
      onOpenChange={onOpenChange}
      title="Huỷ buổi học"
      footer={
        <>
          <HvButton type="button" variant="ghost" onClick={() => onOpenChange(false)}>
            Đóng
          </HvButton>
          <HvButton
            type="submit"
            form="cancel-session-form"
            variant="danger"
            disabled={cancelMutation.isPending}
          >
            {cancelMutation.isPending ? "Đang huỷ…" : "Xác nhận huỷ"}
          </HvButton>
        </>
      }
    >
      <form id="cancel-session-form" onSubmit={(event) => void onSubmit(event)} noValidate>
        <FieldGroup>
          <p className="text-[13px] text-ink-500">
            Buổi học bị huỷ sẽ không tính tiền cho học sinh nào.
          </p>
          <Field data-invalid={Boolean(errors.reason)}>
            <FieldLabel htmlFor="cancel-reason">Lý do huỷ</FieldLabel>
            <textarea
              id="cancel-reason"
              aria-invalid={Boolean(errors.reason)}
              className={textareaClassName}
              {...form.register("reason")}
            />
            <FieldError errors={[errors.reason]} />
          </Field>
          <FieldError errors={[errors.root]} />
        </FieldGroup>
      </form>
    </HvModal>
  );
}
