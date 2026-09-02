import { zodResolver } from "@hookform/resolvers/zod";
import { useEffect, useState } from "react";
import { useForm } from "react-hook-form";

import { HvButton, HvIcon, HvModal, HvNotice, HvSegmented, hvToast } from "@/components/hv";
import { Field, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { useApiFormErrors } from "@/lib/forms/use-api-form-errors";

import { useCreateScoreSet, useUpdateScoreSet } from "../hooks/use-score-sets";
import {
  MAX_SCORE_SET_COMPONENTS,
  findDuplicateIndexes,
  parsePastedComponents,
} from "../lib/score-set-components";
import { scoreSetInputSchema, type ScoreSet, type ScoreSetInput } from "../schemas/grading";
import { ScoreSetPreviewStrip } from "./score-set-preview-strip";

export interface ScoreSetEditorModalProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Present when editing an existing bộ điểm; absent for create. */
  scoreSet?: ScoreSet;
}

type EntryMode = "rows" | "paste";

const ENTRY_MODES = [
  { value: "rows" as const, label: "Từng cột" },
  { value: "paste" as const, label: "Dán danh sách" },
];

const PASTE_PLACEHOLDER = "Miệng\n15 phút\nGiữa kỳ";

function toDefaults(scoreSet?: ScoreSet): ScoreSetInput {
  return {
    name: scoreSet?.name ?? "",
    components: scoreSet && scoreSet.components.length > 0 ? [...scoreSet.components] : [""],
  };
}

/**
 * Create/edit bộ điểm: a name plus an ordered list of column names. Row
 * order maps directly to the server's `position` index, so add/remove/move
 * here are the whole editing surface — no separate reorder step. A second
 * entry mode accepts a pasted list (one name per line, or comma/semicolon
 * separated) and is parsed into the same rows on switch or submit.
 * Duplicate (case-insensitive) and blank names are caught client-side by
 * `scoreSetInputSchema` before the request round-trips.
 */
