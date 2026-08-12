import { zodResolver } from "@hookform/resolvers/zod";
import { useForm } from "react-hook-form";

import { HvButton, hvToast } from "@/components/hv";
import { Field, FieldError, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { ApiError } from "@/lib/api/errors";
import { useApiFormErrors } from "@/lib/forms/use-api-form-errors";

import { useJoinCenter } from "../hooks/use-center";
import { joinCenterInputSchema, type JoinCenterInput } from "../schemas/center-schemas";

/**
 * The raw server messages (English, internal) are replaced with actionable
 * copy: NOT_FOUND — no account owns that phone; CONFLICT — usually this
 * account still holds data or members, but the same code also covers a
 * concurrent-membership race where retrying succeeds, hence the retry hint;
 * VALIDATION_ERROR without fields — the only field is pre-validated
 * client-side, so what remains is the self-join rejection.
 */
const JOIN_ERROR_MESSAGES: Record<string, string> = {
  NOT_FOUND: "Không tìm thấy chủ trung tâm với số này",
  CONFLICT:
    "Chưa thể gia nhập: tài khoản của bạn đã có dữ liệu hoặc thành viên. Vui lòng kiểm tra rồi thử lại.",
  VALIDATION_ERROR: "Không thể tự gia nhập trung tâm của chính mình",
};

export function JoinCenterForm() {
  const mutation = useJoinCenter();
  const form = useForm<JoinCenterInput>({
    resolver: zodResolver(joinCenterInputSchema),
    defaultValues: { owner_phone: "" },
  });
  const handleApiError = useApiFormErrors(form);
  const { errors } = form.formState;

  const onSubmit = form.handleSubmit(async (values) => {
    try {
      await mutation.mutateAsync(values);
      hvToast("Đã gia nhập trung tâm", { variant: "success" });
    } catch (error) {
      const hasFields =
        error instanceof ApiError && error.fields && Object.keys(error.fields).length > 0;
      const mapped = error instanceof ApiError ? JOIN_ERROR_MESSAGES[error.code] : undefined;
      if (!hasFields && mapped) {
        form.setError("root", { type: "server", message: mapped });
        return;
      }
      handleApiError(error);
    }
  });

  return (
    <form onSubmit={(event) => void onSubmit(event)} noValidate className="flex flex-col gap-3">
      <Field data-invalid={Boolean(errors.owner_phone)}>
        <FieldLabel htmlFor="join-owner-phone">Số điện thoại chủ trung tâm</FieldLabel>
        <Input
          id="join-owner-phone"
          type="tel"
          inputMode="tel"
          aria-invalid={Boolean(errors.owner_phone)}
          {...form.register("owner_phone")}
        />
        <FieldError errors={[errors.owner_phone]} />
      </Field>
      {/* Account-level failures (already has data, self-join) are not about
          the phone input, so they render below the field, not inside it. */}
      <FieldError errors={[errors.root]} />
      <div className="flex justify-end">
        <HvButton type="submit" disabled={mutation.isPending}>
          {mutation.isPending ? "Đang gửi…" : "Gia nhập"}
        </HvButton>
      </div>
    </form>
  );
}
