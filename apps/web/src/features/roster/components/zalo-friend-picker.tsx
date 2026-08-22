import { useMemo, useState, type ReactNode } from "react";
import { Link } from "react-router";

import { HvButton, HvModal } from "@/components/hv";
import { Input } from "@/components/ui/input";
import { useZaloFriends, type ZaloFriend } from "@/features/profile";
import { ApiError } from "@/lib/api/errors";

import { useSetContactZaloMapping } from "../hooks/use-contacts";

/**
 * Teachers search friend names with and without diacritics interchangeably,
 * so both sides of the match are folded: NFD strips combining marks, and đ→d
 * is handled separately because NFD does not decompose it.
 */
function foldVietnamese(value: string): string {
  return value
    .normalize("NFD")
    .replace(/[̀-ͯ]/g, "")
    .replace(/đ/g, "d")
    .replace(/Đ/g, "d")
    .toLowerCase();
}

/**
 * A Zalo account can hold ~2000 friends; rendering them all (with avatars)
 * on one open would flood a mobile connection. The teacher narrows by typing
 * instead of scrolling past this cap.
 */
const FRIEND_RENDER_CAP = 100;

export interface ZaloFriendPickerProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Contact the picked friend gets mapped onto. */
  contactId: string;
}

/**
 * Modal listing the teacher's Zalo friends for mapping onto a contact. The
 * friend list only loads while the modal is open, and no debounce is needed —
 * filtering is client-side over the already-fetched list. Callers mount it
 * only while open (the contact detail page does), so closing discards
 * transient query/mutation state.
 */
export function ZaloFriendPicker({ open, onOpenChange, contactId }: ZaloFriendPickerProps) {
  const [query, setQuery] = useState("");
  const [mapError, setMapError] = useState<string | null>(null);
  const { data: friends, isPending, isError, error, refetch } = useZaloFriends(open);
  const mapping = useSetContactZaloMapping(contactId);

  const foldedFriends = useMemo(
    () =>
      (friends ?? []).map((friend) => ({ friend, folded: foldVietnamese(friend.display_name) })),
    [friends],
  );
  const foldedQuery = foldVietnamese(query.trim());
  const matches = foldedFriends.filter((entry) => entry.folded.includes(foldedQuery));
  const visible = matches.slice(0, FRIEND_RENDER_CAP);

  function handlePick(friend: ZaloFriend) {
    setMapError(null);
    mapping.mutate(
      { zalo_user_id: friend.user_id, zalo_name: friend.display_name },
      {
        onSuccess: () => onOpenChange(false),
        onError: (mutationError) => {
          setMapError(
            mutationError instanceof ApiError && mutationError.status === 409
              ? "Bạn này đã được liên kết với người liên hệ khác."
              : "Không thể liên kết. Thử lại sau.",
          );
        },
      },
    );
  }

  // The status card gates the picker, but its answer can be a staleTime old —
  // the session may have died in between. Retrying cannot fix that; only a
  // re-scan on the profile page can.
  const sessionGone =
    isError && error instanceof ApiError && (error.status === 409 || error.status === 404);

  let body: ReactNode;
  if (sessionGone) {
    body = (
      <p className="text-[13px] text-ink-400">
        Phiên Zalo không còn hiệu lực.{" "}
        <Link to="/profile" className="font-bold text-mint-600 underline-offset-4 hover:underline">
          Quét lại mã
        </Link>
      </p>
    );
  } else if (isError) {
    body = (
      <div className="flex flex-col items-start gap-2">
        <p className="text-[13px] text-ink-400">Không tải được danh sách bạn bè.</p>
        <HvButton size="sm" variant="ghost" onClick={() => void refetch()}>
          Thử lại
        </HvButton>
      </div>
    );
  } else {
    body = (
      <div>
        <Input
          role="combobox"
          aria-expanded="true"
          aria-controls="zalo-friend-listbox"
          aria-label="Tìm bạn Zalo"
          placeholder="Tìm theo tên"
          value={query}
          onChange={(event) => setQuery(event.target.value)}
        />
        <div
          id="zalo-friend-listbox"
          role="listbox"
          aria-label="Bạn Zalo"
          className="mt-2 max-h-64 overflow-y-auto rounded-[var(--radius-md)] border border-line-200"
        >
          {isPending ? (
            <p className="p-3 text-[13px] text-ink-400">Đang tải danh sách bạn bè…</p>
          ) : null}
          {!isPending && foldedFriends.length === 0 ? (
            <p className="p-3 text-[13px] text-ink-400">
              Tài khoản Zalo của bạn chưa có bạn bè nào.
            </p>
          ) : null}
          {!isPending && foldedFriends.length > 0 && matches.length === 0 ? (
            <p className="p-3 text-[13px] text-ink-400">Không tìm thấy bạn nào.</p>
          ) : null}
          {visible.map(({ friend }) => (
            <button
              key={friend.user_id}
              type="button"
              role="option"
              aria-selected={false}
              disabled={mapping.isPending}
              onClick={() => handlePick(friend)}
              className="flex w-full items-center gap-3 px-3 py-2 text-left text-[14px] hover:bg-cream-100 disabled:opacity-60"
            >
              {friend.avatar ? (
                <img
                  src={friend.avatar}
                  alt=""
                  loading="lazy"
                  className="h-8 w-8 rounded-full object-cover"
                />
              ) : (
                <span className="flex h-8 w-8 items-center justify-center rounded-full bg-mint-50 font-display text-[13px] font-bold text-mint-600">
                  {friend.display_name.charAt(0)}
                </span>
              )}
              <span className="font-bold text-ink-900">{friend.display_name}</span>
            </button>
          ))}
          {matches.length > FRIEND_RENDER_CAP ? (
            <p className="border-t border-line-200 p-3 text-[13px] text-ink-400">
              Đang hiển thị {FRIEND_RENDER_CAP} bạn đầu tiên. Gõ tên để thu hẹp.
            </p>
          ) : null}
        </div>
        {mapError ? <p className="mt-2 text-[13px] text-coral-600">{mapError}</p> : null}
      </div>
    );
  }

  return (
    <HvModal
      open={open}
      onOpenChange={onOpenChange}
      title="Chọn bạn Zalo"
      description="Tin nhắn học phí của người liên hệ này sẽ gửi tới bạn Zalo được chọn."
    >
      {body}
    </HvModal>
  );
}
