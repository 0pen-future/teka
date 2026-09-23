import { useState } from "react";

import { HvButton, HvIcon, hvToast } from "@/components/hv";
import { FieldError } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { cn } from "@/lib/utils";

import { useSetScoreSet } from "../hooks/use-library";
import { rowErrorsFromApi, type RowErrors } from "../lib/row-errors";
import type { ScoreComponent, ScoreComponentInput } from "../schemas/library-schemas";

/** Mirrors the API's key rule: a stable machine name a class's grading refers to. */
const KEY_PATTERN = /^[a-z0-9_]{1,30}$/;

interface Row {
  key: number;
  code: string;
  label: string;
  /** Kept as typed so a half-entered "0." survives re-render; parsed on save. */
  max: string;
  weight: string;
}

let nextKey = 0;

function rowsFromComponents(components: ScoreComponent[]): Row[] {
  return components.map((component) => ({
    key: nextKey++,
    code: component.key,
    label: component.label,
    max: String(component.max),
    weight: String(component.weight),
  }));
}

function validate(rows: Row[]): RowErrors["rows"] {
  const errors: RowErrors["rows"] = {};
  const seen = new Set<string>();
  rows.forEach((row, index) => {
    const own: Record<string, string> = {};
    const code = row.code.trim();
    if (code === "") own.key = "Bắt buộc nhập mã";
    else if (!KEY_PATTERN.test(code)) own.key = "Mã chỉ gồm chữ thường, số và _ (tối đa 30)";
    else if (seen.has(code)) own.key = "Mã bị trùng";
    seen.add(code);
    if (row.label.trim() === "") own.label = "Bắt buộc nhập tên";
    else if (row.label.trim().length > 100) own.label = "Tối đa 100 ký tự";
    const max = Number(row.max);
    if (row.max.trim() === "" || !Number.isFinite(max) || max <= 0) {
      own.max = "Điểm tối đa phải lớn hơn 0";
    }
    const weight = Number(row.weight);
    if (row.weight.trim() === "" || !Number.isFinite(weight) || weight < 0) {
      own.weight = "Trọng số không được âm";
    }
    if (Object.keys(own).length > 0) errors[index] = own;
  });
  return errors;
}

function toInput(row: Row): ScoreComponentInput {
  return {
    key: row.code.trim(),
    label: row.label.trim(),
    max: Number(row.max),
    weight: Number(row.weight),
  };
}

const NO_ERRORS: RowErrors = { rows: {}, general: null };

const sectionClassName =
  "flex flex-col gap-3 rounded-[var(--radius-lg)] border border-line-200 bg-white p-4";
const headCellClassName =
  "px-2 py-1 text-left text-[12px] font-extrabold uppercase tracking-[0.4px] text-ink-500";

interface ScoreSetEditorProps {
  versionId: string;
  templateId: string;
  components: ScoreComponent[];
}

/**
 * Row editor for a draft version's grade components. Keys are what a
 * class's score sheet is later keyed on, so they are checked for shape
 * and uniqueness before the wholesale replace goes out.
 */
