import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { Link } from "react-router";

import { HvBadge, HvButton, HvModal } from "@/components/hv";
import {
  useMatchZaloFriends,
  useSendZaloFriendRequest,
  ZALO_MATCH_MAX_PHONES,
  type ZaloFriendMatch,
} from "@/features/profile";
import { ApiError } from "@/lib/api/errors";
import { formatPhoneLocal } from "@/lib/utils";

import { listContacts, setContactZaloMapping } from "../api/contacts-api";
import { contactsKeys } from "../hooks/roster-keys";
import type { Contact } from "../schemas/roster-schemas";

/**
 * Pages through the contact list collecting unmapped contacts, stopping once
 * the server-side match cap is exceeded (the caller reports the cut) or the
 * pages run out. per_page 100 is the contacts endpoint's maximum.
 */
async function collectUnmappedContacts(): Promise<{ contacts: Contact[]; capped: boolean }> {
  const unmapped: Contact[] = [];
  // The list is name-sorted with no tiebreaker, so a row can straddle a page
  // boundary and arrive twice; the id set keeps each contact in the scan once.
  const seen = new Set<string>();
  let page = 1;
  for (;;) {
    const res = await listContacts({ page, per_page: 100 });
    for (const contact of res.items) {
      if (!contact.zalo_user_id && !seen.has(contact.id)) {
        seen.add(contact.id);
        unmapped.push(contact);
      }
    }
    if (unmapped.length > ZALO_MATCH_MAX_PHONES || page >= res.meta.total_pages) {
      break;
    }
    page += 1;
  }
  return {
    contacts: unmapped.slice(0, ZALO_MATCH_MAX_PHONES),
    capped: unmapped.length > ZALO_MATCH_MAX_PHONES,
  };
}

interface MatchedRow {
  contact: Contact;
  row: ZaloFriendMatch;
}

/** Per-row friend-request lifecycle; absent means not sent yet. */
type RequestState = "pending" | "sent";

function Avatar({ row }: { row: ZaloFriendMatch }) {
  if (row.avatar) {
    return (
      <img src={row.avatar} alt="" loading="lazy" className="h-8 w-8 rounded-full object-cover" />
    );
  }
  const name = row.display_name ?? row.zalo_name ?? "?";
  return (
    <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-mint-50 font-display text-[13px] font-bold text-mint-600">
      {name.charAt(0)}
    </span>
  );
}

function ContactIdentity({ contact }: { contact: Contact }) {
  return (
    <span className="min-w-0 flex-1">
      <span className="block truncate text-[14px] font-bold text-ink-900">{contact.full_name}</span>
      <span className="block text-[12px] text-ink-400">{formatPhoneLocal(contact.phone)}</span>
    </span>
  );
}

function GroupHeading({ children }: { children: ReactNode }) {
  return (
    <p className="mt-3 mb-1 text-[11px] font-semibold tracking-wide text-ink-400 uppercase first:mt-0">
      {children}
    </p>
  );
}

export interface ZaloAutoMapDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

/**
 * Review dialog for the auto-map flow: one match lookup per open, three row
 * groups (friend / found-but-not-friend / not-found), and a confirm that
 * writes only the rows left checked through the existing zalo-mapping
 * endpoint. Callers mount it only while open, so closing discards all
 * transient state and reopening starts a fresh lookup.
 */
