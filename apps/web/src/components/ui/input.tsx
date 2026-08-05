import * as React from "react";

import { cn } from "@/lib/utils";

function Input({ className, type, ...props }: React.ComponentProps<"input">) {
  return (
    <input
      type={type}
      data-slot="input"
      className={cn(
        "w-full min-w-0 rounded-[14px] border-2 border-line-200 bg-white px-3 py-2.5 text-[14.5px] text-ink-700 transition-colors outline-none",
        "file:inline-flex file:h-6 file:border-0 file:bg-transparent file:text-sm file:font-medium file:text-foreground",
        "placeholder:text-ink-400",
        "focus-visible:border-mint-400",
        "disabled:pointer-events-none disabled:cursor-not-allowed disabled:bg-cream-200 disabled:text-ink-300",
        "aria-invalid:border-coral-400",
        className,
      )}
      {...props}
    />
  );
}

export { Input };
