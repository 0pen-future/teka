import { ArrowLeftIcon } from "lucide-react";
import { useId, useState } from "react";
import { Link, useNavigate, useParams, useSearchParams } from "react-router";

import {
  HvButton,
  HvConfirmDialog,
  HvModal,
  HvNotice,
  HvSegmented,
  HvSelect,
  HvStateBlock,
  hvToast,
  type HvSegmentedOption,
} from "@/components/hv";
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field";
import { useCenterContext } from "@/features/teaching";
import { ApiError } from "@/lib/api/errors";

import { LessonDialog } from "../components/lesson-form";
import { LessonsTable } from "../components/lessons-table";
import { LogFieldsEditor, LogFieldsReadOnly } from "../components/log-fields-editor";
import { ScoreSetEditor, ScoreSetReadOnly } from "../components/score-set-editor";
import { TemplateDialog } from "../components/template-dialog";
import {
  useArchiveVersion,
  useCreateVersion,
  useDeleteLesson,
  useDeleteTemplate,
  useLessons,
  usePublishVersion,
  useReorderLessons,
  useTemplate,
  useVersionDetail,
  useVersions,
} from "../hooks/use-library";
import { defaultVersion, versionLabel } from "../lib/library-labels";
import { textareaClassName } from "@/lib/forms/textarea-class";
import type { ProgramTemplate, TemplateLesson, TemplateVersion } from "../schemas/library-schemas";

function apiMessage(error: unknown, fallback: string): string {
  return error instanceof ApiError ? error.message : fallback;
}

type Section = "lessons" | "grading";

const sectionOptions: HvSegmentedOption<Section>[] = [
  { value: "lessons", label: "Buổi học" },
  { value: "grading", label: "Nhật ký & Điểm" },
];

const SECTION_ID_BASE = "template-section";

/**
 * `/library/templates/:id` — one template, its version picker and the
 * lessons of the picked version. Authoring (add/reorder/delete lessons,
 * archive) needs `library.edit` and a draft; publishing needs
 * `library.publish`, which the API keeps separate because it freezes the
 * version for every class that later binds to it.
 */
