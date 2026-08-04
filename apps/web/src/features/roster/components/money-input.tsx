import { useState } from "react";

import { Input } from "@/components/ui/input";
import { formatMoney } from "@/lib/utils";

import { parseMoney } from "../lib/roster-format";

export interface MoneyInputProps {
  id?: string;
  value: number;
  onChange: (value: number) => void;
  step?: number;
  "aria-invalid"?: boolean;
}

function toGrouped(value: number): string {
  return value > 0 ? value.toLocaleString("vi-VN") : "";
}

/**
 * Plain integer đồng input (`docs/schema_design.sql:24` — BIGINT, never a
 * float). Typing stays raw digits so the cursor never jumps; on blur the
 * field reformats with thousands separators, and a live preview line under
 * the field always shows `formatMoney(value)` so a teacher who means
 * "150.000" but typed "150" sees the mistake before saving (Risk
 * Assessment: "teacher enters price in thousands"). The grouped display is
 * derived at render time from `value`/`focused` rather than mirrored into
 * its own effect-synced state, so an externally driven `value` change (e.g.
 * `form.reset`) is picked up without a render-then-effect round trip.
 */
export function MoneyInput({ id, value, onChange, step = 5000, ...rest }: MoneyInputProps) {
  const [focused, setFocused] = useState(false);
  const [rawDigits, setRawDigits] = useState("");
  const display = focused ? rawDigits : toGrouped(value);

  return (
    <div>
      <div className="flex items-center gap-2">
        <button
          type="button"
          aria-label={`Giảm ${step.toLocaleString("vi-VN")} đồng`}
          onClick={() => onChange(Math.max(0, value - step))}
          className="flex h-10 w-10 shrink-0 items-center justify-center rounded-[var(--radius-md)] border border-line-200 bg-white font-display text-[18px] font-bold text-ink-500 hover:bg-cream-100"
        >
          −
        </button>
        <Input
          id={id}
          inputMode="numeric"
          className="h-10 text-center"
          value={display}
          onFocus={() => {
            setRawDigits(String(value > 0 ? value : ""));
            setFocused(true);
          }}
          onChange={(event) => {
            const raw = event.target.value.replace(/[^\d]/g, "");
            setRawDigits(raw);
            onChange(parseMoney(raw));
          }}
          onBlur={() => setFocused(false)}
          {...rest}
        />
        <button
          type="button"
          aria-label={`Tăng ${step.toLocaleString("vi-VN")} đồng`}
          onClick={() => onChange(value + step)}
          className="flex h-10 w-10 shrink-0 items-center justify-center rounded-[var(--radius-md)] border border-line-200 bg-white font-display text-[18px] font-bold text-ink-500 hover:bg-cream-100"
        >
          +
        </button>
      </div>
      <p className="mt-1 text-[13px] text-ink-400">{formatMoney(value)}</p>
    </div>
  );
}
