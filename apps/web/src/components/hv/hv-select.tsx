import * as React from "react";
import { Popover as PopoverPrimitive } from "radix-ui";
import { Check, ChevronDown, Search } from "lucide-react";

import { useMediaQuery } from "@/lib/hooks/use-media-query";
import { cn } from "@/lib/utils";

import { HvModal } from "./hv-modal";

export interface HvSelectOption {
  value: string;
  label: string;
  /** Muted text after the label on the trigger (`label · meta`) and right-aligned inside the option. */
  meta?: string;
  disabled?: boolean;
}

export interface HvSelectProps {
  options: HvSelectOption[];
  /** Current value; `""` means nothing is selected and the placeholder shows. */
  value: string;
  /** Called on every pick, including re-picking the current value. */
  onValueChange: (value: string) => void;
  /** Title of the bottom sheet under `sm`; also names the listbox. */
  sheetTitle: string;
  placeholder?: string;
  /** Uppercase group label rendered above the options. */
  groupLabel?: string;
  /** Noun for the filter input and its empty note: "Tìm {noun}…", `Không có {noun} nào khớp "q"`. */
  searchNoun?: string;
  /** The filter input appears once `options.length` exceeds this. `Infinity` never shows it. */
  searchThreshold?: number;
  /** Trigger id, for `<label htmlFor>`. */
  id?: string;
  /** Id of a visible label; the accessible name becomes "{label} {trigger text}". */
  labelId?: string;
  "aria-label"?: string;
  "aria-invalid"?: boolean;
  disabled?: boolean;
  /** Width overrides for the trigger (`w-full`, `w-[180px]`, `min-w-[230px] max-sm:w-full`…). */
  className?: string;
  align?: "start" | "end";
}

const DEFAULT_SEARCH_THRESHOLD = 5;

const triggerClassName = cn(
  "inline-flex min-h-11 items-center justify-between gap-2.5 rounded-[14px]",
  "border-2 border-line-200 bg-white pl-3.5 pr-2.5 text-[14.5px] font-extrabold text-ink-900",
  "hover:border-mint-300 data-[state=open]:border-mint-400",
  "focus-visible:ring-4 focus-visible:outline-none",
  "disabled:cursor-not-allowed disabled:bg-cream-200 disabled:text-ink-300",
  "aria-invalid:border-coral-400 data-placeholder:font-bold data-placeholder:text-ink-400",
);

const optionClassName = cn(
  "flex min-h-[42px] w-full items-center gap-2.5 rounded-[11px] px-3 text-left text-[14px] font-bold text-ink-700",
  "hover:bg-cream-100 focus-visible:bg-cream-100 focus-visible:outline-none",
  "aria-selected:bg-mint-50 aria-selected:text-mint-700",
  "aria-disabled:pointer-events-none aria-disabled:opacity-50",
);

const contentClassName = cn(
  "z-50 w-max min-w-(--radix-popover-trigger-width) max-w-[320px] rounded-[16px]",
  "border-2 border-line-200 bg-white p-1.5 shadow-soft-lg",
  "max-h-(--radix-popover-content-available-height) overflow-y-auto",
  "origin-(--radix-popover-content-transform-origin)",
  "data-open:animate-in data-open:fade-in-0 data-open:zoom-in-95 motion-reduce:animate-none",
);

const ENABLED_OPTION_SELECTOR = '[role="option"]:not([aria-disabled="true"])';

// Space is left to the option button so it activates natively at any list size.
function isPrintableKey(event: React.KeyboardEvent): boolean {
  return (
    event.key.length === 1 && event.key !== " " && !event.ctrlKey && !event.metaKey && !event.altKey
  );
}

/**
 * Filter state for the list: the input only appears once the list outgrows
 * the threshold, filters by case-insensitive substring on the label, and
 * reports an inline note when nothing matches. While hidden the query is
 * ignored, so a stale query never filters invisibly.
 */
function useOptionSearch(options: HvSelectOption[], threshold: number, noun: string) {
  const [query, setQuery] = React.useState("");
  const showSearch = options.length > threshold;
  const filtered = React.useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!showSearch || !q) return options;
    return options.filter((option) => option.label.toLowerCase().includes(q));
  }, [options, query, showSearch]);
  const emptyNote =
    showSearch && query.trim() !== "" && filtered.length === 0
      ? `Không có ${noun} nào khớp "${query}"`
      : null;
  return { query, setQuery, filtered, showSearch, emptyNote };
}

