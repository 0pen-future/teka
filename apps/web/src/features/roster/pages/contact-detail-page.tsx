import { useState } from "react";
import { Link, useParams } from "react-router";

import { HvBadge, HvButton, HvCard } from "@/components/hv";
import { formatPhoneLocal } from "@/lib/utils";

import { ContactDialog } from "../components/contact-dialog";
import { StudentDialog } from "../components/student-dialog";
import { useContact } from "../hooks/use-contacts";
import { useStudentsList } from "../hooks/use-students";

/** Contact header (tap-to-call) plus the list of children linked to it. */
export function ContactDetailPage() {
  const { id } = useParams<{ id: string }>();
  const { data: contact, isPending } = useContact(id);
  const { data: studentsPage } = useStudentsList({ contact_id: id, per_page: 50 });
  const students = studentsPage?.items ?? [];
  const [editOpen, setEditOpen] = useState(false);
  const [addStudentOpen, setAddStudentOpen] = useState(false);

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
        <HvButton variant="ghost" size="sm" onClick={() => setEditOpen(true)}>
          Sửa
        </HvButton>
      </HvCard>

      <div className="flex items-center justify-between">
        <h2 className="font-display text-[16px] font-bold text-ink-900">Học sinh</h2>
        <HvButton size="sm" onClick={() => setAddStudentOpen(true)}>
          Thêm học sinh
        </HvButton>
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