export function ScoreSetEditorModal({ open, onOpenChange, scoreSet }: ScoreSetEditorModalProps) {
  const isEdit = Boolean(scoreSet);
  const form = useForm<ScoreSetInput>({
    resolver: zodResolver(scoreSetInputSchema),
    defaultValues: toDefaults(scoreSet),
  });
  const createMutation = useCreateScoreSet();
  const updateMutation = useUpdateScoreSet();
  const mutation = isEdit ? updateMutation : createMutation;
  const handleApiError = useApiFormErrors(form, { conflictField: "name" });
  const [mode, setMode] = useState<EntryMode>("rows");
  const [pasteText, setPasteText] = useState("");
  const [truncated, setTruncated] = useState(false);

  // Re-seed on every open: the mounted form would otherwise keep the values
  // from whichever bộ điểm was last opened (or a discarded draft).
  useEffect(() => {
    if (open) {
      form.reset(toDefaults(scoreSet));
      setMode("rows");
      setPasteText("");
      setTruncated(false);
    }
    // form is stable from react-hook-form and not a meaningful dependency here.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, scoreSet]);

  const { errors } = form.formState;
  const components = form.watch("components");
  const atLimit = components.length >= MAX_SCORE_SET_COMPONENTS;

  function setComponents(next: string[]) {
    form.setValue("components", next, { shouldValidate: true, shouldDirty: true });
  }

  function updateComponent(index: number, value: string) {
    setComponents(components.map((component, i) => (i === index ? value : component)));
  }

  function removeComponent(index: number) {
    setComponents(components.filter((_, i) => i !== index));
  }

  function addComponent() {
    if (atLimit) return;
    setComponents([...components, ""]);
  }

  function moveComponent(index: number, direction: -1 | 1) {
    const target = index + direction;
    if (target < 0 || target >= components.length) {
      return;
    }
    const next = [...components];
    const [moved] = next.splice(index, 1);
    next.splice(target, 0, moved!);
    setComponents(next);
  }

  /** Parse the paste box into rows; an empty box keeps one blank row so the
   *  "không được để trống" rule still has something to point at. */
  function applyPaste(): string[] {
    const parsed = parsePastedComponents(pasteText);
    const next = parsed.names.length > 0 ? parsed.names : [""];
    setTruncated(parsed.truncated);
    setComponents(next);
    return next;
  }

  function changeMode(next: EntryMode) {
    if (next === mode) return;
    if (next === "paste") {
      setPasteText(components.filter((name) => name.trim().length > 0).join("\n"));
      setTruncated(false);
    } else {
      applyPaste();
    }
    setMode(next);
  }

  const pastePreview = parsePastedComponents(pasteText);
  const previewNames = mode === "paste" ? pastePreview.names : components;
  const pasteDuplicates = mode === "paste" ? findDuplicateIndexes(pastePreview.names) : null;
  const pasteTruncated = truncated || (mode === "paste" && pastePreview.truncated);

  function submit(values: ScoreSetInput) {
    const input: ScoreSetInput = {
      name: values.name,
      components: values.components.map((component) => component.trim()),
    };
    if (isEdit && scoreSet) {
      updateMutation.mutate(
        { id: scoreSet.id, input },
        {
          onSuccess: () => {
            hvToast(`Đã lưu bộ điểm ${input.name}`, { variant: "success" });
            onOpenChange(false);
          },
          onError: handleApiError,
        },
      );
      return;
    }
    createMutation.mutate(input, {
      onSuccess: () => {
        hvToast(`Đã tạo bộ điểm ${input.name}`, { variant: "success" });
        onOpenChange(false);
      },
      onError: handleApiError,
    });
  }

  function handleSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (mode === "paste") {
      // Fold the paste box into the form first so validation (and any
      // per-row error) runs against what the user actually typed.
      applyPaste();
      setMode("rows");
    }
    void form.handleSubmit(submit)(event);
  }

  return (
    <HvModal
      open={open}
      onOpenChange={onOpenChange}
      size="lg"
      title={isEdit ? "Sửa bộ điểm" : "Tạo bộ điểm mới"}
      footer={
        <>
          <HvButton type="button" variant="ghost" onClick={() => onOpenChange(false)}>
            Hủy
          </HvButton>
          <HvButton type="submit" form="score-set-editor-form" disabled={mutation.isPending}>
            {mutation.isPending ? "Đang lưu…" : "Lưu"}
          </HvButton>
        </>
      }
    >
      <form id="score-set-editor-form" onSubmit={handleSubmit} noValidate>
        <FieldGroup>
          <Field data-invalid={Boolean(errors.name)}>
            <FieldLabel htmlFor="score-set-name">Tên bộ điểm</FieldLabel>
            <Input
              id="score-set-name"
              placeholder="VD: Giữa kỳ"
              aria-invalid={Boolean(errors.name)}
              {...form.register("name")}
            />
            <FieldError errors={[errors.name]} />
          </Field>

          <Field data-invalid={Boolean(errors.components)}>
            <div className="flex flex-wrap items-center justify-between gap-2">
              <FieldLabel>Cột điểm</FieldLabel>
              <HvSegmented
                aria-label="Cách nhập cột điểm"
                options={ENTRY_MODES}
                value={mode}
                onValueChange={changeMode}
              />
            </div>

            {mode === "rows" ? (
              <>
                <div className="flex flex-col gap-2">
                  {components.map((component, index) => {
                    const rowError = errors.components?.[index];
                    return (
                      <div key={index} className="flex flex-col gap-1">
                        <div className="grid grid-cols-[1fr_auto_auto_auto] items-start gap-2">
                          <Input
                            aria-label={`Tên cột điểm ${index + 1}`}
                            className="min-h-12"
                            value={component}
                            aria-invalid={Boolean(rowError)}
                            onChange={(event) => updateComponent(index, event.target.value)}
                          />
                          <HvButton
                            type="button"
                            variant="ghost"
                            size="sm"
                            className="size-11 px-0"
                            aria-label={`Di chuyển cột điểm ${index + 1} lên`}
                            icon={<HvIcon name="arrow-up" />}
                            onClick={() => moveComponent(index, -1)}
                            disabled={index === 0}
                          />
                          <HvButton
                            type="button"
                            variant="ghost"
                            size="sm"
                            className="size-11 px-0"
                            aria-label={`Di chuyển cột điểm ${index + 1} xuống`}
                            icon={<HvIcon name="arrow-down" />}
                            onClick={() => moveComponent(index, 1)}
                            disabled={index === components.length - 1}
                          />
                          {components.length > 1 ? (
                            <HvButton
                              type="button"
                              variant="ghost"
                              size="sm"
                              className="size-11 px-0 text-coral-500"
                              aria-label={`Xóa cột điểm ${index + 1}`}
                              icon={<HvIcon name="trash" />}
                              onClick={() => removeComponent(index)}
                            />
                          ) : (
                            <span aria-hidden="true" className="size-11" />
                          )}
                        </div>
                        <FieldError errors={[rowError]} />
                      </div>
                    );
                  })}
                </div>
                <div className="mt-1 flex flex-wrap items-center justify-between gap-2">
                  <span className="text-[12.5px] text-ink-400">
                    {components.length}/{MAX_SCORE_SET_COMPONENTS} cột
                  </span>
                  {atLimit ? (
                    <span className="text-[12.5px] text-ink-400">
                      Tối đa {MAX_SCORE_SET_COMPONENTS} cột
                    </span>
                  ) : null}
                </div>
                <HvButton
                  type="button"
                  variant="secondary"
                  size="sm"
                  block
                  icon={<HvIcon name="plus" />}
                  onClick={addComponent}
                  disabled={atLimit}
                >
                  Thêm cột điểm
                </HvButton>
              </>
            ) : (
              <>
                <textarea
                  id="score-set-paste"
                  aria-label="Danh sách cột điểm"
                  rows={6}
                  value={pasteText}
                  placeholder={PASTE_PLACEHOLDER}
                  onChange={(event) => {
                    setPasteText(event.target.value);
                    setTruncated(false);
                  }}
                  className="w-full min-w-0 rounded-[14px] border-2 border-line-200 bg-white px-3 py-2.5 text-[14.5px] text-ink-700 outline-none placeholder:text-ink-400 focus-visible:border-mint-400"
                />
                <p className="text-[12.5px] text-ink-400">
                  Mỗi dòng một cột, hoặc cách nhau bằng dấu phẩy. Tối đa {MAX_SCORE_SET_COMPONENTS}{" "}
                  cột.
                </p>
                {pasteDuplicates && pasteDuplicates.size > 0 ? (
                  <p className="text-[12.5px] font-bold text-coral-500" role="status">
                    Tên cột điểm bị trùng:{" "}
                    {[...pasteDuplicates].map((index) => pastePreview.names[index]).join(", ")}
                  </p>
                ) : null}
              </>
            )}

            {pasteTruncated ? (
              <HvNotice tone="warning">
                Chỉ giữ {MAX_SCORE_SET_COMPONENTS} cột đầu, các cột sau đã bị bỏ.
              </HvNotice>
            ) : null}
            {errors.components?.root?.message ? (
              <HvNotice tone="danger">{errors.components.root.message}</HvNotice>
            ) : null}
            {errors.components?.message && !errors.components.root ? (
              <HvNotice tone="danger">{errors.components.message}</HvNotice>
            ) : null}
          </Field>

          <div className="flex flex-col gap-1.5">
            <p className="text-[12px] font-extrabold tracking-[0.3px] text-ink-400">XEM TRƯỚC</p>
            <ScoreSetPreviewStrip names={previewNames} />
          </div>

          <FieldError errors={[errors.root]} />
        </FieldGroup>
      </form>
    </HvModal>
  );
}
