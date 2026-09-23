import { zodResolver } from "@hookform/resolvers/zod";
import { useState } from "react";
import { useForm } from "react-hook-form";
import { Link, useNavigate } from "react-router";
import { z } from "zod";

import { HvButton, HvStateBlock } from "@/components/hv";
import { Field, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { useCenterContext } from "@/features/teaching";
import { useApiFormErrors } from "@/lib/forms/use-api-form-errors";
import { cn } from "@/lib/utils";

import { TemplateFields } from "../components/template-fields";
import { useCreateTemplate } from "../hooks/use-library";
import {
  templateFormSchema,
  toTemplateInput,
  type TemplateFormInput,
  type TemplateFormValues,
} from "../schemas/library-schemas";

const MAX_LESSONS = 100;
const DEFAULT_LESSON_COUNT = 10;

const EMPTY_TEMPLATE: TemplateFormInput = {
  code: "",
  name: "",
  subject: "",
  level: "",
  description: "",
};

const lessonCountSchema = z.object({
  lesson_count: z.coerce
    .number({ message: "Số buổi từ 1 đến 100" })
    .int("Số buổi từ 1 đến 100")
    .min(1, "Số buổi từ 1 đến 100")
    .max(MAX_LESSONS, "Số buổi từ 1 đến 100"),
});
type LessonCountInput = z.input<typeof lessonCountSchema>;
type LessonCountValues = z.output<typeof lessonCountSchema>;

const STEPS = ["Thông tin", "Số buổi", "Xác nhận"] as const;
type Step = 0 | 1 | 2;

/**
 * `/library/templates/new` — a three-step alternative to the template
 * dialog: the template's fields, how many lessons to pre-create, then a
 * summary. Creating lands on the new draft's preparation board so the
 * checklist work can start at once.
 */
export function TemplateCreateWizardPage() {
  const { has, isResolved } = useCenterContext();
  const navigate = useNavigate();
  const [step, setStep] = useState<Step>(0);

  const templateForm = useForm<TemplateFormInput, unknown, TemplateFormValues>({
    resolver: zodResolver(templateFormSchema),
    defaultValues: EMPTY_TEMPLATE,
  });
  const countForm = useForm<LessonCountInput, unknown, LessonCountValues>({
    resolver: zodResolver(lessonCountSchema),
    defaultValues: { lesson_count: DEFAULT_LESSON_COUNT },
  });
  const [template, setTemplate] = useState<TemplateFormValues | null>(null);
  const [lessonCount, setLessonCount] = useState(DEFAULT_LESSON_COUNT);
  const mutation = useCreateTemplate();
  const handleApiError = useApiFormErrors(templateForm, { conflictField: "code" });

  if (!isResolved) {
    return <HvStateBlock state="loading" title="Đang tải" />;
  }
  if (!has("library.edit")) {
    return (
      <HvStateBlock
        state="error"
        title="Bạn không có quyền tạo chương trình mẫu"
        action={
          <Link
            to="/library"
            className="font-display text-[13px] font-bold text-mint-600 hover:underline"
          >
            Về kho học liệu
          </Link>
        }
      />
    );
  }

  const submitTemplate = templateForm.handleSubmit((values) => {
    setTemplate(values);
    setStep(1);
  });

  const submitCount = countForm.handleSubmit((values) => {
    setLessonCount(values.lesson_count);
    setStep(2);
  });

  function create() {
    if (!template) return;
    mutation.mutate(
      { ...toTemplateInput(template), lesson_count: lessonCount },
      {
        onSuccess: (created) => {
          void navigate(
            created.draft_version_id
              ? `/prep/${created.draft_version_id}/board`
              : `/library/templates/${created.id}`,
          );
        },
        onError: (error) => {
          // A duplicate code (409) lands on the code field, so the wizard
          // goes back to where that field is.
          setStep(0);
          handleApiError(error);
        },
      },
    );
  }

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-col gap-3">
        <Link
          to="/library"
          className="self-start font-display text-[13px] font-bold text-ink-500 hover:text-mint-600"
        >
          ← Kho học liệu
        </Link>
        <h1 className="font-display text-[26px] font-extrabold text-ink-900">
          Tạo chương trình mẫu
        </h1>
      </div>

      <ol className="flex flex-wrap gap-2" aria-label="Các bước">
        {STEPS.map((label, index) => (
          <li
            key={label}
            aria-current={index === step ? "step" : undefined}
            className={cn(
              "rounded-full border px-3 py-1 font-display text-[12px] font-extrabold",
              index === step
                ? "border-mint-400 bg-mint-100 text-mint-700"
                : index < step
                  ? "border-line-200 bg-white text-ink-700"
                  : "border-line-200 bg-cream-100 text-ink-400",
            )}
          >
            {index + 1}. {label}
          </li>
        ))}
      </ol>

      <div className="rounded-[var(--radius-lg)] border border-line-200 bg-white p-4">
        {step === 0 ? (
          <form
            onSubmit={(event) => void submitTemplate(event)}
            noValidate
            className="flex flex-col gap-4"
          >
            <h2 className="font-display text-[17px] font-extrabold text-ink-900">
              Bước 1 · Thông tin chương trình
            </h2>
            <TemplateFields form={templateForm} idPrefix="wizard" />
            <div className="flex justify-end">
              <HvButton type="submit">Tiếp tục</HvButton>
            </div>
          </form>
        ) : step === 1 ? (
          <form
            onSubmit={(event) => void submitCount(event)}
            noValidate
            className="flex flex-col gap-4"
          >
            <h2 className="font-display text-[17px] font-extrabold text-ink-900">
              Bước 2 · Số buổi học
            </h2>
            <p className="text-[14px] text-ink-500">
              Bản nháp sẽ được tạo sẵn các buổi &ldquo;Buổi 1&rdquo;, &ldquo;Buổi 2&rdquo;… để bạn
              đặt tên và soạn nội dung sau.
            </p>
            <FieldGroup>
              <Field data-invalid={Boolean(countForm.formState.errors.lesson_count)}>
                <FieldLabel htmlFor="wizard-lesson-count">Số buổi</FieldLabel>
                <Input
                  id="wizard-lesson-count"
                  type="number"
                  inputMode="numeric"
                  min={1}
                  max={MAX_LESSONS}
                  className="w-[120px]"
                  aria-invalid={Boolean(countForm.formState.errors.lesson_count)}
                  {...countForm.register("lesson_count")}
                />
                <FieldError errors={[countForm.formState.errors.lesson_count]} />
              </Field>
            </FieldGroup>
            <div className="flex justify-between">
              <HvButton type="button" variant="ghost" onClick={() => setStep(0)}>
                Quay lại
              </HvButton>
              <HvButton type="submit">Tiếp tục</HvButton>
            </div>
          </form>
        ) : (
          <div className="flex flex-col gap-4">
            <h2 className="font-display text-[17px] font-extrabold text-ink-900">
              Bước 3 · Xác nhận
            </h2>
            {template ? <TemplateSummary template={template} lessonCount={lessonCount} /> : null}
            <FieldError errors={[templateForm.formState.errors.root]} />
            <div className="flex justify-between">
              <HvButton type="button" variant="ghost" onClick={() => setStep(1)}>
                Quay lại
              </HvButton>
              <HvButton type="button" onClick={create} disabled={mutation.isPending}>
                {mutation.isPending ? "Đang tạo…" : "Tạo chương trình"}
              </HvButton>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}

function TemplateSummary({
  template,
  lessonCount,
}: {
  template: TemplateFormValues;
  lessonCount: number;
}) {
  const rows: [string, string][] = [
    ["Mã chương trình", template.code],
    ["Tên chương trình", template.name],
    ["Môn học", template.subject || "—"],
    ["Trình độ", template.level || "—"],
    ["Mô tả", template.description || "—"],
  ];
  return (
    <div className="flex flex-col gap-4">
      <dl className="grid gap-x-6 gap-y-2 text-[14px] sm:grid-cols-[180px_1fr]">
        {rows.map(([label, value]) => (
          <div key={label} className="contents">
            <dt className="text-ink-500">{label}</dt>
            <dd className="font-bold text-ink-900">{value}</dd>
          </div>
        ))}
      </dl>
      <div>
        <p className="font-display text-[12px] font-extrabold uppercase tracking-[0.4px] text-ink-500">
          {lessonCount} buổi sẽ được tạo
        </p>
        <ul className="mt-2 flex flex-wrap gap-1.5">
          {Array.from({ length: lessonCount }, (_, index) => (
            <li
              key={index}
              className="rounded-full border border-line-200 bg-cream-100 px-2.5 py-0.5 text-[12px] font-bold text-ink-700"
            >
              Buổi {index + 1}
            </li>
          ))}
        </ul>
      </div>
    </div>
  );
}
