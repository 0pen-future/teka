import { useEffect, useState } from "react";
import { Link, useSearchParams } from "react-router";

import { HvButton, HvCard } from "@/components/hv";
import { Input } from "@/components/ui/input";
import { formatPhoneLocal } from "@/lib/utils";

import { ContactDialog } from "../components/contact-dialog";
import { useContactsList } from "../hooks/use-contacts";

/** Off-nav list of contacts, reachable from a student row's contact cell on `StudentsPage`. */
export function ContactsPage() {
  const [searchParams, setSearchParams] = useSearchParams();
  const urlQuery = searchParams.get("q") ?? "";
  const [query, setQuery] = useState(urlQuery);
  const [dialogOpen, setDialogOpen] = useState(false);

  useEffect(() => {
    const timer = setTimeout(() => {
      const next = new URLSearchParams(searchParams);
      if (query) {
        next.set("q", query);
      } else {
        next.delete("q");
      }
      setSearchParams(next, { replace: true });
    }, 300);
    return () => clearTimeout(timer);
    // searchParams/setSearchParams intentionally excluded: only the debounced
    // query should retrigger this effect, not every URL change it causes.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [query]);

  const { data, isPending } = useContactsList({ query: urlQuery, per_page: 50 });
  const contacts = data?.items ?? [];

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h1 className="font-display text-[22px] font-bold text-ink-900">Người liên hệ</h1>
        <HvButton onClick={() => setDialogOpen(true)}>Thêm người liên hệ</HvButton>
      </div>
      <Input
        placeholder="Tìm theo tên hoặc số điện thoại"
        value={query}
        onChange={(event) => setQuery(event.target.value)}
        className="max-w-sm"
      />
      {isPending ? <p className="text-[13px] text-ink-400">Đang tải…</p> : null}
      {!isPending && contacts.length === 0 ? (
        <HvCard variant="flat" className="text-center text-[13px] text-ink-400">
          Không tìm thấy người liên hệ.
        </HvCard>
      ) : null}
      <div className="flex flex-col gap-2">
        {contacts.map((contact) => (
          <Link key={contact.id} to={`/contacts/${contact.id}`}>
            <HvCard variant="flat" interactive className="flex items-center justify-between">
              <div>
                <p className="font-display text-[15px] font-bold text-ink-900">
                  {contact.full_name}
                </p>
                <p className="text-[13px] text-ink-400">{formatPhoneLocal(contact.phone)}</p>
              </div>
              <span className="text-[13px] text-ink-400">{contact.student_count} học sinh</span>
            </HvCard>
          </Link>
        ))}
      </div>
      <ContactDialog open={dialogOpen} onOpenChange={setDialogOpen} />
    </div>
  );
}
