import { useEffect, useState } from "react";

import { Input } from "@/components/ui/input";
import { HvButton } from "@/components/hv";
import { ApiError } from "@/lib/api/errors";
import { cn } from "@/lib/utils";

import { useContactsList, useCreateContact } from "../hooks/use-contacts";
import { contactInputSchema, type Contact } from "../schemas/roster-schemas";

export interface ContactPickerProps {
  value: string;
  onChange: (contactId: string, contact?: Contact) => void;
  id?: string;
  "aria-invalid"?: boolean;
}

/**
 * Searchable contact select used by `StudentDialog`. Debounces the query
 * 300ms before hitting `useContactsList`, and its last row, "— Tạo người
 * liên hệ mới —", expands two inline inputs so a teacher never has to leave
 * the student form to register a brand-new parent/guardian.
 */
export function ContactPicker({ value, onChange, id, ...rest }: ContactPickerProps) {
  const [query, setQuery] = useState("");
  const [debouncedQuery, setDebouncedQuery] = useState("");
  const [creating, setCreating] = useState(false);
  const [newName, setNewName] = useState("");
  const [newPhone, setNewPhone] = useState("");
  const [createError, setCreateError] = useState<string | null>(null);

  useEffect(() => {
    const timer = setTimeout(() => setDebouncedQuery(query), 300);
    return () => clearTimeout(timer);
  }, [query]);

  const { data, isFetching } = useContactsList({ query: debouncedQuery, per_page: 10 });
  const createMutation = useCreateContact();
  const contacts = data?.items ?? [];
  const selected = contacts.find((contact) => contact.id === value);

  function handleCreate() {
    setCreateError(null);
    const parsed = contactInputSchema.safeParse({ full_name: newName, phone: newPhone });
    if (!parsed.success) {
      setCreateError(parsed.error.issues[0]?.message ?? "Dữ liệu không hợp lệ");
      return;
    }
    createMutation.mutate(parsed.data, {
      onSuccess: (contact) => {
        onChange(contact.id, contact);
        setCreating(false);
        setNewName("");
        setNewPhone("");
      },
      onError: (error) => {
        setCreateError(error instanceof ApiError ? error.message : "Không thể tạo người liên hệ");
      },
    });
  }

  if (value && selected) {
    return (
      <div
        id={id}
        className="flex items-center justify-between rounded-[var(--radius-md)] border border-line-200 bg-cream-100 px-3 py-2"
      >
        <div>
          <p className="font-display text-[14px] font-bold text-ink-900">{selected.full_name}</p>
          <p className="text-[13px] text-ink-400">{selected.phone}</p>
        </div>
        <button
          type="button"
          onClick={() => onChange("")}
          className="text-[13px] font-bold text-mint-600 hover:underline"
        >
          Đổi
        </button>
      </div>
    );
  }

  return (
    <div>
      <Input
        id={id}
        role="combobox"
        aria-expanded="true"
        aria-controls={id ? `${id}-listbox` : undefined}
        placeholder="Tìm theo tên hoặc số điện thoại"
        value={query}
        onChange={(event) => setQuery(event.target.value)}
        {...rest}
      />
      <div
        id={id ? `${id}-listbox` : undefined}
        role="listbox"
        aria-label="Người liên hệ"
        className="mt-2 max-h-48 overflow-y-auto rounded-[var(--radius-md)] border border-line-200"
      >
        {isFetching ? <p className="p-3 text-[13px] text-ink-400">Đang tìm…</p> : null}
        {!isFetching && contacts.length === 0 ? (
          <p className="p-3 text-[13px] text-ink-400">Không tìm thấy người liên hệ.</p>
        ) : null}
        {contacts.map((contact) => (
          <button
            key={contact.id}
            type="button"
            role="option"
            aria-selected={contact.id === value}
            onClick={() => onChange(contact.id, contact)}
            className="flex w-full items-center justify-between px-3 py-2 text-left text-[14px] hover:bg-cream-100"
          >
            <span className="font-bold text-ink-900">{contact.full_name}</span>
            <span className="text-ink-400">{contact.phone}</span>
          </button>
        ))}
        <button
          type="button"
          onClick={() => setCreating((current) => !current)}
          className={cn(
            "w-full border-t border-line-200 px-3 py-2 text-left text-[14px] font-bold text-mint-600",
            "hover:bg-cream-100",
          )}
        >
          — Tạo người liên hệ mới —
        </button>
      </div>
      {creating ? (
        <div className="mt-2 space-y-2 rounded-[var(--radius-md)] border border-line-200 p-3">
          <Input
            placeholder="Họ và tên"
            value={newName}
            onChange={(event) => setNewName(event.target.value)}
          />
          <Input
            placeholder="Số điện thoại"
            type="tel"
            inputMode="numeric"
            value={newPhone}
            onChange={(event) => setNewPhone(event.target.value)}
          />
          {createError ? <p className="text-[13px] text-coral-600">{createError}</p> : null}
          <HvButton
            type="button"
            size="sm"
            disabled={createMutation.isPending}
            onClick={handleCreate}
          >
            {createMutation.isPending ? "Đang lưu…" : "Lưu người liên hệ"}
          </HvButton>
        </div>
      ) : null}
    </div>
  );
}
