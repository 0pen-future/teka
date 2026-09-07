import * as React from "react";
import { Popover as PopoverPrimitive } from "radix-ui";
import { Check, ChevronDown, Search } from "lucide-react";

import { HvModal } from "@/components/hv";
import { ClassSearchEmptyNote, useClassSearch, type Class } from "@/features/roster";
import { useMediaQuery } from "@/lib/hooks/use-media-query";
import { cn } from "@/lib/utils";

interface RecordsClassSelectProps {
  classes: Class[];
  selectedId: string;
  onSelect: (classId: string) => void;
  /** id of the visible "LỚP" label; joined with the trigger text for the accessible name. */
  labelId: string;
  id?: string;
}

const triggerClassName = cn(
  "inline-flex min-h-11 min-w-[230px] items-center justify-between gap-2.5 rounded-[14px]",
  "border-2 border-line-200 bg-white pl-3.5 pr-2.5 text-[14.5px] font-extrabold text-ink-900",
  "hover:border-mint-300 data-[state=open]:border-mint-400",
  "focus-visible:ring-4 focus-visible:outline-none max-sm:w-full",
);

const optionClassName = cn(
  "flex min-h-[42px] w-full items-center gap-2.5 rounded-[11px] px-3 text-left text-[14px] font-bold text-ink-700",
  "hover:bg-cream-100 focus-visible:bg-cream-100 focus-visible:outline-none",
  "aria-selected:bg-mint-50 aria-selected:text-mint-700",
);

const OPTION_SELECTOR = '[role="option"]';

// Space is left to the option button so it activates natively at any list size.
function isPrintableKey(event: React.KeyboardEvent): boolean {
  return (
    event.key.length === 1 && event.key !== " " && !event.ctrlKey && !event.metaKey && !event.altKey
  );
}

interface ClassOptionListProps {
  classes: Class[];
  selectedId: string;
  listboxId: string;
  onPick: (classId: string) => void;
  ref: React.Ref<HTMLDivElement>;
}

/**
 * The list body shared by the popover and the bottom sheet: optional filter
 * (only once the teacher has more than 5 classes, same threshold as the
 * roster screens), the "Lớp đang dạy" group label, and a roving-focus
 * listbox. Options are real buttons so Enter/Space activate natively; the
 * listbox only adds arrow/Home/End navigation and hands stray typing to the
 * filter input.
 */
function ClassOptionList({ classes, selectedId, listboxId, onPick, ref }: ClassOptionListProps) {
  const { query, setQuery, filtered, showSearch, emptyNote } = useClassSearch(classes);
  const listRef = React.useRef<HTMLDivElement>(null);
  const searchRef = React.useRef<HTMLInputElement>(null);
  const selectedVisible = filtered.some((klass) => klass.id === selectedId);

  const focusOption = (index: number) => {
    const options = listRef.current?.querySelectorAll<HTMLButtonElement>(OPTION_SELECTOR);
    if (!options || options.length === 0) return;
    const wrapped = ((index % options.length) + options.length) % options.length;
    options[wrapped]?.focus();
  };

  const onListKeyDown = (event: React.KeyboardEvent<HTMLDivElement>) => {
    const options = Array.from(
      listRef.current?.querySelectorAll<HTMLButtonElement>(OPTION_SELECTOR) ?? [],
    );
    const current = options.indexOf(event.target as HTMLButtonElement);
    switch (event.key) {
      case "ArrowDown":
        event.preventDefault();
        focusOption(current + 1);
        return;
      case "ArrowUp":
        event.preventDefault();
        focusOption(current - 1);
        return;
      case "Home":
        event.preventDefault();
        focusOption(0);
        return;
      case "End":
        event.preventDefault();
        focusOption(-1);
        return;
      default:
        if (showSearch && isPrintableKey(event)) {
          event.preventDefault();
          setQuery(query + event.key);
          searchRef.current?.focus();
        }
    }
  };

  const onSearchKeyDown = (event: React.KeyboardEvent<HTMLInputElement>) => {
    if (event.key === "ArrowDown") {
      event.preventDefault();
      focusOption(0);
    }
  };

  return (
    <div ref={ref}>
      {showSearch ? (
        <div className="mx-0.5 mb-1.5 mt-0.5 flex min-h-[38px] items-center gap-2 rounded-[12px] border-2 border-line-200 bg-cream-50 px-2.5">
          <Search className="size-3.5 shrink-0 text-ink-400" aria-hidden="true" />
          <input
            ref={searchRef}
            type="search"
            aria-label="Tìm lớp"
            placeholder="Tìm lớp…"
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            onKeyDown={onSearchKeyDown}
            className="w-full bg-transparent text-[13.5px] font-bold text-ink-700 outline-none placeholder:text-ink-400 focus:shadow-none [&::-webkit-search-cancel-button]:appearance-none"
          />
        </div>
      ) : null}
      <div className="px-3 pt-1.5 pb-0.5 text-[11px] font-extrabold uppercase tracking-[0.4px] text-ink-400">
        Lớp đang dạy
      </div>
      <div
        ref={listRef}
        role="listbox"
        aria-label="Chọn lớp"
        id={listboxId}
        tabIndex={-1}
        onKeyDown={onListKeyDown}
      >
        {filtered.map((klass, index) => {
          const selected = klass.id === selectedId;
          const tabbable = selected || (!selectedVisible && index === 0);
          return (
            <button
              key={klass.id}
              type="button"
              role="option"
              aria-selected={selected}
              tabIndex={tabbable ? 0 : -1}
              onClick={() => onPick(klass.id)}
              className={optionClassName}
            >
              <Check
                className={cn("size-4 shrink-0 text-mint-600", !selected && "invisible")}
                aria-hidden="true"
              />
              <span className="flex-1">{klass.name}</span>
              <span className="text-[12px] font-bold text-ink-400">{klass.student_count} HS</span>
            </button>
          );
        })}
      </div>
      {emptyNote ? (
        <div className="px-3 py-2.5">
          <ClassSearchEmptyNote note={emptyNote} />
        </div>
      ) : null}
    </div>
  );
}

