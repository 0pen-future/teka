import * as React from "react";

import { cn } from "@/lib/utils";

import { parseScoreInput, type ParsedScore } from "./score-input-parse";

export type HvScoreInputState = "idle" | "dirty" | "saved" | "invalid";
export type HvScoreInputSize = "sm" | "md";
export type HvScoreInputDirection = "up" | "down";

export const SCORE_INPUT_ERROR_TEXT = "Điểm 0–10, bước 0,5";

const stateClassName: Record<HvScoreInputState, string> = {
  idle: "border-line-200 bg-white",
  dirty: "border-sun-400 bg-sun-100",
  saved: "border-mint-400 bg-white",
  invalid: "border-coral-400 bg-white",
};

const sizeClassName: Record<HvScoreInputSize, string> = {
  sm: "min-h-11",
  md: "min-h-12",
};

export interface HvScoreInputProps extends Omit<
  React.InputHTMLAttributes<HTMLInputElement>,
  "value" | "onChange" | "size" | "type"
> {
  /** Raw text currently in the cell (controlled). */
  value: string;
  /** Fires on every keystroke with the raw text. */
  onChange: (raw: string) => void;
  /** Fires on blur/Enter with the parsed value so the owner can mark the cell dirty or invalid. */
  onCommit?: (parsed: ParsedScore, raw: string) => void;
  /** Visual state driven by the owner's draft bookkeeping. Defaults to "idle". */
  state?: HvScoreInputState;
  /** Enter moves "down", Shift+Enter moves "up"; the owner decides what that means. */
  onNavigate?: (direction: HvScoreInputDirection) => void;
  /** 44px ("sm") or 48px ("md") tall. Defaults to "md". */
  size?: HvScoreInputSize;
  /** Accessible name — required because the cell has no visible label of its own. */
  "aria-label": string;
}

/**
 * Single score cell: plain text input with a decimal keyboard on mobile so
 * "7,5" is typeable, plus a state ring (dirty/saved/invalid) the grading
 * screens use instead of a per-cell save button.
 */
export const HvScoreInput = React.forwardRef<HTMLInputElement, HvScoreInputProps>(
  (
    {
      id,
      value,
      onChange,
      onCommit,
      state = "idle",
      onNavigate,
      size = "md",
      className,
      disabled,
      onBlur,
      onKeyDown,
      ...rest
    },
    ref,
  ) => {
    const generatedId = React.useId();
    const inputId = id ?? generatedId;
    const errorId = `${inputId}-error`;
    const invalid = state === "invalid";

    const commit = () => onCommit?.(parseScoreInput(value), value);

    return (
      <div className="flex w-full flex-col gap-1">
        <input
          ref={ref}
          id={inputId}
          type="text"
          inputMode="decimal"
          autoComplete="off"
          value={value}
          disabled={disabled}
          data-state={state}
          aria-invalid={invalid || undefined}
          aria-describedby={invalid ? errorId : undefined}
          onChange={(event) => onChange(event.target.value)}
          onBlur={(event) => {
            commit();
            onBlur?.(event);
          }}
          onKeyDown={(event) => {
            onKeyDown?.(event);
            if (event.defaultPrevented || event.key !== "Enter") return;
            event.preventDefault();
            commit();
            onNavigate?.(event.shiftKey ? "up" : "down");
          }}
          className={cn(
            "w-full rounded-[var(--radius-md)] border-2 px-2 text-center",
            "text-[length:var(--text-md)] font-semibold tabular-nums text-ink-900",
            "transition-colors duration-[var(--dur-fast)] ease-[var(--ease-out)]",
            "focus-visible:outline-none focus-visible:ring-4",
            "disabled:cursor-not-allowed disabled:border-line-100 disabled:bg-cream-100 disabled:text-ink-400",
            sizeClassName[size],
            stateClassName[state],
            className,
          )}
          {...rest}
        />
        {invalid ? (
          <p id={errorId} role="alert" className="text-[12px] font-semibold text-coral-600">
            {SCORE_INPUT_ERROR_TEXT}
          </p>
        ) : null}
      </div>
    );
  },
);
HvScoreInput.displayName = "HvScoreInput";
