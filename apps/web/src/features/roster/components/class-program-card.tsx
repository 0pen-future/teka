import { useState } from "react";
import { Link } from "react-router";

import { HvButton, HvConfirmDialog, HvNotice, HvSelect, hvToast } from "@/components/hv";
import { useTemplatesList, useVersions, versionLabel } from "@/features/library";
import { ApiError } from "@/lib/api/errors";

import {
  useApplyClassProgram,
  useClassProgram,
  useCourseDefaultTemplate,
  useRemoveClassProgram,
} from "../hooks/use-class-program";
import type { Class } from "../schemas/roster-schemas";
import { SectionCard } from "./section-card";

const APPLIED_TOAST = "Đã áp dụng chương trình mẫu";
const REMOVED_TOAST = "Đã gỡ chương trình mẫu";
const CURRICULUM_DIFFERS = "CURRICULUM_DIFFERS";

interface ClassProgramCardProps {
  klass: Class;
  isOwner: boolean;
  /** `library.read`: the template page is gated, so the link only shows to those who can open it. */
  canReadLibrary: boolean;
}

/** The counts the API sends back when the classbook already differs from the template. */
interface DiffersPrompt {
  templateVersionId: string;
  currentCount: string;
  templateCount: string;
}

function programErrorMessage(error: unknown, fallback: string): string {
  return error instanceof ApiError ? error.message : fallback;
}

/**
 * "Chương trình học": which template version the class follows. Applying is
 * an owner action because it renames the classbook's lessons; the API asks
 * for a second confirmation when the class already has a differing curriculum.
 */
export function ClassProgramCard({ klass, isOwner, canReadLibrary }: ClassProgramCardProps) {
  const program = useClassProgram(klass.id);
  const apply = useApplyClassProgram(klass.id);
  const remove = useRemoveClassProgram(klass.id);
  const courseDefault = useCourseDefaultTemplate(isOwner ? klass.course?.id : undefined);
  const [picking, setPicking] = useState(false);
  const [differs, setDiffers] = useState<DiffersPrompt | null>(null);
  const [confirmRemove, setConfirmRemove] = useState(false);

  function applyVersion(templateVersionId: string, confirm = false) {
    apply.mutate(
      { template_version_id: templateVersionId, confirm },
      {
        onSuccess: () => {
          setPicking(false);
          setDiffers(null);
          hvToast(APPLIED_TOAST, { variant: "success" });
        },
        onError: (error) => {
          if (error instanceof ApiError && error.code === CURRICULUM_DIFFERS) {
            setDiffers({
              templateVersionId,
              currentCount: error.fields?.current_count ?? "?",
              templateCount: error.fields?.template_count ?? "?",
            });
            return;
          }
          hvToast(programErrorMessage(error, "Không áp dụng được chương trình. Thử lại sau."), {
            variant: "danger",
          });
        },
      },
    );
  }

  function removeProgram() {
    remove.mutate(undefined, {
      onSuccess: () => {
        setConfirmRemove(false);
        hvToast(REMOVED_TOAST, { variant: "success" });
      },
      onError: (error) => {
        setConfirmRemove(false);
        hvToast(programErrorMessage(error, "Không gỡ được chương trình. Thử lại sau."), {
          variant: "danger",
        });
      },
    });
  }

  const defaultVersionId = courseDefault.data?.default_template_version_id ?? null;
  const showCourseDefault =
    isOwner && defaultVersionId !== null && program.data?.template_version_id !== defaultVersionId;

  return (
    <SectionCard title="Chương trình học">
      {program.isPending ? (
        <p className="text-[13px] text-ink-400">Đang tải…</p>
      ) : program.isError ? (
        <p className="text-[13px] text-coral-600">Không tải được chương trình của lớp.</p>
      ) : (
        <div className="flex flex-col gap-3">
          {program.data ? (
            <div className="flex flex-col gap-1">
              <p className="flex flex-wrap items-center gap-2 text-[14px] font-bold text-ink-900">
                <span>
                  {program.data.template_name} · v{program.data.version_no} ·{" "}
                  {program.data.lesson_count} buổi
                </span>
                {program.data.version_status === "archived" ? (
                  <span className="rounded-full bg-ink-100 px-2 py-0.5 text-[12px] font-bold text-ink-600">
                    Đã lưu trữ
                  </span>
                ) : null}
              </p>
              {canReadLibrary ? (
                <Link
                  to={`/library/templates/${program.data.template_id}`}
                  className="font-display text-[13px] font-bold text-mint-600 hover:underline"
                >
                  Mở chương trình mẫu
                </Link>
              ) : null}
              {program.data.version_status === "archived" ? (
                <HvNotice tone="warning">
                  Phiên bản này đã được lưu trữ trong thư viện; lớp vẫn xem được bài, nhưng nên đổi
                  sang phiên bản mới hơn.
                </HvNotice>
              ) : null}
            </div>
          ) : (
            <p className="text-[13px] text-ink-400">Lớp chưa áp dụng chương trình mẫu.</p>
          )}

          {isOwner && !picking ? (
            <div className="flex flex-wrap gap-2">
              <HvButton size="sm" variant="secondary" onClick={() => setPicking(true)}>
                {program.data ? "Đổi phiên bản" : "Thiết lập chương trình"}
              </HvButton>
              {showCourseDefault ? (
                <HvButton
                  size="sm"
                  variant="secondary"
                  disabled={apply.isPending}
                  onClick={() => applyVersion(defaultVersionId)}
                >
                  Áp dụng từ khóa mẫu
                </HvButton>
              ) : null}
              {program.data ? (
                <HvButton size="sm" variant="ghost" onClick={() => setConfirmRemove(true)}>
                  Gỡ chương trình
                </HvButton>
              ) : null}
            </div>
          ) : null}

          {isOwner && picking ? (
            <ProgramPicker
              pending={apply.isPending}
              onApply={(versionId) => applyVersion(versionId)}
              onCancel={() => setPicking(false)}
            />
          ) : null}
        </div>
      )}

      <HvConfirmDialog
        open={differs !== null}
        onOpenChange={(open) => {
          if (!open) setDiffers(null);
        }}
        title="Chương trình hiện tại khác chương trình mẫu"
        description={
          differs
            ? `Lớp đang có ${differs.currentCount} buổi trong sổ đầu bài, chương trình mẫu có ${differs.templateCount} buổi. Áp dụng sẽ thay tên các buổi theo chương trình mẫu; giáo án đã soạn giữ nguyên.`
            : undefined
        }
        confirmLabel="Vẫn áp dụng"
        tone="danger"
        pending={apply.isPending}
        onConfirm={() => {
          if (differs) applyVersion(differs.templateVersionId, true);
        }}
      />

      <HvConfirmDialog
        open={confirmRemove}
        onOpenChange={setConfirmRemove}
        title="Gỡ chương trình mẫu khỏi lớp?"
        description="Sổ đầu bài và giáo án giữ nguyên; lớp chỉ không còn gắn với chương trình mẫu này."
        confirmLabel="Gỡ"
        tone="danger"
        pending={remove.isPending}
        onConfirm={removeProgram}
      />
    </SectionCard>
  );
}

