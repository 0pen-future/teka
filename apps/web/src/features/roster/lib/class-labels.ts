import type { HvSelectOption } from "@/components/hv";
import type { HvBadgeVariant } from "@/components/hv";

import type { ClassShift } from "../api/classes-api";
import type { ClassPhase } from "../schemas/roster-schemas";
import { formatWeekday } from "./roster-format";

/**
 * Vietnamese copy for the lifecycle bucket the API reports on every class.
 * Labels only — the phase itself is never derived here (the server owns the
 * date arithmetic so list filters, stats and detail always agree).
 */
export const phaseLabel: Record<ClassPhase, string> = {
  upcoming: "Sắp khai giảng",
  running: "Đang học",
  ended: "Đã kết thúc",
  archived: "Lưu trữ",
};

/** Chip/badge tint per phase, shared by the list rows and the detail header. */
export const phaseVariant: Record<ClassPhase, HvBadgeVariant> = {
  upcoming: "info",
  running: "success",
  ended: "neutral",
  archived: "warning",
};

/** Monday-first weekday filter options; `""` is the unfiltered choice. */
export const weekdayOptions: HvSelectOption[] = [
  { value: "", label: "Tất cả các ngày" },
  ...[1, 2, 3, 4, 5, 6, 0].map((weekday) => ({
    value: String(weekday),
    label: formatWeekday(weekday),
  })),
];

export const shiftLabel: Record<ClassShift, string> = {
  morning: "Sáng",
  afternoon: "Chiều",
  evening: "Tối",
};

/** Shift filter options; `""` is the unfiltered choice. */
export const shiftOptions: HvSelectOption[] = [
  { value: "", label: "Tất cả ca" },
  ...(Object.keys(shiftLabel) as ClassShift[]).map((shift) => ({
    value: shift,
    label: shiftLabel[shift],
  })),
];
