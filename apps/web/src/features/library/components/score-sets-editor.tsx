import { useState } from "react";

import { HvButton, HvCard, hvToast } from "@/components/hv";
import { FieldError } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { ApiError } from "@/lib/api/errors";
import { cn } from "@/lib/utils";

import { useSetScoreSet } from "../hooks/use-library";
import type { ScoreComponent, ScoreSetGroup, ScoreSetGroupInput } from "../schemas/library-schemas";

const KEY_PATTERN = /^[a-z0-9_]{1,30}$/;
const MAX_KEY_LENGTH = 30;

const headCellClassName =
  "px-2 py-2 text-[11px] font-extrabold uppercase tracking-[0.4px] text-ink-500";

interface Row {
  /** Local identity for React keys; server ids do not survive a wholesale replace. */
  key: number;
  code: string;
  label: string;
  max: string;
  weight: string;
}

interface Group {
  /** Local identity for React keys. */
  uiKey: number;
  /** Slug generated once from the initial title; stable across later title edits. */
  groupKey: string;
  title: string;
  rows: Row[];
}

let nextRowKey = 0;
let nextGroupKey = 0;

function slugify(text: string): string {
  const base = text
    .trim()
    .toLowerCase()
    .normalize("NFD")
    .replace(/[̀-ͯ]/g, "")
    .replace(/đ/g, "d")
    .replace(/[^a-z0-9]+/g, "_")
    .replace(/^_+|_+$/g, "");
  return (base === "" ? "bo_diem" : base).slice(0, MAX_KEY_LENGTH);
}

function uniqueKey(base: string, taken: Set<string>): string {
  if (!taken.has(base)) return base;
  let suffix = 2;
  let candidate = `${base}_${suffix}`.slice(0, MAX_KEY_LENGTH);
  while (taken.has(candidate)) {
    suffix += 1;
    candidate = `${base}_${suffix}`.slice(0, MAX_KEY_LENGTH);
  }
  return candidate;
}

function rowsFromComponents(components: ScoreComponent[]): Row[] {
  return components.map((component) => ({
    key: nextRowKey++,
    code: component.key,
    label: component.label,
    max: String(component.max),
    weight: String(component.weight),
  }));
}

function groupsFromServer(groups: ScoreSetGroup[]): Group[] {
  return groups.map((group) => ({
    uiKey: nextGroupKey++,
    groupKey: group.key,
    title: group.title,
    rows: rowsFromComponents(group.components),
  }));
}

interface GroupErrors {
  group?: string;
  rows: Record<number, Record<string, string>>;
}

function parseScoreSetErrors(error: unknown): {
  byGroup: Record<number, GroupErrors>;
  general: string | null;
} {
  if (!(error instanceof ApiError) || !error.fields) {
    return {
      byGroup: {},
      general: error instanceof ApiError ? error.message : "Không lưu được cơ cấu điểm.",
    };
  }
  const byGroup: Record<number, GroupErrors> = {};
  const generalParts: string[] = [];
  for (const [key, message] of Object.entries(error.fields)) {
    const componentMatch = /^(\d+)\.components\.(\d+)\.(\w+)$/.exec(key);
    const groupMatch = /^(\d+)\.(key|title)$/.exec(key);
    if (componentMatch) {
      const gi = Number(componentMatch[1]);
      const ci = Number(componentMatch[2]);
      const field = componentMatch[3]!;
      const own = byGroup[gi] ?? { rows: {} };
      own.rows[ci] = { ...own.rows[ci], [field]: message };
      byGroup[gi] = own;
    } else if (groupMatch) {
      const gi = Number(groupMatch[1]);
      const own = byGroup[gi] ?? { rows: {} };
      own.group = message;
      byGroup[gi] = own;
    } else {
      generalParts.push(message);
    }
  }
  return { byGroup, general: generalParts.length > 0 ? generalParts.join(" · ") : null };
}

function validateGroups(groups: Group[]): Record<number, GroupErrors> {
  const byGroup: Record<number, GroupErrors> = {};
  groups.forEach((group, gi) => {
    const own: GroupErrors = { rows: {} };
    if (group.title.trim() === "") own.group = "Bắt buộc nhập tiêu đề bộ điểm";
    const seenCodes = new Set<string>();
    group.rows.forEach((row, ri) => {
      const rowErrors: Record<string, string> = {};
      const code = row.code.trim();
      if (code === "") rowErrors.key = "Bắt buộc nhập mã";
      else if (!KEY_PATTERN.test(code))
        rowErrors.key = "Mã chỉ gồm chữ thường, số và _ (tối đa 30)";
      else if (seenCodes.has(code)) rowErrors.key = "Mã bị trùng";
      seenCodes.add(code);
      if (row.label.trim() === "") rowErrors.label = "Bắt buộc nhập tên";
      const max = Number(row.max);
      if (row.max.trim() === "" || !Number.isFinite(max) || max <= 0) {
        rowErrors.max = "Điểm tối đa phải lớn hơn 0";
      }
      const weight = Number(row.weight);
      if (row.weight.trim() === "" || !Number.isFinite(weight) || weight < 0) {
        rowErrors.weight = "Trọng số không được âm";
      }
      if (Object.keys(rowErrors).length > 0) own.rows[ri] = rowErrors;
    });
    if (own.group || Object.keys(own.rows).length > 0) byGroup[gi] = own;
  });
  return byGroup;
}

