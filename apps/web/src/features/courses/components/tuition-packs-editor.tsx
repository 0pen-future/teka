import { useState } from "react";

import { HvButton, HvIcon, hvToast } from "@/components/hv";
import { FieldError } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { rowErrorsFromApi, type RowErrors } from "@/features/library";
import { cn, formatMoney } from "@/lib/utils";

import { useSetTuitionPacks } from "../hooks/use-courses";
import type { TuitionPack, TuitionPackInput } from "../schemas/courses-schemas";

/** The API caps a course at this many packs. */
const MAX_PACKS = 20;

interface Row {
  key: number;
  name: string;
  /** Kept as typed so a half-entered value survives re-render; parsed on save. */
  sessions: string;
  price: string;
}

let nextKey = 0;

function rowsFromPacks(packs: TuitionPack[]): Row[] {
  return packs.map((pack) => ({
    key: nextKey++,
    name: pack.name,
    sessions: String(pack.sessions),
    price: String(pack.price),
  }));
}

function validate(rows: Row[]): RowErrors["rows"] {
  const errors: RowErrors["rows"] = {};
  rows.forEach((row, index) => {
    const own: Record<string, string> = {};
    const name = row.name.trim();
    if (name === "") own.name = "Bắt buộc nhập tên gói";
    else if (name.length > 100) own.name = "Tối đa 100 ký tự";
    const sessions = Number(row.sessions);
    if (!/^\d+$/.test(row.sessions.trim()) || sessions < 1 || sessions > 1000) {
      own.sessions = "Số buổi từ 1 đến 1000";
    }
    if (!/^\d+$/.test(row.price.trim())) {
      own.price = "Giá phải là số nguyên không âm";
    }
    if (Object.keys(own).length > 0) errors[index] = own;
  });
  return errors;
}

function toInput(row: Row): TuitionPackInput {
  return { name: row.name.trim(), sessions: Number(row.sessions), price: Number(row.price) };
}

const NO_ERRORS: RowErrors = { rows: {}, general: null };

const sectionClassName =
  "flex flex-col gap-3 rounded-[var(--radius-lg)] border border-line-200 bg-white p-4";
const headCellClassName =
  "px-2 py-1 text-left text-[12px] font-extrabold uppercase tracking-[0.4px] text-ink-500";

interface TuitionPacksEditorProps {
  courseId: string;
  packs: TuitionPack[];
}

/**
 * Row editor for a course's tuition packs, saved wholesale in display
 * order. Rows are checked client-side first so the common mistakes never
 * cost a round trip.
 */
