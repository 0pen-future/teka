import { toast as sonnerToast, type ExternalToast } from "sonner";

import { cn } from "@/lib/utils";

export type HvToastVariant = "default" | "success" | "danger";

const variantClasses: Record<HvToastVariant, string> = {
  default: "",
  success: "!bg-mint-600",
  danger: "!bg-coral-500",
};

export interface HvToastOptions extends Omit<ExternalToast, "position" | "className"> {
  /** Accent tint. Defaults to "default" (ink-900 pill). */
  variant?: HvToastVariant;
}

/**
 * Design-system-styled wrapper over sonner's `toast()` — bottom-center, an
 * ink-900 pill, Baloo 2 label. Reuses the single `<Toaster />` already
 * mounted in `src/app/providers.tsx`; never mount a second one.
 */
export function hvToast(message: string, options: HvToastOptions = {}) {
  const { variant = "default", ...rest } = options;

  return sonnerToast(message, {
    position: "bottom-center",
    duration: 2600,
    className: cn(
      "!rounded-full !border-0 !bg-ink-900 !px-5 !py-3 !font-display !text-white",
      "animate-[toastIn_var(--dur-base)_var(--ease-soft)]",
      variantClasses[variant],
    ),
    ...rest,
  });
}

/** Hook form for consistency with other Hv* APIs; returns the same helper. */
export function useHvToast() {
  return hvToast;
}
