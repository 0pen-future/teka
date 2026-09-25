import {
  ArrowDownIcon,
  ArrowUpIcon,
  ChevronDownIcon,
  ExternalLinkIcon,
  PlusIcon,
} from "lucide-react";
import { useState } from "react";

import { HvBadge, HvButton, hvToast } from "@/components/hv";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { ApiError } from "@/lib/api/errors";
import { cn } from "@/lib/utils";

import { useSearch } from "../hooks/use-search";
import { useMaterialsList, useSetLessonMaterials } from "../hooks/use-library";
import { materialFormatLabel, materialKindIcon, materialKindLabel } from "../lib/library-labels";
import {
  materialKindSchema,
  type LessonMaterial,
  type LessonMaterialInput,
  type Material,
  type MaterialKind,
  type TemplateLessonDetail,
} from "../schemas/library-schemas";
import { BankPickerModal } from "./bank-picker-modal";
import { MaterialDialog } from "./material-dialog";

interface LessonContentsProps {
  lesson: TemplateLessonDetail;
  templateId: string;
  /** True only on a draft the viewer may author; otherwise the list renders read-only. */
  editable: boolean;
}

/** New content is offered for every kind except the legacy-only `other`. */
const CREATABLE_KINDS = materialKindSchema.options.filter(
  (kind): kind is Exclude<MaterialKind, "other"> => kind !== "other",
);

function apiMessage(error: unknown, fallback: string): string {
  return error instanceof ApiError ? error.message : fallback;
}

function toInput(materials: LessonMaterial[]): LessonMaterialInput[] {
  return materials.map((m) => ({
    material_id: m.id,
    shared_with_students: m.shared_with_students,
  }));
}

/**
 * "Nội dung buổi học": the materials attached to one template lesson, kept
 * as a full display order and rewritten wholesale on every change (share
 * toggle, reorder, add, remove) — the API has no partial-update endpoint.
 */
