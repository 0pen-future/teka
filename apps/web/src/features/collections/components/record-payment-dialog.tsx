import { zodResolver } from "@hookform/resolvers/zod";
import { useEffect, useRef, useState } from "react";
import { useForm, useWatch } from "react-hook-form";

import { HvButton, HvModal, hvToast } from "@/components/hv";
import { Field, FieldDescription, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { useApiFormErrors } from "@/lib/forms/use-api-form-errors";
import { formatMoney } from "@/lib/utils";

import { AllocationEditor, type AllocationLine } from "./allocation-editor";
import { MoneyField } from "./money-field";
import { useReallocatePayment, useRecordPayment } from "../hooks/use-collections";
import {
  recordPaymentInputSchema,
  type PaymentMethod,
  type PaymentResponse,
  type RecordPaymentInput,
} from "../schemas/collections-schemas";

export interface RecordPaymentDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  periodId: string;
  contactId: string;
  contactName: string;
}

const methodLabels: Record<PaymentMethod, string> = {
  cash: "Tiền mặt",
  transfer: "Chuyển khoản",
  other: "Khác",
};

function today(): string {
  return new Date().toISOString().slice(0, 10);
}

function toDefaultValues(contactId: string): RecordPaymentInput {
  return {
    contact_id: contactId,
    amount: 0,
    method: "cash",
    received_on: today(),
    reference_code: "",
    note: "",
  };
}

/**
 * `modalPay` — records a payment, then shows the server's auto-allocated
 * split for review or override. The real API has no preview-before-write
 * endpoint: `POST /payments` always writes and auto-allocates in one step
 * (the D8 oldest-debt-first rule), so step one below *is* the write; step
 * two ("Phân bổ") only ever corrects it afterward through
 * `PUT /payments/:id/allocations`, which is also the only place
 * `allocated_by` flips to `"manual"`.
 */
