import { useState } from "react";
import { Link, useSearchParams } from "react-router";

import { HvBadge, HvButton, HvCard } from "@/components/hv";
import { useContactMonthlyBalances } from "@/features/collections";
import { useZaloStatus } from "@/features/profile";
import { formatMoney, formatPhoneLocal } from "@/lib/utils";

import { useContactsInfinite } from "../hooks/use-contacts";
import { ContactDialog } from "./contact-dialog";
import { LoadMoreButton } from "./load-more-button";
import { ZaloAutoMapDialog } from "./zalo-auto-map-dialog";

const MONTH_PATTERN = /^(\d{4})-(0[1-9]|1[0-2])$/;

function thisMonth(): string {
  const now = new Date();
  return `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, "0")}`;
}

interface ContactsTabProps {
  /** The page-level search, already debounced into `?q`. */
  query: string;
  /** Adding a contact — the owner's alone. */
  canCreate: boolean;
  /** The month's outstanding column — billing.view_all. */
  canSeeDebt: boolean;
  /** Zalo auto-matching writes mappings, which only reports.send may do. */
  canMapZalo: boolean;
}

/**
 * The students page's "Người liên hệ" tab: the center's contacts with phone
 * and child count, plus — for billing.view_all — what each family still owes
 * for the month picked in `?month=YYYY-MM`.
 */
export function ContactsTab({ query, canCreate, canSeeDebt, canMapZalo }: ContactsTabProps) {
  const [searchParams, setSearchParams] = useSearchParams();
  const [dialogOpen, setDialogOpen] = useState(false);
  const rawMonth = searchParams.get("month") ?? "";
  const month = MONTH_PATTERN.test(rawMonth) ? rawMonth : thisMonth();
  const [year, monthNumber] = month.split("-").map(Number) as [number, number];

  const { data, isPending, fetchNextPage, hasNextPage, isFetchingNextPage } = useContactsInfinite({
    query,
    per_page: 50,
  });
  const contacts = data?.pages.flatMap((page) => page.items) ?? [];
  const total = data?.pages[data.pages.length - 1]?.meta.total ?? 0;

  const { data: balances } = useContactMonthlyBalances(year, monthNumber, canSeeDebt);
  const outstandingByContact = new Map(
    (balances ?? []).map((row) => [row.contact_id, row.outstanding]),
  );

  function setMonth(value: string) {
    setSearchParams(
      (prev) => {
        const next = new URLSearchParams(prev);
        if (MONTH_PATTERN.test(value)) {
          next.set("month", value);
        } else {
          next.delete("month");
        }
        return next;
      },
      { replace: true },
    );
  }

  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-wrap items-center gap-2">
        {canSeeDebt ? (
          <label className="flex items-center gap-2 text-[13px] font-bold text-ink-700">
            Công nợ tháng
            <input
              type="month"
              value={month}
              onChange={(event) => setMonth(event.target.value)}
              className="rounded-full border-2 border-line-200 bg-white px-3 py-1.5 text-[13px] font-bold text-ink-700 outline-none focus:border-mint-400"
            />
          </label>
        ) : null}
        <div className="ml-auto flex flex-wrap items-center gap-2">
          {canMapZalo ? <ZaloAutoMapButton /> : null}
          {canCreate ? (
            <HvButton size="sm" onClick={() => setDialogOpen(true)}>
              Thêm người liên hệ
            </HvButton>
          ) : null}
        </div>
      </div>

      {isPending ? <p className="text-[13px] text-ink-500">Đang tải…</p> : null}
      {!isPending && contacts.length === 0 ? (
        <HvCard variant="flat" className="text-center text-[13px] text-ink-500">
          Không tìm thấy người liên hệ.
        </HvCard>
      ) : null}

      <ul className="flex flex-col gap-2" aria-label="Danh sách người liên hệ">
        {contacts.map((contact) => {
          // A family missing from the month's rows owes nothing; a credit
          // (negative) is not debt either.
          const outstanding = outstandingByContact.get(contact.id) ?? 0;
          return (
            <li key={contact.id}>
              <Link to={`/contacts/${contact.id}`}>
                <HvCard
                  variant="flat"
                  interactive
                  className="flex flex-wrap items-center justify-between gap-2"
                >
                  <div>
                    <p className="font-display text-[15px] font-bold text-ink-900">
                      {contact.full_name}
                    </p>
                    <p className="text-[13px] text-ink-500">{formatPhoneLocal(contact.phone)}</p>
                    {contact.zalo_name ? (
                      <HvBadge variant="success" size="sm" dot className="mt-1">
                        {contact.zalo_name}
                      </HvBadge>
                    ) : null}
                  </div>
                  <div className="flex items-center gap-4 text-[13px] text-ink-500">
                    <span>{contact.student_count} học sinh</span>
                    {canSeeDebt ? (
                      <span>
                        Còn nợ:{" "}
                        <span
                          className={
                            outstanding > 0 ? "font-bold text-coral-600" : "font-bold text-ink-700"
                          }
                        >
                          {outstanding > 0 ? formatMoney(outstanding) : "—"}
                        </span>
                      </span>
                    ) : null}
                  </div>
                </HvCard>
              </Link>
            </li>
          );
        })}
      </ul>

      <LoadMoreButton
        loaded={contacts.length}
        total={total}
        hasNextPage={hasNextPage}
        isFetchingNextPage={isFetchingNextPage}
        onLoadMore={() => void fetchNextPage()}
      />

      {canCreate ? <ContactDialog open={dialogOpen} onOpenChange={setDialogOpen} /> : null}
    </div>
  );
}

/**
 * Its own component so the Zalo status query only runs for viewers who may
 * write mappings.
 */
function ZaloAutoMapButton() {
  const [open, setOpen] = useState(false);
  const { data: zaloStatus } = useZaloStatus();
  // The match endpoint needs a live session; "expired" shows the disabled
  // trigger too — re-linking happens on the profile page, not here.
  const zaloReady = zaloStatus?.linked === true && zaloStatus.status === "linked";

  return (
    <>
      <HvButton
        variant="secondary"
        size="sm"
        disabled={!zaloReady}
        title={zaloReady ? undefined : "Kết nối Zalo ở trang Hồ sơ để dùng tính năng này"}
        onClick={() => setOpen(true)}
      >
        Tự động ghép Zalo
      </HvButton>
      {zaloStatus && !zaloReady ? (
        // `title` on the disabled button never surfaces on touch, and phones
        // are the primary device — the reason has to live in the page itself.
        <p className="basis-full text-right text-[13px] text-ink-500">
          Muốn tự động ghép?{" "}
          <Link
            to="/profile"
            className="font-bold text-mint-600 underline-offset-4 hover:underline"
          >
            Kết nối Zalo ở trang Hồ sơ
          </Link>{" "}
          trước nhé.
        </p>
      ) : null}
      {/* Mounted only while open: each open runs a fresh scan + lookup. */}
      {open ? <ZaloAutoMapDialog open onOpenChange={setOpen} /> : null}
    </>
  );
}
