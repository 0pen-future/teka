import { ArrowDownIcon, ArrowUpIcon, ChevronsUpDownIcon } from "lucide-react";
import type { ReactNode } from "react";

import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import type { Meta } from "@/lib/api/envelope";

export interface DataTableColumn<T> {
  key: string;
  header: string;
  /** Base API sort key (e.g. "name"); omit for unsortable columns. */
  sortKey?: string;
  className?: string;
  cell: (row: T) => ReactNode;
}

interface DataTableProps<T> {
  columns: DataTableColumn<T>[];
  rows: T[];
  rowKey: (row: T) => string;
  loading?: boolean;
  skeletonRows?: number;
  /** Current API sort value, e.g. "-created_at". */
  sort?: string;
  onSortChange?: (sort: string) => void;
  meta?: Meta;
  onPageChange?: (page: number) => void;
  /** Rendered instead of the table body when there is no data. */
  empty?: ReactNode;
}

function SortHeader({
  label,
  sortKey,
  sort,
  onSortChange,
}: {
  label: string;
  sortKey: string;
  sort?: string;
  onSortChange?: (sort: string) => void;
}) {
  const direction = sort === sortKey ? "asc" : sort === `-${sortKey}` ? "desc" : null;
  const Icon =
    direction === "asc" ? ArrowUpIcon : direction === "desc" ? ArrowDownIcon : ChevronsUpDownIcon;
  return (
    <Button
      type="button"
      variant="ghost"
      size="sm"
      className="-ml-2 h-8"
      onClick={() => onSortChange?.(direction === "asc" ? `-${sortKey}` : sortKey)}
    >
      {label}
      <Icon aria-hidden className="size-3.5" />
    </Button>
  );
}

export function DataTable<T>({
  columns,
  rows,
  rowKey,
  loading = false,
  skeletonRows = 5,
  sort,
  onSortChange,
  meta,
  onPageChange,
  empty,
}: DataTableProps<T>) {
  const showEmpty = !loading && rows.length === 0;

  return (
    <div className="space-y-4">
      <div className="overflow-x-auto rounded-md border">
        <Table>
          <TableHeader>
            <TableRow>
              {columns.map((column) => (
                <TableHead
                  key={column.key}
                  className={column.className}
                  aria-sort={
                    column.sortKey
                      ? sort === column.sortKey
                        ? "ascending"
                        : sort === `-${column.sortKey}`
                          ? "descending"
                          : "none"
                      : undefined
                  }
                >
                  {column.sortKey ? (
                    <SortHeader
                      label={column.header}
                      sortKey={column.sortKey}
                      sort={sort}
                      onSortChange={onSortChange}
                    />
                  ) : (
                    column.header
                  )}
                </TableHead>
              ))}
            </TableRow>
          </TableHeader>
          <TableBody>
            {loading
              ? Array.from({ length: skeletonRows }, (_, index) => (
                  <TableRow key={index}>
                    {columns.map((column) => (
                      <TableCell key={column.key} className={column.className}>
                        <Skeleton className="h-4 w-full max-w-32" />
                      </TableCell>
                    ))}
                  </TableRow>
                ))
              : rows.map((row) => (
                  <TableRow key={rowKey(row)}>
                    {columns.map((column) => (
                      <TableCell key={column.key} className={column.className}>
                        {column.cell(row)}
                      </TableCell>
                    ))}
                  </TableRow>
                ))}
            {showEmpty ? (
              <TableRow>
                <TableCell colSpan={columns.length} className="p-0">
                  {empty ?? (
                    <p className="px-4 py-8 text-center text-sm text-muted-foreground">
                      No results.
                    </p>
                  )}
                </TableCell>
              </TableRow>
            ) : null}
          </TableBody>
        </Table>
      </div>
      {meta && meta.total_pages > 1 ? (
        <div className="flex items-center justify-between gap-4">
          <p className="text-sm text-muted-foreground">
            Page {meta.page} of {meta.total_pages} · {meta.total} total
          </p>
          <div className="flex gap-2">
            <Button
              variant="outline"
              size="sm"
              disabled={loading || meta.page <= 1}
              onClick={() => onPageChange?.(meta.page - 1)}
            >
              Previous
            </Button>
            <Button
              variant="outline"
              size="sm"
              disabled={loading || meta.page >= meta.total_pages}
              onClick={() => onPageChange?.(meta.page + 1)}
            >
              Next
            </Button>
          </div>
        </div>
      ) : null}
    </div>
  );
}