interface OptionListProps {
  options: HvSelectOption[];
  value: string;
  listboxId: string;
  listboxLabel: string;
  groupLabel?: string;
  searchNoun: string;
  searchThreshold: number;
  onPick: (value: string) => void;
  ref: React.Ref<HTMLDivElement>;
}

/**
 * The list body shared by the popover and the bottom sheet: optional filter,
 * optional group label, and a roving-focus listbox. Options are real buttons
 * so Enter/Space activate natively; the listbox only adds arrow/Home/End
 * navigation (skipping disabled options) and hands stray typing to the
 * filter input.
 */
function OptionList({
  options,
  value,
  listboxId,
  listboxLabel,
  groupLabel,
  searchNoun,
  searchThreshold,
  onPick,
  ref,
}: OptionListProps) {
  const { query, setQuery, filtered, showSearch, emptyNote } = useOptionSearch(
    options,
    searchThreshold,
    searchNoun,
  );
  const listRef = React.useRef<HTMLDivElement>(null);
  const searchRef = React.useRef<HTMLInputElement>(null);
  const selectedVisible = filtered.some((option) => option.value === value && !option.disabled);
  const firstEnabledIndex = filtered.findIndex((option) => !option.disabled);

  const enabledOptions = () =>
    Array.from(listRef.current?.querySelectorAll<HTMLButtonElement>(ENABLED_OPTION_SELECTOR) ?? []);

  const focusOption = (index: number) => {
    const list = enabledOptions();
    if (list.length === 0) return;
    const wrapped = ((index % list.length) + list.length) % list.length;
    list[wrapped]?.focus();
  };

  const onListKeyDown = (event: React.KeyboardEvent<HTMLDivElement>) => {
    const current = enabledOptions().indexOf(event.target as HTMLButtonElement);
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
            aria-label={`Tìm ${searchNoun}`}
            placeholder={`Tìm ${searchNoun}…`}
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            onKeyDown={onSearchKeyDown}
            className="w-full bg-transparent text-[13.5px] font-bold text-ink-700 outline-none placeholder:text-ink-400 focus:shadow-none [&::-webkit-search-cancel-button]:appearance-none"
          />
        </div>
      ) : null}
      {groupLabel ? (
        <div className="px-3 pt-1.5 pb-0.5 text-[11px] font-extrabold uppercase tracking-[0.4px] text-ink-400">
          {groupLabel}
        </div>
      ) : null}
      <div
        ref={listRef}
        role="listbox"
        aria-label={listboxLabel}
        id={listboxId}
        tabIndex={-1}
        onKeyDown={onListKeyDown}
      >
        {filtered.map((option, index) => {
          const selected = option.value === value;
          const tabbable =
            !option.disabled && (selected || (!selectedVisible && index === firstEnabledIndex));
          return (
            <button
              key={option.value}
              type="button"
              role="option"
              aria-selected={selected}
              aria-disabled={option.disabled ? true : undefined}
              disabled={option.disabled}
              tabIndex={tabbable ? 0 : -1}
              onClick={() => onPick(option.value)}
              className={optionClassName}
            >
              <Check
                className={cn("size-4 shrink-0 text-mint-600", !selected && "invisible")}
                aria-hidden="true"
              />
              <span className="flex-1">{option.label}</span>
              {option.meta ? (
                <span className="text-[12px] font-bold text-ink-400">{option.meta}</span>
              ) : null}
            </button>
          );
        })}
      </div>
      {emptyNote ? (
        <div className="px-3 py-2.5">
          <p className="text-[13px] font-bold text-ink-400">{emptyNote}</p>
        </div>
      ) : null}
    </div>
  );
}

/**
 * Single-value picker of the hv kit, extracted from the records class
 * dropdown. From `sm` up it is a Popover anchored under the trigger; below
 * `sm` the same list opens in an `HvModal` bottom sheet titled `sheetTitle`.
 * Both branches hand initial focus to the filter input when present,
 * otherwise to the selected option, and return focus to the trigger on
 * close. The trigger exposes the current value as `data-value` and shows
 * `label · meta`; options show label and meta as adjacent spans.
 */
