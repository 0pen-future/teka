import { useState } from "react";

import { HvBadge, type HvBadgeVariant } from "@/components/hv";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { formatDateTime } from "@/lib/utils";

import type { AuditLog } from "../schemas/audit-schemas";

// Matches the roster/billing header band; keeps the default px-2 so header
// text stays aligned with the p-2 body cells.
const headCellClassName =
  "bg-cream-200 text-[12px] font-extrabold uppercase tracking-[0.4px] text-ink-500";

function statusVariant(code: number): HvBadgeVariant {
  if (code >= 500) {
    return "danger";
  }
  if (code >= 400) {
    return "warning";
  }
  return "success";
}

/**
 * Empty actor_name with a non-null actor id means the teacher row is gone
 * (LEFT JOIN miss server-side); a null actor id means the event carried no
 * actor at all.
 */
function actorLabel(log: AuditLog): string {
  if (log.actor_user_id === null) {
    return "Ẩn danh";
  }
  return log.actor_name === "" ? "(đã xóa)" : log.actor_name;
}

export function AuditTable({ logs }: { logs: AuditLog[] }) {
  const [expandedId, setExpandedId] = useState<string | null>(null);

  return (
    <div className="overflow-x-auto rounded-2xl border border-line-200 bg-white">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead className={headCellClassName}>Thời gian</TableHead>
            <TableHead className={headCellClassName}>Người thao tác</TableHead>
            <TableHead className={headCellClassName}>Hành động</TableHead>
            <TableHead className={headCellClassName}>Đối tượng</TableHead>
            <TableHead className={headCellClassName}>Trạng thái</TableHead>
            <TableHead className={headCellClassName}>IP</TableHead>
            <TableHead className={`${headCellClassName} w-px`} />
          </TableRow>
        </TableHeader>
        <TableBody>
          {logs.map((log) => (
            <AuditRow
              key={log.id}
              log={log}
              expanded={expandedId === log.id}
              onToggle={() => setExpandedId(expandedId === log.id ? null : log.id)}
            />
          ))}
        </TableBody>
      </Table>
    </div>
  );
}

function AuditRow({
  log,
  expanded,
  onToggle,
}: {
  log: AuditLog;
  expanded: boolean;
  onToggle: () => void;
}) {
  return (
    <>
      <TableRow>
        <TableCell className="whitespace-nowrap text-ink-500">
          {formatDateTime(log.occurred_at)}
        </TableCell>
        <TableCell className="font-semibold text-ink-900">{actorLabel(log)}</TableCell>
        <TableCell className="font-mono text-[13px]">{log.action}</TableCell>
        <TableCell className="text-ink-500">{log.entity_type || "—"}</TableCell>
        <TableCell>
          <HvBadge variant={statusVariant(log.status_code)} size="sm">
            {log.status_code}
          </HvBadge>
        </TableCell>
        <TableCell className="font-mono text-[13px] text-ink-500">{log.ip}</TableCell>
        <TableCell>
          <button
            type="button"
            onClick={onToggle}
            aria-expanded={expanded}
            aria-label={`Chi tiết ${log.action}`}
            aria-controls={expanded ? `audit-details-${log.id}` : undefined}
            className="rounded-[var(--radius-md)] px-2 py-1 text-[12px] font-semibold text-mint-600 hover:bg-mint-50"
          >
            Chi tiết
          </button>
        </TableCell>
      </TableRow>
      {expanded ? (
        <TableRow>
          <TableCell colSpan={7} className="bg-cream-100">
            <div id={`audit-details-${log.id}`} className="space-y-2 py-1 text-[13px]">
              <p className="font-mono text-ink-900">
                {log.method} {log.path}
              </p>
              <p className="text-ink-500">
                {log.entity_type
                  ? `Đối tượng: ${log.entity_type}${log.entity_id ? ` (${log.entity_id})` : ""} · `
                  : ""}
                Vai trò: {log.actor_role}
              </p>
              <p className="text-ink-500">{log.user_agent}</p>
              {log.metadata ? (
                <pre className="overflow-x-auto rounded-[var(--radius-md)] bg-white p-3 font-mono text-[12px] text-ink-700">
                  {JSON.stringify(log.metadata, null, 2)}
                </pre>
              ) : null}
            </div>
          </TableCell>
        </TableRow>
      ) : null}
    </>
  );
}
