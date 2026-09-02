"use client";

import * as React from "react";
import { Dialog as DialogPrimitive } from "radix-ui";
import { X } from "lucide-react";

import { cn } from "@/lib/utils";
import { Dialog, DialogPortal } from "@/components/ui/dialog";

/**
 * Overlay tinted with the DS scrim color (ink-900 @ 40% alpha) instead of the
 * shadcn default, layered above the page while the modal is open.
 */
function HvModalOverlay({
  className,
  ...props
}: React.ComponentProps<typeof DialogPrimitive.Overlay>) {
  return (
    <DialogPrimitive.Overlay
      data-slot="hv-modal-overlay"
      className={cn(
        "fixed inset-0 z-50 bg-[rgba(28,58,49,0.4)]",
        "data-open:animate-in data-open:fade-in-0 data-closed:animate-out data-closed:fade-out-0",
        className,
      )}
      {...props}
    />
  );
}

/**
 * Responsive panel: a bottom sheet under the `sm` breakpoint (full width,
 * only the top corners rounded, anchored to the viewport bottom) and a
 * centered `max-w-md` card from `sm` up. Built directly on radix's Dialog
 * primitive (the same one `@/components/ui/dialog` wraps) because the shadcn
 * `DialogContent` hardcodes centered positioning that a bottom sheet cannot
 * express through className overrides alone.
 */
export type HvModalSize = "md" | "lg" | "xl";

/**
 * "md" and "lg" are content-sized cards; "xl" is a page-width workspace that
 * grows with its content up to 90dvh, after which the body scrolls while
 * title and footer stay put.
 */
const sizeClassName: Record<HvModalSize, string> = {
  md: "sm:max-w-md",
  lg: "sm:max-w-[720px]",
  xl: "flex max-h-[95dvh] flex-col overflow-hidden sm:max-h-[90dvh] sm:max-w-[var(--w-page)]",
};

function HvModalContent({
  className,
  children,
  showCloseButton = true,
  size = "md",
  ...props
}: React.ComponentProps<typeof DialogPrimitive.Content> & {
  showCloseButton?: boolean;
  size?: HvModalSize;
}) {
  return (
    <DialogPortal>
      <HvModalOverlay />
      <DialogPrimitive.Content
        data-slot="hv-modal-content"
        data-size={size}
        className={cn(
          "fixed inset-x-0 bottom-0 top-auto z-50 max-h-[85vh] w-full translate-x-0 translate-y-0",
          "overflow-y-auto rounded-t-[var(--radius-xl)] rounded-b-none bg-white p-6",
          "outline-none max-sm:animate-[slideUp_var(--dur-base)_var(--ease-soft)]",
          "sm:animate-[popIn_var(--dur-base)_var(--ease-soft)] sm:inset-x-auto sm:bottom-auto",
          "sm:left-1/2 sm:top-1/2 sm:-translate-x-1/2 sm:-translate-y-1/2",
          "sm:rounded-[var(--radius-xl)]",
          sizeClassName[size],
          className,
        )}
        aria-describedby={undefined}
        {...props}
      >
        {children}
        {showCloseButton ? (
          <DialogPrimitive.Close
            data-slot="hv-modal-close"
            className={cn(
              "absolute right-2 top-2 inline-flex h-12 w-12 items-center justify-center rounded-full",
              "text-ink-400 transition-colors duration-[var(--dur-fast)] ease-[var(--ease-out)]",
              "hover:bg-cream-200 hover:text-ink-700",
              "focus-visible:outline-none focus-visible:ring-4",
            )}
          >
            <X className="h-5 w-5" strokeWidth={2} />
            <span className="sr-only">Đóng hộp thoại</span>
          </DialogPrimitive.Close>
        ) : null}
      </DialogPrimitive.Content>
    </DialogPortal>
  );
}

function HvModalTitle({ className, ...props }: React.ComponentProps<typeof DialogPrimitive.Title>) {
  return (
    <DialogPrimitive.Title
      data-slot="hv-modal-title"
      className={cn(
        "pr-10 font-display text-[length:var(--text-lg)] font-bold text-ink-900",
        className,
      )}
      {...props}
    />
  );
}

export interface HvModalProps {
  /** Whether the modal is currently visible. */
  open: boolean;
  /** Called with the next open state on close (esc, overlay click, close button). */
  onOpenChange: (open: boolean) => void;
  /** Optional heading rendered above the content. */
  title?: React.ReactNode;
  /** Optional muted subtitle rendered directly under the title. */
  description?: React.ReactNode;
  /** Modal body. */
  children: React.ReactNode;
  /** Optional trailing action row (buttons), right-aligned. */
  footer?: React.ReactNode;
  /** Panel width preset. Defaults to "md". */
  size?: HvModalSize;
  /** Extra classes applied to the panel. */
  className?: string;
  /** Radix open-autofocus hook; call `preventDefault` and focus a ref to pick the initial control. */
  onOpenAutoFocus?: (event: Event) => void;
}

export function HvModal({
  open,
  onOpenChange,
  title,
  description,
  children,
  footer,
  size = "md",
  className,
  onOpenAutoFocus,
}: HvModalProps) {
  const descriptionId = React.useId();
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <HvModalContent
        size={size}
        className={className}
        aria-describedby={description != null ? descriptionId : undefined}
        onOpenAutoFocus={onOpenAutoFocus}
      >
        {title != null ? (
          <HvModalTitle className={description != null ? "mb-0.5" : "mb-[var(--space-4)]"}>
            {title}
          </HvModalTitle>
        ) : (
          <DialogPrimitive.Title className="sr-only">Hộp thoại</DialogPrimitive.Title>
        )}
        {description != null ? (
          <DialogPrimitive.Description
            id={descriptionId}
            className="mb-[var(--space-4)] pr-10 text-[12.5px] text-ink-500"
          >
            {description}
          </DialogPrimitive.Description>
        ) : null}
        <div
          className={cn("font-body text-ink-700", size === "xl" && "min-h-0 flex-1 overflow-auto")}
        >
          {children}
        </div>
        {footer != null ? (
          <div
            className={cn(
              "mt-[var(--space-5)] flex justify-end gap-[var(--space-2)]",
              size === "xl" && "shrink-0",
            )}
          >
            {footer}
          </div>
        ) : null}
      </HvModalContent>
    </Dialog>
  );
}
