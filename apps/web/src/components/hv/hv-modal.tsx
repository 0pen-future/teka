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
function HvModalContent({
  className,
  children,
  showCloseButton = true,
  ...props
}: React.ComponentProps<typeof DialogPrimitive.Content> & { showCloseButton?: boolean }) {
  return (
    <DialogPortal>
      <HvModalOverlay />
      <DialogPrimitive.Content
        data-slot="hv-modal-content"
        className={cn(
          "fixed inset-x-0 bottom-0 top-auto z-50 max-h-[85vh] w-full translate-x-0 translate-y-0",
          "overflow-y-auto rounded-t-[var(--radius-xl)] rounded-b-none bg-white p-[var(--pad-card)]",
          "outline-none max-sm:animate-[slideUp_var(--dur-base)_var(--ease-soft)]",
          "sm:animate-[popIn_var(--dur-base)_var(--ease-soft)] sm:inset-x-auto sm:bottom-auto",
          "sm:left-1/2 sm:top-1/2 sm:max-w-md sm:-translate-x-1/2 sm:-translate-y-1/2",
          "sm:rounded-[var(--radius-xl)]",
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
            <span className="sr-only">Đóng</span>
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
  /** Modal body. */
  children: React.ReactNode;
  /** Optional trailing action row (buttons), right-aligned. */
  footer?: React.ReactNode;
  /** Extra classes applied to the panel. */
  className?: string;
}

export function HvModal({ open, onOpenChange, title, children, footer, className }: HvModalProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <HvModalContent className={className}>
        {title != null ? (
          <HvModalTitle className="mb-[var(--space-4)]">{title}</HvModalTitle>
        ) : (
          <DialogPrimitive.Title className="sr-only">Hộp thoại</DialogPrimitive.Title>
        )}
        <div className="font-body text-ink-700">{children}</div>
        {footer != null ? (
          <div className="mt-[var(--space-5)] flex justify-end gap-[var(--space-2)]">{footer}</div>
        ) : null}
      </HvModalContent>
    </Dialog>
  );
}