export function LessonContents({ lesson, templateId, editable }: LessonContentsProps) {
  const save = useSetLessonMaterials(lesson.id, lesson.version_id, templateId);
  const [sorting, setSorting] = useState(false);
  const [order, setOrder] = useState<LessonMaterial[]>(lesson.materials);
  const [expanded, setExpanded] = useState<Set<string>>(new Set());
  const [creatingKind, setCreatingKind] = useState<MaterialKind | null>(null);
  const [editingMaterial, setEditingMaterial] = useState<Material | null>(null);
  const [pickerOpen, setPickerOpen] = useState(false);
  const search = useSearch();

  // While not sorting, keep the local order copy synced with the source of
  // truth so a future sort session always starts from the latest list.
  // React's "adjusting state when a prop changes" pattern: comparing during
  // render and calling setState there avoids the extra commit an effect
  // would cost; the reference check makes this self-terminating.
  if (!sorting && order !== lesson.materials) {
    setOrder(lesson.materials);
  }

  const attachedIds = new Set(lesson.materials.map((m) => m.id));
  const catalog = useMaterialsList(
    { q: search.q || undefined, active: true, per_page: 100, sort: "title" },
    pickerOpen,
  );
  const pickerItems = catalog.data?.items.filter((m) => !attachedIds.has(m.id));

  function persist(items: LessonMaterialInput[], message: string) {
    save.mutate(items, {
      onSuccess: () => hvToast(message),
      onError: (error) =>
        hvToast(apiMessage(error, "Không lưu được nội dung buổi học."), { variant: "danger" }),
    });
  }

  function toggleExpand(id: string) {
    setExpanded((current) => {
      const next = new Set(current);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }

  function toggleShared(id: string) {
    const next = lesson.materials.map((m) =>
      m.id === id ? { ...m, shared_with_students: !m.shared_with_students } : m,
    );
    persist(toInput(next), "Đã cập nhật chia sẻ nội dung");
  }

  function remove(id: string) {
    persist(toInput(lesson.materials.filter((m) => m.id !== id)), "Đã gỡ nội dung khỏi buổi học");
  }

  function move(index: number, direction: -1 | 1) {
    setOrder((current) => {
      const target = index + direction;
      if (target < 0 || target >= current.length) return current;
      const next = [...current];
      const [moved] = next.splice(index, 1);
      if (moved) next.splice(target, 0, moved);
      return next;
    });
  }

  function stopSorting() {
    setSorting(false);
    const changed = order.some((m, index) => m.id !== lesson.materials[index]?.id);
    if (changed) persist(toInput(order), "Đã lưu thứ tự nội dung");
  }

  function addCreated(material: Material) {
    setCreatingKind(null);
    persist(
      [...toInput(lesson.materials), { material_id: material.id, shared_with_students: false }],
      "Đã thêm nội dung vào buổi học",
    );
  }

  function addFromBank(materials: Material[]) {
    setPickerOpen(false);
    persist(
      [
        ...toInput(lesson.materials),
        ...materials.map((m) => ({ material_id: m.id, shared_with_students: false })),
      ],
      "Đã thêm nội dung vào buổi học",
    );
  }

  const rows = sorting ? order : lesson.materials;

  return (
    <section
      aria-label="Nội dung buổi học"
      className="flex flex-col gap-3 rounded-[var(--radius-lg)] border border-line-200 bg-white p-4"
    >
      <div className="flex flex-wrap items-start justify-between gap-2">
        <div>
          <h2 className="font-display text-[17px] font-extrabold text-ink-900">
            Nội dung buổi học
          </h2>
          <p className="text-[13px] text-ink-500">
            Video, âm thanh, hình ảnh, tài liệu, ghi chú, buổi học trực tuyến hoặc liên kết ngoài.
          </p>
        </div>
        {editable ? (
          <div className="flex items-center gap-2">
            {lesson.materials.length > 1 ? (
              <HvButton
                type="button"
                variant={sorting ? "primary" : "secondary"}
                size="sm"
                onClick={() => (sorting ? stopSorting() : setSorting(true))}
              >
                {sorting ? "Xong sắp xếp" : "Sắp xếp thứ tự"}
              </HvButton>
            ) : null}
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <HvButton type="button" size="sm">
                  <PlusIcon aria-hidden="true" className="size-4" />
                  Thêm nội dung
                </HvButton>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end">
                <DropdownMenuLabel>Tạo mới</DropdownMenuLabel>
                {CREATABLE_KINDS.map((kind) => (
                  <DropdownMenuItem key={kind} onSelect={() => setCreatingKind(kind)}>
                    {materialKindLabel[kind]}
                  </DropdownMenuItem>
                ))}
                <DropdownMenuSeparator />
                <DropdownMenuItem onSelect={() => setPickerOpen(true)}>
                  Chọn từ ngân hàng nội dung
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          </div>
        ) : null}
      </div>

      {rows.length === 0 ? (
        <p className="text-[14px] text-ink-400">Chưa thêm nội dung cho buổi học mẫu.</p>
      ) : (
        <ul className="flex flex-col">
          {rows.map((material, index) => (
            <MaterialRow
              key={material.id}
              material={material}
              index={index}
              total={rows.length}
              sorting={sorting}
              editable={editable}
              expanded={expanded.has(material.id)}
              onToggleExpand={toggleExpand}
              onToggleShared={toggleShared}
              onMove={move}
              onEdit={setEditingMaterial}
              onRemove={remove}
            />
          ))}
        </ul>
      )}

      {editable ? (
        <MaterialDialog
          open={creatingKind !== null}
          onOpenChange={(open) => {
            if (!open) setCreatingKind(null);
          }}
          mode="create"
          defaultKind={creatingKind ?? undefined}
          onCreated={addCreated}
        />
      ) : null}
      {editable && editingMaterial ? (
        <MaterialDialog
          open
          onOpenChange={(open) => {
            if (!open) setEditingMaterial(null);
          }}
          mode="edit"
          material={editingMaterial}
          onSaved={() => setEditingMaterial(null)}
        />
      ) : null}
      {editable ? (
        <BankPickerModal
          open={pickerOpen}
          onOpenChange={setPickerOpen}
          title="Chọn từ ngân hàng nội dung"
          noun="nội dung"
          searchLabel="Tìm nội dung"
          search={search}
          items={pickerItems}
          isPending={catalog.isPending}
          isError={catalog.isError}
          getLabel={(material) => material.title}
          renderMeta={(material) => (
            <HvBadge variant="neutral" size="sm">
              {materialKindLabel[material.kind]}
            </HvBadge>
          )}
          onConfirm={addFromBank}
          confirmPending={save.isPending}
        />
      ) : null}
    </section>
  );
}

interface MaterialRowProps {
  material: LessonMaterial;
  index: number;
  total: number;
  sorting: boolean;
  editable: boolean;
  expanded: boolean;
  onToggleExpand: (id: string) => void;
  onToggleShared: (id: string) => void;
  onMove: (index: number, direction: -1 | 1) => void;
  onEdit: (material: Material) => void;
  onRemove: (id: string) => void;
}

function MaterialRow({
  material,
  index,
  total,
  sorting,
  editable,
  expanded,
  onToggleExpand,
  onToggleShared,
  onMove,
  onEdit,
  onRemove,
}: MaterialRowProps) {
  const Icon = materialKindIcon[material.kind];
  return (
    <li className="flex flex-col gap-2 border-t border-line-100 py-2 first:border-t-0">
      <div className="flex flex-wrap items-center gap-2">
        <Icon aria-hidden="true" className="size-4 shrink-0 text-ink-500" />
        <span className="min-w-0 flex-1 truncate text-[14px] font-bold text-ink-900">
          {material.title}
        </span>
        {!material.active ? (
          <HvBadge variant="neutral" size="sm">
            Ngừng
          </HvBadge>
        ) : null}
        <span className="text-[12px] text-ink-500">{materialFormatLabel(material.url)}</span>
        {editable ? (
          <label className="flex items-center gap-1.5 text-[12.5px] font-bold text-ink-700">
            <input
              type="checkbox"
              className="size-4 accent-mint-600"
              aria-label={`Chia sẻ ${material.title} với học viên`}
              checked={material.shared_with_students}
              onChange={() => onToggleShared(material.id)}
            />
            Chia sẻ cho học viên
          </label>
        ) : material.shared_with_students ? (
          <HvBadge variant="success" size="sm">
            Chia sẻ HV
          </HvBadge>
        ) : null}
        <button
          type="button"
          aria-label={expanded ? `Thu gọn ${material.title}` : `Xem thêm ${material.title}`}
          aria-expanded={expanded}
          onClick={() => onToggleExpand(material.id)}
          className="inline-flex size-7 items-center justify-center rounded-full text-ink-400 hover:bg-cream-100 hover:text-ink-700"
        >
          <ChevronDownIcon
            aria-hidden="true"
            className={cn("size-4 transition-transform", expanded && "rotate-180")}
          />
        </button>
        {editable && sorting ? (
          <div className="flex items-center gap-1">
            <button
              type="button"
              aria-label={`Đưa ${material.title} lên`}
              disabled={index === 0}
              onClick={() => onMove(index, -1)}
              className="inline-flex size-7 items-center justify-center rounded-full text-ink-500 hover:bg-cream-100 disabled:cursor-not-allowed disabled:opacity-40"
            >
              <ArrowUpIcon aria-hidden="true" className="size-4" />
            </button>
            <button
              type="button"
              aria-label={`Đưa ${material.title} xuống`}
              disabled={index === total - 1}
              onClick={() => onMove(index, 1)}
              className="inline-flex size-7 items-center justify-center rounded-full text-ink-500 hover:bg-cream-100 disabled:cursor-not-allowed disabled:opacity-40"
            >
              <ArrowDownIcon aria-hidden="true" className="size-4" />
            </button>
          </div>
        ) : editable ? (
          <div className="flex items-center gap-1">
            <HvButton type="button" variant="ghost" size="sm" onClick={() => onEdit(material)}>
              Sửa
            </HvButton>
            <HvButton type="button" variant="ghost" size="sm" onClick={() => onRemove(material.id)}>
              Gỡ khỏi buổi
            </HvButton>
          </div>
        ) : null}
      </div>
      {expanded ? (
        <div className="ml-6 flex flex-col gap-1.5 text-[13px] text-ink-700">
          {material.description ? (
            <p className="whitespace-pre-wrap">{material.description}</p>
          ) : null}
          {material.url ? (
            <a
              href={material.url}
              target="_blank"
              rel="noreferrer"
              className="inline-flex w-fit items-center gap-1 font-bold text-mint-600 hover:underline"
            >
              <ExternalLinkIcon aria-hidden="true" className="size-3.5" />
              Mở liên kết
            </a>
          ) : null}
        </div>
      ) : null}
    </li>
  );
}
