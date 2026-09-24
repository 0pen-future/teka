import { useState } from "react";

import { HvBadge, HvButton, HvIcon, HvSelect, hvToast } from "@/components/hv";
import { FieldError } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { cn } from "@/lib/utils";

import { useSetLogFields } from "../hooks/use-library";
import { logFieldKindLabel } from "../lib/library-labels";
import { rowErrorsFromApi, type RowErrors } from "../lib/row-errors";
import {
  splitTags,
  type LogField,
  type LogFieldInput,
  type LogFieldKind,
} from "../schemas/library-schemas";

/** New rows may only pick one of the four v5 kinds; legacy `number`/`select` rows stay editable as-is. */
const NEW_ROW_KINDS: LogFieldKind[] = ["text", "long_text", "checkbox", "student"];

const newRowKindOptions = NEW_ROW_KINDS.map((kind) => ({
  value: kind,
  label: logFieldKindLabel[kind],
}));

/** A row keeps its own current kind selectable even when it is a legacy kind no longer offered for new rows. */
function kindOptionsFor(currentKind: LogFieldKind) {
  if (NEW_ROW_KINDS.includes(currentKind)) return newRowKindOptions;
  return [...newRowKindOptions, { value: currentKind, label: logFieldKindLabel[currentKind] }];
}

interface Row {
  /** Local identity for React keys; server ids do not survive a wholesale replace. */
  key: number;
  label: string;
  kind: LogFieldKind;
  /** Comma-separated; only read for kind `select`. */
  optionsText: string;
  required: boolean;
}

let nextKey = 0;

function rowsFromFields(fields: LogField[]): Row[] {
  return fields.map((field) => ({
    key: nextKey++,
    label: field.label,
    kind: field.kind,
    optionsText: field.options.join(", "),
    required: field.required,
  }));
}

function validate(rows: Row[]): RowErrors["rows"] {
  const errors: RowErrors["rows"] = {};
  rows.forEach((row, index) => {
    const own: Record<string, string> = {};
    if (row.label.trim() === "") own.label = "Bắt buộc nhập nhãn";
    else if (row.label.trim().length > 100) own.label = "Tối đa 100 ký tự";
    if (row.kind === "select") {
      const options = splitTags(row.optionsText);
      if (options.length === 0) own.options = "Nhập ít nhất một tuỳ chọn";
      else if (options.length > 20) own.options = "Tối đa 20 tuỳ chọn";
      else if (options.some((option) => option.length > 100)) {
        own.options = "Mỗi tuỳ chọn tối đa 100 ký tự";
      }
    }
    if (Object.keys(own).length > 0) errors[index] = own;
  });
  return errors;
}

function toInput(row: Row): LogFieldInput {
  return {
    label: row.label.trim(),
    kind: row.kind,
    options: row.kind === "select" ? splitTags(row.optionsText) : [],
    required: row.required,
  };
}

const NO_ERRORS: RowErrors = { rows: {}, general: null };

const sectionClassName =
  "flex flex-col gap-3 rounded-[var(--radius-lg)] border border-line-200 bg-white p-4";
const rowClassName = "flex flex-col gap-2 border-t border-line-100 py-3 sm:flex-row sm:items-start";

interface LogFieldsEditorProps {
  versionId: string;
  templateId: string;
  fields: LogField[];
}

/**
 * Row editor for a draft version's per-session log fields. The list is
 * replaced wholesale on save; client checks catch the obvious gaps and
 * the API's `<index>.<field>` errors land on the same rows.
 */
