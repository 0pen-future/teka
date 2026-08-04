import { zodResolver } from "@hookform/resolvers/zod";
import { useEffect } from "react";
import { useForm, useWatch } from "react-hook-form";

import { HvButton, HvModal } from "@/components/hv";
import { Field, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { useApiFormErrors } from "@/lib/forms/use-api-form-errors";
import { cn, formatMoney } from "@/lib/utils";

import { useCreateAdjustment } from "../hooks/use-billing";
import {
  adjustmentInputSchema,
  type AdjustmentInput,
  type ReviewRow,
} from "../schemas/billing-schemas";

/**
 * No shared `Textarea` primitive exists yet (`apps/web/src/components/ui`
 * only has single-line `Input`), so the reason field reuses `Input`'s token
 * classes on a raw `<textarea>`, following `CancelSessionDialog`'s pattern.
 */
const textareaClassName = cn(
  "min-h-20 w-full rounded-lg border border-input bg-transparent px-2.5 py-2 text-base transition-colors outline-none",
  "placeholder:text-muted-foreground focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50",
  "aria-invalid:border-destructive aria-invalid:ring-3 aria-invalid:ring-destructive/20 md:text-sm",
);

export interface AdjustmentDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  periodId: string;
  /** The review row being adjusted; required to resolve `invoice_id` and the current total. */
  row: ReviewRow | null;
  onSuccess?: () => void;
}

/**
 * `modalAdjust` recipe. A row's `invoice_id` is only ever null before the
 * review draft has been created, which cannot happen here — the review page
 * only offers this dialog once rows are loaded from `useReview`, and
 * `useReview` calls `POST .../draft`, which always populates `invoice_id`.
 */
export function AdjustmentDialog({
  open,
  onOpenChange,
  periodId,
  row,
  onSuccess,
}: AdjustmentDialogProps) {
  const invoiceId = row?.invoice_id ?? "";
  const form = useForm<AdjustmentInput>({
    resolver: zodResolver(adjustmentInputSchema),
    defaultValues: { amount: 0, reason: "" },
  });
  const mutation = useCreateAdjustment(invoiceId, periodId);
  const handleApiError = useApiFormErrors(form);
  const amount = useWatch({ control: form.control, name: "amount" });
  const reason = useWatch({ control: form.control, name: "reason" });

  useEffect(() => {
    if (open) {
      form.reset({ amount: 0, reason: "" });
    }
    // form is stable from react-hook-form and not a meaningful dependency here.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, row?.invoice_id]);

  const onSubmit = form.handleSubmit((values) => {
    if (!invoiceId) {
      return;
    }
    mutation.mutate(values, {
      onSuccess: () => {
        onOpenChange(false);
        onSuccess?.();
      },
      onError: handleApiError,
    });
  });

  const { errors } = form.formState;
  const reasonTrimmed = reason?.trim() ?? "";
  const currentTotal = row?.total_due ?? 0;
  const newTotal = currentTotal + (Number.isFinite(amount) ? amount : 0);
  const canSubmit = Boolean(invoiceId) && reasonTrimmed.length >= 3 && amount !== 0;

  return (
    <HvModal
      open={open}
      onOpenChange={onOpenChange}
      title="Sửa thủ công"
      footer={
        <>
          <HvButton type="button" variant="ghost" onClick={() => onOpenChange(false)}>
            Huỷ
          </HvButton>
          <HvButton
            type="submit"
            form="adjustment-dialog-form"
            disabled={!canSubmit || mutation.isPending}
          >
            {mutation.isPending ? "Đang lưu…" : "Lưu điều chỉnh"}
          </HvButton>
        </>
      }
    >
      {row ? (
        <form id="adjustment-dialog-form" onSubmit={(event) => void onSubmit(event)} noValidate>
          <FieldGroup>
            <p className="text-[13px] text-ink-500">{row.student_name}</p>
            <Field data-invalid={Boolean(errors.amount)}>
              <FieldLabel htmlFor="adjustment-amount">Số tiền điều chỉnh (đồng)</FieldLabel>
              <Input
                id="adjustment-amount"
                type="number"
                step={10000}
                inputMode="numeric"
                aria-invalid={Boolean(errors.amount)}
                {...form.register("amount", { valueAsNumber: true })}
              />
              <FieldError errors={[errors.amount]} />
            </Field>
            <Field data-invalid={Boolean(errors.reason)}>
              <FieldLabel htmlFor="adjustment-reason">Lý do</FieldLabel>
              <textarea
                id="adjustment-reason"
                aria-invalid={Boolean(errors.reason)}
                className={textareaClassName}
                {...form.register("reason")}
              />
              <FieldError errors={[errors.reason]} />
            </Field>
            <p className="text-[14px] text-ink-700">
              Tổng mới: <span className="font-display font-bold">{formatMoney(newTotal)}</span>
            </p>
            <FieldError errors={[errors.root]} />
          </FieldGroup>
        </form>
      ) : null}
    </HvModal>
  );
}
