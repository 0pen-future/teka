import { ArrowLeftIcon } from "lucide-react";
import { useId, useState } from "react";
import { Link, useNavigate, useParams, useSearchParams } from "react-router";
import { z } from "zod";

import {
  HvBadge,
  HvButton,
  HvConfirmDialog,
  HvModal,
  HvSegmented,
  HvStateBlock,
  hvToast,
  type HvSegmentedOption,
} from "@/components/hv";
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field";
import { useCenterContext } from "@/features/teaching";
import { ApiError } from "@/lib/api/errors";
import { textareaClassName } from "@/lib/forms/textarea-class";

import { ExerciseGroupsTab } from "../components/exercise-groups-tab";
import { LessonsTab } from "../components/lessons-tab";
import { LogFieldsEditor, LogFieldsReadOnly } from "../components/log-fields-editor";
import { ScoreSetsEditor, ScoreSetsReadOnly } from "../components/score-sets-editor";
import { TemplateDialog } from "../components/template-dialog";
import { VersionBanner } from "../components/version-banner";
import { VersionChips } from "../components/version-chips";
import { VersionExercisesTab } from "../components/version-exercises-tab";
import { VersionMaterialsTab } from "../components/version-materials-tab";
import { VersionsTab } from "../components/versions-tab";
import {
  useArchiveVersion,
  useCreateVersion,
  useDeleteTemplate,
  usePublishVersion,
  useTemplate,
  useVersionDetail,
  useVersions,
} from "../hooks/use-library";
import { defaultVersion, versionStatusVariant } from "../lib/library-labels";
import type { ProgramTemplate, TemplateVersion } from "../schemas/library-schemas";

function apiMessage(error: unknown, fallback: string): string {
  return error instanceof ApiError ? error.message : fallback;
}

const TAB_VALUES = [
  "lessons",
  "exercises",
  "materials",
  "groups",
  "scores",
  "logs",
  "versions",
] as const;
type Tab = (typeof TAB_VALUES)[number];
const tabSchema = z.enum(TAB_VALUES).catch("lessons");

const tabOptions: HvSegmentedOption<Tab>[] = [
  { value: "lessons", label: "Buổi học" },
  { value: "exercises", label: "Bài tập" },
  { value: "materials", label: "Tài liệu" },
  { value: "groups", label: "Nhóm bài tập" },
  { value: "scores", label: "Bộ điểm" },
  { value: "logs", label: "Nhật ký" },
  { value: "versions", label: "Phiên bản" },
];

const TAB_ID_BASE = "template-tab";

