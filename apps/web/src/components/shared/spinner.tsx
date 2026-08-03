import { Loader2Icon } from "lucide-react";

import { cn } from "@/lib/utils";

export function Spinner({ className }: { className?: string }) {
  return (
    <output aria-label="Loading" className="flex items-center justify-center">
      <Loader2Icon
        aria-hidden
        className={cn("size-5 animate-spin text-muted-foreground", className)}
      />
    </output>
  );
}
