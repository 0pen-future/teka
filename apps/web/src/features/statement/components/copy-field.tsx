import { useEffect, useRef, useState } from "react";

import { HvButton } from "@/components/hv";
import { cn, copyToClipboard } from "@/lib/utils";

export interface CopyFieldProps {
  label: string;
  value: string;
  /** "light" for cards on the cream background, "dark" for the surface-dark total block. */
  tone?: "light" | "dark";
}

/**
 * A labelled read-only value with a ≥44px copy button — used for the QR
 * transfer note, so a parent whose banking app can't scan can still paste it
 * in manually. `copyToClipboard` reports whether the copy actually
 * succeeded, so the confirmation only shows on a real success.
 */
export function CopyField({ label, value, tone = "light" }: CopyFieldProps) {
  const [copied, setCopied] = useState(false);
  const timeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    return () => {
      if (timeoutRef.current) {
        clearTimeout(timeoutRef.current);
      }
    };
  }, []);

  async function handleCopy() {
    const succeeded = await copyToClipboard(value);
    if (succeeded) {
      setCopied(true);
      timeoutRef.current = setTimeout(() => setCopied(false), 2000);
    }
  }

  const isDark = tone === "dark";

  return (
    <div
      className={cn(
        "flex w-full items-center justify-between gap-2 rounded-[var(--radius-md)] px-3 py-2 text-[13px]",
        isDark ? "bg-white/10 text-white" : "bg-cream-200 text-ink-700",
      )}
    >
      <div className="flex min-w-0 flex-col">
        <span className={cn("font-semibold", isDark ? "text-white/70" : "text-ink-400")}>
          {label}
        </span>
        <span className="truncate font-bold">{value}</span>
      </div>
      <HvButton
        type="button"
        variant={isDark ? "ghost" : "secondary"}
        size="sm"
        className="min-w-[44px] shrink-0"
        onClick={() => void handleCopy()}
        aria-label={copied ? "Đã sao chép" : `Sao chép ${label}`}
      >
        {copied ? "Đã chép" : "Chép"}
      </HvButton>
    </div>
  );
}