export function TuitionPacksEditor({ courseId, packs }: TuitionPacksEditorProps) {
  const [rows, setRows] = useState<Row[]>(() => rowsFromPacks(packs));
  const [errors, setErrors] = useState<RowErrors>(NO_ERRORS);
  const save = useSetTuitionPacks(courseId);

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
    setRows((current) =>
      current.length >= MAX_PACKS
        ? current
        : [...current, { key: nextKey++, name: "", sessions: "", price: "0" }],
    );
  }

  function submit() {
    const clientErrors = validate(rows);
    if (Object.keys(clientErrors).length > 0) {
      setErrors({ rows: clientErrors, general: null });
      return;
    }
    setErrors(NO_ERRORS);
    save.mutate(rows.map(toInput), {
      onSuccess: () => hvToast("Đã lưu gói học phí"),
      onError: (error) => setErrors(rowErrorsFromApi(error, "Không lưu được gói học phí.")),
    });
  }

  return (
    <section aria-label="Gói học phí" className={sectionClassName}>
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h2 className="font-display text-[16px] font-extrabold text-ink-900">Gói học phí</h2>
        <HvButton
          type="button"
          size="sm"
          variant="secondary"
          disabled={rows.length >= MAX_PACKS}
          onClick={add}
        >
          Thêm gói
        </HvButton>
      </div>
      <p className="text-[13px] text-ink-500">
        Mỗi gói là số buổi bán kèm mức giá trọn gói; thứ tự ở đây là thứ tự hiển thị khi ghi danh.
      </p>
      {rows.length === 0 ? (
        <p className="text-[14px] text-ink-400">Chưa có gói học phí nào.</p>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full min-w-[560px] border-collapse">
            <thead>
              <tr>
                <th className={cn(headCellClassName, "w-8")}>#</th>
                <th className={headCellClassName}>Tên gói</th>
                <th className={cn(headCellClassName, "w-[120px]")}>Số buổi</th>
                <th className={cn(headCellClassName, "w-[160px]")}>Giá (đ)</th>
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
                        aria-label={`Tên gói ${n}`}
                        placeholder="VD: Gói 12 buổi"
                        value={row.name}
                        aria-invalid={Boolean(own.name)}
                        onChange={(event) => update(index, { name: event.target.value })}
                      />
                      <FieldError errors={[own.name ? { message: own.name } : undefined]} />
                    </td>
                    <td className="px-2 py-2">
                      <Input
                        type="number"
                        min={1}
                        aria-label={`Số buổi ${n}`}
                        value={row.sessions}
                        aria-invalid={Boolean(own.sessions)}
                        onChange={(event) => update(index, { sessions: event.target.value })}
                      />
                      <FieldError errors={[own.sessions ? { message: own.sessions } : undefined]} />
                    </td>
                    <td className="px-2 py-2">
                      <Input
                        type="number"
                        min={0}
                        step={10000}
                        aria-label={`Giá ${n}`}
                        value={row.price}
                        aria-invalid={Boolean(own.price)}
                        onChange={(event) => update(index, { price: event.target.value })}
                      />
                      <FieldError errors={[own.price ? { message: own.price } : undefined]} />
                    </td>
                    <td className="px-2 py-2 whitespace-nowrap text-right">
                      <div className="inline-flex items-center gap-1">
                        <HvButton
                          type="button"
                          size="sm"
                          variant="ghost"
                          aria-label={`Chuyển lên gói ${n}`}
                          disabled={index === 0}
                          onClick={() => move(index, -1)}
                        >
                          <HvIcon name="arrow-up" size={16} />
                        </HvButton>
                        <HvButton
                          type="button"
                          size="sm"
                          variant="ghost"
                          aria-label={`Chuyển xuống gói ${n}`}
                          disabled={index === rows.length - 1}
                          onClick={() => move(index, 1)}
                        >
                          <HvIcon name="arrow-down" size={16} />
                        </HvButton>
                        <HvButton
                          type="button"
                          size="sm"
                          variant="ghost"
                          aria-label={`Xoá gói ${n}`}
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
          {save.isPending ? "Đang lưu…" : "Lưu gói học phí"}
        </HvButton>
      </div>
    </section>
  );
}

export function TuitionPacksReadOnly({ packs }: { packs: TuitionPack[] }) {
  return (
    <section aria-label="Gói học phí" className={sectionClassName}>
      <h2 className="font-display text-[16px] font-extrabold text-ink-900">Gói học phí</h2>
      {packs.length === 0 ? (
        <p className="text-[14px] text-ink-400">Chưa có gói học phí nào.</p>
      ) : (
        <table className="w-full border-collapse text-[14px]">
          <thead>
            <tr>
              <th className={headCellClassName}>Tên gói</th>
              <th className={cn(headCellClassName, "text-right")}>Số buổi</th>
              <th className={cn(headCellClassName, "text-right")}>Giá</th>
            </tr>
          </thead>
          <tbody>
            {packs.map((pack) => (
              <tr key={pack.id} className="border-t border-line-100">
                <td className="px-2 py-2 font-bold text-ink-900">{pack.name}</td>
                <td className="px-2 py-2 text-right text-ink-700">{pack.sessions}</td>
                <td className="px-2 py-2 text-right text-ink-700">{formatMoney(pack.price)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </section>
  );
}
