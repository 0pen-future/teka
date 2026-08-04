import type { Statement } from "../types/statement-types";
import { ChildSection } from "./child-section";
import { GrandTotal } from "./grand-total";

export interface StatementViewProps {
  statement: Statement;
}

/**
 * The full parent statement: header naming the period and family, one card
 * per child, the family grand total with QR, and a closing note. The "cập
 * nhật trực tiếp" sub-line doubles as the freshness cue — the figures reflect
 * whatever the teacher has recorded up to the moment this page loaded.
 */
export function StatementView({ statement }: StatementViewProps) {
  return (
    <div className="flex flex-col gap-4">
      <header className="flex flex-col gap-1 rounded-b-[var(--radius-xl)] bg-mint-400 px-5 py-6 text-white">
        <span className="text-[13px] opacity-90">Học phí tháng {statement.period}</span>
        <h1 className="font-display text-[21px] font-extrabold">{statement.contact_name}</h1>
        <span className="text-[12px] opacity-90">cập nhật trực tiếp, không cần đăng nhập</span>
      </header>

      {statement.children.map((child, index) => (
        <ChildSection key={`${child.student_name}-${index}`} child={child} />
      ))}

      <GrandTotal totals={statement.totals} qr={statement.qr} />

      <footer className="flex flex-col items-center gap-1 pb-4 text-center text-[12px] text-ink-400">
        <p>Link riêng của gia đình, hết hiệu lực sau khi thanh toán xong hoặc sau 90 ngày.</p>
        <p>Có thắc mắc về số liệu, vui lòng liên hệ trực tiếp thầy/cô.</p>
      </footer>
    </div>
  );
}
