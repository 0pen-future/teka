import { zodResolver } from "@hookform/resolvers/zod";
import { useForm } from "react-hook-form";
import { Link, useLocation, useNavigate } from "react-router";

import { HvButton, HvCard } from "@/components/hv";
import { Field, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { useApiFormErrors } from "@/lib/forms/use-api-form-errors";

import { useLogin } from "../hooks/use-auth";
import { loginSchema, type LoginInput } from "../schemas/auth-schemas";

interface LocationState {
  from?: string;
}

export function LoginPage() {
  const navigate = useNavigate();
  const location = useLocation();
  const from = (location.state as LocationState | null)?.from;

  const form = useForm<LoginInput>({
    resolver: zodResolver(loginSchema),
    defaultValues: { phone: "", password: "" },
  });
  const loginMutation = useLogin();
  const handleApiError = useApiFormErrors(form);

  const onSubmit = form.handleSubmit((values) => {
    loginMutation.mutate(values, {
      onSuccess: () => void navigate(from ?? "/", { replace: true }),
      onError: handleApiError,
    });
  });

  const { errors } = form.formState;

  return (
    <HvCard variant="raised" padding="lg" className="mx-auto w-full max-w-[var(--w-phone)]">
      <div className="mb-6 text-center">
        <p className="font-display text-[22px] font-extrabold text-ink-900">Teka</p>
        <p className="mt-1 text-[13px] text-ink-400">Đăng nhập để tiếp tục</p>
      </div>
      <form onSubmit={(event) => void onSubmit(event)} noValidate>
        <FieldGroup>
          <Field data-invalid={Boolean(errors.phone)}>
            <FieldLabel htmlFor="phone" className="font-display text-[14px] font-bold text-ink-700">
              Số điện thoại
            </FieldLabel>
            <Input
              id="phone"
              type="tel"
              inputMode="numeric"
              autoComplete="tel"
              placeholder="0912345678"
              className="h-12"
              aria-invalid={Boolean(errors.phone)}
              {...form.register("phone")}
            />
            <FieldError className="text-[14px] text-coral-600" errors={[errors.phone]} />
          </Field>
          <Field data-invalid={Boolean(errors.password)}>
            <FieldLabel
              htmlFor="password"
              className="font-display text-[14px] font-bold text-ink-700"
            >
              Mật khẩu
            </FieldLabel>
            <Input
              id="password"
              type="password"
              autoComplete="current-password"
              className="h-12"
              aria-invalid={Boolean(errors.password)}
              {...form.register("password")}
            />
            <FieldError className="text-[14px] text-coral-600" errors={[errors.password]} />
          </Field>
          <FieldError className="text-[14px] text-coral-600" errors={[errors.root]} />
        </FieldGroup>
        <div className="mt-6 flex flex-col gap-3">
          <HvButton
            type="submit"
            variant="primary"
            size="lg"
            block
            disabled={loginMutation.isPending}
          >
            {loginMutation.isPending ? "Đang đăng nhập…" : "Đăng nhập"}
          </HvButton>
          <p className="text-center text-[13px] text-ink-400">
            <Link
              to="/forgot-password"
              className="font-bold text-mint-600 underline-offset-4 hover:underline"
            >
              Quên mật khẩu?
            </Link>
          </p>
        </div>
      </form>
    </HvCard>
  );
}
