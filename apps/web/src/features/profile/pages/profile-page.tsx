import { zodResolver } from "@hookform/resolvers/zod";
import { useForm } from "react-hook-form";

import { HvButton, HvCard, hvToast } from "@/components/hv";
import { Field, FieldError, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { useAuthStore } from "@/features/auth";
import { useCurrentPeriod } from "@/features/billing";
import { useClassesList, useStudentsList } from "@/features/roster";
import { useApiFormErrors } from "@/lib/forms/use-api-form-errors";
import { formatPhoneLocal, nameInitial } from "@/lib/utils";

import { ZaloConnectCard } from "../components/zalo-connect-card";
import { useUpdateMe } from "../hooks/use-profile";
import { profileFormSchema, type ProfileFormInput } from "../schemas/profile-schemas";

const DATA_PROMISES = [
  "Chỉ lưu tên, ngày nhập học và lớp của học sinh",
  "Xoá học sinh là xoá thật dữ liệu cá nhân — chỉ giữ bản ghi tài chính ẩn danh",
  "Link phụ huynh hết hiệu lực sau khi thanh toán hoặc 90 ngày",
];

/**
 * "Hồ sơ giáo viên" (prototype `isProfile` screen). Only "Tên hiển thị"
 * persists (PUT /me); môn dạy and bank fields have no server columns yet, so
 * they render empty, feed the live Zalo-footer preview, and reset on reload.
 * The .xlsx export is later scope — its control matches the prototype but only
 * announces that via toast.
 */
export function ProfilePage() {
  const user = useAuthStore((state) => state.user);
  const { data: period } = useCurrentPeriod();
  const { data: classesPage } = useClassesList({ per_page: 1 });
  const { data: studentsPage } = useStudentsList({ per_page: 1 });
  const updateMutation = useUpdateMe();

  const form = useForm<ProfileFormInput>({
    resolver: zodResolver(profileFormSchema),
    defaultValues: {
      full_name: user?.full_name ?? "",
      subject: "",
      bank: "",
      account: "",
      holder: "",
    },
  });
  const handleApiError = useApiFormErrors(form);
  const { errors } = form.formState;

  const values = form.watch();
  const displayName = values.full_name.trim() || (user?.full_name ?? "");
  const summaryParts = [
    classesPage ? `${classesPage.meta.total} lớp` : null,
    studentsPage ? `${studentsPage.meta.total} học sinh` : null,
  ].filter(Boolean);
  const periodLabel = period ? `T${period.month}/${period.year}` : "";
  const transferPreview = [
    values.holder.trim(),
    [values.bank.trim(), values.account.trim()].filter(Boolean).join(" "),
  ]
    .filter(Boolean)
    .join(" · ");

  const onSubmit = form.handleSubmit(async (formValues) => {
    if (!user) {
      return;
    }
    try {
      await updateMutation.mutateAsync({
        full_name: formValues.full_name.trim(),
        timezone: user.timezone,
      });
      hvToast("Đã lưu hồ sơ");
    } catch (error) {
      handleApiError(error);
    }
  });

  return (
    <div>
      <h1 className="font-display text-[26px] font-extrabold text-ink-900">Hồ sơ giáo viên</h1>
      <p className="mt-1 text-[14px] text-ink-500">
        Thông tin này hiện trên tin nhắn học phí và trang phụ huynh.
      </p>

      <div className="mt-[18px] flex flex-wrap items-start gap-5">
        <form
          onSubmit={(event) => void onSubmit(event)}
          noValidate
          className="flex min-w-[340px] flex-[1.3] flex-col gap-4"
        >
          <HvCard>
            <div className="flex items-center gap-4">
              <span
                aria-hidden
                className="flex size-16 shrink-0 items-center justify-center rounded-full bg-mint-100 font-display text-[26px] font-extrabold text-mint-600"
              >
                {nameInitial(displayName)}
              </span>
              <div className="min-w-0">
                <p className="truncate font-display text-[19px] font-bold text-ink-900">
                  {displayName}
                </p>
                <p className="text-[13px] text-ink-500">{summaryParts.join(" · ")}</p>
              </div>
            </div>
            <div className="mt-4 grid gap-3 sm:grid-cols-2">
              <Field data-invalid={Boolean(errors.full_name)}>
                <FieldLabel htmlFor="profile-full-name">Tên hiển thị</FieldLabel>
                <Input
                  id="profile-full-name"
                  aria-invalid={Boolean(errors.full_name)}
                  {...form.register("full_name")}
                />
                <FieldError errors={[errors.full_name]} />
              </Field>
              <Field>
                <FieldLabel htmlFor="profile-subject">Môn dạy</FieldLabel>
                <Input id="profile-subject" {...form.register("subject")} />
                <p className="text-[12px] text-ink-400">
                  Chưa lưu trên máy chủ — tính năng đang phát triển.
                </p>
              </Field>
            </div>
            <Field className="mt-3">
              <FieldLabel htmlFor="profile-phone">Số điện thoại (Zalo)</FieldLabel>
              <Input id="profile-phone" readOnly value={user ? formatPhoneLocal(user.phone) : ""} />
              <p className="text-[12px] text-ink-400">
                Phụ huynh nhận thông báo học phí từ số Zalo này.
              </p>
            </Field>
            <FieldError errors={[errors.root]} />
          </HvCard>

          <ZaloConnectCard />

          <HvCard>
            <p className="font-display text-[17px] font-bold text-ink-900">
              Tài khoản nhận học phí
            </p>
            <p className="mt-0.5 text-[12.5px] text-ink-500">
              Dùng để sinh mã QR chuyển khoản trên trang phụ huynh.
            </p>
            <div className="mt-3.5 grid gap-3 sm:grid-cols-2">
              <Field>
                <FieldLabel htmlFor="profile-bank">Ngân hàng</FieldLabel>
                <Input id="profile-bank" {...form.register("bank")} />
              </Field>
              <Field>
                <FieldLabel htmlFor="profile-account">Số tài khoản</FieldLabel>
                <Input id="profile-account" {...form.register("account")} />
              </Field>
            </div>
            <Field className="mt-3">
              <FieldLabel htmlFor="profile-holder">Chủ tài khoản</FieldLabel>
              <Input id="profile-holder" {...form.register("holder")} />
            </Field>
            <p className="mt-2.5 text-[12px] text-ink-400">
              Chưa lưu trên máy chủ — tính năng đang phát triển.
            </p>
          </HvCard>

          <div className="flex justify-end">
            <HvButton type="submit" disabled={updateMutation.isPending}>
              {updateMutation.isPending ? "Đang lưu…" : "Lưu hồ sơ"}
            </HvButton>
          </div>
        </form>

        <div className="flex min-w-[280px] flex-1 flex-col gap-4">
          <div className="rounded-[24px] bg-surface-dark p-5 text-[var(--text-on-dark)]">
            <p className="text-[12px] font-extrabold tracking-[0.4px] opacity-80">
              XEM TRƯỚC — CHÂN TIN NHẮN ZALO
            </p>
            <div className="mt-2.5 rounded-[16px] bg-white/10 px-3.5 py-3 text-[13.5px] leading-[1.6]">
              <p className="font-extrabold">
                [Học phí {periodLabel}] {displayName}
                {values.subject.trim() ? ` — ${values.subject.trim()}` : ""}
              </p>
              {transferPreview ? (
                <p className="opacity-85">Chuyển khoản: {transferPreview}</p>
              ) : null}
            </div>
            <p className="mt-2.5 text-[12px] opacity-75">
              Thay đổi hồ sơ áp dụng cho các lần gửi sau — tin đã gửi không đổi.
            </p>
          </div>

          <HvCard>
            <p className="font-display text-[17px] font-bold text-ink-900">Dữ liệu của bạn</p>
            <ul className="mt-3 flex flex-col gap-2.5 text-[13.5px] text-ink-700">
              {DATA_PROMISES.map((promise) => (
                <li key={promise} className="flex gap-2.5">
                  <span aria-hidden className="font-black text-mint-600">
                    ✓
                  </span>
                  <span>{promise}</span>
                </li>
              ))}
            </ul>
            <button
              type="button"
              onClick={() => hvToast("Tải dữ liệu — tính năng đang phát triển")}
              className="mt-3.5 cursor-pointer rounded-xl border-[1.5px] border-line-300 px-3.5 py-2 text-[13px] font-extrabold text-ink-500 transition-colors hover:border-mint-400 hover:text-mint-600"
            >
              Tải toàn bộ dữ liệu (.xlsx)
            </button>
          </HvCard>
        </div>
      </div>
    </div>
  );
}
