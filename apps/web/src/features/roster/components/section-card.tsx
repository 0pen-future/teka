import { useId } from "react";

import { HvBadge, HvCard } from "@/components/hv";

import type { ClassStaff } from "../schemas/roster-schemas";

/** A titled region card on the class detail's "Thông tin" tab, with an optional header action. */
export function SectionCard({
  title,
  action,
  children,
}: {
  title: string;
  action?: React.ReactNode;
  children: React.ReactNode;
}) {
  const headingId = useId();
  return (
    <HvCard role="region" aria-labelledby={headingId}>
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h2 id={headingId} className="font-display text-[16px] font-bold text-ink-900">
          {title}
        </h2>
        {action}
      </div>
      <div className="mt-3">{children}</div>
    </HvCard>
  );
}

/** Name + role badge per stint, with the shared loading / error / empty copy. */
export function StaffList({
  staff,
  isPending,
  isError,
  emptyText,
}: {
  staff: ClassStaff[];
  isPending: boolean;
  isError: boolean;
  emptyText: string;
}) {
  if (isPending) {
    return <p className="text-[13px] text-ink-400">Đang tải…</p>;
  }
  if (isError) {
    return <p className="text-[13px] text-coral-600">Không tải được nhân sự lớp.</p>;
  }
  if (staff.length === 0) {
    return <p className="text-[13px] text-ink-400">{emptyText}</p>;
  }
  return (
    <ul className="flex flex-col gap-2">
      {staff.map((item) => (
        <li key={item.id} className="flex items-center justify-between gap-2 text-[14px]">
          <span className="font-bold text-ink-900">{item.teacher_name}</span>
          <HvBadge variant="neutral" size="sm">
            {item.role_label}
          </HvBadge>
        </li>
      ))}
    </ul>
  );
}