export function ZaloAutoMapDialog({ open, onOpenChange }: ZaloAutoMapDialogProps) {
  const queryClient = useQueryClient();
  const [unchecked, setUnchecked] = useState<ReadonlySet<string>>(new Set());
  const [saving, setSaving] = useState(false);
  const [summary, setSummary] = useState<{ done: number; total: number } | null>(null);
  const [requests, setRequests] = useState<Record<string, RequestState>>({});
  const [requestError, setRequestError] = useState<string | null>(null);

  // gcTime 0: a reopened dialog must re-scan, never show last open's snapshot.
  // The key sits beside contactsKeys.lists() (not under it) so the confirm
  // invalidation cannot re-run the whole paged scan, and focus must not
  // either — a refreshed contact set would desync from the match rows.
  const contactsQuery = useQuery({
    queryKey: [...contactsKeys.all, "zalo-unmapped-scan"],
    queryFn: collectUnmappedContacts,
    gcTime: 0,
    staleTime: 0,
    refetchOnWindowFocus: false,
  });

  const match = useMatchZaloFriends();
  const { mutate: runMatch } = match;
  const contactsData = contactsQuery.data;

  // Aborted from handleOpenChange, not the effect cleanup: StrictMode runs
  // mount→cleanup→mount, and a cleanup abort would kill the one lookup the
  // startedRef guard allows. Every dismiss path funnels through the handler.
  const abortRef = useRef<AbortController | null>(null);
  const startMatch = useCallback(
    (phones: string[]) => {
      const controller = new AbortController();
      abortRef.current = controller;
      runMatch({ phones, signal: controller.signal });
    },
    [runMatch],
  );

  // One lookup per open: the mutation fires once when the contact scan lands.
  // The ref (not effect deps) is the guard — it survives StrictMode's double
  // effect run, so dev mode cannot fire a duplicate paced Zalo lookup.
  const startedRef = useRef(false);
  useEffect(() => {
    if (!contactsData || contactsData.contacts.length === 0 || startedRef.current) {
      return;
    }
    startedRef.current = true;
    startMatch(contactsData.contacts.map((contact) => contact.phone));
  }, [contactsData, startMatch]);

  function handleOpenChange(next: boolean) {
    if (!next) {
      if (saving) {
        // Mapping writes are in flight; dismissing now would hide their outcome.
        return;
      }
      // Ends the server's paced Zalo work for a lookup nobody will read.
      abortRef.current?.abort();
    }
    onOpenChange(next);
  }

  const groups = useMemo(() => {
    if (!contactsData || !match.data) {
      return null;
    }
    // Rows echo the phone exactly as sent, so the join key is the stored phone.
    const byPhone = new Map(match.data.map((row) => [row.phone, row]));
    const matched: MatchedRow[] = [];
    const notFriend: MatchedRow[] = [];
    const notFound: Contact[] = [];
    for (const contact of contactsData.contacts) {
      const row = byPhone.get(contact.phone);
      if (row?.matched && row.user_id) {
        (row.is_friend ? matched : notFriend).push({ contact, row });
      } else {
        notFound.push(contact);
      }
    }
    return { matched, notFriend, notFound };
  }, [contactsData, match.data]);

  const acceptedRows = (groups?.matched ?? []).filter(({ contact }) => !unchecked.has(contact.id));

  function toggleRow(contactId: string) {
    setUnchecked((prev) => {
      const next = new Set(prev);
      if (next.has(contactId)) {
        next.delete(contactId);
      } else {
        next.add(contactId);
      }
      return next;
    });
  }

  async function handleConfirm() {
    setSaving(true);
    let done = 0;
    // Siblings' guardians can share one phone, and the server keeps one
    // contact per Zalo user — a second write for the same user can only 409.
    const writtenUserIds = new Set<string>();
    for (const { contact, row } of acceptedRows) {
      const zaloName = (row.display_name ?? row.zalo_name ?? "").slice(0, 100);
      if (!row.user_id || !zaloName || writtenUserIds.has(row.user_id)) {
        continue;
      }
      try {
        await setContactZaloMapping(contact.id, {
          zalo_user_id: row.user_id,
          zalo_name: zaloName,
        });
        writtenUserIds.add(row.user_id);
        done += 1;
      } catch (error) {
        // A failed row stays unmapped; the summary reports the split and a
        // later re-run is idempotent for the rows that did land. A dead app
        // session is the one failure every remaining write shares.
        if (error instanceof ApiError && error.status === 401) {
          break;
        }
      }
    }
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: contactsKeys.lists() }),
      queryClient.invalidateQueries({ queryKey: contactsKeys.details() }),
    ]);
    setSaving(false);
    setSummary({ done, total: acceptedRows.length });
  }

  const sendRequest = useSendZaloFriendRequest();

  function handleSendRequest(userId: string) {
    setRequestError(null);
    setRequests((prev) => ({ ...prev, [userId]: "pending" }));
    sendRequest.mutate(userId, {
      onSuccess: () => setRequests((prev) => ({ ...prev, [userId]: "sent" })),
      onError: () => {
        setRequests((prev) => {
          const next = { ...prev };
          delete next[userId];
          return next;
        });
        setRequestError("Không gửi được lời mời. Thử lại.");
      },
    });
  }

  // Like the manual picker: a 409/404 means the stored session died between
  // the status read and this lookup — only a re-scan on the profile page helps.
  const sessionGone =
    match.isError &&
    match.error instanceof ApiError &&
    (match.error.status === 409 || match.error.status === 404);

  let body: ReactNode;
  let footer: ReactNode = null;

  if (summary) {
    body = (
      <div className="flex flex-col gap-1">
        <p className="text-[14px] font-bold text-ink-900">
          Đã ghép {summary.done}/{summary.total}
        </p>
        {summary.done < summary.total ? (
          <p className="text-[13px] text-ink-400">Các dòng còn lại chưa được ghép.</p>
        ) : null}
      </div>
    );
    footer = <HvButton onClick={() => handleOpenChange(false)}>Đóng</HvButton>;
  } else if (contactsQuery.isError) {
    body = (
      <div className="flex flex-col items-start gap-2">
        <p className="text-[13px] text-ink-400">Không tải được danh sách người liên hệ.</p>
        <HvButton size="sm" variant="ghost" onClick={() => void contactsQuery.refetch()}>
          Thử lại
        </HvButton>
      </div>
    );
  } else if (sessionGone) {
    body = (
      <p className="text-[13px] text-ink-400">
        Phiên Zalo không còn hiệu lực.{" "}
        <Link to="/profile" className="font-bold text-mint-600 underline-offset-4 hover:underline">
          Quét lại mã
        </Link>
      </p>
    );
  } else if (match.isError) {
    body = (
      <div className="flex flex-col items-start gap-2">
        <p className="text-[13px] text-ink-400">Không đối chiếu được với Zalo.</p>
        <HvButton
          size="sm"
          variant="ghost"
          disabled={match.isPending}
          onClick={() => startMatch((contactsData?.contacts ?? []).map((contact) => contact.phone))}
        >
          Thử lại
        </HvButton>
      </div>
    );
  } else if (contactsData?.contacts.length === 0) {
    body = <p className="text-[13px] text-ink-400">Tất cả người liên hệ đã được ghép.</p>;
  } else if (!groups) {
    body = <p className="text-[13px] text-ink-400">Đang đối chiếu với danh bạ Zalo…</p>;
  } else {
    body = (
      <div className="max-h-[55vh] overflow-y-auto">
        {contactsData?.capped ? (
          <p className="mb-2 rounded-[var(--radius-md)] bg-cream-100 p-2 text-[12.5px] text-ink-500">
            Danh sách quá dài — chỉ tìm {ZALO_MATCH_MAX_PHONES} người liên hệ đầu tiên. Ghép xong
            nhóm này rồi chạy lại nhé.
          </p>
        ) : null}

        {groups.matched.length > 0 ? (
          <>
            <GroupHeading>Bạn bè Zalo — {groups.matched.length}</GroupHeading>
            <ul className="flex flex-col">
              {groups.matched.map(({ contact, row }) => (
                <li
                  key={contact.id}
                  className="flex items-center gap-3 border-b border-line-200 py-2 last:border-b-0"
                >
                  <input
                    type="checkbox"
                    aria-label={`Ghép ${contact.full_name}`}
                    checked={!unchecked.has(contact.id)}
                    disabled={saving}
                    onChange={() => toggleRow(contact.id)}
                    className="size-4 shrink-0 accent-mint-600"
                  />
                  <ContactIdentity contact={contact} />
                  <span className="flex items-center gap-2">
                    <Avatar row={row} />
                    <span className="max-w-28 truncate text-[13px] font-bold text-ink-700">
                      {row.display_name ?? row.zalo_name}
                    </span>
                  </span>
                </li>
              ))}
            </ul>
          </>
        ) : null}

        {groups.notFriend.length > 0 ? (
          <>
            <GroupHeading>Có Zalo nhưng chưa kết bạn — {groups.notFriend.length}</GroupHeading>
            <ul className="flex flex-col">
              {groups.notFriend.map(({ contact, row }) => {
                const state = row.user_id ? requests[row.user_id] : undefined;
                return (
                  <li
                    key={contact.id}
                    className="flex items-center gap-3 border-b border-line-200 py-2 last:border-b-0"
                  >
                    <ContactIdentity contact={contact} />
                    <HvBadge variant="warning" size="sm">
                      Chưa kết bạn
                    </HvBadge>
                    <HvButton
                      size="sm"
                      variant="secondary"
                      disabled={state === "pending" || state === "sent"}
                      onClick={() => row.user_id && handleSendRequest(row.user_id)}
                    >
                      {state === "sent" ? "Đã gửi" : state === "pending" ? "Đang gửi…" : "Kết bạn"}
                    </HvButton>
                  </li>
                );
              })}
            </ul>
          </>
        ) : null}

        {groups.notFound.length > 0 ? (
          <>
            <GroupHeading>Không tìm thấy trên Zalo — {groups.notFound.length}</GroupHeading>
            <ul className="flex flex-col">
              {groups.notFound.map((contact) => (
                <li
                  key={contact.id}
                  className="flex items-center gap-3 border-b border-line-200 py-2 last:border-b-0"
                >
                  <ContactIdentity contact={contact} />
                  <span className="text-[12.5px] text-ink-400">Không tìm thấy</span>
                </li>
              ))}
            </ul>
          </>
        ) : null}

        {requestError ? <p className="mt-2 text-[13px] text-coral-600">{requestError}</p> : null}
      </div>
    );
    footer = (
      <>
        <HvButton variant="ghost" disabled={saving} onClick={() => handleOpenChange(false)}>
          Huỷ
        </HvButton>
        <HvButton
          disabled={saving || acceptedRows.length === 0}
          onClick={() => void handleConfirm()}
        >
          {saving ? "Đang ghép…" : `Ghép ${acceptedRows.length} đã chọn`}
        </HvButton>
      </>
    );
  }

  return (
    <HvModal
      open={open}
      onOpenChange={handleOpenChange}
      title="Tự động ghép Zalo"
      description="Đối chiếu số điện thoại người liên hệ với danh bạ Zalo của bạn."
      footer={footer}
      className="sm:max-w-lg"
    >
      {body}
    </HvModal>
  );
}
