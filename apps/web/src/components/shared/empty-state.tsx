import type { ReactNode } from "react";

interface EmptyStateProps {
  title: string;
  description?: string;
  /** Optional call to action, e.g. a "clear filters" or "create" button. */
  children?: ReactNode;
}

export function EmptyState({ title, description, children }: EmptyStateProps) {
  return (
    <div className="flex flex-col items-center justify-center gap-2 rounded-lg border border-dashed px-6 py-12 text-center">
      <p className="text-sm font-medium">{title}</p>
      {description ? <p className="text-sm text-muted-foreground">{description}</p> : null}
      {children ? <div className="mt-2">{children}</div> : null}
    </div>
  );
}
