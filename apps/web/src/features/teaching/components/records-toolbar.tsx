import { useId, type RefObject } from "react";
import { Search, X } from "lucide-react";

import type { Class } from "@/features/roster";
import { cn } from "@/lib/utils";

import { RecordsClassSelect } from "./records-class-select";

interface RecordsToolbarProps {
  classes: Class[];
  selectedClassId: string;
  onSelectClass: (classId: string) => void;
  query: string;
  onQueryChange: (query: string) => void;
  /** Rows left after the student filter. */
  matched: number;
  /** Rows in the class, or null while the roster is still loading (the counter stays silent). */
  total: number | null;
  /** The search input, so the page can focus it on the `/` hotkey. */
  searchRef: RefObject<HTMLInputElement | null>;
  /** Below `md`: stacked fields, and the counter moves out of the toolbar (the page renders it under the table). */
  compact: boolean;
}

const labelClassName = "text-[11.5px] font-extrabold uppercase tracking-[0.35px] text-ink-400";

function Field({ className, children }: { className?: string; children: React.ReactNode }) {
  return <div className={cn("flex flex-col gap-[5px]", className)}>{children}</div>;
}

/**
 * "12 / 28 học sinh" while a search is active, "28 học sinh" otherwise. The
 * live region reads every change politely; before the roster loads it is
 * empty so screen readers never hear a stray "0 học sinh".
 */
export function ResultCount({
  matched,
  total,
  query,
}: {
  matched: number;
  total: number | null;
  query: string;
}) {
  return (
    <span
      role="status"
      aria-live="polite"
      className="whitespace-nowrap text-[13px] font-extrabold text-ink-500"
    >
      {total === null ? null : query ? (
        <>
          <b className="text-ink-900">{matched}</b> / {total} học sinh
        </>
      ) : (
        <>
          <b className="text-ink-900">{total}</b> học sinh
        </>
      )}
    </span>
  );
}

function SearchBox({
  id,
  query,
  onQueryChange,
  searchRef,
}: {
  id: string;
  query: string;
  onQueryChange: (query: string) => void;
  searchRef: RefObject<HTMLInputElement | null>;
}) {
  return (
    <div className="relative flex min-h-11 items-center gap-2 rounded-[14px] border-2 border-line-200 bg-white pl-3 pr-2 transition-colors focus-within:border-mint-400">
      <Search className="size-4 shrink-0 text-ink-400" aria-hidden="true" />
      <input
        ref={searchRef}
        id={id}
        type="search"
        aria-label="Tìm học sinh"
        placeholder="Gõ tên học sinh…"
        autoComplete="off"
        enterKeyHint="search"
        value={query}
        onChange={(event) => onQueryChange(event.target.value)}
        className="min-w-0 flex-1 bg-transparent text-[14.5px] font-bold text-ink-900 outline-none placeholder:font-semibold placeholder:text-ink-400 focus:shadow-none [&::-webkit-search-cancel-button]:appearance-none"
      />
      {query === "" ? (
        <kbd
          aria-hidden="true"
          className="rounded-[6px] border-[1.5px] border-line-200 bg-cream-50 px-1.5 py-1 font-mono text-[11px] leading-none font-bold text-ink-400"
        >
          /
        </kbd>
      ) : (
        <button
          type="button"
          aria-label="Xoá tìm kiếm"
          onClick={() => {
            onQueryChange("");
            searchRef.current?.focus();
          }}
          className="inline-flex h-8 w-8 items-center justify-center rounded-[10px] bg-cream-200 text-ink-500 hover:bg-cream-300 hover:text-ink-900 focus-visible:ring-4 focus-visible:outline-none"
        >
          <X className="size-3.5" aria-hidden="true" />
        </button>
      )}
    </div>
  );
}

/**
 * The white filter bar over the records table: class picker, live student
 * search and, from `md` up, the match counter. Filtering happens on every
 * keystroke; the page owns the query (it lives in `?q=`) and the class.
 */
export function RecordsToolbar({
  classes,
  selectedClassId,
  onSelectClass,
  query,
  onQueryChange,
  matched,
  total,
  searchRef,
  compact,
}: RecordsToolbarProps) {
  const id = useId();
  const classLabelId = `${id}-class-label`;
  const searchId = `${id}-search`;
  return (
    <div
      className={cn(
        "flex flex-wrap items-end gap-3 rounded-[20px] bg-white shadow-soft-sm",
        compact ? "p-3" : "px-4 py-3.5",
      )}
    >
      <Field className={compact ? "w-full" : undefined}>
        <span id={classLabelId} className={labelClassName}>
          Lớp
        </span>
        <RecordsClassSelect
          classes={classes}
          selectedId={selectedClassId}
          onSelect={onSelectClass}
          labelId={classLabelId}
        />
      </Field>
      <Field className="flex-1 basis-[260px]">
        <label htmlFor={searchId} className={labelClassName}>
          Tìm học sinh
        </label>
        <SearchBox
          id={searchId}
          query={query}
          onQueryChange={onQueryChange}
          searchRef={searchRef}
        />
      </Field>
      {compact ? null : (
        <Field>
          <span aria-hidden="true" className={labelClassName}>
            &nbsp;
          </span>
          <div className="flex min-h-11 items-center">
            <ResultCount matched={matched} total={total} query={query} />
          </div>
        </Field>
      )}
    </div>
  );
}
