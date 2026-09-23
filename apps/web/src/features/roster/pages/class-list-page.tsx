import { useCallback, useState } from "react";
import { useNavigate } from "react-router";

import { HvButton, HvNotice, HvStateBlock } from "@/components/hv";
import { useCenterContext } from "@/features/teaching";

import type { ListClassesParams } from "../api/classes-api";
import { ClassDialog } from "../components/class-dialog";
import { ClassFilterBar } from "../components/class-filter-bar";
import { ClassStatusChips } from "../components/class-status-chips";
import { ClassTable } from "../components/class-table";
import { useClassesList, useClassStats } from "../hooks/use-classes";
import {
  isClassPhaseView,
  useClassListUrlState,
  type ClassListUrlValues,
} from "../hooks/use-class-list-url-state";

/**
 * `/classes` — the class catalog. Filters live in the URL; the status chip
 * maps to the API's `phase` filter except "Cần tuyển sinh", which the API
 * has no list filter for and is applied to the fetched page instead.
 */
export function ClassListPage() {
  const navigate = useNavigate();
  const { has } = useCenterContext();
  const canCreate = has("classes.create");
  const [creating, setCreating] = useState(false);
  const { view, q, weekday, shift, set } = useClassListUrlState();
  const today = new Date().toISOString().slice(0, 10);

  const params: ListClassesParams = { status: "all", per_page: 100 };
  if (isClassPhaseView(view)) {
    params.phase = view;
  }
  if (q !== "") {
    params.q = q;
  }
  if (weekday !== "") {
    params.weekday = Number(weekday);
  }
  if (shift !== "") {
    params.shift = shift;
  }

  const list = useClassesList(params);
  const stats = useClassStats();
  const classes = (list.data?.items ?? []).filter(
    (klass) => view !== "recruiting" || klass.recruiting,
  );
  // The page fetches one API page; past it the catalog is cut, and the
  // recruiting chip (a client-side filter) may then miss classes.
  const fetched = list.data?.items.length ?? 0;
  const total = list.data?.meta.total ?? 0;
  const truncated = fetched < total;
  const filtered = view !== "all" || q !== "" || weekday !== "" || shift !== "";

  const handleFilterChange = useCallback(
    (partial: Partial<ClassListUrlValues>) => set(partial),
    // `set` is rebuilt every render; the bar keeps the latest one through this callback.
    [set],
  );

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h1 className="font-display text-[26px] font-extrabold text-ink-900">
            Danh sách lớp học
          </h1>
          <p className="mt-1 text-[14px] text-ink-500">
            Mở một lớp để xem lịch, học viên và buổi học.
          </p>
        </div>
        {canCreate ? (
          <HvButton size="sm" onClick={() => setCreating(true)}>
            + Lớp học
          </HvButton>
        ) : null}
      </div>

      <ClassFilterBar q={q} weekday={weekday} shift={shift} onChange={handleFilterChange} />
      <ClassStatusChips stats={stats.data} value={view} onChange={(next) => set({ view: next })} />

      {truncated ? (
        <HvNotice tone="warning">
          Đang hiển thị {fetched}/{total} lớp. Thu hẹp bằng bộ lọc hoặc từ khoá để thấy phần còn
          lại.
        </HvNotice>
      ) : null}

      {list.isPending ? (
        <HvStateBlock state="loading" title="Đang tải danh sách lớp" />
      ) : list.isError ? (
        <HvStateBlock
          state="error"
          title="Không tải được danh sách lớp"
          action={
            <HvButton size="sm" variant="ghost" onClick={() => void list.refetch()}>
              Thử lại
            </HvButton>
          }
        />
      ) : classes.length === 0 ? (
        <HvStateBlock
          state="empty"
          title={filtered ? "Không có lớp nào khớp bộ lọc." : "Chưa có lớp học nào."}
          description={
            filtered ? "Đổi bộ lọc hoặc xoá từ khoá." : "Tạo lớp đầu tiên bằng nút + Lớp học."
          }
        />
      ) : (
        <ClassTable
          classes={classes}
          today={today}
          onOpen={(klass) => void navigate(`/classes/${klass.id}`)}
        />
      )}

      {canCreate ? <ClassDialog open={creating} onOpenChange={setCreating} /> : null}
    </div>
  );
}
