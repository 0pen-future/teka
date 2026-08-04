import { zodResolver } from "@hookform/resolvers/zod";
import { useForm } from "react-hook-form";
import { Link, useNavigate } from "react-router";

import { HvButton, HvCard } from "@/components/hv";
import { Field, FieldDescription, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { useApiFormErrors } from "@/lib/forms/use-api-form-errors";

import { useRegister } from "../hooks/use-auth";
import { registerSchema, type RegisterInput } from "../schemas/auth-schemas";

export function RegisterPage() {
  const navigate = useNavigate();
  const form = useForm<RegisterInput>({
    resolver: zodResolver(registerSchema),
    defaultValues: { full_name: "", phone: "", password: "" },
  });
  const registerMutation = useRegister();
  // A duplicate phone comes back as CONFLICT with no fields map — pin it to
  // the phone input instead of surfacing a toast.
  const handleApiError = useApiFormErrors(form, { conflictField: "phone" });

  const onSubmit = form.handleSubmit((values) => {
    registerMutation.mutate(values, {
      onSuccess: () => void navigate("/", { replace: true }),
      onError: handleApiError,
    });
  });

  const { errors } = form.formState;

  return (
    <HvCard variant="raised" padding="lg" className="mx-auto w-full max-w-[var(--w-phone)]">
      <div className="mb-6 text-center">
        <p className="font-display text-[22px] font-extrabold text-ink-900">Sổ Lớp</p>
        <p className="mt-1 text-[13px] text-ink-400">Tạo tài khoản giáo viên</p>
      </div>
      <form onSubmit={(event) => void onSubmit(event)} noValidate>
        <FieldGroup>
          <Field data-invalid={Boolean(errors.full_name)}>
            <FieldLabel
              htmlFor="full_name"
              className="font-display text-[14px] font-bold text-ink-700"
            >
              Họ và tên
            </FieldLabel>
            <Input
              id="full_name"
              autoComplete="name"
              className="h-12"
              aria-invalid={Boolean(errors.full_name)}
              {...form.register("full_name")}
            />
            <FieldError className="text-[14px] text-coral-600" errors={[errors.full_name]} />
          </Field>
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
              autoComplete="new-password"
              className="h-12"
              aria-invalid={Boolean(errors.password)}
              {...form.register("password")}
            />
            <FieldDescription className="text-[13px] text-ink-400">
              Tối thiểu 8 ký tự.
            </FieldDescription>
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
            disabled={registerMutation.isPending}
          >
            {registerMutation.isPending ? "Đang tạo tài khoản…" : "Tạo tài khoản"}
          </HvButton>
          <p className="text-center text-[13px] text-ink-400">
            Đã có tài khoản?{" "}
            <Link
              to="/login"
              className="font-bold text-mint-600 underline-offset-4 hover:underline"
            >
              Đăng nhập
            </Link>
          </p>
        </div>
      </form>
    </HvCard>
  );
}
