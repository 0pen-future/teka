import { Link } from "react-router";

import { hvButtonVariants } from "@/components/hv";

export function NotFound() {
  return (
    <main
      id="main-content"
      className="flex min-h-svh flex-col items-center justify-center gap-3 bg-cream-100 p-4 text-center"
    >
      <p aria-hidden="true" className="font-display text-[56px] font-extrabold text-mint-600">
        404
      </p>
      <h1 className="font-display text-[22px] font-extrabold text-ink-900">Không tìm thấy trang</h1>
      <p className="max-w-[var(--w-phone)] text-[14px] text-ink-500">
        Đường dẫn có thể đã đổi hoặc không còn tồn tại.
      </p>
      <Link to="/" className={hvButtonVariants({ variant: "primary", size: "md" })}>
        Về trang chủ
      </Link>
    </main>
  );
}
