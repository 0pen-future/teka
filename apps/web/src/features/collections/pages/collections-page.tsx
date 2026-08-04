import { useState } from "react";
import { useParams, useSearchParams } from "react-router";

import { HvCard } from "@/components/hv";
import { useClassesList } from "@/features/roster";
import { cn, formatMoney } from "@/lib/utils";

import { ClassCollectionGroup } from "../components/class-collection-group";
import { CollectionsViewToggle } from "../components/collections-view-toggle";
import { ContactCollectionRow } from "../components/contact-collection-row";
import { RecordPaymentDialog } from "../components/record-payment-dialog";
import {
  useClassCollectionsList,
  useCollectionsSummary,
  useContactCollectionsList,
} from "../hooks/use-collections";
import type { ContactBalanceRow, PaymentStatus } from "../schemas/collections-schemas";
import type { CollectionsView } from "../types/collections-types";

const statusChips: { value: PaymentStatus | ""; label: string }[] = [
  { value: "", label: "Tất cả" },
  { value: "unpaid", label: "Chưa đóng" },
  { value: "partial", label: "Đóng thiếu" },
  { value: "paid", label: "Đã đóng" },
];

/**
 * "Thu tiền" — the collections board. Defaults to the by-contact view when
 * `?view` is absent (PRD-stated default). The by-class view additionally
 * needs a class picker: the real endpoint requires `class_id` per request
 * (422 without it), unlike the spec's implied all-classes-at-once grouping.
 */
export function CollectionsPage() {
  const { periodId } = useParams<{ periodId: string }>();
  const [searchParams, setSearchParams] = useSearchParams();
  const view = (searchParams.get("view") as CollectionsView | null) ?? "contact";
  const status = (searchParams.get("status") as PaymentStatus | null) ?? undefined;
  const classId = searchParams.get("class_id") ?? "";
  const [payingContact, setPayingContact] = useState<ContactBalanceRow | null>(null);

  function setParam(key: string, value: string) {
    const next = new URLSearchParams(searchParams);
    if (value) {
      next.set(key, value);
    } else {
      next.delete(key);
    }
    setSearchParams(next, { replace: true });
  }

  const { data: summary } = useCollectionsSummary(periodId);
  const { data: classesPage } = useClassesList({ status: "active" });
  const classes = classesPage?.items ?? [];

  const { data: contactRows, isPending: contactPending } = useContactCollectionsList(
    view === "contact" ? periodId : undefined,
    { status },
  );
  const { data: classRows, isPending: classPending } = useClassCollectionsList(
    view === "class" ? periodId : undefined,
    { class_id: classId, status },
  );

  if (!periodId) {
    return null;
  }

  const rows = contactRows?.items ?? [];
  const rowsByClass = classRows?.items ?? [];
  const percent =
    summary && summary.total_due > 0
      ? Math.round((summary.total_paid / summary.total_due) * 100)
      : 0;

  return (
    <div className="flex flex-col gap-4">
      <h1 className="font-display text-[22px] font-bold text-ink-900">Thu tiền</h1>

      {summary ? (
        <HvCard variant="flat" className="flex flex-col gap-3">
          <div className="flex flex-wrap items-center gap-6">
            <div>
              <p className="text-[13px] text-ink-400">Phải thu</p>
              <p className="font-display text-[18px] font-bold text-ink-900">
                {formatMoney(summary.total_due)}
              </p>
            </div>
            <div>
              <p className="text-[13px] text-ink-400">Đã thu</p>
              <p className="font-display text-[18px] font-bold text-mint-600">
                {formatMoney(summary.total_paid)}
              </p>
            </div>
            <div>
              <p className="text-[13px] text-ink-400">Còn lại</p>
              <p className="font-display text-[18px] font-bold text-coral-600">
                {formatMoney(summary.total_outstanding)}
              </p>
            </div>
          </div>
          <div className="h-2 w-full overflow-hidden rounded-[var(--radius-pill)] bg-cream-200">
            <div
              className="h-full rounded-[var(--radius-pill)] bg-mint-400"
              style={{ width: `${Math.min(100, Math.max(0, percent))}%` }}
            />
          </div>
        </HvCard>
      ) : null}

      <div className="flex flex-wrap items-center justify-between gap-3">
        <CollectionsViewToggle
          value={view}
          onChange={(next) => setParam("view", next === "contact" ? "" : next)}
        />
        <div className="flex flex-wrap gap-2">
          {statusChips.map((chip) => (
            <button
              key={chip.value}
              type="button"
              onClick={() => setParam("status", chip.value)}
              className={cn(
                "min-h-9 rounded-[var(--radius-pill)] border px-4 font-display text-[13px] font-bold transition-colors",
                status === chip.value || (!status && chip.value === "")
                  ? "border-ink-900 bg-ink-900 text-white"
                  : "border-line-200 bg-white text-ink-500 hover:bg-cream-100",
              )}
            >
              {chip.label}
            </button>
          ))}
        </div>
      </div>

      {view === "class" ? (
        <div className="flex flex-wrap gap-2" role="tablist" aria-label="Lớp">
          {classes.map((klass) => (
            <button
              key={klass.id}
              type="button"
              role="tab"
              aria-selected={classId === klass.id}
              onClick={() => setParam("class_id", klass.id)}
              className={cn(
                "min-h-9 rounded-[var(--radius-pill)] border px-4 font-display text-[13px] font-bold transition-colors",
                classId === klass.id
                  ? "border-mint-400 bg-mint-400 text-white"
                  : "border-line-200 bg-white text-ink-500 hover:bg-cream-100",
              )}
            >
              {klass.name}
            </button>
          ))}
        </div>
      ) : null}

      {view === "contact" ? (
        <div className="flex flex-col gap-3">
          {contactPending ? <p className="text-[13px] text-ink-400">Đang tải…</p> : null}
          {!contactPending && rows.length === 0 ? (
            <HvCard variant="flat" className="text-center text-[13px] text-ink-400">
              Không có phụ huynh nào.
            </HvCard>
          ) : null}
          {rows.map((row) => (
            <ContactCollectionRow
              key={row.contact_id}
              row={row}
              periodId={periodId}
              onRecordPayment={setPayingContact}
            />
          ))}
        </div>
      ) : (
        <div className="flex flex-col gap-3">
          {!classId ? (
            <HvCard variant="flat" className="text-center text-[13px] text-ink-400">
              Chọn một lớp để xem.
            </HvCard>
          ) : null}
          {classId && classPending ? <p className="text-[13px] text-ink-400">Đang tải…</p> : null}
          {classId && !classPending && rowsByClass.length === 0 ? (
            <HvCard variant="flat" className="text-center text-[13px] text-ink-400">
              Lớp này chưa có buổi học nào trong kỳ.
            </HvCard>
          ) : null}
          {classId && rowsByClass.length > 0 ? (
            <ClassCollectionGroup className={rowsByClass[0]!.class_name} rows={rowsByClass} />
          ) : null}
        </div>
      )}

      {payingContact ? (
        <RecordPaymentDialog
          open={Boolean(payingContact)}
          onOpenChange={(open) => {
            if (!open) {
              setPayingContact(null);
            }
          }}
          periodId={periodId}
          contactId={payingContact.contact_id}
          contactName={payingContact.full_name}
        />
      ) : null}
    </div>
  );
}