export function TemplateDetailPage() {
  const { id = "" } = useParams<{ id: string }>();
  const { isResolved } = useCenterContext();
  const template = useTemplate(id);
  const versions = useVersions(id);

  // Wait for the permission set too, so authoring controls never pop in late.
  if (template.isPending || versions.isPending || !isResolved) {
    return <HvStateBlock state="loading" title="Đang tải chương trình mẫu" />;
  }
  if (template.isError || versions.isError) {
    const error = template.error ?? versions.error;
    const notFound = error instanceof ApiError && error.status === 404;
    return (
      <HvStateBlock
        state="error"
        title={notFound ? "Không tìm thấy chương trình mẫu" : "Không tải được chương trình mẫu"}
        description={
          notFound
            ? "Chương trình có thể đã bị xoá hoặc đường dẫn không đúng."
            : "Thử tải lại trang."
        }
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

  return <TemplateWorkspace template={template.data} versions={versions.data} />;
}

interface TemplateWorkspaceProps {
  template: ProgramTemplate;
  versions: TemplateVersion[];
}

function TemplateWorkspace({ template, versions }: TemplateWorkspaceProps) {
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();
  const { has } = useCenterContext();
  const canEdit = has("library.edit");
  const canPublish = has("library.publish");

  const requested = Number(searchParams.get("v"));
  const selected =
    versions.find((version) => version.version_no === requested) ?? defaultVersion(versions);
  const hasDraft = versions.some((version) => version.status === "draft");
  const isDraft = selected?.status === "draft";
  const authoring = canEdit && isDraft;

  const lessons = useLessons(selected?.id);
  const publish = usePublishVersion(template.id);
  const archive = useArchiveVersion(template.id);
  const createVersion = useCreateVersion(template.id);
  const deleteTemplate = useDeleteTemplate();
  const reorder = useReorderLessons(selected?.id ?? "", template.id);
  const removeLesson = useDeleteLesson(selected?.id ?? "", template.id);

  const [editing, setEditing] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [publishing, setPublishing] = useState(false);
  const [archiving, setArchiving] = useState(false);
  const [drafting, setDrafting] = useState(false);
  const [addingLesson, setAddingLesson] = useState(false);
  const [lessonToDelete, setLessonToDelete] = useState<TemplateLesson | null>(null);
  const [changelog, setChangelog] = useState("");
  const [section, setSection] = useState<Section>("lessons");
  const changelogId = useId();

  function selectVersion(versionNo: number) {
    const params = new URLSearchParams(searchParams);
    params.set("v", String(versionNo));
    setSearchParams(params, { replace: true });
  }

  function moveLesson(lesson: TemplateLesson, direction: -1 | 1) {
    const ordered = lessons.data ?? [];
    const index = ordered.findIndex((row) => row.id === lesson.id);
    const target = index + direction;
    if (index < 0 || target < 0 || target >= ordered.length) {
      return;
    }
    const ids = ordered.map((row) => row.id);
    [ids[index], ids[target]] = [ids[target]!, ids[index]!];
    reorder.mutate(ids, {
      onError: (error) =>
        hvToast(apiMessage(error, "Không đổi được thứ tự buổi học."), { variant: "danger" }),
    });
  }

  const versionOptions = [...versions]
    .sort((a, b) => b.version_no - a.version_no)
    .map((version) => ({ value: version.id, label: versionLabel(version) }));
  const subjectLine = [template.subject, template.level].filter(Boolean).join(" · ");
  const lessonRows = lessons.data ?? [];

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-col gap-3">
        <Link
          to="/library"
          className="inline-flex items-center gap-1 self-start font-display text-[13px] font-bold text-ink-500 hover:text-mint-600"
        >
          <ArrowLeftIcon aria-hidden="true" className="size-4" />
          Kho học liệu
        </Link>
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div className="min-w-0">
            <h1 className="font-display text-[26px] font-extrabold text-ink-900">
              {template.name}
            </h1>
            <div className="mt-1 flex flex-wrap items-center gap-2 text-[14px] text-ink-500">
              <span className="font-mono text-[13px]">Mã: {template.code}</span>
              {subjectLine ? <span>{subjectLine}</span> : null}
            </div>
            {template.description ? (
              <p className="mt-2 max-w-[640px] text-[14px] text-ink-700">{template.description}</p>
            ) : null}
          </div>
          {canEdit ? (
            <div className="flex flex-wrap gap-2">
              <HvButton size="sm" variant="secondary" onClick={() => setEditing(true)}>
                Sửa
              </HvButton>
              <HvButton size="sm" variant="ghost" onClick={() => setDeleting(true)}>
                Xoá chương trình
              </HvButton>
            </div>
          ) : null}
        </div>
      </div>

      <div className="flex flex-wrap items-center gap-3 rounded-[var(--radius-lg)] border border-line-200 bg-white p-3">
        {selected ? (
          <HvSelect
            options={versionOptions}
            value={selected.id}
            onValueChange={(versionId) => {
              const next = versions.find((version) => version.id === versionId);
              if (next) selectVersion(next.version_no);
            }}
            sheetTitle="Chọn phiên bản"
            aria-label="Phiên bản"
            searchThreshold={Infinity}
            className="min-w-[200px]"
          />
        ) : (
          <span className="text-[14px] text-ink-500">Chưa có phiên bản nào.</span>
        )}
        {selected?.changelog ? (
          <span className="text-[14px] text-ink-500">{selected.changelog}</span>
        ) : null}
        <div className="ml-auto flex flex-wrap gap-2">
          {canPublish && isDraft ? (
            <HvButton size="sm" onClick={() => setPublishing(true)}>
              Phát hành
            </HvButton>
          ) : null}
          {canEdit && selected?.status === "published" ? (
            <HvButton size="sm" variant="ghost" onClick={() => setArchiving(true)}>
              Lưu trữ
            </HvButton>
          ) : null}
          {canEdit && !hasDraft ? (
            <HvButton size="sm" variant="secondary" onClick={() => setDrafting(true)}>
              Tạo bản nháp mới
            </HvButton>
          ) : null}
        </div>
      </div>

      {selected && selected.status !== "draft" ? (
        <HvNotice tone="info">
          {selected.status === "published"
            ? "Phiên bản này đã phát hành, nội dung được khoá. Tạo bản nháp mới để chỉnh sửa."
            : "Phiên bản này đã lưu trữ, nội dung được khoá."}
        </HvNotice>
      ) : null}

      <HvSegmented
        variant="tabs"
        idBase={SECTION_ID_BASE}
        aria-label="Nội dung phiên bản"
        options={sectionOptions}
        value={section}
        onValueChange={setSection}
      />

      <div
        role="tabpanel"
        id={`${SECTION_ID_BASE}-panel-${section}`}
        aria-labelledby={`${SECTION_ID_BASE}-tab-${section}`}
        className="flex flex-col gap-4"
      >
        {section === "lessons" ? (
          <>
            <div className="flex flex-wrap items-center justify-between gap-2">
              <h2 className="font-display text-[18px] font-extrabold text-ink-900">Buổi học mẫu</h2>
              {authoring && selected ? (
                <HvButton size="sm" onClick={() => setAddingLesson(true)}>
                  Thêm buổi học
                </HvButton>
              ) : null}
            </div>

            {!selected ? (
              <HvStateBlock state="empty" title="Chương trình này chưa có phiên bản nào." />
            ) : lessons.isPending ? (
              <HvStateBlock state="loading" title="Đang tải buổi học" />
            ) : lessons.isError ? (
              <HvStateBlock
                state="error"
                title="Không tải được buổi học"
                action={
                  <HvButton size="sm" variant="ghost" onClick={() => void lessons.refetch()}>
                    Thử lại
                  </HvButton>
                }
              />
            ) : lessonRows.length === 0 ? (
              <HvStateBlock
                state="empty"
                title="Phiên bản này chưa có buổi học nào."
                description={authoring ? "Thêm buổi đầu tiên bằng nút Thêm buổi học." : undefined}
              />
            ) : (
              <LessonsTable
                lessons={lessonRows}
                templateId={template.id}
                editing={
                  authoring
                    ? {
                        pending: reorder.isPending || removeLesson.isPending,
                        onMove: moveLesson,
                        onDelete: setLessonToDelete,
                      }
                    : undefined
                }
              />
            )}
          </>
        ) : (
          <GradingPanel version={selected} templateId={template.id} authoring={authoring} />
        )}
      </div>

      {canEdit ? (
        <>
          <TemplateDialog
            mode="edit"
            template={template}
            open={editing}
            onOpenChange={setEditing}
            onSaved={() => hvToast("Đã lưu chương trình mẫu")}
          />
          <HvConfirmDialog
            open={deleting}
            onOpenChange={setDeleting}
            title={`Xoá chương trình "${template.name}"?`}
            description="Chương trình sẽ bị gỡ khỏi kho. Lớp đã dùng phiên bản của nó vẫn đọc được."
            confirmLabel="Xoá chương trình"
            tone="danger"
            pending={deleteTemplate.isPending}
            onConfirm={() =>
              deleteTemplate.mutate(template.id, {
                onSuccess: () => {
                  hvToast(`Đã xoá chương trình ${template.code}`);
                  void navigate("/library");
                },
                onError: (error) =>
                  hvToast(apiMessage(error, "Không xoá được chương trình."), {
                    variant: "danger",
                  }),
              })
            }
          />
          <HvModal
            open={drafting}
            onOpenChange={setDrafting}
            title="Tạo bản nháp mới"
            description="Bản nháp sao chép các buổi của phiên bản gần nhất đã phát hành, kể cả khi phiên bản đó đã lưu trữ."
            footer={
              <>
                <HvButton type="button" variant="ghost" onClick={() => setDrafting(false)}>
                  Hủy
                </HvButton>
                <HvButton
                  type="button"
                  disabled={createVersion.isPending}
                  onClick={() =>
                    createVersion.mutate(changelog.trim() || null, {
                      onSuccess: (version) => {
                        setDrafting(false);
                        setChangelog("");
                        hvToast(`Đã tạo bản nháp v${version.version_no}`);
                        selectVersion(version.version_no);
                      },
                      onError: (error) =>
                        hvToast(apiMessage(error, "Không tạo được bản nháp."), {
                          variant: "danger",
                        }),
                    })
                  }
                >
                  {createVersion.isPending ? "Đang tạo…" : "Tạo bản nháp"}
                </HvButton>
              </>
            }
          >
            <FieldGroup>
              <Field>
                <FieldLabel htmlFor={changelogId}>Ghi chú thay đổi</FieldLabel>
                <textarea
                  id={changelogId}
                  rows={3}
                  className={textareaClassName}
                  value={changelog}
                  onChange={(event) => setChangelog(event.target.value)}
                />
              </Field>
            </FieldGroup>
          </HvModal>
        </>
      ) : null}

      {selected ? (
        <>
          <HvConfirmDialog
            open={publishing}
            onOpenChange={setPublishing}
            title={`Phát hành v${selected.version_no}?`}
            description="Sau khi phát hành, buổi học của phiên bản này bị khoá; muốn sửa phải tạo bản nháp mới."
            confirmLabel="Phát hành"
            pending={publish.isPending}
            onConfirm={() =>
              publish.mutate(selected.id, {
                onSuccess: (version) => {
                  setPublishing(false);
                  hvToast(`Đã phát hành v${version.version_no}`);
                },
                onError: (error) => {
                  setPublishing(false);
                  hvToast(apiMessage(error, "Không phát hành được."), { variant: "danger" });
                },
              })
            }
          />
          <HvConfirmDialog
            open={archiving}
            onOpenChange={setArchiving}
            title={`Lưu trữ v${selected.version_no}?`}
            description="Phiên bản lưu trữ không còn là phiên bản chính thức của chương trình."
            confirmLabel="Lưu trữ"
            pending={archive.isPending}
            onConfirm={() =>
              archive.mutate(selected.id, {
                onSuccess: (version) => {
                  setArchiving(false);
                  hvToast(`Đã lưu trữ v${version.version_no}`);
                },
                onError: (error) => {
                  setArchiving(false);
                  hvToast(apiMessage(error, "Không lưu trữ được."), { variant: "danger" });
                },
              })
            }
          />
        </>
      ) : null}

      {authoring && selected ? (
        <>
          <LessonDialog
            open={addingLesson}
            onOpenChange={setAddingLesson}
            versionId={selected.id}
            templateId={template.id}
            onCreated={(lesson) => hvToast(`Đã thêm buổi ${lesson.position}`)}
          />
          <HvConfirmDialog
            open={lessonToDelete != null}
            onOpenChange={(open) => {
              if (!open) setLessonToDelete(null);
            }}
            title={`Xoá buổi "${lessonToDelete?.title ?? ""}"?`}
            description="Các buổi phía sau sẽ được đánh số lại."
            confirmLabel="Xoá buổi"
            tone="danger"
            pending={removeLesson.isPending}
            onConfirm={() => {
              if (!lessonToDelete) return;
              removeLesson.mutate(lessonToDelete.id, {
                onSuccess: () => {
                  setLessonToDelete(null);
                  hvToast("Đã xoá buổi học");
                },
                onError: (error) => {
                  setLessonToDelete(null);
                  hvToast(apiMessage(error, "Không xoá được buổi học."), { variant: "danger" });
                },
              });
            }}
          />
        </>
      ) : null}
    </div>
  );
}

interface GradingPanelProps {
  version: TemplateVersion | undefined;
  templateId: string;
  authoring: boolean;
}

/**
 * The version's log fields and score set. Editors are keyed on the
 * server's copy so a save (or a version switch) starts them fresh.
 */
function GradingPanel({ version, templateId, authoring }: GradingPanelProps) {
  const detail = useVersionDetail(version?.id);

  if (!version) {
    return <HvStateBlock state="empty" title="Chương trình này chưa có phiên bản nào." />;
  }
  if (detail.isPending) {
    return <HvStateBlock state="loading" title="Đang tải nhật ký và cơ cấu điểm" />;
  }
  if (detail.isError) {
    return (
      <HvStateBlock
        state="error"
        title="Không tải được nhật ký và cơ cấu điểm"
        action={
          <HvButton size="sm" variant="ghost" onClick={() => void detail.refetch()}>
            Thử lại
          </HvButton>
        }
      />
    );
  }

  const { log_fields: logFields, score_set: scoreSet } = detail.data;
  const logFieldsKey = logFields.map((f) => f.id).join(",");
  const scoreSetKey = scoreSet.map((c) => `${c.key}:${c.label}:${c.max}:${c.weight}`).join(",");
  return (
    <>
      {authoring ? (
        <LogFieldsEditor
          key={`${version.id}:${logFieldsKey}`}
          versionId={version.id}
          templateId={templateId}
          fields={logFields}
        />
      ) : (
        <LogFieldsReadOnly fields={logFields} />
      )}
      {authoring ? (
        <ScoreSetEditor
          key={`${version.id}:${scoreSetKey}`}
          versionId={version.id}
          templateId={templateId}
          components={scoreSet}
        />
      ) : (
        <ScoreSetReadOnly components={scoreSet} />
      )}
    </>
  );
}
