import { zodResolver } from "@hookform/resolvers/zod";
import { useState } from "react";
import { useForm } from "react-hook-form";
import { Link } from "react-router";

import { HvButton, HvCard } from "@/components/hv";
import { Field, FieldError, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";

import { useForgotPassword } from "../hooks/use-auth";
import { forgotPasswordInputSchema, type ForgotPasswordInput } from "../schemas/auth-schemas";

/**
 * Public, unauthenticated route. Submitting always lands on the same
 * generic confirmation, whatever the outcome — the API itself never
 * reveals whether the phone matched an eligible account (anti-enumeration),
 * so the page must not distinguish success from failure either.
 */
export function ForgotPasswordPage() {
  const [submitted, setSubmitted] = useState(false);
  const mutation = useForgotPassword();
  const form = useForm<ForgotPasswordInput>({
    resolver: zodResolver(forgotPasswordInputSchema),
    defaultValues: { phone: "" },
  });
  const { errors } = form.formState;

  const onSubmit = form.handleSubmit((values) => {
    mutation.mutate(values, { onSettled: () => setSubmitted(true) });
  });

  if (submitted) {
    return (
      <HvCard variant="raised" padding="lg" className="mx-auto w-full max-w-[var(--w-phone)]">
        <div className="flex flex-col items-center gap-3 text-center">
          <p className="font-display text-[17px] font-bold text-ink-900">Đã gửi yêu cầu</p>
          <p className="text-[14px] text-ink-500">
            Nếu số điện thoại hợp lệ, liên kết đặt lại đã được gửi qua Zalo.
          </p>
          <Link to="/login" className="font-bold text-mint-600 underline-offset-4 hover:underline">
            Quay lại đăng nhập
          </Link>
        </div>
      </HvCard>
    );
  }

  return (
    <HvCard variant="raised" padding="lg" className="mx-auto w-full max-w-[var(--w-phone)]">
      <div className="mb-6 text-center">
        <p className="font-display text-[17px] font-bold text-ink-900">Quên mật khẩu?</p>
        <p className="mt-1 text-[13px] text-ink-400">
          Nhập số điện thoại đăng nhập để nhận liên kết đặt lại mật khẩu.
        </p>
      </div>
      <form onSubmit={(event) => void onSubmit(event)} noValidate>
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
        <div className="mt-6">
          <HvButton type="submit" variant="primary" size="lg" block disabled={mutation.isPending}>
            {mutation.isPending ? "Đang gửi…" : "Gửi liên kết đặt lại"}
          </HvButton>
        </div>
      </form>
    </HvCard>
  );
}
