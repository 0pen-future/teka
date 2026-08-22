import { useEffect, useRef, useState } from "react";

import { HvBadge, HvButton, HvModal, type HvBadgeVariant } from "@/components/hv";
import { Input } from "@/components/ui/input";
import { copyToClipboard, formatPhoneLocal } from "@/lib/utils";

import type { CreateInviteResponse } from "../schemas/invitation-schemas";

export interface CopyLinkDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  invite: CreateInviteResponse;
}

const DM_STATUS_COPY: Record<
  CreateInviteResponse["dm_status"],
  { label: string; variant: HvBadgeVariant }
> = {
  sent: { label: "Đã gửi qua Zalo", variant: "success" },
  skipped: { label: "Chưa gửi qua Zalo", variant: "neutral" },
  failed: { label: "Gửi Zalo thất bại", variant: "danger" },
};

/**
 * Shown right after an invite is created. The link works regardless of the
 * Zalo delivery outcome, so the badge is informational, not an error — a
 * "failed" or "skipped" status is a nudge to share the link another way
 * (chat, SMS), not something to retry. The link stays in a plain readonly
 * `<input>` (not a styled chip) so it is a real, selectable value the
 * clipboard-fallback textarea path and Playwright can both read directly.
 */
export function CopyLinkDialog({ open, onOpenChange, invite }: CopyLinkDialogProps) {
  const [copied, setCopied] = useState(false);
  const timeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    return () => {
      if (timeoutRef.current) {
        clearTimeout(timeoutRef.current);
      }
    };
  }, []);

  async function handleCopy() {
    const succeeded = await copyToClipboard(invite.link);
    if (succeeded) {
      setCopied(true);
      timeoutRef.current = setTimeout(() => setCopied(false), 2000);
    }
  }

  const dmStatus = DM_STATUS_COPY[invite.dm_status];

  return (
    <HvModal
      open={open}
      onOpenChange={onOpenChange}
      title="Đã tạo lời mời"
      description={`Gửi tới ${formatPhoneLocal(invite.phone)}`}
      footer={
        <HvButton type="button" variant="ghost" onClick={() => onOpenChange(false)}>
          Đóng
        </HvButton>
      }
    >
      <div className="flex flex-col gap-3">
        <HvBadge variant={dmStatus.variant} size="sm" className="self-start">
          {dmStatus.label}
        </HvBadge>
        <div className="flex items-center gap-2">
          <Input
            readOnly
            value={invite.link}
            aria-label="Liên kết mời"
            onFocus={(event) => event.target.select()}
          />
          <HvButton
            type="button"
            variant="secondary"
            size="sm"
            className="min-w-[44px] shrink-0"
            onClick={() => void handleCopy()}
            aria-label={copied ? "Đã sao chép liên kết" : "Sao chép liên kết"}
          >
            {copied ? "Đã chép" : "Chép"}
          </HvButton>
        </div>
      </div>
    </HvModal>
  );
}
