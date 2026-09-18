import { cn } from "@/lib/utils";

export type AssigneeAvatarSize = "md" | "sm" | "xs";

export interface AssigneeAvatarProps {
  name: string;
  /** 28px default; "sm" (26px) sits in a card header, "xs" (20px) in a filter chip. */
  size?: AssigneeAvatarSize;
  /** Hide from assistive tech when the name is already spelled out next to it (filter chips). */
  decorative?: boolean;
  className?: string;
}

/** Solid hue rotation, keyed by a simple hash of the name so the same person always gets the same hue. */
const HUES = ["bg-mint-500", "bg-sky-400", "bg-sun-500", "bg-ink-400"] as const;

const SIZE_CLASS: Record<AssigneeAvatarSize, string> = {
  md: "size-7 text-[11px]",
  sm: "size-[26px] text-[10.5px]",
  xs: "size-5 text-[9px]",
};

/** Up to two initials — first and last word — with diacritics stripped so "Đ" and "D" collapse to the same letter. */
function initialsOf(name: string): string {
  const words = name.normalize("NFD").replace(/[̀-ͯ]/g, "").split(/\s+/).filter(Boolean);
  const first = words[0]?.charAt(0) ?? "?";
  const last = words.length > 1 ? (words[words.length - 1]?.charAt(0) ?? "") : "";
  return `${first}${last}`.toUpperCase();
}

function hashHue(name: string): (typeof HUES)[number] {
  let sum = 0;
  for (let i = 0; i < name.length; i += 1) {
    sum += name.charCodeAt(i);
  }
  // `sum % HUES.length` is always a valid index (0..length-1), but
  // `noUncheckedIndexedAccess` can't see that from a plain array index — the
  // `?? HUES[0]` fallback never actually triggers.
  return HUES[sum % HUES.length] ?? HUES[0];
}

/**
 * Initials stand-in for a teacher's photo. `aria-label` and `title` both
 * carry the full name so the initials alone never are the only way to
 * identify who a task is assigned to.
 */
export function AssigneeAvatar({
  name,
  size = "md",
  decorative = false,
  className,
}: AssigneeAvatarProps) {
  const a11y = decorative
    ? { "aria-hidden": true as const }
    : { role: "img", "aria-label": `Phụ trách: ${name}`, title: name };
  return (
    <span
      {...a11y}
      className={cn(
        "inline-flex shrink-0 items-center justify-center rounded-full font-display font-extrabold text-white",
        SIZE_CLASS[size],
        hashHue(name),
        className,
      )}
    >
      {initialsOf(name)}
    </span>
  );
}