/** Template → published version, offering only what the class could actually follow. */
function ProgramPicker({
  pending,
  onApply,
  onCancel,
}: {
  pending: boolean;
  onApply: (versionId: string) => void;
  onCancel: () => void;
}) {
  const [templateId, setTemplateId] = useState("");
  const [versionId, setVersionId] = useState("");
  const templates = useTemplatesList({ per_page: 100 });
  const versions = useVersions(templateId || undefined);

  const templateOptions = (templates.data?.items ?? [])
    .filter((template) => template.published_version_no !== null)
    .map((template) => ({ value: template.id, label: template.name, meta: template.code }));
  const versionOptions = (versions.data ?? [])
    .filter((version) => version.status === "published")
    .map((version) => ({ value: version.id, label: versionLabel(version) }));

  function pickTemplate(next: string) {
    setTemplateId(next);
    setVersionId("");
  }

  // A template carries one published version at a time, so preselect it.
  const effectiveVersionId =
    versionId || (versionOptions.length === 1 ? versionOptions[0]!.value : "");

  return (
    <div className="flex flex-col gap-2 rounded-[var(--radius-md)] border border-line-200 p-3">
      <HvSelect
        aria-label="Chương trình mẫu"
        sheetTitle="Chọn chương trình mẫu"
        placeholder={templates.isPending ? "Đang tải…" : "Chọn chương trình mẫu"}
        options={templateOptions}
        value={templateId}
        onValueChange={pickTemplate}
        disabled={templates.isPending}
        className="w-full"
      />
      <HvSelect
        aria-label="Phiên bản"
        sheetTitle="Chọn phiên bản"
        placeholder="Chọn phiên bản"
        options={versionOptions}
        value={effectiveVersionId}
        onValueChange={setVersionId}
        disabled={!templateId || versions.isPending}
        className="w-full"
      />
      <div className="flex justify-end gap-2">
        <HvButton size="sm" variant="ghost" onClick={onCancel} disabled={pending}>
          Huỷ
        </HvButton>
        <HvButton
          size="sm"
          disabled={!effectiveVersionId || pending}
          onClick={() => onApply(effectiveVersionId)}
        >
          Áp dụng
        </HvButton>
      </div>
    </div>
  );
}
