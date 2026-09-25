import { ArrowLeftIcon } from "lucide-react";
import { Link, useParams, useSearchParams } from "react-router";
import { z } from "zod";

import {
  HvBadge,
  HvNotice,
  HvSegmented,
  HvStateBlock,
  type HvSegmentedOption,
} from "@/components/hv";
import { useCenterContext } from "@/features/teaching";
import { ApiError } from "@/lib/api/errors";

import { LessonContents } from "../components/lesson-contents";
import { LessonExercises } from "../components/lesson-exercises";
import { LessonInfoCard } from "../components/lesson-info-card";
import { useLesson, useTemplate, useVersions } from "../hooks/use-library";
import { versionLabel, versionStatusVariant } from "../lib/library-labels";
import type {
  ProgramTemplate,
  TemplateLessonDetail,
  TemplateVersion,
} from "../schemas/library-schemas";

const tabs = ["info", "exercises"] as const;
type Tab = (typeof tabs)[number];
const tabSchema = z.enum(tabs).catch("info");

const tabOptions: HvSegmentedOption<Tab>[] = [
  { value: "info", label: "Thông tin" },
  { value: "exercises", label: "Bài tập" },
];

const TAB_ID_BASE = "lesson-section";

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
  lesson: TemplateLessonDetail;
}

function LessonView({ template, version, lesson }: LessonViewProps) {
  const { has } = useCenterContext();
  const canEdit = has("library.edit");
  const editable = canEdit && version.status === "draft";
  const [searchParams, setSearchParams] = useSearchParams();
  const tab: Tab = tabSchema.parse(searchParams.get("tab") ?? undefined);

  function selectTab(next: Tab) {
    const params = new URLSearchParams(searchParams);
    if (next === "info") {
      params.delete("tab");
    } else {
      params.set("tab", next);
    }
    setSearchParams(params, { replace: true });
  }

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

      {!editable ? (
        <HvNotice tone="info">
          {version.status === "published"
            ? `Phiên bản v${version.version_no} đã phát hành (${version.class_count} lớp đang gắn) — chỉ xem. Muốn sửa, tạo bản nháp mới ở màn chương trình mẫu.`
            : version.status === "archived"
              ? `Phiên bản v${version.version_no} đã ngừng (${version.class_count} lớp đang gắn) — chỉ xem. Muốn sửa, tạo bản nháp mới ở màn chương trình mẫu.`
              : "Bạn không có quyền soạn buổi học mẫu."}
        </HvNotice>
      ) : null}

      <HvSegmented
        variant="tabs"
        idBase={TAB_ID_BASE}
        aria-label="Nội dung buổi học mẫu"
        options={tabOptions}
        value={tab}
        onValueChange={selectTab}
      />

      <div
        role="tabpanel"
        id={`${TAB_ID_BASE}-panel-${tab}`}
        aria-labelledby={`${TAB_ID_BASE}-tab-${tab}`}
        className="flex flex-col gap-4"
      >
        {tab === "info" ? (
          <>
            <LessonInfoCard lesson={lesson} templateId={template.id} editable={editable} />
            <LessonContents lesson={lesson} templateId={template.id} editable={editable} />
          </>
        ) : (
          <LessonExercises lesson={lesson} templateId={template.id} editable={editable} />
        )}
      </div>
    </div>
  );
}