function toInput(group: Group): ScoreSetGroupInput {
  return {
    key: group.groupKey,
    title: group.title.trim(),
    components: group.rows.map((row) => ({
      key: row.code.trim(),
      label: row.label.trim(),
      max: Number(row.max),
      weight: Number(row.weight),
    })),
  };
}

interface ScoreSetsEditorProps {
  versionId: string;
  templateId: string;
  groups: ScoreSetGroup[];
}

/**
 * Editable score structure of a draft version: any number of named groups
 * (e.g. "Giữa kỳ", "Cuối kỳ"), each with its own weighted components. Saved
 * wholesale as an array of groups; a group's key is generated once from its
 * initial title and never regenerated, so it survives later title edits.
 */
export function ScoreSetsEditor({
  versionId,
  templateId,
  groups: initialGroups,
}: ScoreSetsEditorProps) {
  const [groups, setGroups] = useState<Group[]>(() => groupsFromServer(initialGroups));
  const [byGroup, setByGroup] = useState<Record<number, GroupErrors>>({});
  const [general, setGeneral] = useState<string | null>(null);
  const save = useSetScoreSet(versionId, templateId);

  function updateGroupTitle(gi: number, title: string) {
    setGroups((current) => current.map((group, i) => (i === gi ? { ...group, title } : group)));
  }

  function addGroup() {
    setGroups((current) => {
      const taken = new Set(current.map((group) => group.groupKey));
      const title = "Bộ điểm mới";
      return [
        ...current,
        { uiKey: nextGroupKey++, groupKey: uniqueKey(slugify(title), taken), title, rows: [] },
      ];
    });
  }

  function removeGroup(gi: number) {
    setGroups((current) => current.filter((_, i) => i !== gi));
    setByGroup({});
    setGeneral(null);
  }

  function addRow(gi: number) {
    setGroups((current) =>
      current.map((group, i) =>
        i === gi
          ? {
              ...group,
              rows: [
                ...group.rows,
                { key: nextRowKey++, code: "", label: "", max: "10", weight: "0" },
              ],
            }
          : group,
      ),
    );
  }

  function updateRow(gi: number, ri: number, patch: Partial<Row>) {
    setGroups((current) =>
      current.map((group, i) =>
        i === gi
          ? { ...group, rows: group.rows.map((row, j) => (j === ri ? { ...row, ...patch } : row)) }
          : group,
      ),
    );
  }

  function removeRow(gi: number, ri: number) {
    setGroups((current) =>
      current.map((group, i) =>
        i === gi ? { ...group, rows: group.rows.filter((_, j) => j !== ri) } : group,
      ),
    );
  }

  function submit() {
    const clientErrors = validateGroups(groups);
    if (Object.keys(clientErrors).length > 0) {
      setByGroup(clientErrors);
      setGeneral(null);
      return;
    }
    setByGroup({});
    setGeneral(null);
    save.mutate(groups.map(toInput), {
      onSuccess: () => hvToast("Đã lưu cơ cấu điểm"),
      onError: (error) => {
        const parsed = parseScoreSetErrors(error);
        setByGroup(parsed.byGroup);
        setGeneral(parsed.general);
      },
    });
  }

  return (
    <section aria-label="Cơ cấu điểm" className="flex flex-col gap-3">
      <p className="text-[13px] text-ink-500">
        Hai lớp dùng hai phiên bản khác nhau có thể có cấu trúc điểm khác nhau — báo cáo join qua
        phiên bản, không qua khóa.
      </p>
      {groups.length === 0 ? (
        <p className="text-[14px] text-ink-400">Chưa có bộ điểm nào.</p>
      ) : null}
      {groups.map((group, gi) => {
        const groupErrors = byGroup[gi];
        const sum = group.rows.reduce((total, row) => total + (Number(row.weight) || 0), 0);
        const roundedSum = Math.round(sum * 100) / 100;
        return (
          <HvCard key={group.uiKey} variant="flat" className="flex flex-col gap-3">
            <div className="flex flex-wrap items-start justify-between gap-2">
              <div className="flex min-w-[200px] flex-1 flex-col gap-1">
                <Input
                  aria-label={`Tiêu đề bộ điểm ${gi + 1}`}
                  value={group.title}
                  aria-invalid={Boolean(groupErrors?.group)}
                  onChange={(event) => updateGroupTitle(gi, event.target.value)}
                  className="max-w-[280px] font-display text-[16px] font-extrabold"
                />
                <FieldError
                  errors={[groupErrors?.group ? { message: groupErrors.group } : undefined]}
                />
              </div>
              <HvButton type="button" size="sm" variant="ghost" onClick={() => removeGroup(gi)}>
                Xoá bộ
              </HvButton>
            </div>
            <p
              className={cn(
                "text-[13px] font-bold",
                roundedSum === 100 ? "text-ink-700" : "text-coral-600",
              )}
            >
              Tổng trọng số: {roundedSum}%
            </p>
            <p className="text-[12px] text-ink-500">
              Mã là tên máy dùng trong bảng điểm của lớp; trọng số 0 nghĩa là không tính vào tổng.
            </p>
            {group.rows.length === 0 ? (
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
                      <th className={cn(headCellClassName, "w-[120px]")}>Trọng số %</th>
                      <th className={headCellClassName}>
                        <span className="sr-only">Thao tác</span>
                      </th>
                    </tr>
                  </thead>
                  <tbody>
                    {group.rows.map((row, ri) => {
                      const n = ri + 1;
                      const own = groupErrors?.rows[ri] ?? {};
                      return (
                        <tr key={row.key} className="border-t border-line-100 align-top">
                          <td className="px-2 py-2 font-mono text-[13px] text-ink-500">{n}</td>
                          <td className="px-2 py-2">
                            <Input
                              aria-label={`Mã thành phần ${gi + 1}.${n}`}
                              value={row.code}
                              aria-invalid={Boolean(own.key)}
                              onChange={(event) => updateRow(gi, ri, { code: event.target.value })}
                            />
                            <FieldError errors={[own.key ? { message: own.key } : undefined]} />
                          </td>
                          <td className="px-2 py-2">
                            <Input
                              aria-label={`Tên thành phần ${gi + 1}.${n}`}
                              value={row.label}
                              aria-invalid={Boolean(own.label)}
                              onChange={(event) => updateRow(gi, ri, { label: event.target.value })}
                            />
                            <FieldError errors={[own.label ? { message: own.label } : undefined]} />
                          </td>
                          <td className="px-2 py-2">
                            <Input
                              type="number"
                              min={0}
                              step="any"
                              aria-label={`Điểm tối đa ${gi + 1}.${n}`}
                              value={row.max}
                              aria-invalid={Boolean(own.max)}
                              onChange={(event) => updateRow(gi, ri, { max: event.target.value })}
                            />
                            <FieldError errors={[own.max ? { message: own.max } : undefined]} />
                          </td>
                          <td className="px-2 py-2">
                            <Input
                              type="number"
                              min={0}
                              step="any"
                              aria-label={`Trọng số ${gi + 1}.${n}`}
                              value={row.weight}
                              aria-invalid={Boolean(own.weight)}
                              onChange={(event) =>
                                updateRow(gi, ri, { weight: event.target.value })
                              }
                            />
                            <FieldError
                              errors={[own.weight ? { message: own.weight } : undefined]}
                            />
                          </td>
                          <td className="whitespace-nowrap px-2 py-2 text-right">
                            <HvButton
                              type="button"
                              size="sm"
                              variant="ghost"
                              aria-label={`Xoá thành phần ${gi + 1}.${n}`}
                              onClick={() => removeRow(gi, ri)}
                            >
                              Xoá
                            </HvButton>
                          </td>
                        </tr>
                      );
                    })}
                  </tbody>
                </table>
              </div>
            )}
            <div>
              <HvButton type="button" size="sm" variant="secondary" onClick={() => addRow(gi)}>
                + Thêm điểm thành phần
              </HvButton>
            </div>
          </HvCard>
        );
      })}
      <div className="flex flex-wrap items-center justify-between gap-2">
        <HvButton type="button" size="sm" variant="secondary" onClick={addGroup}>
          + Thêm bộ điểm
        </HvButton>
        <div className="flex items-center gap-2">
          <FieldError errors={[general ? { message: general } : undefined]} />
          <HvButton type="button" size="sm" disabled={save.isPending} onClick={submit}>
            {save.isPending ? "Đang lưu…" : "Lưu cơ cấu điểm"}
          </HvButton>
        </div>
      </div>
    </section>
  );
}

/** The same score groups on a published or archived version, or for a reader without `library.edit`. */
export function ScoreSetsReadOnly({ groups }: { groups: ScoreSetGroup[] }) {
  return (
    <section aria-label="Cơ cấu điểm" className="flex flex-col gap-3">
      {groups.length === 0 ? (
        <p className="text-[14px] text-ink-400">Chưa có bộ điểm nào.</p>
      ) : (
        groups.map((group) => (
          <HvCard key={group.key} variant="flat" className="flex flex-col gap-2">
            <h3 className="font-display text-[15px] font-extrabold text-ink-900">{group.title}</h3>
            {group.components.length === 0 ? (
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
                  {group.components.map((component) => (
                    <tr key={component.key} className="border-t border-line-100">
                      <td className="px-2 py-2 font-mono text-[13px] text-ink-700">
                        {component.key}
                      </td>
                      <td className="px-2 py-2 font-bold text-ink-900">{component.label}</td>
                      <td className="px-2 py-2 text-right text-ink-700">{component.max}</td>
                      <td className="px-2 py-2 text-right text-ink-700">{component.weight}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </HvCard>
        ))
      )}
    </section>
  );
}