export function ScoreSetEditor({ versionId, templateId, components }: ScoreSetEditorProps) {
  const [rows, setRows] = useState<Row[]>(() => rowsFromComponents(components));
  const [errors, setErrors] = useState<RowErrors>(NO_ERRORS);
  const save = useSetScoreSet(versionId, templateId);

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
      { key: nextKey++, code: "", label: "", max: "10", weight: "0" },
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
      onSuccess: () => hvToast("Đã lưu cơ cấu điểm"),
      onError: (error) => setErrors(rowErrorsFromApi(error, "Không lưu được cơ cấu điểm.")),
    });
  }

  return (
    <section aria-label="Cơ cấu điểm" className={sectionClassName}>
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h2 className="font-display text-[16px] font-extrabold text-ink-900">Cơ cấu điểm</h2>
        <HvButton type="button" size="sm" variant="secondary" onClick={add}>
          Thêm thành phần
        </HvButton>
      </div>
      <p className="text-[13px] text-ink-500">
        Mã là tên máy dùng trong bảng điểm của lớp; trọng số 0 nghĩa là không tính vào tổng.
      </p>
      {rows.length === 0 ? (
        <p className="text-[14px] text-ink-400">Chưa có thành phần nào.</p>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full min-w-[640px] border-collapse">
            <thead>
              <tr>
                <th className={cn(headCellClassName, "w-8")}>#</th>
                <th className={headCellClassName}>Mã</th>
                <th className={headCellClassName}>Tên</th>
                <th className={cn(headCellClassName, "w-[120px]")}>Điểm tối đa</th>
                <th className={cn(headCellClassName, "w-[120px]")}>Trọng số</th>
                <th className={headCellClassName}>
                  <span className="sr-only">Thao tác</span>
                </th>
              </tr>
            </thead>
            <tbody>
              {rows.map((row, index) => {
                const n = index + 1;
                const own = errors.rows[index] ?? {};
                return (
                  <tr key={row.key} className="border-t border-line-100 align-top">
                    <td className="px-2 py-2 font-mono text-[13px] text-ink-500">{n}</td>
                    <td className="px-2 py-2">
                      <Input
                        aria-label={`Mã thành phần ${n}`}
                        placeholder="VD: kt"
                        value={row.code}
                        aria-invalid={Boolean(own.key)}
                        onChange={(event) => update(index, { code: event.target.value })}
                      />
                      <FieldError errors={[own.key ? { message: own.key } : undefined]} />
                    </td>
                    <td className="px-2 py-2">
                      <Input
                        aria-label={`Tên thành phần ${n}`}
                        placeholder="VD: Kiểm tra"
                        value={row.label}
                        aria-invalid={Boolean(own.label)}
                        onChange={(event) => update(index, { label: event.target.value })}
                      />
                      <FieldError errors={[own.label ? { message: own.label } : undefined]} />
                    </td>
                    <td className="px-2 py-2">
                      <Input
                        type="number"
                        min={0}
                        step="any"
                        aria-label={`Điểm tối đa ${n}`}
                        value={row.max}
                        aria-invalid={Boolean(own.max)}
                        onChange={(event) => update(index, { max: event.target.value })}
                      />
                      <FieldError errors={[own.max ? { message: own.max } : undefined]} />
                    </td>
                    <td className="px-2 py-2">
                      <Input
                        type="number"
                        min={0}
                        step="any"
                        aria-label={`Trọng số ${n}`}
                        value={row.weight}
                        aria-invalid={Boolean(own.weight)}
                        onChange={(event) => update(index, { weight: event.target.value })}
                      />
                      <FieldError errors={[own.weight ? { message: own.weight } : undefined]} />
                    </td>
                    <td className="px-2 py-2 whitespace-nowrap text-right">
                      <div className="inline-flex items-center gap-1">
                        <HvButton
                          type="button"
                          size="sm"
                          variant="ghost"
                          aria-label={`Chuyển lên thành phần ${n}`}
                          disabled={index === 0}
                          onClick={() => move(index, -1)}
                        >
                          <HvIcon name="arrow-up" size={16} />
                        </HvButton>
                        <HvButton
                          type="button"
                          size="sm"
                          variant="ghost"
                          aria-label={`Chuyển xuống thành phần ${n}`}
                          disabled={index === rows.length - 1}
                          onClick={() => move(index, 1)}
                        >
                          <HvIcon name="arrow-down" size={16} />
                        </HvButton>
                        <HvButton
                          type="button"
                          size="sm"
                          variant="ghost"
                          aria-label={`Xoá thành phần ${n}`}
                          onClick={() => remove(index)}
                        >
                          Xoá
                        </HvButton>
                      </div>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
      <FieldError errors={[errors.general ? { message: errors.general } : undefined]} />
      <div className="flex justify-end">
        <HvButton type="button" size="sm" disabled={save.isPending} onClick={submit}>
          {save.isPending ? "Đang lưu…" : "Lưu cơ cấu điểm"}
        </HvButton>
      </div>
    </section>
  );
}

export function ScoreSetReadOnly({ components }: { components: ScoreComponent[] }) {
  return (
    <section aria-label="Cơ cấu điểm" className={sectionClassName}>
      <h2 className="font-display text-[16px] font-extrabold text-ink-900">Cơ cấu điểm</h2>
      {components.length === 0 ? (
        <p className="text-[14px] text-ink-400">Chưa có thành phần nào.</p>
      ) : (
        <table className="w-full border-collapse text-[14px]">
          <thead>
            <tr>
              <th className={headCellClassName}>Mã</th>
              <th className={headCellClassName}>Tên</th>
              <th className={cn(headCellClassName, "text-right")}>Điểm tối đa</th>
              <th className={cn(headCellClassName, "text-right")}>Trọng số</th>
            </tr>
          </thead>
          <tbody>
            {components.map((component) => (
              <tr key={component.key} className="border-t border-line-100">
                <td className="px-2 py-2 font-mono text-[13px] text-ink-700">{component.key}</td>
                <td className="px-2 py-2 font-bold text-ink-900">{component.label}</td>
                <td className="px-2 py-2 text-right text-ink-700">{component.max}</td>
                <td className="px-2 py-2 text-right text-ink-700">{component.weight}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </section>
  );
}
