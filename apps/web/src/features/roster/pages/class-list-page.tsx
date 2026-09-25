import { RefreshCwIcon } from "lucide-react";
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
import { canWriteClass } from "../lib/class-permissions";

export type ClassListVariant = "all" | "recruiting";

const variantCopy: Record<
  ClassListVariant,
  { title: string; subtitle: string; emptyLabel: string; path: string }
> = {
  all: {
    title: "Danh sách lớp học",
    subtitle: "Theo dõi, tổ chức và quản lý các lớp học của trung tâm.",
    emptyLabel: "Chưa có lớp học nào. Tạo lớp đầu tiên bằng nút + Lớp học.",
    path: "/classes",
  },
  recruiting: {
    title: "Lớp cần tuyển sinh",
    subtitle: "Các lớp đang bật “Cần tuyển sinh” — chưa đủ sĩ số hoặc sắp khai giảng.",
    emptyLabel: "Không có lớp nào cần tuyển sinh.",
    path: "/classes/recruiting",
  },
};

interface ClassListPageProps {
  /**
   * `recruiting` is `/classes/recruiting`: the same table narrowed to the
   * classes open for recruitment, with chip counts over that set.
   */
  variant?: ClassListVariant;
}

/**
 * `/classes` — the class catalog. Filters live in the URL; the status chip
 * maps to the API's `phase` filter and "Cần tuyển sinh" to its `recruiting`
 * filter, so every view is filtered server-side.
 */
export function ClassListPage({ variant = "all" }: ClassListPageProps) {
  const navigate = useNavigate();
  const { has, isOwner } = useCenterContext();
  const canCreate = has("classes.create");
  const [creating, setCreating] = useState(false);
  const [editingId, setEditingId] = useState<string | null>(null);
  const url = useClassListUrlState();
  const { q, weekday, shift, set } = url;
  const recruitingPage = variant === "recruiting";
  // The recruiting page has no "Cần tuyển sinh" chip; a shared link carrying
  // it falls back to "all", which is the same set there.
  const view = recruitingPage && url.view === "recruiting" ? "all" : url.view;
  const copy = variantCopy[variant];
  const today = new Date().toISOString().slice(0, 10);

  const params: ListClassesParams = { status: "all", per_page: 100 };
  if (isClassPhaseView(view)) {
    params.phase = view;
  }
  if (recruitingPage || view === "recruiting") {
    params.recruiting = true;
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
  const stats = useClassStats(recruitingPage ? { recruiting: true } : {});
  const classes = list.data?.items ?? [];
  // The page fetches one API page; past it the catalog is cut.
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
    <div className="flex flex-col gap-3">
      <div className="mb-1 flex flex-wrap items-start gap-3">
        <div className="min-w-[260px] flex-1">
          <h1 className="mb-1 font-display text-[26px] font-extrabold text-ink-900">
            {copy.title}
          </h1>
          <p className="text-[14px] text-ink-500">{copy.subtitle}</p>
        </div>
        <button
          type="button"
          title="Tải lại"
          aria-label="Tải lại"
          onClick={() => {
            void list.refetch();
            void stats.refetch();
          }}
          className="flex size-11 items-center justify-center rounded-[14px] border-2 border-line-200 bg-white text-ink-500 hover:border-mint-400 hover:text-mint-600"
        >
          <RefreshCwIcon
            aria-hidden="true"
            className={list.isFetching ? "size-[18px] animate-spin" : "size-[18px]"}
          />
        </button>
        {canCreate ? (
          <HvButton size="sm" onClick={() => setCreating(true)}>
            + Lớp học
          </HvButton>
        ) : null}
      </div>

      <div className="flex flex-col gap-3 rounded-[20px] bg-white px-[18px] py-3.5 shadow-soft-md">
        <ClassFilterBar q={q} weekday={weekday} shift={shift} onChange={handleFilterChange} />
        <ClassStatusChips
          stats={stats.data}
          value={view}
          onChange={(next) => set({ view: next })}
          includeRecruiting={!recruitingPage}
        />
      </div>

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
      ) : (
        <ClassTable
          classes={classes}
          today={today}
          onOpen={(klass) =>
            void navigate(`/classes/${klass.id}`, {
              state: { from: copy.path },
            })
          }
          canEdit={(klass) => canWriteClass(isOwner, klass)}
          onEdit={(klass) => setEditingId(klass.id)}
          emptyLabel={
            filtered && !recruitingPage
              ? "Không có lớp nào khớp bộ lọc. Đổi bộ lọc hoặc xoá từ khoá."
              : copy.emptyLabel
          }
        />
      )}

      {canCreate ? <ClassDialog open={creating} onOpenChange={setCreating} /> : null}
      {editingId ? (
        <ClassDialog
          mode="edit"
          classId={editingId}
          open
          onOpenChange={(open) => {
            if (!open) setEditingId(null);
          }}
        />
      ) : null}
    </div>
  );
}

/** `/classes/recruiting` — "Lớp cần tuyển sinh", the catalog narrowed to classes open for recruitment. */
export function RecruitingClassListPage() {
  return <ClassListPage variant="recruiting" />;
}
