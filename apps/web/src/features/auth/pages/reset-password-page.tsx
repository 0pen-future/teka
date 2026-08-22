import { zodResolver } from "@hookform/resolvers/zod";
import { useForm } from "react-hook-form";
import { Link, useNavigate, useParams } from "react-router";

import { HvButton, HvCard, hvToast } from "@/components/hv";
import { Field, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { useNoIndex } from "@/lib/hooks/use-no-index";

import { useResetPassword } from "../hooks/use-auth";
import { resetPasswordFormSchema, type ResetPasswordFormInput } from "../schemas/auth-schemas";

/**
 * Route element for `/reset-password/:token` — a public, unauthenticated,
 * token-bearing URL, so `useNoIndex` keeps it out of search results the same
 * way the invite-accept page does. The API collapses every rejection reason
 * (unknown/used/expired/superseded token, inactive account) into the same
 * generic 400, so the failure state here draws no distinction either.
 */
export function ResetPasswordPage() {
  const { token } = useParams<{ token: string }>();
  const navigate = useNavigate();
  useNoIndex();

  const mutation = useResetPassword();
  const form = useForm<ResetPasswordFormInput>({
    resolver: zodResolver(resetPasswordFormSchema),
    defaultValues: { password: "", confirm_password: "" },
  });
  const { errors } = form.formState;

  const onSubmit = form.handleSubmit(async (values) => {
    if (!token) {
      return;
    }
    try {
      await mutation.mutateAsync({ token, password: values.password });
      hvToast("Đã đặt lại mật khẩu, mời đăng nhập", { variant: "success" });
      void navigate("/login", { replace: true });
    } catch {
      form.setError("root", {
        type: "server",
        message: "Không thể đặt lại mật khẩu. Liên kết có thể đã hết hạn hoặc đã được dùng.",
      });
    }
  });

  if (!token) {
    return (
      <HvCard variant="raised" padding="lg" className="mx-auto w-full max-w-[var(--w-phone)]">
        <div className="flex flex-col items-center gap-3 text-center">
          <p className="font-display text-[17px] font-bold text-ink-900">
            Không mở được liên kết này.
          </p>
          <Link
            to="/forgot-password"
            className="font-bold text-mint-600 underline-offset-4 hover:underline"
          >
            Gửi lại yêu cầu đặt lại mật khẩu
          </Link>
        </div>
      </HvCard>
    );
  }

  return (
    <HvCard variant="raised" padding="lg" className="mx-auto w-full max-w-[var(--w-phone)]">
      <div className="mb-6 text-center">
        <p className="font-display text-[17px] font-bold text-ink-900">Đặt lại mật khẩu</p>
      </div>
      <form onSubmit={(event) => void onSubmit(event)} noValidate>
        <FieldGroup>
          <Field data-invalid={Boolean(errors.password)}>
            <FieldLabel htmlFor="password">Mật khẩu mới</FieldLabel>
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
          <HvButton type="submit" variant="primary" size="lg" block disabled={mutation.isPending}>
            {mutation.isPending ? "Đang đặt lại…" : "Đặt lại mật khẩu"}
          </HvButton>
        </div>
      </form>
    </HvCard>
  );
}
