import { zodResolver } from "@hookform/resolvers/zod";
import { XIcon } from "lucide-react";
import { useId, useState } from "react";
import { useForm } from "react-hook-form";

import { HvBadge, HvButton, HvCard, hvToast } from "@/components/hv";
import { Field, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { ApiError } from "@/lib/api/errors";
import { cn } from "@/lib/utils";

import { useUpdateClass } from "../hooks/use-classes";
import {
  classOpsInputSchema,
  toClassUpdateInput,
  type Class,
  type ClassOpsInput,
} from "../schemas/roster-schemas";

const textareaClassName = cn(
  "min-h-20 w-full rounded-lg border border-input bg-transparent px-2.5 py-2 text-base transition-colors outline-none",
  "placeholder:text-muted-foreground focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50",
  "aria-invalid:border-destructive aria-invalid:ring-3 aria-invalid:ring-destructive/20 md:text-sm",
);

const UPDATED_TOAST = "Đã cập nhật lớp";

function updateErrorMessage(error: unknown): string {
  return error instanceof ApiError ? error.message : "Không cập nhật được lớp. Thử lại sau.";
}

interface ClassOpsCardProps {
  klass: Class;
  canWrite: boolean;
}

/**
 * "Thông tin vận hành": the recruiting flag, tags and operational note.
 * Every write goes through `toClassUpdateInput`, so the request carries only
 * the field that changed and never clobbers a rename made elsewhere.
 */
export function ClassOpsCard({ klass, canWrite }: ClassOpsCardProps) {
  const [editing, setEditing] = useState(false);
  const update = useUpdateClass(klass.id);
  const headingId = useId();
  const recruitingLabelId = useId();

  function toggleRecruiting() {
    update.mutate(toClassUpdateInput(klass, { recruiting: !klass.recruiting }), {
      onSuccess: () => hvToast(UPDATED_TOAST, { variant: "success" }),
      onError: (error) => hvToast(updateErrorMessage(error), { variant: "danger" }),
    });
  }

  return (
    <HvCard aria-labelledby={headingId} role="region">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h2 id={headingId} className="font-display text-[16px] font-bold text-ink-900">
          Thông tin vận hành
        </h2>
        {canWrite && !editing ? (
          <HvButton size="sm" variant="ghost" onClick={() => setEditing(true)}>
            Sửa thông tin vận hành
          </HvButton>
        ) : null}
      </div>

      <div className="mt-3 flex items-center justify-between gap-3">
        <span id={recruitingLabelId} className="text-[14px] font-bold text-ink-700">
          Cần tuyển sinh
        </span>
        {canWrite ? (
          <button
            type="button"
            role="switch"
            aria-checked={klass.recruiting}
            aria-labelledby={recruitingLabelId}
            disabled={update.isPending}
            onClick={toggleRecruiting}
            className={cn(
              "relative inline-flex h-7 w-12 shrink-0 items-center rounded-full border-2 border-transparent transition-colors",
              "focus-visible:outline-none focus-visible:ring-4 focus-visible:ring-mint-100 disabled:opacity-50",
              klass.recruiting ? "bg-mint-400" : "bg-line-300",
            )}
          >
            <span
              aria-hidden="true"
              className={cn(
                "inline-block size-5 rounded-full bg-white shadow transition-transform",
                klass.recruiting ? "translate-x-5" : "translate-x-0",
              )}
            />
          </button>
        ) : (
          <HvBadge variant={klass.recruiting ? "success" : "neutral"} size="sm">
            {klass.recruiting ? "Có" : "Không"}
          </HvBadge>
        )}
      </div>

      {editing ? (
        <ClassOpsForm
          klass={klass}
          onCancel={() => setEditing(false)}
          onSaved={() => setEditing(false)}
        />
      ) : (
        <dl className="mt-3 flex flex-col gap-3">
          <div>
            <dt className="text-[12px] font-bold tracking-[0.3px] text-ink-400">Thẻ</dt>
            <dd className="mt-1">
              {klass.tags.length === 0 ? (
                <span className="text-[13px] text-ink-400">Chưa có thẻ</span>
              ) : (
                <div className="flex flex-wrap gap-1">
                  {klass.tags.map((tag) => (
                    <HvBadge key={tag} variant="neutral" size="sm">
                      {tag}
                    </HvBadge>
                  ))}
                </div>
              )}
            </dd>
          </div>
          <div>
            <dt className="text-[12px] font-bold tracking-[0.3px] text-ink-400">Ghi chú</dt>
            <dd className="mt-1 text-[14px] whitespace-pre-wrap text-ink-900">
              {klass.note ?? <span className="text-ink-400">Chưa có ghi chú</span>}
            </dd>
          </div>
        </dl>
      )}
    </HvCard>
  );
}

function ClassOpsForm({
  klass,
  onCancel,
  onSaved,
}: {
  klass: Class;
  onCancel: () => void;
  onSaved: () => void;
}) {
  const update = useUpdateClass(klass.id);
  const [tagDraft, setTagDraft] = useState("");
  const noteId = useId();
  const tagId = useId();
  const form = useForm<ClassOpsInput>({
    resolver: zodResolver(classOpsInputSchema),
    defaultValues: { recruiting: klass.recruiting, tags: klass.tags, note: klass.note ?? "" },
  });
  const tags = form.watch("tags");
  const errors = form.formState.errors;

  function addTag() {
    const tag = tagDraft.trim();
    if (tag === "" || tags.includes(tag)) {
      setTagDraft("");
      return;
    }
    form.setValue("tags", [...tags, tag], { shouldValidate: true, shouldDirty: true });
    setTagDraft("");
  }

  function removeTag(tag: string) {
    form.setValue(
      "tags",
      tags.filter((item) => item !== tag),
      { shouldValidate: true, shouldDirty: true },
    );
  }

  const onSubmit = form.handleSubmit((values) => {
    // The recruiting switch saves on its own; the form only owns tags + note.
    update.mutate(toClassUpdateInput(klass, { tags: values.tags, note: values.note }), {
      onSuccess: () => {
        hvToast(UPDATED_TOAST, { variant: "success" });
        onSaved();
      },
      onError: (error) => form.setError("root", { message: updateErrorMessage(error) }),
    });
  });

  return (
    <form onSubmit={(event) => void onSubmit(event)} noValidate className="mt-3">
      <FieldGroup>
        <Field data-invalid={Boolean(errors.tags)}>
          <FieldLabel htmlFor={tagId}>Thêm thẻ</FieldLabel>
          {tags.length > 0 ? (
            <div className="flex flex-wrap gap-1">
              {tags.map((tag) => (
                <HvBadge key={tag} variant="neutral" size="sm" className="gap-1 pr-1">
                  {tag}
                  <button
                    type="button"
                    aria-label={`Bỏ thẻ ${tag}`}
                    onClick={() => removeTag(tag)}
                    className="rounded-full p-0.5 hover:bg-line-200"
                  >
                    <XIcon aria-hidden="true" className="size-3" />
                  </button>
                </HvBadge>
              ))}
            </div>
          ) : null}
          <Input
            id={tagId}
            value={tagDraft}
            placeholder="Nhập thẻ rồi nhấn Enter"
            aria-invalid={Boolean(errors.tags)}
            onChange={(event) => setTagDraft(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === "Enter") {
                event.preventDefault();
                addTag();
              }
            }}
          />
          <FieldError errors={[errors.tags]} />
        </Field>
        <Field data-invalid={Boolean(errors.note)}>
          <FieldLabel htmlFor={noteId}>Ghi chú</FieldLabel>
          <textarea
            id={noteId}
            aria-invalid={Boolean(errors.note)}
            className={textareaClassName}
            placeholder="Phòng học, lưu ý vận hành…"
            {...form.register("note")}
          />
          <FieldError errors={[errors.note]} />
        </Field>
        <FieldError errors={[errors.root]} />
      </FieldGroup>
      <div className="mt-3 flex justify-end gap-2">
        <HvButton
          type="button"
          size="sm"
          variant="ghost"
          onClick={onCancel}
          disabled={update.isPending}
        >
          Huỷ
        </HvButton>
        <HvButton type="submit" size="sm" disabled={update.isPending}>
          {update.isPending ? "Đang lưu…" : "Lưu"}
        </HvButton>
      </div>
    </form>
  );
}