export function RecordPaymentDialog({
  open,
  onOpenChange,
  periodId,
  contactId,
  contactName,
}: RecordPaymentDialogProps) {
  const [payment, setPayment] = useState<PaymentResponse | null>(null);
  const [allocations, setAllocations] = useState<AllocationLine[]>([]);
  const prevOpenRef = useRef(open);

  const form = useForm<RecordPaymentInput>({
    resolver: zodResolver(recordPaymentInputSchema),
    defaultValues: toDefaultValues(contactId),
  });
  const handleApiError = useApiFormErrors(form);
  const recordMutation = useRecordPayment(periodId);
  const reallocateMutation = useReallocatePayment(periodId);
  const amount = useWatch({ control: form.control, name: "amount" });
  const method = useWatch({ control: form.control, name: "method" });

  useEffect(() => {
    // Reset state only when dialog transitions from closed to open
    if (open && !prevOpenRef.current) {
      form.reset(toDefaultValues(contactId));
      prevOpenRef.current = true;
      // Schedule state resets asynchronously to avoid cascading renders
      const timer = setTimeout(() => {
        setPayment(null);
        setAllocations([]);
      }, 0);
      return () => clearTimeout(timer);
    }
    prevOpenRef.current = open;
    // form is stable from react-hook-form and not a meaningful dependency here.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, contactId]);

  const { errors } = form.formState;

  const onSubmit = form.handleSubmit((values) => {
    recordMutation.mutate(values, {
      onSuccess: (result) => {
        setPayment(result);
        setAllocations(
          result.allocations.map((allocation) => ({
            invoice_id: allocation.invoice_id,
            student_name: allocation.student_name,
            amount: allocation.amount,
          })),
        );
      },
      onError: handleApiError,
    });
  });

  function close() {
    onOpenChange(false);
  }

  if (payment) {
    const isDefaultAllocation =
      allocations.length === payment.allocations.length &&
      allocations.every((line) => {
        const original = payment.allocations.find((a) => a.invoice_id === line.invoice_id);
        return original?.amount === line.amount;
      });
    const allocationSum = allocations.reduce((total, line) => total + line.amount, 0);
    const allocationValid = allocationSum === payment.amount;

    function submitReallocation() {
      reallocateMutation.mutate(
        {
          paymentId: payment!.id,
          input: {
            allocations: allocations.map((line) => ({
              invoice_id: line.invoice_id,
              amount: line.amount,
            })),
          },
        },
        {
          onSuccess: (result) => {
            setPayment(result);
            hvToast("Đã cập nhật phân bổ", { variant: "success" });
          },
          onError: () => {
            hvToast("Không thể cập nhật phân bổ", { variant: "danger" });
          },
        },
      );
    }

    return (
      <HvModal
        open={open}
        onOpenChange={onOpenChange}
        title="Ghi nhận thu"
        footer={
          isDefaultAllocation ? (
            <HvButton onClick={close}>Xong</HvButton>
          ) : (
            <>
              <HvButton variant="ghost" onClick={close}>
                Đóng
              </HvButton>
              <HvButton
                onClick={submitReallocation}
                disabled={!allocationValid || reallocateMutation.isPending}
              >
                {reallocateMutation.isPending ? "Đang lưu…" : "Cập nhật phân bổ"}
              </HvButton>
            </>
          )
        }
      >
        <div className="flex flex-col gap-3">
          <p className="text-[14px] text-ink-700">
            Đã ghi nhận <strong>{formatMoney(payment.amount)}</strong> từ {contactName}.
          </p>
          <AllocationEditor
            defaultAllocations={payment.allocations}
            value={allocations}
            amountTotal={payment.amount}
            onChange={setAllocations}
            disabled={reallocateMutation.isPending}
          />
        </div>
      </HvModal>
    );
  }

  return (
    <HvModal
      open={open}
      onOpenChange={onOpenChange}
      title="Ghi nhận thu"
      footer={
        <>
          <HvButton type="button" variant="ghost" onClick={close}>
            Huỷ
          </HvButton>
          <HvButton type="submit" form="record-payment-form" disabled={recordMutation.isPending}>
            {recordMutation.isPending ? "Đang lưu…" : "Ghi nhận"}
          </HvButton>
        </>
      }
    >
      <form id="record-payment-form" onSubmit={(event) => void onSubmit(event)} noValidate>
        <FieldGroup>
          <p className="text-[13px] text-ink-400">Từ {contactName}</p>
          <Field data-invalid={Boolean(errors.amount)}>
            <FieldLabel htmlFor="payment-amount">Số tiền</FieldLabel>
            <MoneyField
              id="payment-amount"
              value={amount}
              onChange={(next) => form.setValue("amount", next, { shouldValidate: true })}
              aria-invalid={Boolean(errors.amount)}
            />
            <FieldError errors={[errors.amount]} />
          </Field>
          <Field data-invalid={Boolean(errors.method)}>
            <FieldLabel htmlFor="payment-method">Hình thức</FieldLabel>
            <Select
              value={method}
              onValueChange={(value) =>
                form.setValue("method", value as PaymentMethod, { shouldValidate: true })
              }
            >
              <SelectTrigger
                id="payment-method"
                className="w-full"
                aria-invalid={Boolean(errors.method)}
              >
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {(Object.keys(methodLabels) as PaymentMethod[]).map((m) => (
                  <SelectItem key={m} value={m}>
                    {methodLabels[m]}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <FieldError errors={[errors.method]} />
          </Field>
          <Field data-invalid={Boolean(errors.received_on)}>
            <FieldLabel htmlFor="payment-received-on">Ngày thu</FieldLabel>
            <Input
              id="payment-received-on"
              type="date"
              aria-invalid={Boolean(errors.received_on)}
              {...form.register("received_on")}
            />
            <FieldError errors={[errors.received_on]} />
          </Field>
          <Field data-invalid={Boolean(errors.reference_code)}>
            <FieldLabel htmlFor="payment-reference-code">Mã tham chiếu</FieldLabel>
            <Input
              id="payment-reference-code"
              aria-invalid={Boolean(errors.reference_code)}
              {...form.register("reference_code")}
            />
            <FieldDescription>Không bắt buộc — mã giao dịch chuyển khoản, nếu có</FieldDescription>
            <FieldError errors={[errors.reference_code]} />
          </Field>
          <Field data-invalid={Boolean(errors.note)}>
            <FieldLabel htmlFor="payment-note">Ghi chú</FieldLabel>
            <Input
              id="payment-note"
              aria-invalid={Boolean(errors.note)}
              {...form.register("note")}
            />
            <FieldError errors={[errors.note]} />
          </Field>
          <FieldError errors={[errors.root]} />
        </FieldGroup>
      </form>
    </HvModal>
  );
}
