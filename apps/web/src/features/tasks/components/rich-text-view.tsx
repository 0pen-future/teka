import { useMemo } from "react";

import { cn } from "@/lib/utils";

import { normalizeIncoming } from "../lib/rich-text";

export interface RichTextViewProps {
  html: string;
  className?: string;
  /** Shown instead of nothing when the description is empty. */
  emptyText?: string;
}

/**
 * Renders a task description. The API already stores a sanitized subset,
 * but nothing reaches `innerHTML` here without going through DOMPurify
 * again: this is the only component allowed to use `dangerouslySetInnerHTML`
 * (an ESLint rule guards every other file). A description still in the
 * plain-text shape is wrapped the same way the edit form seeds it, so its
 * line breaks survive.
 */
export function RichTextView({ html, className, emptyText }: RichTextViewProps) {
  const clean = useMemo(() => normalizeIncoming(html), [html]);
  if (clean === "") {
    return emptyText ? (
      <p className={cn("text-[13px] text-ink-400", className)}>{emptyText}</p>
    ) : null;
  }
  return (
    <div
      className={cn("prose-task text-[14px] text-ink-700", className)}
      dangerouslySetInnerHTML={{ __html: clean }}
    />
  );
}
