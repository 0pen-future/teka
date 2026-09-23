import { Trash2Icon } from "lucide-react";
import { useState } from "react";

import { HvButton, HvNotice, HvStateBlock, hvToast } from "@/components/hv";
import { useAuthStore } from "@/features/auth";
import { ApiError } from "@/lib/api/errors";
import { cn, formatDateTime } from "@/lib/utils";

import {
  useClassMessages,
  useDeleteClassMessage,
  usePostClassMessage,
} from "../hooks/use-class-chat";
import { CLASS_MESSAGE_MAX } from "../schemas/class-program-schemas";
import type { Class } from "../schemas/roster-schemas";

const textareaClassName = cn(
  "min-h-20 w-full rounded-lg border border-input bg-transparent px-2.5 py-2 text-base transition-colors outline-none",
  "placeholder:text-muted-foreground focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50",
  "aria-invalid:border-destructive aria-invalid:ring-3 aria-invalid:ring-destructive/20 md:text-sm",
);

const TOO_LONG = `Tối đa ${CLASS_MESSAGE_MAX} ký tự`;

interface ClassChatPanelProps {
  klass: Class;
  canPost: boolean;
  isOwner: boolean;
}

/**
 * The "Chat" tab: the class's internal thread for its teaching team. Read
 * access follows the API (active stint or owner); posting needs the
 * `class_messages.post` permission; deleting is author-or-owner.
 */
export function ClassChatPanel({ klass, canPost, isOwner }: ClassChatPanelProps) {
  const selfId = useAuthStore((state) => state.user)?.id ?? null;
  const messages = useClassMessages(klass.id);
  const post = usePostClassMessage(klass.id);
  const remove = useDeleteClassMessage(klass.id);
  const [draft, setDraft] = useState("");

  // Pages arrive newest first; the thread reads oldest first.
  const thread = (messages.data?.pages ?? []).flatMap((page) => page.items).reverse();
  const tooLong = draft.length > CLASS_MESSAGE_MAX;
  const canSend = draft.trim().length > 0 && !tooLong && !post.isPending;

  function send() {
    post.mutate(draft.trim(), {
      onSuccess: () => setDraft(""),
      onError: (error) =>
        hvToast(error instanceof ApiError ? error.message : "Không gửi được tin nhắn.", {
          variant: "danger",
        }),
    });
  }

  function deleteMessage(messageId: string) {
    remove.mutate(messageId, {
      onError: (error) =>
        hvToast(error instanceof ApiError ? error.message : "Không xoá được tin nhắn.", {
          variant: "danger",
        }),
    });
  }

  if (messages.isPending) {
    return <HvStateBlock state="loading" title="Đang tải tin nhắn" />;
  }
  if (messages.isError) {
    const reason =
      messages.error instanceof ApiError
        ? messages.error.message
        : "Không tải được tin nhắn. Thử lại sau.";
    return <HvStateBlock state="error" title="Không mở được chat lớp" description={reason} />;
  }

  return (
    <div className="flex flex-col gap-3">
      <HvNotice tone="info">Tin nhắn nội bộ, không đồng bộ Zalo</HvNotice>

      {messages.hasNextPage ? (
        <HvButton
          size="sm"
          variant="ghost"
          disabled={messages.isFetchingNextPage}
          onClick={() => void messages.fetchNextPage()}
        >
          Tải tin cũ hơn
        </HvButton>
      ) : null}

      {thread.length === 0 ? (
        <p className="text-[13px] text-ink-400">Chưa có tin nhắn nào.</p>
      ) : (
        <ul className="flex flex-col gap-2">
          {thread.map((message) => {
            const canDelete = isOwner || message.author_id === selfId;
            return (
              <li
                key={message.id}
                className="flex items-start justify-between gap-3 rounded-[var(--radius-md)] border border-line-200 bg-white p-3"
              >
                <div className="min-w-0 flex-1">
                  <p className="flex flex-wrap items-baseline gap-x-2 text-[13px]">
                    <span className="font-bold text-ink-900">{message.author_name}</span>
                    <span className="text-ink-400">{formatDateTime(message.created_at)}</span>
                  </p>
                  <p className="mt-1 text-[14px] whitespace-pre-wrap text-ink-900">
                    {message.body}
                  </p>
                </div>
                {canDelete ? (
                  <button
                    type="button"
                    aria-label="Xoá tin nhắn"
                    disabled={remove.isPending}
                    onClick={() => deleteMessage(message.id)}
                    className="shrink-0 rounded-md p-1 text-ink-400 hover:text-coral-600 disabled:opacity-50"
                  >
                    <Trash2Icon aria-hidden="true" className="size-4" />
                  </button>
                ) : null}
              </li>
            );
          })}
        </ul>
      )}

      {canPost ? (
        <form
          className="flex flex-col gap-2"
          onSubmit={(event) => {
            event.preventDefault();
            if (canSend) send();
          }}
        >
          <textarea
            aria-label="Nội dung tin nhắn"
            aria-invalid={tooLong}
            className={textareaClassName}
            value={draft}
            placeholder="Nhắn cho đội ngũ lớp…"
            onChange={(event) => setDraft(event.target.value)}
          />
          {tooLong ? <p className="text-[13px] text-coral-600">{TOO_LONG}</p> : null}
          <div className="flex justify-end">
            <HvButton size="sm" type="submit" disabled={!canSend}>
              Gửi
            </HvButton>
          </div>
        </form>
      ) : (
        <p className="text-[13px] text-ink-400">Bạn không có quyền nhắn tin trong lớp này.</p>
      )}
    </div>
  );
}