/**
 * The records toolbar's class picker. From `sm` up it is a Popover anchored
 * under the trigger; below `sm` the same list opens in an `HvModal` bottom
 * sheet titled "Chọn lớp". Both branches hand initial focus to the filter
 * input when present, otherwise to the selected option, and return focus to
 * the trigger on close. Selection only fires `onSelect` when it changes.
 */
export function RecordsClassSelect({
  classes,
  selectedId,
  onSelect,
  labelId,
  id,
}: RecordsClassSelectProps) {
  const [open, setOpen] = React.useState(false);
  const isSmUp = useMediaQuery("(min-width: 640px)");
  const generatedId = React.useId();
  const listboxId = `${generatedId}-listbox`;
  const triggerTextId = `${generatedId}-text`;
  const bodyRef = React.useRef<HTMLDivElement>(null);
  const triggerRef = React.useRef<HTMLButtonElement>(null);
  const selected = classes.find((klass) => klass.id === selectedId);

  const focusInitial = (event: Event) => {
    event.preventDefault();
    const body = bodyRef.current;
    const target =
      body?.querySelector<HTMLElement>('input[type="search"]') ??
      body?.querySelector<HTMLElement>(`${OPTION_SELECTOR}[aria-selected="true"]`) ??
      body?.querySelector<HTMLElement>(OPTION_SELECTOR);
    target?.focus();
  };

  // HvModal has no Dialog.Trigger, so radix would drop focus on close;
  // send it back to our trigger instead (the Popover branch does this itself).
  const restoreTriggerFocus = (event: Event) => {
    event.preventDefault();
    triggerRef.current?.focus();
  };

  // Re-picking the current class still reports it so the page can pin the
  // default class into the URL, exactly like the former pills did.
  const pick = (classId: string) => {
    setOpen(false);
    onSelect(classId);
  };

  const triggerContent = (
    <>
      <span id={triggerTextId} className="min-w-0 truncate">
        {selected?.name ?? "Chọn lớp"}
        {selected ? (
          <>
            {" "}
            <span className="ml-0.5 text-[12.5px] font-bold text-ink-400">
              · {selected.student_count} HS
            </span>
          </>
        ) : null}
      </span>
      <ChevronDown
        className={cn(
          "size-4 shrink-0 text-ink-400 transition-transform motion-reduce:transition-none",
          open && "rotate-180",
        )}
        aria-hidden="true"
      />
    </>
  );

  const list = (
    <ClassOptionList
      ref={bodyRef}
      classes={classes}
      selectedId={selectedId}
      listboxId={listboxId}
      onPick={pick}
    />
  );

  const triggerA11y = {
    id,
    type: "button" as const,
    "aria-haspopup": "listbox" as const,
    "aria-expanded": open,
    "aria-controls": open ? listboxId : undefined,
    "aria-labelledby": `${labelId} ${triggerTextId}`,
    className: triggerClassName,
  };

  if (!isSmUp) {
    return (
      <>
        <button
          {...triggerA11y}
          ref={triggerRef}
          data-state={open ? "open" : "closed"}
          onClick={() => setOpen(true)}
        >
          {triggerContent}
        </button>
        <HvModal
          open={open}
          onOpenChange={setOpen}
          title="Chọn lớp"
          size="md"
          onOpenAutoFocus={focusInitial}
          onCloseAutoFocus={restoreTriggerFocus}
        >
          {list}
        </HvModal>
      </>
    );
  }

  return (
    <PopoverPrimitive.Root open={open} onOpenChange={setOpen}>
      <PopoverPrimitive.Trigger asChild>
        <button {...triggerA11y}>{triggerContent}</button>
      </PopoverPrimitive.Trigger>
      <PopoverPrimitive.Portal>
        <PopoverPrimitive.Content
          align="start"
          sideOffset={6}
          onOpenAutoFocus={focusInitial}
          className={cn(
            "z-50 w-max min-w-(--radix-popover-trigger-width) max-w-[320px] rounded-[16px]",
            "border-2 border-line-200 bg-white p-1.5 shadow-soft-lg",
            "origin-(--radix-popover-content-transform-origin)",
            "data-open:animate-in data-open:fade-in-0 data-open:zoom-in-95 motion-reduce:animate-none",
          )}
        >
          {list}
        </PopoverPrimitive.Content>
      </PopoverPrimitive.Portal>
    </PopoverPrimitive.Root>
  );
}
