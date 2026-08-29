import { useState } from "react";
import { Navigate } from "react-router";

import { HvButton } from "@/components/hv";
import { useCenter } from "@/features/center";
import { useCenterContext } from "@/features/teaching";

import { AuditFilters } from "../components/audit-filters";
import { AuditTable } from "../components/audit-table";
import { useAuditLogs } from "../hooks/use-audit-logs";
import type { AuditLogFilters } from "../schemas/audit-schemas";

/**
 * Audit trail for holders of `audit.read` (the owner always holds it). The
 * list query is enabled-gated on the resolved permission so a viewer without
 * it deep-linking here never fires a request that would only 403 — they
 * redirect to the dashboard instead. The member filter only offers the roster
 * the caller can see: a non-owner grantee gets no member list from
 * `/centers/me`, so that dropdown stays empty for them.
 */
export function AuditPage() {
  const { has, isResolved, isError } = useCenterContext();
  const { data: centerMe } = useCenter();
  const [filters, setFilters] = useState<AuditLogFilters>({});
  const canRead = has("audit.read");
  const query = useAuditLogs(filters, isResolved && canRead);

  if (!isResolved && !isError) {
    return null;
  }
  if (!canRead) {
    return <Navigate to="/" replace />;
  }

  const members = centerMe && "members" in centerMe ? centerMe.members : [];
  const logs = query.data?.pages.flatMap((page) => page.items) ?? [];
  const hasFilters = Object.values(filters).some(Boolean);

  return (
    <div className="space-y-4">
      <h1 className="font-display text-[22px] font-extrabold text-ink-900">Nhật ký hoạt động</h1>
      <AuditFilters members={members} filters={filters} onChange={setFilters} />
      {/* isError can coexist with populated data (a failed fetchNextPage or
        background refetch), so the error renders inline and never replaces
        rows already on screen. */}
      {query.isError ? (
        <p className="text-[13px] text-coral-600">Không tải được nhật ký hoạt động. Thử lại sau.</p>
      ) : null}
      {query.isPending ? (
        <p className="text-[13px] text-ink-400">Đang tải…</p>
      ) : logs.length > 0 ? (
        <AuditTable logs={logs} />
      ) : query.isError ? null : (
        // A filtered window may legitimately be empty (kể cả window ngược,
        // API trả 200 rỗng) — chỉ khi không có filter mới nói "chưa có gì".
        <p className="text-[13px] text-ink-400">
          {hasFilters
            ? "Không có bản ghi phù hợp với bộ lọc."
            : "Chưa có hoạt động nào được ghi nhận."}
        </p>
      )}
      {query.hasNextPage ? (
        <HvButton
          variant="secondary"
          onClick={() => {
            void query.fetchNextPage();
          }}
          disabled={query.isFetchingNextPage}
        >
          Tải thêm
        </HvButton>
      ) : null}
    </div>
  );
}
