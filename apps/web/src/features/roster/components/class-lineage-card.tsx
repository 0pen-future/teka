import { zodResolver } from "@hookform/resolvers/zod";
import { useState } from "react";
import { useForm } from "react-hook-form";
import { Link } from "react-router";

import { HvButton, HvSelect, hvToast } from "@/components/hv";
import { Field, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field";
import { useAuditLogs } from "@/features/audit";
import { ApiError } from "@/lib/api/errors";
import { cn, formatDateTime } from "@/lib/utils";

import { useClassesList, useUpdateClass } from "../hooks/use-classes";
import {
  classLineageInputSchema,
  toClassLineageUpdateInput,
  type Class,
  type ClassLineageInput,
} from "../schemas/roster-schemas";
import { SectionCard } from "./section-card";

const textareaClassName = cn(
  "min-h-20 w-full rounded-lg border border-input bg-transparent px-2.5 py-2 text-base transition-colors outline-none",
  "placeholder:text-muted-foreground focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50",
  "aria-invalid:border-destructive aria-invalid:ring-3 aria-invalid:ring-destructive/20 md:text-sm",
);

const UPDATED_TOAST = "Đã cập nhật lớp";
const NO_PARENT = "";

interface ClassLineageCardProps {
  klass: Class;
  canWrite: boolean;
  canReadAudit: boolean;
}

/**
 * "Lịch sử lớp": where the class was split from, which classes split off it,
 * and, for audit readers, the change log of this class fetched on demand.
 */
export function ClassLineageCard({ klass, canWrite, canReadAudit }: ClassLineageCardProps) {
  const [editing, setEditing] = useState(false);
  const [showAudit, setShowAudit] = useState(false);
  // Every class of the center, live or archived: a parent is usually an ended class.
  const classes = useClassesList({ status: "all", per_page: 100 });
  const others = (classes.data?.items ?? []).filter((item) => item.id !== klass.id);
  const parent = others.find((item) => item.id === klass.parent_class_id) ?? null;
  const children = others.filter((item) => item.parent_class_id === klass.id);

  return (
    <SectionCard
      title="Lịch sử lớp"
      action={
        canWrite && !editing ? (
          <HvButton size="sm" variant="ghost" onClick={() => setEditing(true)}>
            Sửa lịch sử lớp
          </HvButton>
        ) : null
      }
    >
      {editing ? (
        <LineageForm
          klass={klass}
          options={others.map((item) => ({ value: item.id, label: item.name, meta: item.code }))}
          onCancel={() => setEditing(false)}
          onSaved={() => setEditing(false)}
        />
      ) : (
        <div className="flex flex-col gap-3">
          {klass.parent_class_id === null ? (
            <p className="text-[13px] text-ink-400">Lớp không tách từ lớp nào.</p>
          ) : (
            <p className="text-[14px] text-ink-900">
              {parent ? (
                <ClassLink klass={parent} />
              ) : (
                <span className="text-ink-500">Lớp gốc</span>
              )}
              <span className="mx-2 text-ink-400">→</span>
              <span className="font-bold">{klass.name}</span>
            </p>
          )}
          {klass.lineage_note ? (
            <p className="text-[14px] whitespace-pre-wrap text-ink-700">{klass.lineage_note}</p>
          ) : null}
          {children.length > 0 ? (
            <div>
              <p className="text-[12px] font-bold tracking-[0.3px] text-ink-400">Lớp tách ra</p>
              <ul className="mt-1 flex flex-wrap gap-x-3 gap-y-1 text-[14px]">
                {children.map((child) => (
                  <li key={child.id}>
                    <ClassLink klass={child} />
                  </li>
                ))}
              </ul>
            </div>
          ) : null}
        </div>
      )}

      {canReadAudit ? (
        <div className="mt-4 border-t border-line-100 pt-3">
          <HvButton
            size="sm"
            variant="ghost"
            aria-pressed={showAudit}
            onClick={() => setShowAudit((value) => !value)}
          >
            Lịch sử thay đổi
          </HvButton>
          {showAudit ? <ClassAuditList classId={klass.id} /> : null}
        </div>
      ) : null}
    </SectionCard>
  );
}

function ClassLink({ klass }: { klass: Pick<Class, "id" | "name"> }) {
  return (
    <Link
      to={`/classes/${klass.id}`}
      className="font-display font-bold text-mint-600 hover:underline"
    >
      {klass.name}
    </Link>
  );
}

function LineageForm({
  klass,
  options,
  onCancel,
  onSaved,
}: {
  klass: Class;
  options: { value: string; label: string; meta?: string }[];
  onCancel: () => void;
  onSaved: () => void;
}) {
  const update = useUpdateClass(klass.id);
  const form = useForm<ClassLineageInput>({
    resolver: zodResolver(classLineageInputSchema),
    defaultValues: {
      parent_class_id: klass.parent_class_id ?? NO_PARENT,
      lineage_note: klass.lineage_note ?? "",
    },
  });
  const parentId = form.watch("parent_class_id");

  function submit(values: ClassLineageInput) {
    update.mutate(toClassLineageUpdateInput(klass, values), {
      onSuccess: () => {
        hvToast(UPDATED_TOAST, { variant: "success" });
        onSaved();
      },
      onError: (error) => {
        hvToast(
          error instanceof ApiError ? error.message : "Không cập nhật được lớp. Thử lại sau.",
          { variant: "danger" },
        );
      },
    });
  }

  return (
    <form onSubmit={(event) => void form.handleSubmit(submit)(event)} noValidate>
      <FieldGroup>
        <Field>
          <FieldLabel htmlFor="lineage-parent">Lớp gốc</FieldLabel>
          <HvSelect
            id="lineage-parent"
            aria-label="Lớp gốc"
            sheetTitle="Chọn lớp gốc"
            placeholder="Không có"
            options={[{ value: NO_PARENT, label: "Không có" }, ...options]}
            value={parentId}
            onValueChange={(value) =>
              form.setValue("parent_class_id", value, { shouldDirty: true })
            }
            className="w-full"
          />
        </Field>
        <Field>
          <FieldLabel htmlFor="lineage-note">Ghi chú lịch sử</FieldLabel>
          <textarea
            id="lineage-note"
            className={textareaClassName}
            aria-invalid={Boolean(form.formState.errors.lineage_note)}
            {...form.register("lineage_note")}
          />
          <FieldError errors={[form.formState.errors.lineage_note]} />
        </Field>
      </FieldGroup>
      <div className="mt-3 flex justify-end gap-2">
        <HvButton size="sm" variant="ghost" type="button" onClick={onCancel}>
          Huỷ
        </HvButton>
        <HvButton size="sm" type="submit" disabled={update.isPending}>
          Lưu
        </HvButton>
      </div>
    </form>
  );
}

function ClassAuditList({ classId }: { classId: string }) {
  const logs = useAuditLogs({ entity_type: "class", entity_id: classId }, true);
  const items = logs.data?.pages.flatMap((page) => page.items) ?? [];

  if (logs.isPending) {
    return <p className="mt-2 text-[13px] text-ink-400">Đang tải…</p>;
  }
  if (logs.isError) {
    return <p className="mt-2 text-[13px] text-coral-600">Không tải được lịch sử thay đổi.</p>;
  }
  if (items.length === 0) {
    return <p className="mt-2 text-[13px] text-ink-400">Chưa có thay đổi nào được ghi lại.</p>;
  }
  return (
    <div className="mt-2 flex flex-col gap-2">
      <ul className="flex flex-col gap-1 text-[13px]">
        {items.map((log) => (
          <li key={log.id} className="flex flex-wrap items-center gap-x-2 text-ink-700">
            <span className="font-bold text-ink-900">{log.actor_name}</span>
            <span className="text-ink-400">·</span>
            <span>{log.action}</span>
            <span className="text-ink-400">·</span>
            <span className="text-ink-500">{formatDateTime(log.occurred_at)}</span>
          </li>
        ))}
      </ul>
      {logs.hasNextPage ? (
        <HvButton
          size="sm"
          variant="ghost"
          disabled={logs.isFetchingNextPage}
          onClick={() => void logs.fetchNextPage()}
        >
          Tải thêm
        </HvButton>
      ) : null}
    </div>
  );
}
