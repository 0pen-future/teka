import { useState } from "react";

import { HvButton } from "@/components/hv";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import type { CenterMember } from "@/features/center";

import type { AuditLogFilters } from "../schemas/audit-schemas";

/** Action prefixes actually recorded by the API's action map. */
const ACTION_GROUPS = [
  { value: "auth.", label: "Tài khoản" },
  { value: "teacher.", label: "Hồ sơ giáo viên" },
  { value: "attendance.", label: "Điểm danh" },
  { value: "class.", label: "Lớp học" },
  { value: "student.", label: "Học sinh" },
  { value: "enrollment.", label: "Ghi danh" },
  { value: "session.", label: "Buổi học" },
  { value: "lesson_plan.", label: "Giáo án" },
  { value: "curriculum.", label: "Chương trình" },
  { value: "payment.", label: "Thanh toán" },
  { value: "billing.", label: "Hóa đơn" },
  { value: "notification.", label: "Thông báo" },
  { value: "statement.", label: "Sao kê" },
  { value: "contact.", label: "Phụ huynh" },
  { value: "invitation.", label: "Lời mời" },
  { value: "center.", label: "Trung tâm" },
  { value: "import.", label: "Nhập liệu" },
  { value: "zalo.", label: "Zalo" },
];

interface AuditFiltersProps {
  members: CenterMember[];
  filters: AuditLogFilters;
  onChange: (filters: AuditLogFilters) => void;
}

/**
 * Server-side filters for the audit trail. The free-text action filter only
 * applies on submit (Enter) so typing does not fire a request per keystroke;
 * the group select and date inputs apply immediately since each change is a
 * deliberate discrete choice.
 */
export function AuditFilters({ members, filters, onChange }: AuditFiltersProps) {
  const [actionDraft, setActionDraft] = useState(filters.action ?? "");
  const [fromDate, setFromDate] = useState("");
  const [toDate, setToDate] = useState("");

  const set = (patch: Partial<AuditLogFilters>) => onChange({ ...filters, ...patch });
  // A free-text action that is not one of the group prefixes shows as
  // "Tùy chỉnh" — the trigger must not claim "Tất cả" while a filter is live.
  const isCustomAction =
    Boolean(filters.action) && !ACTION_GROUPS.some((group) => group.value === filters.action);
  const groupValue = isCustomAction ? "custom" : (filters.action ?? "all");

  return (
    <form
      className="flex flex-wrap items-center gap-3"
      onSubmit={(event) => {
        event.preventDefault();
        set({ action: actionDraft || undefined });
      }}
    >
      <Select
        value={filters.actor_id ?? "all"}
        onValueChange={(value) => set({ actor_id: value === "all" ? undefined : value })}
      >
        <SelectTrigger aria-label="Giáo viên" className="w-[180px]">
          <SelectValue placeholder="Tất cả giáo viên" />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="all">Tất cả giáo viên</SelectItem>
          {members.map((member) => (
            <SelectItem key={member.id} value={member.id}>
              {member.full_name}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>

      <Select
        value={groupValue}
        onValueChange={(value) => {
          const action = value === "all" ? undefined : value;
          setActionDraft(action ?? "");
          set({ action });
        }}
      >
        <SelectTrigger aria-label="Nhóm hành động" className="w-[180px]">
          <SelectValue placeholder="Tất cả hành động" />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="all">Tất cả hành động</SelectItem>
          {isCustomAction ? (
            <SelectItem value="custom" disabled>
              Tùy chỉnh
            </SelectItem>
          ) : null}
          {ACTION_GROUPS.map((group) => (
            <SelectItem key={group.value} value={group.value}>
              {group.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>

      <Input
        aria-label="Hành động"
        placeholder="vd: class.create — Enter để lọc"
        value={actionDraft}
        onChange={(event) => setActionDraft(event.target.value)}
        className="w-[220px]"
      />
      <HvButton type="submit" variant="secondary" size="sm">
        Lọc
      </HvButton>

      <Input
        aria-label="Từ ngày"
        type="date"
        value={fromDate}
        onChange={(event) => {
          const value = event.target.value;
          setFromDate(value);
          // Local start-of-day → instant; both bounds are inclusive server-side.
          set({ from: value ? new Date(`${value}T00:00:00`).toISOString() : undefined });
        }}
        className="w-[150px]"
      />
      <Input
        aria-label="Đến ngày"
        type="date"
        value={toDate}
        onChange={(event) => {
          const value = event.target.value;
          setToDate(value);
          set({ to: value ? new Date(`${value}T23:59:59.999`).toISOString() : undefined });
        }}
        className="w-[150px]"
      />
    </form>
  );
}
