import { zodResolver } from "@hookform/resolvers/zod";
import { useEffect } from "react";
import { useForm } from "react-hook-form";

import { HvButton, HvModal, hvToast } from "@/components/hv";
import { Field, FieldError, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { useApiFormErrors } from "@/lib/forms/use-api-form-errors";

import { useRenameCenter } from "../hooks/use-center";
import { renameCenterInputSchema, type RenameCenterInput } from "../schemas/center-schemas";

export interface RenameCenterDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  currentName: string;
}

export function RenameCenterDialog({ open, onOpenChange, currentName }: RenameCenterDialogProps) {
  const mutation = useRenameCenter();
  const form = useForm<RenameCenterInput>({
    resolver: zodResolver(renameCenterInputSchema),
    defaultValues: { name: currentName },
  });
  const handleApiError = useApiFormErrors(form);
  const { errors } = form.formState;

  // Re-seed on every open: the mounted form would otherwise keep the name
  // from the previous rename (or a discarded draft) as its value.
  useEffect(() => {
    if (open) {
      form.reset({ name: currentName });
    }
  }, [open, currentName, form]);

  const onSubmit = form.handleSubmit(async (values) => {
    try {
      await mutation.mutateAsync(values);
      hvToast("Đã đổi tên trung tâm", { variant: "success" });
      onOpenChange(false);
    } catch (error) {
      handleApiError(error);
    }
  });

  return (
    <HvModal open={open} onOpenChange={onOpenChange} title="Đổi tên trung tâm">
      <form onSubmit={(event) => void onSubmit(event)} noValidate className="flex flex-col gap-4">
        <Field data-invalid={Boolean(errors.name)}>
          <FieldLabel htmlFor="center-name">Tên trung tâm</FieldLabel>
          <Input id="center-name" aria-invalid={Boolean(errors.name)} {...form.register("name")} />
          <FieldError errors={[errors.name, errors.root]} />
        </Field>
        <div className="flex justify-end gap-2">
          <HvButton type="button" variant="ghost" onClick={() => onOpenChange(false)}>
            Huỷ
          </HvButton>
          <HvButton type="submit" disabled={mutation.isPending}>
            {mutation.isPending ? "Đang lưu…" : "Lưu"}
          </HvButton>
        </div>
      </form>
    </HvModal>
  );
}
