import { CopyIcon } from "lucide-react";

import { hvToast } from "@/components/hv";

interface CopyCodeButtonProps {
  code: string;
}

/** Monospace exercise code with a one-click copy, shared by every exercise table. */
export function CopyCodeButton({ code }: CopyCodeButtonProps) {
  const copy = async () => {
    try {
      await navigator.clipboard.writeText(code);
      hvToast("Đã sao chép mã");
    } catch {
      hvToast("Không sao chép được mã", { variant: "danger" });
    }
  };
  return (
    <span className="inline-flex items-center gap-1.5">
      <span className="font-mono text-[13px] font-bold text-ink-700">{code}</span>
      <button
        type="button"
        onClick={() => void copy()}
        aria-label={`Sao chép mã ${code}`}
        className="inline-flex h-8 w-8 items-center justify-center rounded-md text-ink-500 hover:bg-cream-100 hover:text-mint-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-mint-400"
      >
        <CopyIcon className="h-4 w-4" aria-hidden />
      </button>
    </span>
  );
}
