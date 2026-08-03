import { zodResolver } from "@hookform/resolvers/zod";
import { useEffect } from "react";
import { Controller, useForm } from "react-hook-form";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Field, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { useApiFormErrors } from "@/lib/forms/use-api-form-errors";

import { useUpdateUser } from "../hooks/use-users";
import { updateUserSchema } from "../schemas/user-schemas";
import type { UpdateUserInput, User } from "../types/user-types";

interface EditUserDialogProps {
  user: User | null;
  onOpenChange: (open: boolean) => void;
  /** Role changes are admin-only server-side; hide the field for non-admins. */
  canEditRole: boolean;
}

export function EditUserDialog({ user, onOpenChange, canEditRole }: EditUserDialogProps) {
  const form = useForm<UpdateUserInput>({
    resolver: zodResolver(updateUserSchema),
    defaultValues: { name: "", role: "user" },
  });
  const updateMutation = useUpdateUser(user?.id ?? "");
  const handleApiError = useApiFormErrors(form);

  useEffect(() => {
    if (user) {
      form.reset({ name: user.name, role: user.role });
    }
  }, [user, form]);

  const onSubmit = form.handleSubmit((values) => {
    if (!user) {
      return;
    }
    // PATCH semantics: send only what changed so a non-admin editing their own
    // name never trips the admin-only role check.
    const patch: Partial<UpdateUserInput> = {};
    if (values.name !== user.name) {
      patch.name = values.name;
    }
    if (canEditRole && values.role !== user.role) {
      patch.role = values.role;
    }
    if (Object.keys(patch).length === 0) {
      onOpenChange(false);
      return;
    }
    updateMutation.mutate(patch, {
      onSuccess: (updated) => {
        toast.success(`User ${updated.email} updated`);
        onOpenChange(false);
      },
      onError: handleApiError,
    });
  });

  const { errors } = form.formState;

  return (
    <Dialog open={user !== null} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Edit user</DialogTitle>
          <DialogDescription>{user?.email}</DialogDescription>
        </DialogHeader>
        <form onSubmit={(event) => void onSubmit(event)} noValidate>
          <FieldGroup>
            <Field data-invalid={Boolean(errors.name)}>
              <FieldLabel htmlFor="edit-user-name">Name</FieldLabel>
              <Input
                id="edit-user-name"
                aria-invalid={Boolean(errors.name)}
                {...form.register("name")}
              />
              <FieldError errors={[errors.name]} />
            </Field>
            {canEditRole ? (
              <Field data-invalid={Boolean(errors.role)}>
                <FieldLabel htmlFor="edit-user-role">Role</FieldLabel>
                <Controller
                  control={form.control}
                  name="role"
                  render={({ field }) => (
                    <Select value={field.value} onValueChange={field.onChange}>
                      <SelectTrigger id="edit-user-role" className="w-full">
                        <SelectValue placeholder="Select a role" />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="user">User</SelectItem>
                        <SelectItem value="admin">Admin</SelectItem>
                      </SelectContent>
                    </Select>
                  )}
                />
                <FieldError errors={[errors.role]} />
              </Field>
            ) : null}
            <FieldError errors={[errors.root]} />
          </FieldGroup>
          <DialogFooter className="mt-6">
            <Button
              type="button"
              variant="outline"
              disabled={updateMutation.isPending}
              onClick={() => onOpenChange(false)}
            >
              Cancel
            </Button>
            <Button type="submit" disabled={updateMutation.isPending}>
              {updateMutation.isPending ? "Saving…" : "Save changes"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
