import { HvCard } from "@/components/hv";

import type { ImportReport, ImportReportEntity } from "../schemas/import-schemas";

/** Row order follows the order the import creates them, so the counts read top-down. */
const ENTITY_LABELS: { key: keyof Omit<ImportReport, "committed">; label: string }[] = [
  { key: "classes", label: "Lớp" },
  { key: "schedules", label: "Buổi học trong tuần" },
  { key: "contacts", label: "Phụ huynh" },
  { key: "students", label: "Học sinh" },
  { key: "enrollments", label: "Ghi danh" },
];

function EntityRow({ label, entity }: { label: string; entity: ImportReportEntity }) {
  return (
    <div className="flex items-center justify-between border-t border-line-100 py-2 first:border-t-0">
      <span className="text-[13.5px] text-ink-500">{label}</span>
      <span className="text-[13.5px]">
        <span className="font-extrabold text-mint-600">{entity.created}</span>
        <span className="text-ink-400"> tạo mới · </span>
        <span className="font-extrabold text-ink-900">{entity.reused}</span>
        <span className="text-ink-400"> đã có sẵn</span>
      </span>
    </div>
  );
}

/**
 * The created/reused split for one run. A re-import of an unchanged file
 * reports every entity as "đã có sẵn" with nothing created — that is the
 * operator's only evidence the second upload was a no-op rather than a
 * silent duplication, so the split is always shown, even when zero.
 */
export function ImportReportSummary({ report }: { report: ImportReport }) {
  return (
    <HvCard>
      <p className="font-display text-[16px] font-bold text-ink-900">
        {report.committed ? "Đã nhập xong" : "File hợp lệ"}
      </p>
      <p className="mt-0.5 text-[13px] text-ink-400">
        {report.committed
          ? "Dữ liệu đã được ghi vào hệ thống."
          : "Chưa ghi gì cả — bấm “Nhập dữ liệu” để lưu vào hệ thống."}
      </p>
      <div className="mt-3">
        {ENTITY_LABELS.map(({ key, label }) => (
          <EntityRow key={key} label={label} entity={report[key]} />
        ))}
      </div>
    </HvCard>
  );
}