export function LogFieldsEditor({ versionId, templateId, fields }: LogFieldsEditorProps) {
  const [rows, setRows] = useState<Row[]>(() => rowsFromFields(fields));
  const [errors, setErrors] = useState<RowErrors>(NO_ERRORS);
  const save = useSetLogFields(versionId, templateId);

  function update(index: number, patch: Partial<Row>) {
    setRows((current) => current.map((row, i) => (i === index ? { ...row, ...patch } : row)));
  }

  function move(index: number, direction: -1 | 1) {
    setRows((current) => {
      const target = index + direction;
      if (target < 0 || target >= current.length) return current;
      const next = [...current];
      [next[index], next[target]] = [next[target]!, next[index]!];
      return next;
    });
    setErrors(NO_ERRORS);
  }

  function remove(index: number) {
    setRows((current) => current.filter((_, i) => i !== index));
    setErrors(NO_ERRORS);
  }

  function add() {
    setRows((current) => [
      ...current,
      { key: nextKey++, label: "", kind: "text", optionsText: "", required: false },
    ]);
  }

  function submit() {
    const clientErrors = validate(rows);
    if (Object.keys(clientErrors).length > 0) {
      setErrors({ rows: clientErrors, general: null });
      return;
    }
    setErrors(NO_ERRORS);
    save.mutate(rows.map(toInput), {
      onSuccess: () => hvToast("Đã lưu trường nhật ký"),
      onError: (error) => setErrors(rowErrorsFromApi(error, "Không lưu được trường nhật ký.")),
    });
  }

  return (
    <section aria-label="Trường nhật ký" className={sectionClassName}>
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h2 className="font-display text-[16px] font-extrabold text-ink-900">Trường nhật ký</h2>
        <HvButton type="button" size="sm" variant="secondary" onClick={add}>
          Thêm trường
        </HvButton>
      </div>
      <p className="text-[13px] text-ink-500">
        Giáo viên điền các trường này sau mỗi buổi học của lớp dùng phiên bản này.
      </p>
      {rows.length === 0 ? (
        <p className="text-[14px] text-ink-400">Chưa có trường nào.</p>
      ) : (
        <ul className="flex flex-col">
          {rows.map((row, index) => {
            const n = index + 1;
            const own = errors.rows[index] ?? {};
            return (
              <li key={row.key} className={rowClassName}>
                <span className="w-6 pt-2.5 font-mono text-[13px] text-ink-500">{n}</span>
                <div className="grid flex-1 gap-2 sm:grid-cols-[1fr_180px]">
                  <div className="flex flex-col gap-1">
                    <Input
                      aria-label={`Nhãn trường ${n}`}
                      placeholder="VD: Mức độ tập trung"
                      value={row.label}
                      aria-invalid={Boolean(own.label)}
                      onChange={(event) => update(index, { label: event.target.value })}
                    />
                    <FieldError errors={[own.label ? { message: own.label } : undefined]} />
                  </div>
                  <HvSelect
                    options={kindOptionsFor(row.kind)}
                    value={row.kind}
                    onValueChange={(kind) => update(index, { kind: kind as LogFieldKind })}
                    sheetTitle="Chọn loại trường"
                    searchThreshold={Infinity}
                    aria-label={`Loại trường ${n}`}
                    aria-invalid={Boolean(own.kind)}
                    className="w-full"
                  />
                  {row.kind === "select" ? (
                    <div className="flex flex-col gap-1 sm:col-span-2">
                      <Input
                        aria-label={`Tuỳ chọn trường ${n}`}
                        placeholder="Các tuỳ chọn, cách nhau bằng dấu phẩy"
                        value={row.optionsText}
                        aria-invalid={Boolean(own.options)}
                        onChange={(event) => update(index, { optionsText: event.target.value })}
                      />
                      <FieldError errors={[own.options ? { message: own.options } : undefined]} />
                    </div>
                  ) : null}
                  <label className="flex items-center gap-2 text-[13px] text-ink-700 sm:col-span-2">
                    <input
                      type="checkbox"
                      className="size-4 accent-mint-600"
                      aria-label={`Bắt buộc trường ${n}`}
                      checked={row.required}
                      onChange={(event) => update(index, { required: event.target.checked })}
                    />
                    Bắt buộc
                  </label>
                </div>
                <div className="inline-flex items-center gap-1 self-end sm:self-start">
                  <HvButton
                    type="button"
                    size="sm"
                    variant="ghost"
                    aria-label={`Chuyển lên trường ${n}`}
                    disabled={index === 0}
                    onClick={() => move(index, -1)}
                  >
                    <HvIcon name="arrow-up" size={16} />
                  </HvButton>
                  <HvButton
                    type="button"
                    size="sm"
                    variant="ghost"
                    aria-label={`Chuyển xuống trường ${n}`}
                    disabled={index === rows.length - 1}
                    onClick={() => move(index, 1)}
                  >
                    <HvIcon name="arrow-down" size={16} />
                  </HvButton>
                  <HvButton
                    type="button"
                    size="sm"
                    variant="ghost"
                    aria-label={`Xoá trường ${n}`}
                    onClick={() => remove(index)}
                  >
                    Xoá
                  </HvButton>
                </div>
              </li>
            );
          })}
        </ul>
      )}
      <FieldError errors={[errors.general ? { message: errors.general } : undefined]} />
      <div className="flex justify-end">
        <HvButton type="button" size="sm" disabled={save.isPending} onClick={submit}>
          {save.isPending ? "Đang lưu…" : "Lưu trường nhật ký"}
        </HvButton>
      </div>
    </section>
  );
}

/** The same list on a published or archived version, or for a reader without `library.edit`. */
export function LogFieldsReadOnly({ fields }: { fields: LogField[] }) {
  return (
    <section aria-label="Trường nhật ký" className={sectionClassName}>
      <h2 className="font-display text-[16px] font-extrabold text-ink-900">Trường nhật ký</h2>
      {fields.length === 0 ? (
        <p className="text-[14px] text-ink-400">Chưa có trường nào.</p>
      ) : (
        <ul className="flex flex-col">
          {fields.map((field) => (
            <li
              key={field.id}
              className="flex flex-wrap items-center gap-x-3 gap-y-1 border-t border-line-100 py-2"
            >
              <span className="font-mono text-[13px] text-ink-500">{field.position}</span>
              <span className="min-w-0 flex-1 text-[14px] font-bold text-ink-900">
                {field.label}
              </span>
              <HvBadge variant="neutral" size="sm">
                {logFieldKindLabel[field.kind]}
              </HvBadge>
              {field.required ? (
                <HvBadge variant="warning" size="sm">
                  Bắt buộc
                </HvBadge>
              ) : null}
              {field.kind === "select" ? (
                <span className={cn("basis-full text-[13px] text-ink-500", "sm:basis-auto")}>
                  {field.options.join(", ")}
                </span>
              ) : null}
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
