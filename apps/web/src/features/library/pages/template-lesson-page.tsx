import { ArrowLeftIcon } from "lucide-react";
import { Link, useParams } from "react-router";

import { HvBadge, HvButton, HvNotice, HvStateBlock, hvToast } from "@/components/hv";
import { useCenterContext } from "@/features/teaching";
import { ApiError } from "@/lib/api/errors";
import { useApiFormErrors } from "@/lib/forms/use-api-form-errors";

import { LessonFields } from "../components/lesson-form";
import { useLessonForm } from "../hooks/use-lesson-form";
import { useLesson, useTemplate, useUpdateLesson, useVersions } from "../hooks/use-library";
import { formatDuration, versionLabel, versionStatusVariant } from "../lib/library-labels";
import {
  toLessonForm,
  toLessonInput,
  type ProgramTemplate,
  type TemplateLesson,
  type TemplateVersion,
} from "../schemas/library-schemas";

/**
 * `/library/templates/:id/lessons/:lessonId` — one lesson of a template
 * version. Editable only while its version is a draft and the viewer holds
 * `library.edit`; otherwise the same fields render read-only.
 */
export function TemplateLessonPage() {
  const { id = "", lessonId = "" } = useParams<{ id: string; lessonId: string }>();
  const { isResolved } = useCenterContext();
  const template = useTemplate(id);
  const versions = useVersions(id);
  const lesson = useLesson(lessonId);

  // Wait for the permission set too, so the editor never mounts over a read-only flash.
  if (template.isPending || versions.isPending || lesson.isPending || !isResolved) {
    return <HvStateBlock state="loading" title="Đang tải buổi học" />;
  }
  const error = template.error ?? versions.error ?? lesson.error;
  const version = versions.data?.find((row) => row.id === lesson.data?.version_id);
  if (error || !template.data || !lesson.data || !version) {
    const notFound = !error || (error instanceof ApiError && error.status === 404);
    return (
      <HvStateBlock
        state="error"
        title={notFound ? "Không tìm thấy buổi học mẫu" : "Không tải được buổi học"}
        description={
          notFound ? "Buổi học có thể đã bị xoá hoặc đường dẫn không đúng." : "Thử tải lại trang."
        }
        action={
          <Link
            to={`/library/templates/${id}`}
            className="font-display text-[13px] font-bold text-mint-600 hover:underline"
          >
            Về chương trình mẫu
          </Link>
        }
      />
    );
  }

  return <LessonView template={template.data} version={version} lesson={lesson.data} />;
}

interface LessonViewProps {
  template: ProgramTemplate;
  version: TemplateVersion;
  lesson: TemplateLesson;
}

function LessonView({ template, version, lesson }: LessonViewProps) {
  const { has } = useCenterContext();
  const canEdit = has("library.edit");
  const editable = canEdit && version.status === "draft";

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-col gap-3">
        <Link
          to={`/library/templates/${template.id}`}
          className="inline-flex items-center gap-1 self-start font-display text-[13px] font-bold text-ink-500 hover:text-mint-600"
        >
          <ArrowLeftIcon aria-hidden="true" className="size-4" />
          {template.name}
        </Link>
        <div className="flex flex-wrap items-center gap-2">
          <h1 className="font-display text-[26px] font-extrabold text-ink-900">
            Buổi {lesson.position} · {lesson.title}
          </h1>
          <HvBadge variant={versionStatusVariant[version.status]} dot>
            {versionLabel(version)}
          </HvBadge>
        </div>
      </div>

      {editable ? (
        <LessonEditor key={lesson.updated_at} lesson={lesson} templateId={template.id} />
      ) : (
        <>
          <HvNotice tone="info">
            {version.status === "published"
              ? `Phiên bản v${version.version_no} đã phát hành, nội dung được khoá.`
              : version.status === "archived"
                ? `Phiên bản v${version.version_no} đã lưu trữ, nội dung được khoá.`
                : "Bạn không có quyền soạn buổi học mẫu."}
          </HvNotice>
          <LessonReadOnly lesson={lesson} />
        </>
      )}
    </div>
  );
}

function LessonEditor({ lesson, templateId }: { lesson: TemplateLesson; templateId: string }) {
  const form = useLessonForm(toLessonForm(lesson));
  const mutation = useUpdateLesson(lesson.id, lesson.version_id, templateId);
  const handleApiError = useApiFormErrors(form);

  const onSubmit = form.handleSubmit((values) => {
    mutation.mutate(toLessonInput(values), {
      onSuccess: (saved) => {
        form.reset(toLessonForm(saved));
        hvToast("Đã lưu buổi học");
      },
      onError: handleApiError,
    });
  });

  return (
    <form
      onSubmit={(event) => void onSubmit(event)}
      noValidate
      className="flex flex-col gap-4 rounded-[var(--radius-lg)] border border-line-200 bg-white p-4"
    >
      <LessonFields form={form} idPrefix="lesson" />
      <div className="flex justify-end">
        <HvButton type="submit" disabled={mutation.isPending}>
          {mutation.isPending ? "Đang lưu…" : "Lưu"}
        </HvButton>
      </div>
    </form>
  );
}

function ReadOnlyField({ label, value }: { label: string; value: string | null }) {
  return (
    <div role="group" aria-label={label} className="flex flex-col gap-1">
      <span className="text-[12px] font-extrabold uppercase tracking-[0.4px] text-ink-500">
        {label}
      </span>
      {value ? (
        <p className="whitespace-pre-wrap text-[14px] text-ink-900">{value}</p>
      ) : (
        <p className="text-[14px] text-ink-400">Chưa có</p>
      )}
    </div>
  );
}

function LessonReadOnly({ lesson }: { lesson: TemplateLesson }) {
  return (
    <div className="flex flex-col gap-4 rounded-[var(--radius-lg)] border border-line-200 bg-white p-4">
      <ReadOnlyField label="Mục tiêu" value={lesson.objectives} />
      <ReadOnlyField label="Thời lượng" value={formatDuration(lesson.duration_min)} />
      <ReadOnlyField label="Bài tập về nhà" value={lesson.homework_note} />
    </div>
  );
}
