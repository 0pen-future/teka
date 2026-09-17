import { cn } from "@/lib/utils";

export interface AssigneeAvatarProps {
  name: string;
  className?: string;
}

/** Background tint rotation, keyed by a simple hash of the assignee's name so the same person always gets the same tint on this device. */
const TINTS = ["bg-mint-100", "bg-sky-100", "bg-sun-100", "bg-coral-100"] as const;

function initialOf(name: string): string {
  const normalized = name.normalize("NFD").replace(/[̀-ͯ]/g, "");
  const letter = /\p{L}/u.exec(normalized);
  return (letter?.[0] ?? name.charAt(0) ?? "?").toUpperCase();
}

function hashTint(name: string): (typeof TINTS)[number] {
  let sum = 0;
  for (let i = 0; i < name.length; i += 1) {
    sum += name.charCodeAt(i);
  }
  // `sum % TINTS.length` is always a valid index (0..length-1), but
  // `noUncheckedIndexedAccess` can't see that from a plain array index — the
  // `?? TINTS[0]` fallback never actually triggers.
  return TINTS[sum % TINTS.length] ?? TINTS[0];
}

/**
 * 24px initial-letter stand-in for a teacher's photo. `aria-label` and
 * `title` both carry the full name so the letter alone never is the only
 * way to identify who a task is assigned to.
 */
export function AssigneeAvatar({ name, className }: AssigneeAvatarProps) {
  return (
    <span
      role="img"
      aria-label={`Phụ trách: ${name}`}
      title={name}
      className={cn(
        "inline-flex size-6 shrink-0 items-center justify-center rounded-full text-[11px] font-bold text-ink-900",
        hashTint(name),
        className,
      )}
    >
      {initialOf(name)}
    </span>
  );
}
