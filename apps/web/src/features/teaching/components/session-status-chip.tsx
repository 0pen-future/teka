import { cn } from "@/lib/utils";

export type SessionChipTone = "mint" | "sun" | "coral" | "muted";

const toneClasses: Record<SessionChipTone, string> = {
  mint: "bg-mint-50 text-mint-600",
  sun: "bg-sun-100 text-sun-600",
  coral: "bg-coral-100 text-coral-600",
  muted: "bg-cream-200 text-ink-400",
};

/** The ledger's small state pill — "Đã có", "3/12", "Buổi hủy"… */
export function SessionStatusChip({
  tone,
  children,
  className,
}: {
  tone: SessionChipTone;
  children: React.ReactNode;
  className?: string;
}) {
  return (
    <span
      className={cn(
        "inline-flex whitespace-nowrap rounded-full px-[10px] py-1 text-[12px] font-extrabold tabular-nums",
        toneClasses[tone],
        className,
      )}
    >
      {children}
    </span>
  );
}