export function HvSelect({
  options,
  value,
  onValueChange,
  sheetTitle,
  placeholder = "Chọn…",
  groupLabel,
  searchNoun = "mục",
  searchThreshold = DEFAULT_SEARCH_THRESHOLD,
  id,
  labelId,
  "aria-label": ariaLabel,
  "aria-invalid": ariaInvalid,
  disabled,
  className,
  align = "start",
}: HvSelectProps) {
  const [open, setOpen] = React.useState(false);
  const isSmUp = useMediaQuery("(min-width: 640px)");
  const generatedId = React.useId();
  const listboxId = `${generatedId}-listbox`;
  const triggerTextId = `${generatedId}-text`;
  const bodyRef = React.useRef<HTMLDivElement>(null);
  const triggerRef = React.useRef<HTMLButtonElement>(null);
  const selected = options.find((option) => option.value === value);

  const focusInitial = (event: Event) => {
    event.preventDefault();
    const body = bodyRef.current;
    // Last resort is the listbox itself, so an empty or fully disabled list
    // still keeps focus inside the surface (a disabled button ignores focus()).
    const target =
      body?.querySelector<HTMLElement>('input[type="search"]') ??
      body?.querySelector<HTMLElement>(`${ENABLED_OPTION_SELECTOR}[aria-selected="true"]`) ??
      body?.querySelector<HTMLElement>(ENABLED_OPTION_SELECTOR) ??
      body?.querySelector<HTMLElement>('[role="listbox"]');
    target?.focus();
  };

  // HvModal has no Dialog.Trigger, so radix would drop focus on close;
  // send it back to our trigger instead (the Popover branch does this itself).
  const restoreTriggerFocus = (event: Event) => {
    event.preventDefault();
    triggerRef.current?.focus();
  };

  // Re-picking the current value still reports it (records pins the default
  // class into the URL this way); consumers with side effects guard themselves.
  const pick = (next: string) => {
    setOpen(false);
    onValueChange(next);
  };

  const triggerContent = (
    <>
      <span id={triggerTextId} className="min-w-0 truncate">
        {selected ? selected.label : placeholder}
        {selected?.meta ? (
          <>
            {" "}
            <span className="ml-0.5 text-[12.5px] font-bold text-ink-400">· {selected.meta}</span>
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
    <OptionList
      ref={bodyRef}
      options={options}
      value={value}
      listboxId={listboxId}
      listboxLabel={sheetTitle}
      groupLabel={groupLabel}
      searchNoun={searchNoun}
      searchThreshold={searchThreshold}
      onPick={pick}
    />
  );

  const triggerA11y = {
    id,
    type: "button" as const,
    role: "combobox" as const,
    "aria-haspopup": "listbox" as const,
    "aria-expanded": open,
    "aria-controls": listboxId,
    "aria-labelledby": labelId ? `${labelId} ${triggerTextId}` : undefined,
    "aria-label": ariaLabel,
    "aria-invalid": ariaInvalid ? true : undefined,
    disabled,
    "data-value": value,
    "data-placeholder": selected ? undefined : "",
    className: cn(triggerClassName, className),
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
          title={sheetTitle}
          size="md"
          onOpenAutoFocus={focusInitial}
          onCloseAutoFocus={restoreTriggerFocus}
        >
          <div className="max-h-[60dvh] overflow-y-auto">{list}</div>
        </HvModal>
      </>
    );
  }

  return (
    <PopoverPrimitive.Root open={open} onOpenChange={setOpen}>
      <PopoverPrimitive.Trigger asChild>
        <button {...triggerA11y} ref={triggerRef}>
          {triggerContent}
        </button>
      </PopoverPrimitive.Trigger>
      <PopoverPrimitive.Portal>
        <PopoverPrimitive.Content
          align={align}
          sideOffset={6}
          onOpenAutoFocus={focusInitial}
          className={contentClassName}
        >
          {list}
        </PopoverPrimitive.Content>
      </PopoverPrimitive.Portal>
    </PopoverPrimitive.Root>
  );
}
