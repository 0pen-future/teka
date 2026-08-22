import { zodResolver } from "@hookform/resolvers/zod";
import { useForm } from "react-hook-form";
import { useNavigate, useParams } from "react-router";

import { HvButton, HvCard, hvToast } from "@/components/hv";
import { Field, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { ApiError } from "@/lib/api/errors";
import { useNoIndex } from "@/lib/hooks/use-no-index";

import { InviteError } from "../components/invite-error";
import { useAcceptInvite, useInvitePreview } from "../hooks/use-invitation";
import { acceptInviteFormSchema, type AcceptInviteFormInput } from "../schemas/invitation-schemas";

/**
 * Route element for `/invite/:token` — a public, unauthenticated onboarding
 * page. Preview and accept both answer the same generic failure for every
 * rejection reason (unknown/expired/revoked/already-accepted token), so this
 * page draws no distinction either: any preview error or accept error lands
 * on the same `InviteError` state, never a per-field or per-reason message.
 */
export function AcceptInvitePage() {
  const { token } = useParams<{ token: string }>();
  const navigate = useNavigate();
  useNoIndex();

  const { data: preview, isPending, isError: previewError } = useInvitePreview(token);
  const acceptMutation = useAcceptInvite();

  const form = useForm<AcceptInviteFormInput>({
    resolver: zodResolver(acceptInviteFormSchema),
    defaultValues: { full_name: "", password: "", confirm_password: "" },
  });
  const { errors } = form.formState;

  const onSubmit = form.handleSubmit(async (values) => {
    if (!token) {
      return;
    }
    try {
      await acceptMutation.mutateAsync({
        token,
        full_name: values.full_name,
        password: values.password,
      });
      hvToast("Đã tạo tài khoản, mời đăng nhập", { variant: "success" });
      void navigate("/login", { replace: true });
    } catch (error) {
      // Anti-enumeration: the server never says *why* it rejected, so there
      // is nothing more specific to map onto a field — every failure lands
      // on the same generic root message, regardless of ApiError.code.
      const message =
        error instanceof ApiError
          ? "Không thể tạo tài khoản. Liên kết có thể đã hết hạn hoặc đã được dùng."
          : "Có lỗi xảy ra, thử lại sau";
      form.setError("root", { type: "server", message });
    }
  });

  if (!token || previewError) {
    return <InviteError />;
  }
  if (isPending || !preview) {
    return <p className="text-center text-[14px] text-ink-400">Đang tải…</p>;
  }

  return (
    <HvCard variant="raised" padding="lg" className="mx-auto w-full max-w-[var(--w-phone)]">
      <div className="mb-6 text-center">
        <p className="font-display text-[22px] font-extrabold text-ink-900">
          {preview.center_name}
        </p>
        <p className="mt-1 text-[13px] text-ink-400">Tạo tài khoản cho số {preview.phone_masked}</p>
      </div>
      <form onSubmit={(event) => void onSubmit(event)} noValidate>
        <FieldGroup>
          <Field data-invalid={Boolean(errors.full_name)}>
            <FieldLabel htmlFor="full_name">Họ và tên</FieldLabel>
            <Input
              id="full_name"
              autoComplete="name"
              aria-invalid={Boolean(errors.full_name)}
              {...form.register("full_name")}
            />
            <FieldError errors={[errors.full_name]} />
          </Field>
          <Field data-invalid={Boolean(errors.password)}>
            <FieldLabel htmlFor="password">Mật khẩu</FieldLabel>
            <Input
              id="password"
              type="password"
              autoComplete="new-password"
              aria-invalid={Boolean(errors.password)}
              {...form.register("password")}
            />
            <FieldError errors={[errors.password]} />
          </Field>
          <Field data-invalid={Boolean(errors.confirm_password)}>
            <FieldLabel htmlFor="confirm_password">Xác nhận mật khẩu</FieldLabel>
            <Input
              id="confirm_password"
              type="password"
              autoComplete="new-password"
              aria-invalid={Boolean(errors.confirm_password)}
              {...form.register("confirm_password")}
            />
            <FieldError errors={[errors.confirm_password]} />
          </Field>
          <FieldError errors={[errors.root]} />
        </FieldGroup>
        <div className="mt-6">
          <HvButton
            type="submit"
            variant="primary"
            size="lg"
            block
            disabled={acceptMutation.isPending}
          >
            {acceptMutation.isPending ? "Đang tạo tài khoản…" : "Tạo tài khoản"}
          </HvButton>
        </div>
      </form>
    </HvCard>
  );
}
