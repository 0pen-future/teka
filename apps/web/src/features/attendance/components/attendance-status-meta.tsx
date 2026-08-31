import type { LucideProps } from "lucide-react";
import type { ComponentType } from "react";

import { HvCheckIcon, HvClockIcon, HvFileIcon, HvXIcon } from "@/components/hv";

import type { AttendanceRow } from "../schemas/attendance-schemas";

export type AttendanceStatus = NonNullable<AttendanceRow["status"]>;

/** The wire form of an exception — `present` is the default, never sent. */
export type AttendanceMarkStatus = Exclude<AttendanceStatus, "present">;

export interface AttendanceStatusMeta {
  value: AttendanceStatus;
  /** Vietnamese label — the radio's accessible name and the column title. */
  label: string;
  icon: ComponentType<LucideProps>;
  /** Column-title / chip ink on light backgrounds. */
  inkClass: string;
  /** Count chip pill (soft bg + ink). */
  chipClass: string;
  /** Selected radio cell: solid fill + the DS's pressed drop shadow. */
  selectedClass: string;
  /** Row background tint while this status is selected. */
  rowTintClass: string;
}

/**
 * Single source of truth for the 4-column sheet: header titles, count chips,
 * and each row's radio cells all read from here so the columns can never
 * disagree on order, label, or color. All colors are DS tokens (`colors.css`
 * / `effects.css`) — no hex in components.
 */
export const ATTENDANCE_STATUSES: readonly AttendanceStatusMeta[] = [
  {
    value: "present",
    label: "Đúng giờ",
    icon: HvCheckIcon,
    inkClass: "text-mint-600",
    chipClass: "bg-mint-50 text-mint-600",
    selectedClass: "bg-mint-400 text-white shadow-[0_3px_0_var(--mint-500)]",
    rowTintClass: "bg-cream-50",
  },
  {
    value: "late",
    label: "Muộn",
    icon: HvClockIcon,
    inkClass: "text-sun-600",
    chipClass: "bg-sun-100 text-sun-600",
    selectedClass: "bg-sun-400 text-sun-600 shadow-[0_3px_0_var(--sun-500)]",
    rowTintClass: "bg-sun-100",
  },
  {
    value: "absent",
    label: "Vắng",
    icon: HvXIcon,
    inkClass: "text-coral-600",
    chipClass: "bg-coral-100 text-coral-600",
    selectedClass: "bg-coral-400 text-white shadow-[0_3px_0_var(--coral-500)]",
    rowTintClass: "bg-coral-100",
  },
  {
    value: "excused",
    label: "Có lý do",
    icon: HvFileIcon,
    inkClass: "text-sky-500",
    chipClass: "bg-sky-50 text-sky-500",
    selectedClass: "bg-sky-300 text-white shadow-[0_3px_0_var(--sky-400)]",
    rowTintClass: "bg-sky-50",
  },
];

/**
 * Shared grid template for the header row and every student row — one name
 * column that can shrink (`minmax(0,1fr)` keeps long names truncating instead
 * of overflowing 390px) plus four fixed 44px touch-target columns.
 */
export const attendanceGridClass =
  "grid grid-cols-[minmax(0,1fr)_44px_44px_44px_44px] items-center gap-x-[6px]";
