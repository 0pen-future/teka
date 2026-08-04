import { useState } from "react";

import { Input } from "@/components/ui/input";
import { formatMoney } from "@/lib/utils";

import { parseMoney } from "../lib/money-format";

export interface MoneyFieldProps {
  id?: string;
  value: number;
  onChange: (value: number) => void;
  "aria-invalid"?: boolean;
  disabled?: boolean;
}

/**
 * Plain integer đồng input (BIGINT column, never a float,
 * `docs/schema_design.sql:24`). Typing stays raw digits so the cursor never
 * jumps; a live `formatMoney` preview under the field confirms the amount
 * before submit.
 */
export function MoneyField({ id, value, onChange, disabled, ...rest }: MoneyFieldProps) {
  const [focused, setFocused] = useState(false);
  const [rawDigits, setRawDigits] = useState("");
  const display = focused ? rawDigits : value > 0 ? value.toLocaleString("vi-VN") : "";

  return (
    <div>
      <Input
        id={id}
        inputMode="numeric"
        disabled={disabled}
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
      <p className="mt-1 text-[13px] text-ink-400">{formatMoney(value)}</p>
    </div>
  );
}
