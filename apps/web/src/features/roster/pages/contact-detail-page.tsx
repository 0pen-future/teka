import { useState, type ReactNode } from "react";
import { Link, useParams } from "react-router";

import { HvBadge, HvButton, HvCard, hvToast } from "@/components/hv";
import { ConfirmDialog } from "@/components/shared/confirm-dialog";
import { useZaloStatus } from "@/features/profile";
import { useCenterContext } from "@/features/teaching";
import { formatPhoneLocal } from "@/lib/utils";

import { ContactDialog } from "../components/contact-dialog";
import { StudentDialog } from "../components/student-dialog";
import { ZaloFriendPicker } from "../components/zalo-friend-picker";
import { useClearContactZaloMapping, useContact } from "../hooks/use-contacts";
import { useStudentsList } from "../hooks/use-students";
import type { Contact } from "../schemas/roster-schemas";

/**
 * The contact's Zalo mapping: which friend receives this family's fee
 * notifications. Every state that cannot pick a friend (unlinked, expired)
 * routes to the profile page instead of opening the picker, so the friends
 * API — a live call from the teacher's Zalo account — is never hit while it
 * cannot succeed.
 */
function ContactZaloCard({ contact }: { contact: Contact }) {
  const { data: status, isPending, isError, refetch } = useZaloStatus();
  const [pickerOpen, setPickerOpen] = useState(false);
  const [confirmOpen, setConfirmOpen] = useState(false);
  const clearMapping = useClearContactZaloMapping(contact.id);

  let body: ReactNode;
  if (isPending) {
    body = <p className="text-[13px] text-ink-400">Đang tải…</p>;
  } else if (isError) {
    // An unreadable status must not be drawn as "not connected": a linked
    // teacher would be sent to the profile page to link a second time.
    body = (
      <div className="flex flex-wrap items-center justify-between gap-2">
        <p className="text-[13px] text-ink-400">Không tải được trạng thái Zalo.</p>
        <HvButton variant="ghost" size="sm" onClick={() => void refetch()}>
          Thử lại
        </HvButton>
      </div>
    );
  } else if (!status?.linked) {
    body = (
      <p className="text-[13px] text-ink-400">
        Chưa kết nối Zalo để gửi thông báo học phí.{" "}
        <Link to="/profile" className="font-bold text-mint-600 underline-offset-4 hover:underline">
          Kết nối Zalo trước
        </Link>
      </p>
    );
  } else if (status.status === "expired") {
    body = (
      <p className="text-[13px] text-ink-400">
        Phiên Zalo đã hết hạn.{" "}
        <Link to="/profile" className="font-bold text-mint-600 underline-offset-4 hover:underline">
          Quét lại mã
        </Link>
      </p>
    );
  } else if (contact.zalo_name) {
    body = (
      <div className="flex flex-wrap items-center justify-between gap-2">
        <HvBadge variant="success" dot>
          {contact.zalo_name}
        </HvBadge>
        <HvButton variant="ghost" size="sm" onClick={() => setConfirmOpen(true)}>
          Bỏ liên kết
        </HvButton>
      </div>
    );
  } else {
    body = (
      <div className="flex flex-wrap items-center justify-between gap-2">
        <p className="text-[13px] text-ink-400">Chưa liên kết bạn Zalo nào.</p>
        <HvButton size="sm" onClick={() => setPickerOpen(true)}>
          Chọn bạn Zalo
        </HvButton>
      </div>
    );
  }

  return (
    <HvCard className="flex flex-col gap-2">
      <h2 className="font-display text-[16px] font-bold text-ink-900">Zalo</h2>
      {body}
      {/* Mounted only while open so a closed picker keeps no stale query/mutation state. */}
      {pickerOpen ? (
        <ZaloFriendPicker open={pickerOpen} onOpenChange={setPickerOpen} contactId={contact.id} />
      ) : null}
      <ConfirmDialog
        open={confirmOpen}
        onOpenChange={setConfirmOpen}
        title="Bỏ liên kết Zalo?"
        description={`Thông báo học phí sẽ không gửi được cho ${contact.full_name} qua Zalo nữa.`}
        confirmLabel="Bỏ liên kết"
        destructive
        pending={clearMapping.isPending}
        onConfirm={() =>
          clearMapping.mutate(undefined, {
            onSuccess: () => setConfirmOpen(false),
            onError: () => hvToast("Không thể bỏ liên kết. Thử lại sau.", { variant: "danger" }),
          })
        }
      />
    </HvCard>
  );
}

/** Contact header (tap-to-call) plus the list of children linked to it. */
export function ContactDetailPage() {
  const { id } = useParams<{ id: string }>();
  const { data: contact, isPending } = useContact(id);
  const { data: studentsPage } = useStudentsList({ contact_id: id, per_page: 50 });
  const students = studentsPage?.items ?? [];
  const [editOpen, setEditOpen] = useState(false);
  const [addStudentOpen, setAddStudentOpen] = useState(false);
  // Contact and student records are owner-managed; a reader who is not the
  // owner (an oversight secretary) sees the data without the edit controls.
  const { isOwner } = useCenterContext();

  if (isPending) {
    return <p className="text-[13px] text-ink-400">Đang tải…</p>;
  }

  if (!contact) {
    return <p className="text-[13px] text-ink-400">Không tìm thấy người liên hệ.</p>;
  }

  return (
    <div className="flex flex-col gap-4">
      <HvCard className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="font-display text-[22px] font-bold text-ink-900">{contact.full_name}</h1>
          <a
            href={`tel:${contact.phone}`}
            className="mt-1 inline-block text-[14px] font-bold text-mint-600 underline-offset-4 hover:underline"
          >
            {formatPhoneLocal(contact.phone)}
          </a>
        </div>
        {isOwner ? (
          <HvButton variant="ghost" size="sm" onClick={() => setEditOpen(true)}>
            Sửa
          </HvButton>
        ) : null}
      </HvCard>

      <ContactZaloCard contact={contact} />

      <div className="flex items-center justify-between">
        <h2 className="font-display text-[16px] font-bold text-ink-900">Học sinh</h2>
        {isOwner ? (
          <HvButton size="sm" onClick={() => setAddStudentOpen(true)}>
            Thêm học sinh
          </HvButton>
        ) : null}
      </div>
      <div className="flex flex-col gap-2">
        {students.map((student) => (
          <Link key={student.id} to={`/students/${student.id}`}>
            <HvCard variant="flat" interactive className="flex items-center justify-between">
              <p className="font-display text-[15px] font-bold text-ink-900">{student.full_name}</p>
              {student.display_note ? (
                <HvBadge variant="info">{student.display_note}</HvBadge>
              ) : null}
            </HvCard>
          </Link>
        ))}
        {students.length === 0 ? (
          <p className="text-[13px] text-ink-400">Người liên hệ này chưa có học sinh nào.</p>
        ) : null}
      </div>

      <ContactDialog open={editOpen} onOpenChange={setEditOpen} contact={contact} />
      <StudentDialog
        open={addStudentOpen}
        onOpenChange={setAddStudentOpen}
        defaultContactId={contact.id}
      />
    </div>
  );
}
