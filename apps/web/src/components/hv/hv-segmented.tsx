import * as React from "react";
import { RadioGroup } from "radix-ui";

import { cn } from "@/lib/utils";

export type HvSegmentedVariant = "segmented" | "tabs";

export interface HvSegmentedOption<T extends string> {
  value: T;
  label: React.ReactNode;
  icon?: React.ReactNode;
  disabled?: boolean;
}

export interface HvSegmentedProps<T extends string> {
  options: readonly HvSegmentedOption<T>[];
  value: T;
  onValueChange: (value: T) => void;
  /**
   * "segmented" (default) is a radio group for switching a mode or filter;
   * "tabs" exposes tablist/tab semantics for switching between panels and
   * expects the owner to render matching `id="{idBase}-panel-{value}"` panels.
   */
  variant?: HvSegmentedVariant;
  /** Prefix for tab/panel ids. Required in practice for the "tabs" variant. */
  idBase?: string;
  "aria-label": string;
  /** Stretch to the container width; each item shares the space equally. */
  block?: boolean;
  className?: string;
}

const containerClassName = "inline-flex gap-1 rounded-[var(--radius-md)] bg-cream-100 p-1";

const itemClassName = cn(
  "inline-flex min-h-11 flex-1 cursor-pointer select-none items-center justify-center gap-1.5",
  "rounded-[calc(var(--radius-md)-4px)] px-3 font-display text-[length:var(--text-sm)] font-bold",
  "text-ink-500 transition-colors duration-[var(--dur-fast)] ease-[var(--ease-out)]",
  "hover:text-ink-700 focus-visible:outline-none focus-visible:ring-4",
  "disabled:cursor-not-allowed disabled:opacity-50",
);

const activeClassName = "bg-white text-ink-900 shadow-sm";

function ItemBody<T extends string>({ option }: { option: HvSegmentedOption<T> }) {
  return (
    <>
      {option.icon != null ? (
        <span aria-hidden="true" className="inline-flex shrink-0 items-center">
          {option.icon}
        </span>
      ) : null}
      <span>{option.label}</span>
    </>
  );
}

export function HvSegmented<T extends string>({
  options,
  value,
  onValueChange,
  variant = "segmented",
  idBase,
  "aria-label": ariaLabel,
  block,
  className,
}: HvSegmentedProps<T>) {
  const generatedId = React.useId();
  const base = idBase ?? generatedId;
  const tabRefs = React.useRef<Map<T, HTMLButtonElement>>(new Map());

  if (variant === "tabs") {
    const enabled = options.filter((option) => !option.disabled);

    const moveTo = (next: HvSegmentedOption<T> | undefined) => {
      if (!next) return;
      onValueChange(next.value);
      tabRefs.current.get(next.value)?.focus();
    };

    const handleKeyDown = (event: React.KeyboardEvent<HTMLButtonElement>, current: T) => {
      const index = enabled.findIndex((option) => option.value === current);
      if (index === -1) return;
      switch (event.key) {
        case "ArrowRight":
        case "ArrowDown":
          event.preventDefault();
          moveTo(enabled[(index + 1) % enabled.length]);
          break;
        case "ArrowLeft":
        case "ArrowUp":
          event.preventDefault();
          moveTo(enabled[(index - 1 + enabled.length) % enabled.length]);
          break;
        case "Home":
          event.preventDefault();
          moveTo(enabled[0]);
          break;
        case "End":
          event.preventDefault();
          moveTo(enabled[enabled.length - 1]);
          break;
        default:
          break;
      }
    };

    return (
      <div
        role="tablist"
        aria-label={ariaLabel}
        aria-orientation="horizontal"
        className={cn(containerClassName, block && "flex w-full", className)}
      >
        {options.map((option) => {
          const active = option.value === value;
          return (
            <button
              key={option.value}
              ref={(node) => {
                if (node) tabRefs.current.set(option.value, node);
                else tabRefs.current.delete(option.value);
              }}
              type="button"
              role="tab"
              id={`${base}-tab-${option.value}`}
              aria-selected={active}
              aria-controls={`${base}-panel-${option.value}`}
              tabIndex={active ? 0 : -1}
              disabled={option.disabled}
              onClick={() => onValueChange(option.value)}
              onKeyDown={(event) => handleKeyDown(event, option.value)}
              className={cn(itemClassName, active && activeClassName)}
            >
              <ItemBody option={option} />
            </button>
          );
        })}
      </div>
    );
  }

  return (
    <RadioGroup.Root
      aria-label={ariaLabel}
      orientation="horizontal"
      value={value}
      onValueChange={(next) => onValueChange(next as T)}
      className={cn(containerClassName, block && "flex w-full", className)}
    >
      {options.map((option) => (
        <RadioGroup.Item
          key={option.value}
          value={option.value}
          disabled={option.disabled}
          className={cn(
            itemClassName,
            "data-[state=checked]:bg-white data-[state=checked]:text-ink-900 data-[state=checked]:shadow-sm",
          )}
        >
          <ItemBody option={option} />
        </RadioGroup.Item>
      ))}
    </RadioGroup.Root>
  );
}