/**
 * `/library/templates/:id` — one template, its version picker and 7 tabs
 * scoped to the picked version. Authoring (lessons, exercise groups, score
 * sets, log fields) needs `library.edit` and a draft; publishing needs
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
  const tab = tabSchema.parse(searchParams.get("tab") ?? undefined);

  const createVersion = useCreateVersion(template.id);
  const deleteTemplate = useDeleteTemplate();
  const publish = usePublishVersion(template.id);
  const archive = useArchiveVersion(template.id);

  const [editing, setEditing] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [drafting, setDrafting] = useState(false);
  const [publishing, setPublishing] = useState(false);
  const [archiving, setArchiving] = useState(false);
  const [changelog, setChangelog] = useState("");
  const changelogId = useId();

  function selectVersion(versionNo: number) {
    const params = new URLSearchParams(searchParams);
    params.set("v", String(versionNo));
    setSearchParams(params, { replace: true });
  }

  function selectTab(next: Tab) {
    const params = new URLSearchParams(searchParams);
    params.set("tab", next);
    setSearchParams(params, { replace: true });
  }

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
            <div className="flex flex-wrap items-center gap-2">
              <h1 className="font-display text-[26px] font-extrabold text-ink-900">
                {template.name}
              </h1>
              {selected ? (
                <HvBadge variant={versionStatusVariant[selected.status]}>
                  v{selected.version_no}
                </HvBadge>
              ) : null}
            </div>
            <div className="mt-1 flex flex-wrap items-center gap-2 text-[14px] text-ink-500">
              <span className="font-mono text-[13px]">Mã: {template.code}</span>
              {[template.subject, template.level].filter(Boolean).join(" · ") ? (
                <span>{[template.subject, template.level].filter(Boolean).join(" · ")}</span>
              ) : null}
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
              <HvButton
                size="sm"
                variant="ghost"
                disabled={template.class_count > 0}
                title={
                  template.class_count > 0
                    ? "Đang có lớp gắn chương trình này, không thể xoá."
                    : undefined
                }
                onClick={() => setDeleting(true)}
              >
                Xoá chương trình
              </HvButton>
            </div>
          ) : null}
        </div>
      </div>

      <VersionChips
        versions={versions}
        selectedId={selected?.id}
        canEdit={canEdit}
        hasDraft={hasDraft}
        onSelect={selectVersion}
        onNewDraft={() => setDrafting(true)}
      />

      {selected ? (
        <VersionBanner
          version={selected}
          canEdit={canEdit}
          canPublish={canPublish}
          hasDraft={hasDraft}
          onPublish={() => setPublishing(true)}
          onCreateDraft={() => setDrafting(true)}
          onArchive={() => setArchiving(true)}
        />
      ) : (
        <HvStateBlock state="empty" title="Chương trình này chưa có phiên bản nào." />
      )}

      {selected ? (
        <>
          <HvSegmented
            variant="tabs"
            idBase={TAB_ID_BASE}
            aria-label="Nội dung phiên bản"
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
            {tab === "lessons" ? (
              <LessonsTab version={selected} templateId={template.id} authoring={authoring} />
            ) : null}
            {tab === "exercises" ? <VersionExercisesTab version={selected} /> : null}
            {tab === "materials" ? <VersionMaterialsTab version={selected} /> : null}
            {tab === "groups" ? (
              <ExerciseGroupsTab
                version={selected}
                templateId={template.id}
                authoring={authoring}
              />
            ) : null}
            {tab === "scores" ? (
              <ScoresPanel version={selected} templateId={template.id} authoring={authoring} />
            ) : null}
            {tab === "logs" ? (
              <LogsPanel version={selected} templateId={template.id} authoring={authoring} />
            ) : null}
            {tab === "versions" ? (
              <VersionsTab
                versions={versions}
                templateId={template.id}
                canEdit={canEdit}
                canPublish={canPublish}
                onSelect={selectVersion}
              />
            ) : null}
          </div>
        </>
      ) : null}

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
            title={`Kích hoạt v${selected.version_no}?`}
            description="Sau khi kích hoạt, buổi học của phiên bản này bị khoá; muốn sửa phải tạo bản nháp mới."
            confirmLabel="Kích hoạt"
            pending={publish.isPending}
            onConfirm={() =>
              publish.mutate(selected.id, {
                onSuccess: (version) => {
                  setPublishing(false);
                  hvToast(`Đã kích hoạt v${version.version_no}`);
                },
                onError: (error) => {
                  setPublishing(false);
                  hvToast(apiMessage(error, "Không kích hoạt được phiên bản."), {
                    variant: "danger",
                  });
                },
              })
            }
          />
          <HvConfirmDialog
            open={archiving}
            onOpenChange={setArchiving}
            title={`Ngừng v${selected.version_no}?`}
            description={
              selected.class_count > 0
                ? `${selected.class_count} lớp đang gắn phiên bản này sẽ không còn dùng được bản mới.`
                : "Phiên bản sẽ chuyển sang chỉ xem."
            }
            confirmLabel="Ngừng"
            tone="danger"
            pending={archive.isPending}
            onConfirm={() =>
              archive.mutate(selected.id, {
                onSuccess: (version) => {
                  setArchiving(false);
                  hvToast(`Đã ngừng v${version.version_no}`);
                },
                onError: (error) => {
                  setArchiving(false);
                  hvToast(apiMessage(error, "Không ngừng được phiên bản."), { variant: "danger" });
                },
              })
            }
          />
        </>
      ) : null}
    </div>
  );
}

interface VersionScopedPanelProps {
  version: TemplateVersion;
  templateId: string;
  authoring: boolean;
}

/**
 * The version's score sets. The editor is keyed on the server's copy so a
 * save, or switching to a different version, starts it fresh.
 */
function ScoresPanel({ version, templateId, authoring }: VersionScopedPanelProps) {
  const detail = useVersionDetail(version.id);

  if (detail.isPending) return <HvStateBlock state="loading" title="Đang tải cơ cấu điểm" />;
  if (detail.isError) {
    return (
      <HvStateBlock
        state="error"
        title="Không tải được cơ cấu điểm"
        action={
          <HvButton size="sm" variant="ghost" onClick={() => void detail.refetch()}>
            Thử lại
          </HvButton>
        }
      />
    );
  }

  const groups = detail.data.score_set;
  const groupsKey = groups
    .map(
      (group) =>
        `${group.key}:${group.components.map((c) => `${c.key}:${c.label}:${c.max}:${c.weight}`).join(",")}`,
    )
    .join("|");

  return authoring ? (
    <ScoreSetsEditor
      key={`${version.id}:${groupsKey}`}
      versionId={version.id}
      templateId={templateId}
      groups={groups}
    />
  ) : (
    <ScoreSetsReadOnly groups={groups} />
  );
}

/**
 * The version's log fields. The editor is keyed on the server's copy so a
 * save, or switching to a different version, starts it fresh.
 */
function LogsPanel({ version, templateId, authoring }: VersionScopedPanelProps) {
  const detail = useVersionDetail(version.id);

  if (detail.isPending) return <HvStateBlock state="loading" title="Đang tải trường nhật ký" />;
  if (detail.isError) {
    return (
      <HvStateBlock
        state="error"
        title="Không tải được trường nhật ký"
        action={
          <HvButton size="sm" variant="ghost" onClick={() => void detail.refetch()}>
            Thử lại
          </HvButton>
        }
      />
    );
  }

  const fields = detail.data.log_fields;
  const fieldsKey = fields.map((field) => field.id).join(",");

  return authoring ? (
    <LogFieldsEditor
      key={`${version.id}:${fieldsKey}`}
      versionId={version.id}
      templateId={templateId}
      fields={fields}
    />
  ) : (
    <LogFieldsReadOnly fields={fields} />
  );
}
