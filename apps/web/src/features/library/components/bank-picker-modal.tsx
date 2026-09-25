import { SearchIcon } from "lucide-react";
import { type ReactNode, useState } from "react";

import { HvButton, HvModal, HvStateBlock } from "@/components/hv";
import { Input } from "@/components/ui/input";

import type { SearchState } from "../hooks/use-search";

export interface BankPickerModalProps<T extends { id: string }> {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  /** Vietnamese noun for the picked kind ("nội dung", "bài tập"), used in status and confirm copy. */
  noun: string;
  searchLabel: string;
  search: SearchState;
  items: T[] | undefined;
  isPending: boolean;
  isError: boolean;
  getLabel: (item: T) => string;
  renderMeta?: (item: T) => ReactNode;
  onConfirm: (items: T[]) => void;
  confirmPending?: boolean;
}

/**
 * Generic multi-select picker over one active-only catalog list, reused by
 * the lesson's content and exercise sections to "add from the bank". Its own
 * selection resets every time the modal reopens; the caller owns the actual
 * attach call.
 */
export function BankPickerModal<T extends { id: string }>({
  open,
  onOpenChange,
  title,
  noun,
  searchLabel,
  search,
  items,
  isPending,
  isError,
  getLabel,
  renderMeta,
  onConfirm,
  confirmPending = false,
}: BankPickerModalProps<T>) {
  const [selected, setSelected] = useState<Set<string>>(new Set());
  // Reset the selection when the modal transitions from closed to open,
  // following React's "adjusting state when a prop changes" pattern:
  // comparing during render and calling setState there avoids the extra
  // commit an effect would cost.
  const [prevOpen, setPrevOpen] = useState(open);
  if (open !== prevOpen) {
    setPrevOpen(open);
    if (open) setSelected(new Set());
  }

  function toggle(id: string) {
    setSelected((current) => {
      const next = new Set(current);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }

  function confirm() {
    if (!items || selected.size === 0) return;
    onConfirm(items.filter((item) => selected.has(item.id)));
  }

  return (
    <HvModal
      open={open}
      onOpenChange={onOpenChange}
      title={title}
      size="lg"
      stickyFooter
      footer={
        <>
          <HvButton type="button" variant="ghost" onClick={() => onOpenChange(false)}>
            Hủy
          </HvButton>
          <HvButton
            type="button"
            disabled={selected.size === 0 || confirmPending}
            onClick={confirm}
          >
            {confirmPending ? "Đang thêm…" : `Thêm ${selected.size} ${noun}`}
          </HvButton>
        </>
      }
    >
      <div className="flex flex-col gap-3">
        <div className="relative">
          <SearchIcon
            aria-hidden="true"
            className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-ink-400"
          />
          <Input
            type="search"
            aria-label={searchLabel}
            placeholder="Tìm theo tên…"
            value={search.query}
            onChange={(event) => search.setQuery(event.target.value)}
            className="pl-9"
          />
        </div>
        {isPending ? (
          <HvStateBlock state="loading" title={`Đang tải ${noun}`} compact />
        ) : isError ? (
          <HvStateBlock state="error" title={`Không tải được ${noun}`} compact />
        ) : !items || items.length === 0 ? (
          <HvStateBlock
            state="empty"
            title={search.q ? `Không có ${noun} nào khớp từ khoá.` : `Kho chưa có ${noun} nào.`}
            compact
          />
        ) : (
          <ul className="flex max-h-[50vh] flex-col overflow-y-auto">
            {items.map((item) => (
              <li
                key={item.id}
                className="flex items-center gap-3 border-t border-line-100 py-2 first:border-t-0"
              >
                <label className="flex min-w-0 flex-1 items-center gap-3 text-[14px] text-ink-900">
                  <input
                    type="checkbox"
                    className="size-4 accent-mint-600"
                    aria-label={getLabel(item)}
                    checked={selected.has(item.id)}
                    onChange={() => toggle(item.id)}
                  />
                  <span className="min-w-0 flex-1 truncate font-bold">{getLabel(item)}</span>
                  {renderMeta ? renderMeta(item) : null}
                </label>
              </li>
            ))}
          </ul>
        )}
      </div>
    </HvModal>
  );
}
