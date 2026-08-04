import { zodResolver } from "@hookform/resolvers/zod";
import { useEffect } from "react";
import { useForm } from "react-hook-form";

import { HvButton, HvModal } from "@/components/hv";
import { Field, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { useApiFormErrors } from "@/lib/forms/use-api-form-errors";

import { useCreateContact, useUpdateContact } from "../hooks/use-contacts";
import { contactInputSchema, type Contact, type ContactInput } from "../schemas/roster-schemas";

export interface ContactDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Present in edit mode; absent when creating a new contact. */
  contact?: Contact;
  /** Fires with the created/updated contact after a successful submit. */
  onSuccess?: (contact: Contact) => void;
}

const emptyValues: ContactInput = { full_name: "", phone: "" };

/**
 * Create/edit for a contact — the phone-owning parent/guardian record every
 * student must reference (PRD R1: phone lives on the contact, never on the
 * student).
 */
export function ContactDialog({ open, onOpenChange, contact, onSuccess }: ContactDialogProps) {
  const isEdit = Boolean(contact);
  const form = useForm<ContactInput>({
    resolver: zodResolver(contactInputSchema),
    defaultValues: contact ? { full_name: contact.full_name, phone: contact.phone } : emptyValues,
  });
  const createMutation = useCreateContact();
  const updateMutation = useUpdateContact(contact?.id ?? "");
  const mutation = isEdit ? updateMutation : createMutation;
  // A duplicate phone comes back as CONFLICT with no fields map.
  const handleApiError = useApiFormErrors(form, { conflictField: "phone" });

  useEffect(() => {
    if (open) {
      form.reset(contact ? { full_name: contact.full_name, phone: contact.phone } : emptyValues);
    }
    // form is stable from react-hook-form and not a meaningful dependency here.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, contact]);

  const onSubmit = form.handleSubmit((values) => {
    mutation.mutate(values, {
      onSuccess: (result) => {
        onOpenChange(false);
        onSuccess?.(result);
      },
      onError: handleApiError,
    });
  });

  const { errors } = form.formState;

  return (
    <HvModal
      open={open}
      onOpenChange={onOpenChange}
      title={isEdit ? "Sửa người liên hệ" : "Thêm người liên hệ"}
      footer={
        <>
          <HvButton type="button" variant="ghost" onClick={() => onOpenChange(false)}>
            Huỷ
          </HvButton>
          <HvButton type="submit" form="contact-dialog-form" disabled={mutation.isPending}>
            {mutation.isPending ? "Đang lưu…" : "Lưu"}
          </HvButton>
        </>
      }
    >
      <form id="contact-dialog-form" onSubmit={(event) => void onSubmit(event)} noValidate>
        <FieldGroup>
          <Field data-invalid={Boolean(errors.full_name)}>
            <FieldLabel htmlFor="contact-full-name">Họ và tên</FieldLabel>
            <Input
              id="contact-full-name"
              autoComplete="name"
              aria-invalid={Boolean(errors.full_name)}
              {...form.register("full_name")}
            />
            <FieldError errors={[errors.full_name]} />
          </Field>
          <Field data-invalid={Boolean(errors.phone)}>
            <FieldLabel htmlFor="contact-phone">Số điện thoại</FieldLabel>
            <Input
              id="contact-phone"
              type="tel"
              inputMode="numeric"
              autoComplete="tel"
              placeholder="0912345678"
              aria-invalid={Boolean(errors.phone)}
              {...form.register("phone")}
            />
            <FieldError errors={[errors.phone]} />
          </Field>
          <FieldError errors={[errors.root]} />
        </FieldGroup>
      </form>
    </HvModal>
  );
}
