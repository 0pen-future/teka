import { zodResolver } from "@hookform/resolvers/zod";
import { useState } from "react";
import { useForm } from "react-hook-form";

import { HvButton, HvCard } from "@/components/hv";
import { Field, FieldError, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { useApiFormErrors } from "@/lib/forms/use-api-form-errors";

import { useCreateInvite } from "../hooks/use-invitation";
import {
  createInviteInputSchema,
  type CreateInviteInput,
  type CreateInviteResponse,
} from "../schemas/invitation-schemas";
import { CopyLinkDialog } from "./copy-link-dialog";
import { InviteList } from "./invite-list";

/**
 * Owner-only card: invite a new teacher by phone, then hand back a copyable
 * link plus the pending-invite roster. Creating an invite for a phone with
 * an existing pending invite supersedes it server-side, so the form never
 * needs to check for a duplicate before submitting.
 */
export function InviteSection() {
  const mutation = useCreateInvite();
  const [created, setCreated] = useState<CreateInviteResponse | null>(null);
  const form = useForm<CreateInviteInput>({
    resolver: zodResolver(createInviteInputSchema),
    defaultValues: { phone: "" },
  });
  const handleApiError = useApiFormErrors(form);
  const { errors } = form.formState;

  const onSubmit = form.handleSubmit(async (values) => {
    try {
      const result = await mutation.mutateAsync(values);
      form.reset({ phone: "" });
      setCreated(result);
    } catch (error) {
      handleApiError(error);
    }
  });

  return (
    <HvCard>
      <p className="font-display text-[17px] font-bold text-ink-900">Mời giáo viên</p>
      <p className="mt-0.5 mb-3 text-[12.5px] text-ink-500">
        Nhập số điện thoại giáo viên để gửi lời mời gia nhập trung tâm.
      </p>
      <form onSubmit={(event) => void onSubmit(event)} noValidate className="flex flex-col gap-3">
        <Field data-invalid={Boolean(errors.phone)}>
          <FieldLabel htmlFor="invite-phone">Số điện thoại</FieldLabel>
          <Input
            id="invite-phone"
            type="tel"
            inputMode="tel"
            aria-invalid={Boolean(errors.phone)}
            {...form.register("phone")}
          />
          <FieldError errors={[errors.phone, errors.root]} />
        </Field>
        <div className="flex justify-end">
          <HvButton type="submit" disabled={mutation.isPending}>
            {mutation.isPending ? "Đang gửi…" : "Gửi lời mời"}
          </HvButton>
        </div>
      </form>

      <div className="mt-4 border-t border-line-200 pt-4">
        <InviteList />
      </div>

      {created ? (
        <CopyLinkDialog
          open
          onOpenChange={(open) => {
            if (!open) {
              setCreated(null);
            }
          }}
          invite={created}
        />
      ) : null}
    </HvCard>
  );
}
